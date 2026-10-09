// Package inference abstracts "send a prompt to a model, get text back".
//
// It is deliberately stdlib-only and must not import internal/eval so that
// other packages (the agent manager, a future direct-API harness) can depend
// on it without creating import cycles.
//
// Two backends ship today:
//
//   - "kiro-cli": shells out to the kiro-cli binary (the historical behaviour).
//   - "stub": deterministic, in-process, never spawns a process or touches
//     the network. Used for harness self-tests.
package inference

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Role identifies what a request is for.
type Role string

const (
	// RoleAgent is the agent under test.
	RoleAgent Role = "agent"
	// RoleJudge is an LLM-as-judge scoring call.
	RoleJudge Role = "judge"
)

// UsageSource describes where token counts came from.
type UsageSource string

const (
	// UsageReported means the backend/model reported the token counts.
	UsageReported UsageSource = "reported"
	// UsageEstimated means the counts were estimated from text length.
	UsageEstimated UsageSource = "estimated"
)

// Usage holds token accounting for one invocation.
type Usage struct {
	InputTokens  int         `json:"InputTokens"`
	OutputTokens int         `json:"OutputTokens"`
	Source       UsageSource `json:"Source"`
}

// StubScript is test-double data carried on a Request; real backends ignore it.
type StubScript struct {
	Turns []StubTurn `yaml:"turns" json:"turns"`
	// Judge scripts the answers to yes/no judge requests (Request.YesNo).
	Judge StubJudge `yaml:"judge,omitempty" json:"judge,omitempty"`

	judgeNext int // cursor into Judge; guarded by judgeMu
}

// StubJudge is the scripted list of yes/no judge answers. In YAML it is either
// a single scalar ("judge: yes") or a sequence ("judge: [yes, no]").
type StubJudge []string

// UnmarshalYAML accepts a scalar (one answer) or a sequence of scalars.
func (j *StubJudge) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var s string
		if err := node.Decode(&s); err != nil {
			return err
		}
		*j = StubJudge{s}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := node.Decode(&list); err != nil {
			return err
		}
		*j = StubJudge(list)
		return nil
	default:
		return fmt.Errorf("line %d: stub judge must be a string or a list of strings", node.Line)
	}
}

// judgeMu guards every StubScript's judgeNext cursor.
var judgeMu sync.Mutex

// NextJudgeAnswer returns the next scripted judge answer, cycling through
// Judge. It reports false when Judge is empty. The cursor lives on the script,
// so it persists across Invoke calls and is independent per script.
func (s *StubScript) NextJudgeAnswer() (string, bool) {
	judgeMu.Lock()
	defer judgeMu.Unlock()
	if len(s.Judge) == 0 {
		return "", false
	}
	answer := s.Judge[s.judgeNext%len(s.Judge)]
	s.judgeNext = (s.judgeNext + 1) % len(s.Judge)
	return answer, true
}

// StubTurn is one scripted model response.
type StubTurn struct {
	Response string     `yaml:"response" json:"response"`
	Model    string     `yaml:"model,omitempty" json:"model,omitempty"`
	Usage    *StubUsage `yaml:"usage,omitempty" json:"usage,omitempty"`
	// Commands are shell commands (run with `sh -c`, in Request.WorkDir) that
	// the stub executes before returning Response, simulating an agent that
	// changes files in its workspace. They require a non-empty WorkDir.
	Commands []string `yaml:"commands,omitempty" json:"commands,omitempty"`
	// ToolCalls model the agent invoking a tool. After Commands, each call runs
	// its Command (`sh -c`, in Request.WorkDir) only if Request.ToolTrust is nil
	// or trusts the (alias-normalized) Tool; otherwise the call is skipped and
	// recorded in Response.ToolDenials. Commands themselves are never gated.
	ToolCalls []StubToolCall `yaml:"tool_calls,omitempty" json:"tool_calls,omitempty"`
}

// StubToolCall is one scripted tool invocation subject to the trust gate.
type StubToolCall struct {
	Tool    string `yaml:"tool" json:"tool"`
	Command string `yaml:"command" json:"command"`
}

// StubUsage is scripted, "reported" token usage for a StubTurn.
type StubUsage struct {
	InputTokens  int `yaml:"input_tokens" json:"input_tokens"`
	OutputTokens int `yaml:"output_tokens" json:"output_tokens"`
}

