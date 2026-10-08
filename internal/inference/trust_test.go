package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNewToolTrust_NormalizesAliasesAndDedupes(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"aliases", []string{"read", "write", "shell"}, []string{"fs_read", "fs_write", "execute_bash"}},
		{"aws alias", []string{"aws"}, []string{"use_aws"}},
		{"canonical passes through", []string{"fs_read", "execute_bash", "use_aws"}, []string{"fs_read", "execute_bash", "use_aws"}},
		{"duplicates dropped, order kept", []string{"read", "fs_read", "write", "read", "write"}, []string{"fs_read", "fs_write"}},
		{"mcp names unchanged", []string{"@srv/tool", "@srv", "web_search"}, []string{"@srv/tool", "@srv", "web_search"}},
		{"whitespace trimmed, empties dropped", []string{" read ", "", "  "}, []string{"fs_read"}},
		{"nil input", nil, []string{}},
		{"empty input", []string{}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewToolTrust(tt.in)
			if got == nil {
				t.Fatal("NewToolTrust returned nil; a restricted set must be non-nil")
			}
			if got.Tools == nil {
				t.Error("Tools is nil; want non-nil (even when empty)")
			}
			if !reflect.DeepEqual(got.Tools, tt.want) {
				t.Errorf("Tools = %#v, want %#v", got.Tools, tt.want)
			}
		})
	}
}

func TestToolTrust_Allows(t *testing.T) {
	tr := NewToolTrust([]string{"read", "@srv/tool"})
	for tool, want := range map[string]bool{
		"fs_read":      true,
		"read":         true, // alias is normalized before the check
		"fs_write":     false,
		"write":        false,
		"execute_bash": false,
		"@srv/tool":    true,
		"@srv/other":   false,
	} {
		if got := tr.Allows(tool); got != want {
			t.Errorf("Allows(%q) = %v, want %v", tool, got, want)
		}
	}

	var unrestricted *ToolTrust
	if !unrestricted.Allows("anything") {
		t.Error("nil ToolTrust must allow every tool")
	}
	if NewToolTrust(nil).Allows("fs_read") {
		t.Error("empty restricted ToolTrust must allow nothing")
	}
}

func TestToolTrust_CSVAndNames(t *testing.T) {
	if got := NewToolTrust([]string{"read", "write"}).CSV(); got != "fs_read,fs_write" {
		t.Errorf("CSV = %q", got)
	}
	if got := NewToolTrust(nil).CSV(); got != "" {
		t.Errorf("empty CSV = %q, want empty", got)
	}
	tr := NewToolTrust([]string{"read"})
	names := tr.Names()
	names[0] = "mutated"
	if tr.Tools[0] != "fs_read" {
		t.Error("Names must return a copy")
	}
	if NewToolTrust(nil).Names() == nil {
		t.Error("Names of an empty restricted set must be non-nil")
	}
}

func TestKiroCLIAgentCommand_ToolTrust(t *testing.T) {
	tests := []struct {
		name        string
		trust       *ToolTrust
		model       string
		wantArgs    []string
		wantCommand string
	}{
		{
			name:        "nil keeps trust-all-tools",
			trust:       nil,
			wantArgs:    []string{"chat", "--agent", "builder", "--no-interactive", "--trust-all-tools"},
			wantCommand: "kiro-cli chat --agent builder --no-interactive --trust-all-tools",
		},
		{
			name:        "nil with model",
			trust:       nil,
			model:       "claude-x",
			wantArgs:    []string{"chat", "--agent", "builder", "--no-interactive", "--trust-all-tools", "--model", "claude-x"},
			wantCommand: "kiro-cli chat --agent builder --no-interactive --trust-all-tools --model claude-x",
		},
		{
			name:        "single tool",
			trust:       NewToolTrust([]string{"fs_read"}),
			wantArgs:    []string{"chat", "--agent", "builder", "--no-interactive", "--trust-tools=fs_read"},
			wantCommand: "kiro-cli chat --agent builder --no-interactive --trust-tools=fs_read",
		},
		{
			name:        "several tools are one argv element",
			trust:       NewToolTrust([]string{"read", "write", "shell", "@srv/tool"}),
			wantArgs:    []string{"chat", "--agent", "builder", "--no-interactive", "--trust-tools=fs_read,fs_write,execute_bash,@srv/tool"},
			wantCommand: "kiro-cli chat --agent builder --no-interactive --trust-tools=fs_read,fs_write,execute_bash,@srv/tool",
		},
		{
			name:        "empty restricted set trusts nothing",
			trust:       NewToolTrust(nil),
			wantArgs:    []string{"chat", "--agent", "builder", "--no-interactive", "--trust-tools="},
			wantCommand: "kiro-cli chat --agent builder --no-interactive --trust-tools=",
		},
		{
			name:        "with model",
			trust:       NewToolTrust([]string{"read"}),
			model:       "claude-x",
			wantArgs:    []string{"chat", "--agent", "builder", "--no-interactive", "--trust-tools=fs_read", "--model", "claude-x"},
			wantCommand: "kiro-cli chat --agent builder --no-interactive --trust-tools=fs_read --model claude-x",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, command := KiroCLIAgentCommand(Request{Agent: "builder", Model: tt.model, ToolTrust: tt.trust})
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args = %q, want %q", args, tt.wantArgs)
			}
			if command != tt.wantCommand {
				t.Errorf("command = %q, want %q", command, tt.wantCommand)
			}
			if tt.trust != nil {
				for _, a := range args {
					if a == "--trust-all-tools" {
						t.Errorf("restricted request must not pass --trust-all-tools: %q", args)
					}
				}
			}
		})
	}
}

