package inference

import "strings"

// toolAliases maps the short tool names used in agent configs (allowedTools)
// to the names kiro-cli's --trust-tools flag expects. This is the single place
// to fix a name if kiro-cli's tool table turns out to differ. Names not in the
// table (fs_*, execute_bash, use_aws, @server, @server/tool, ...) pass through
// unchanged.
var toolAliases = map[string]string{
	"read":  "fs_read",
	"write": "fs_write",
	"shell": "execute_bash",
	"aws":   "use_aws",
}

// NormalizeToolName returns the kiro-cli --trust-tools spelling of name:
// surrounding whitespace is trimmed and aliases (read, write, shell, aws) are
// expanded. Unknown names pass through unchanged.
func NormalizeToolName(name string) string {
	name = strings.TrimSpace(name)
	if canonical, ok := toolAliases[name]; ok {
		return canonical
	}
	return name
}

// ToolTrust is the whole-tool trust set for one request. A nil *ToolTrust on a
// Request means "unrestricted" (--trust-all-tools). A non-nil ToolTrust, even
// with zero tools, means "restricted to exactly these tools".
//
// Trust is per tool, not per argument.
type ToolTrust struct {
	// Tools are normalized tool names, de-duplicated, in first-seen order. It
	// is never nil for a ToolTrust built by NewToolTrust.
	Tools []string `json:"tools"`
}

// NewToolTrust builds a restricted trust set from names: each name is trimmed
// and alias-normalized (read→fs_read, write→fs_write, shell→execute_bash,
// aws→use_aws), empty names and duplicates are dropped, and order is kept.
// The result is non-nil even when names is empty (trust nothing).
func NewToolTrust(names []string) *ToolTrust {
	tools := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		n = NormalizeToolName(n)
		if n == "" {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		tools = append(tools, n)
	}
	return &ToolTrust{Tools: tools}
}

// Allows reports whether tool (alias or canonical name) is trusted. A nil
// receiver is unrestricted and allows everything.
func (t *ToolTrust) Allows(tool string) bool {
	if t == nil {
		return true
	}
	want := NormalizeToolName(tool)
	for _, have := range t.Tools {
		if NormalizeToolName(have) == want {
			return true
		}
	}
	return false
}

// CSV renders the trust set as the --trust-tools value: comma-joined, empty
// for a restricted-to-nothing set.
func (t *ToolTrust) CSV() string {
	if t == nil {
		return ""
	}
	return strings.Join(t.Tools, ",")
}

// Names returns a copy of the trusted tool names, never nil. It is the shape
// recorded in CallRecord.TrustedTools.
func (t *ToolTrust) Names() []string {
	if t == nil {
		return nil
	}
	out := make([]string, len(t.Tools))
	copy(out, t.Tools)
	return out
}

// ToolDenial records a tool call that was refused by the trust gate.
type ToolDenial struct {
	Tool    string `json:"tool"`
	Command string `json:"command,omitempty"`
	Reason  string `json:"reason"`
}

// ReasonToolNotTrusted is the Reason recorded when a tool call is refused
// because the tool is outside the request's ToolTrust set.
const ReasonToolNotTrusted = "tool not trusted"