// Request describes one inference call.
//
// The json tags keep the Go field names as keys so a Request can be sent over
// the wire (see ServeExec); Timeout is integer nanoseconds.
type Request struct {
	Role   Role   `json:"Role"`
	Agent  string `json:"Agent"` // agent name (RoleAgent only)
	Prompt string `json:"Prompt"`
	// Timeout bounds the call. Zero means DefaultTimeout.
	Timeout time.Duration `json:"Timeout"`
	// AgentConfigDir optionally names a directory holding <agent>.json (plus
	// any prompt files it references). When it contains <Agent>.json it takes
	// precedence over the working directory's .kiro/agents.
	AgentConfigDir string `json:"AgentConfigDir"`
	// WorkDir is the working directory for the call. The kiro-cli backend runs
	// in it when non-empty (and then does not use the AgentConfigDir overlay:
	// the workspace is expected to hold its own .kiro/agents). The stub backend
	// runs StubTurn.Commands in it. Empty means the process working directory.
	WorkDir string `json:"WorkDir"`
	// Model pins the model for this call. "" means the backend default
	// (unpinned). The kiro-cli backend passes it as --model; the stub backend
	// ignores it.
	Model string `json:"Model"`
	// Stub is used only by the stub backend.
	Stub *StubScript `json:"Stub"`
	// Turn is the stub turn index to answer with (0 for now).
	Turn int `json:"Turn"`
	// YesNo marks a RoleJudge request as a yes/no question (as opposed to the
	// legacy 1-5 scoring call). The stub backend answers it from Stub.Judge.
	YesNo bool `json:"YesNo"`
	// ToolTrust restricts which tools the agent may use without prompting. nil
	// means unrestricted (kiro-cli --trust-all-tools, the historical behaviour);
	// non-nil, even with zero tools, means restricted to exactly that set
	// (--trust-tools=<csv>). Only RoleAgent requests use it.
	ToolTrust *ToolTrust `json:"ToolTrust,omitempty"`
}

// Response is the result of an invocation. Invoke populates it (Command,
// Stderr, ExitCode, Duration) even when it also returns an error.
//
// The json tags keep the Go field names as keys; Duration is integer nanoseconds.
type Response struct {
	Text     string        `json:"Text"`
	Model    string        `json:"Model"` // "" when unknown
	Usage    Usage         `json:"Usage"`
	Command  string        `json:"Command"` // human-readable command line (for error context)
	Stderr   string        `json:"Stderr"`
	ExitCode int           `json:"ExitCode"`
	Duration time.Duration `json:"Duration"`
	// ToolDenials lists tool calls refused by the trust gate. Only the stub
	// backend populates it; kiro-cli denials are not reliably detectable.
	ToolDenials []ToolDenial `json:"ToolDenials,omitempty"`
}

// Backend performs inference.
type Backend interface {
	// Name is the registry name of the backend.
	Name() string
	// Available reports whether the backend can be used in this environment.
	Available() error
	// StartupProbe measures backend start-up overhead. Backends that start no
	// process return 0.
	StartupProbe() time.Duration
	// Invoke runs one request. Timeouts return an error for which
	// errors.Is(err, ErrTimeout) is true.
	Invoke(ctx context.Context, req Request) (Response, error)
}

// DefaultTimeout applies when Request.Timeout is zero.
const DefaultTimeout = 2 * time.Minute

// ErrTimeout is matched (via errors.Is) by every timeout error.
var ErrTimeout = errors.New("inference timeout")

// Backend names.
const (
	NameKiroCLI = "kiro-cli"
	NameStub    = "stub"
)

var registry = map[string]func() Backend{
	NameKiroCLI: func() Backend { return newKiroCLI() },
	NameStub:    func() Backend { return newStub() },
}

// New returns the backend registered under name.
func New(name string) (Backend, error) {
	ctor, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown backend %q; valid backends: %s", name, strings.Join(Names(), ", "))
	}
	return ctor(), nil
}

// Names returns the sorted names of all registered backends.
func Names() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// EstimateUsage estimates token usage at roughly 4 characters per token.
func EstimateUsage(input, output string) Usage {
	return Usage{
		InputTokens:  len(input) / 4,
		OutputTokens: len(output) / 4,
		Source:       UsageEstimated,
	}
}

// invokeError carries a fixed message while unwrapping to one or more
// underlying errors, so error strings can stay byte-identical to historical
// output while still matching sentinels like ErrTimeout.
type invokeError struct {
	msg  string
	errs []error
}

func (e *invokeError) Error() string   { return e.msg }
func (e *invokeError) Unwrap() []error { return e.errs }

func timeoutOrDefault(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultTimeout
	}
	return d
}