func TestKiroCLIAgentResponse_CommandReflectsToolTrust(t *testing.T) {
	req := Request{Agent: "a", ToolTrust: NewToolTrust([]string{"read"})}
	resp := KiroCLIAgentResponse(req, "x", "", 0, 0)
	if want := "kiro-cli chat --agent a --no-interactive --trust-tools=fs_read"; resp.Command != want {
		t.Errorf("Command = %q, want %q", resp.Command, want)
	}
}

func TestRequestResponse_ToolTrustJSONRoundTrip(t *testing.T) {
	for _, trust := range []*ToolTrust{nil, NewToolTrust(nil), NewToolTrust([]string{"read", "@srv/tool"})} {
		req := Request{
			Role: RoleAgent, Agent: "a", Prompt: "p", ToolTrust: trust,
			Stub: &StubScript{Turns: []StubTurn{{
				Response:  "r",
				ToolCalls: []StubToolCall{{Tool: "fs_write", Command: "echo x > f"}},
			}}},
		}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		var got Request
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, req) {
			t.Errorf("request round trip:\n got %+v\nwant %+v\njson %s", got, req, data)
		}
		if (got.ToolTrust == nil) != (trust == nil) {
			t.Errorf("nil-ness of ToolTrust changed over JSON: got nil=%v want nil=%v", got.ToolTrust == nil, trust == nil)
		}
	}

	resp := Response{Text: "t", ToolDenials: []ToolDenial{{Tool: "fs_write", Command: "c", Reason: ReasonToolNotTrusted}}}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var gotResp Response
	if err := json.Unmarshal(data, &gotResp); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotResp, resp) {
		t.Errorf("response round trip = %+v, want %+v", gotResp, resp)
	}

	// Legacy shape: no ToolDenials key when empty.
	data, _ = json.Marshal(Response{Text: "t"})
	if strings.Contains(string(data), "ToolDenials") {
		t.Errorf("empty ToolDenials should be omitted: %s", data)
	}
}

// TestServeExec_ToolTrustAcrossTheWire sends a restricted Request through
// ServeExec and reads the denials back with DecodeExecResult, the path the
// container transport uses.
func TestServeExec_ToolTrustAcrossTheWire(t *testing.T) {
	dir := t.TempDir()
	req := Request{
		Role: RoleAgent, Agent: "a", Prompt: "p", WorkDir: dir,
		ToolTrust: NewToolTrust([]string{"read"}),
		Stub: &StubScript{Turns: []StubTurn{{
			Response:  "done",
			ToolCalls: []StubToolCall{{Tool: "write", Command: "echo x > denied.txt"}},
		}}},
	}
	in, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := ServeExec(context.Background(), NameStub, bytes.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	resp, err := DecodeExecResult(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "done" {
		t.Errorf("Text = %q, want the turn response", resp.Text)
	}
	want := []ToolDenial{{Tool: "fs_write", Command: "echo x > denied.txt", Reason: ReasonToolNotTrusted}}
	if !reflect.DeepEqual(resp.ToolDenials, want) {
		t.Errorf("ToolDenials = %+v, want %+v", resp.ToolDenials, want)
	}
}

func TestCallRecord_TrustedToolsJSON(t *testing.T) {
	keys := func(rec CallRecord) map[string]json.RawMessage {
		t.Helper()
		b, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	m := keys(CallRecord{Role: "agent", Model: "m"})
	for _, k := range []string{"trusted_tools", "tool_denials"} {
		if _, ok := m[k]; ok {
			t.Errorf("%q must be omitted when unset", k)
		}
	}

	none := []string{}
	m = keys(CallRecord{Role: "agent", Model: "m", TrustedTools: &none})
	if got := string(m["trusted_tools"]); got != "[]" {
		t.Errorf("restricted-to-nothing trusted_tools = %s, want []", got)
	}

	some := []string{"fs_read"}
	denials := []ToolDenial{{Tool: "fs_write", Command: "c", Reason: ReasonToolNotTrusted}}
	in := CallRecord{Role: "agent", Model: "m", TrustedTools: &some, ToolDenials: denials}
	m = keys(in)
	if got := string(m["trusted_tools"]); got != `["fs_read"]` {
		t.Errorf("trusted_tools = %s", got)
	}
	if got := string(m["tool_denials"]); got != `[{"tool":"fs_write","command":"c","reason":"tool not trusted"}]` {
		t.Errorf("tool_denials = %s", got)
	}

	// Round trip, including the [] vs absent distinction.
	for _, rec := range []CallRecord{in, {Role: "agent", Model: "m", TrustedTools: &none}, {Role: "agent", Model: "m"}} {
		b, _ := json.Marshal(rec)
		var out CallRecord
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(rec, out) {
			t.Errorf("round trip mismatch:\n in=%+v\nout=%+v", rec, out)
		}
	}
}
