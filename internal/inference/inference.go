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
	"time"
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
	InputTokens  int
	OutputTokens int
	Source       UsageSource
}

// StubScript is test-double data carried on a Request; real backends ignore it.
type StubScript struct {
	Turns []StubTurn `yaml:"turns" json:"turns"`
}

// StubTurn is one scripted model response.
type StubTurn struct {
	Response string     `yaml:"response" json:"response"`
	Model    string     `yaml:"model,omitempty" json:"model,omitempty"`
	Usage    *StubUsage `yaml:"usage,omitempty" json:"usage,omitempty"`
}

// StubUsage is scripted, "reported" token usage for a StubTurn.
type StubUsage struct {
	InputTokens  int `yaml:"input_tokens" json:"input_tokens"`
	OutputTokens int `yaml:"output_tokens" json:"output_tokens"`
}

// Request describes one inference call.
type Request struct {
	Role   Role
	Agent  string // agent name (RoleAgent only)
	Prompt string
	// Timeout bounds the call. Zero means DefaultTimeout.
	Timeout time.Duration
	// AgentConfigDir optionally names a directory holding <agent>.json (plus
	// any prompt files it references). When it contains <Agent>.json it takes
	// precedence over the working directory's .kiro/agents.
	AgentConfigDir string
	// Stub is used only by the stub backend.
	Stub *StubScript
	// Turn is the stub turn index to answer with (0 for now).
	Turn int
}

// Response is the result of an invocation. Invoke populates it (Command,
// Stderr, ExitCode, Duration) even when it also returns an error.
type Response struct {
	Text     string
	Model    string // "" when unknown
	Usage    Usage
	Command  string // human-readable command line (for error context)
	Stderr   string
	ExitCode int
	Duration time.Duration
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
