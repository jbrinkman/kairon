package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestKiroCLIAgentCommand(t *testing.T) {
	t.Run("without model", func(t *testing.T) {
		args, command := KiroCLIAgentCommand(Request{Agent: "builder", Prompt: "SECRET-PROMPT"})
		want := []string{"chat", "--agent", "builder", "--no-interactive", "--trust-all-tools"}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("args = %v, want %v", args, want)
		}
		if command != "kiro-cli chat --agent builder --no-interactive --trust-all-tools" {
			t.Errorf("command = %q", command)
		}
	})

	t.Run("with model", func(t *testing.T) {
		args, command := KiroCLIAgentCommand(Request{Agent: "builder", Model: "claude-x", Prompt: "SECRET-PROMPT"})
		want := []string{"chat", "--agent", "builder", "--no-interactive", "--trust-all-tools", "--model", "claude-x"}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("args = %v, want %v", args, want)
		}
		if command != "kiro-cli chat --agent builder --no-interactive --trust-all-tools --model claude-x" {
			t.Errorf("command = %q", command)
		}
	})

	t.Run("prompt never in args", func(t *testing.T) {
		args, command := KiroCLIAgentCommand(Request{Agent: "a", Model: "m", Prompt: "SECRET-PROMPT"})
		if strings.Contains(strings.Join(args, " "), "SECRET-PROMPT") || strings.Contains(command, "SECRET-PROMPT") {
			t.Errorf("prompt leaked into args=%v command=%q", args, command)
		}
	})
}

func TestKiroCLIAgentResponse(t *testing.T) {
	req := Request{Agent: "builder", Model: "claude-x", Prompt: "12345678"}
	resp := KiroCLIAgentResponse(req, "\x1b[32mhello wo\x1b[0mrld!", "warn", 0, 3*time.Second)

	if resp.Text != "hello world!" {
		t.Errorf("Text = %q, want ANSI stripped", resp.Text)
	}
	if resp.Model != "claude-x" {
		t.Errorf("Model = %q, want request model echoed", resp.Model)
	}
	if want := EstimateUsage(req.Prompt, "hello world!"); resp.Usage != want {
		t.Errorf("Usage = %+v, want %+v", resp.Usage, want)
	}
	if resp.Usage.Source != UsageEstimated {
		t.Errorf("Usage.Source = %q, want estimated", resp.Usage.Source)
	}
	if resp.Command != "kiro-cli chat --agent builder --no-interactive --trust-all-tools --model claude-x" {
		t.Errorf("Command = %q", resp.Command)
	}
	if resp.Stderr != "warn" || resp.ExitCode != 0 || resp.Duration != 3*time.Second {
		t.Errorf("passthrough fields wrong: %+v", resp)
	}

	// No requested model: Model stays empty (unknown).
	if got := KiroCLIAgentResponse(Request{Agent: "a"}, "x", "", 0, 0); got.Model != "" {
		t.Errorf("Model = %q, want empty", got.Model)
	}
}

func TestRequestResponseJSONRoundTrip(t *testing.T) {
	req := Request{
		Role: RoleAgent, Agent: "a", Prompt: "p \"q\"\n$(x)", Timeout: 5 * time.Second,
		AgentConfigDir: "/cfg", Model: "m", Turn: 1,
		Stub: &StubScript{Turns: []StubTurn{{Response: "r", Model: "sm", Usage: &StubUsage{InputTokens: 1, OutputTokens: 2}}}},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"Role", "Agent", "Prompt", "Timeout", "AgentConfigDir", "Model", "Stub", "Turn"} {
		if _, ok := keys[k]; !ok {
			t.Errorf("request JSON missing key %q: %s", k, data)
		}
	}
	var got Request
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, req) {
		t.Errorf("request round trip = %+v, want %+v", got, req)
	}

	resp := Response{Text: "t", Model: "m", Usage: Usage{InputTokens: 1, OutputTokens: 2, Source: UsageReported},
		Command: "c", Stderr: "e", ExitCode: 3, Duration: time.Second}
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var gotResp Response
	if err := json.Unmarshal(data, &gotResp); err != nil {
		t.Fatal(err)
	}
	if gotResp != resp {
		t.Errorf("response round trip = %+v, want %+v", gotResp, resp)
	}
}

func serve(t *testing.T, name string, req Request) ([]byte, error) {
	t.Helper()
	in, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = ServeExec(context.Background(), name, bytes.NewReader(in), &out)
	return out.Bytes(), err
}

func TestServeExec_StubReported(t *testing.T) {
	out, err := serve(t, NameStub, Request{
		Role: RoleAgent, Agent: "a", Prompt: "hi",
		Stub: &StubScript{Turns: []StubTurn{{
			Response: "scripted", Model: "stub-model",
			Usage: &StubUsage{InputTokens: 7, OutputTokens: 9},
		}}},
	})
	if err != nil {
		t.Fatalf("ServeExec: %v", err)
	}
	if bytes.Count(bytes.TrimSpace(out), []byte("\n")) != 0 {
		t.Errorf("expected a single JSON document, got %q", out)
	}
	resp, err := DecodeExecResult(out)
	if err != nil {
		t.Fatalf("DecodeExecResult: %v", err)
	}
	if resp.Text != "scripted" || resp.Model != "stub-model" {
		t.Errorf("resp = %+v", resp)
	}
	want := Usage{InputTokens: 7, OutputTokens: 9, Source: UsageReported}
	if resp.Usage != want {
		t.Errorf("Usage = %+v, want %+v", resp.Usage, want)
	}
}

func TestServeExec_StubEstimated(t *testing.T) {
	out, err := serve(t, NameStub, Request{
		Role: RoleAgent, Agent: "a", Prompt: "12345678",
		Stub: &StubScript{Turns: []StubTurn{{Response: "abcdefgh"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := DecodeExecResult(out)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "abcdefgh" || resp.Usage != EstimateUsage("12345678", "abcdefgh") {
		t.Errorf("resp = %+v", resp)
	}
}

func TestServeExec_Judge(t *testing.T) {
	out, err := serve(t, NameStub, Request{Role: RoleJudge, Prompt: "judge this"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := DecodeExecResult(out)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != stubJudgeOutput || resp.Model != NameStub {
		t.Errorf("resp = %+v", resp)
	}
}

func TestServeExec_BackendErrorInEnvelope(t *testing.T) {
	// No stub script: the stub backend fails the call.
	out, err := serve(t, NameStub, Request{Role: RoleAgent, Agent: "a", Prompt: "p"})
	if err != nil {
		t.Fatalf("ServeExec must carry backend errors in the envelope, got %v", err)
	}
	var env ExecEnvelope
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env.Error, "stub.turns[0].response") {
		t.Errorf("envelope error = %q", env.Error)
	}
	if env.Timeout {
		t.Error("Timeout must be false")
	}
	if env.Response.Command != "stub agent" {
		t.Errorf("partial response not carried: %+v", env.Response)
	}

	resp, derr := DecodeExecResult(out)
	if derr == nil || !strings.Contains(derr.Error(), "stub.turns[0].response") {
		t.Fatalf("DecodeExecResult err = %v", derr)
	}
	if errors.Is(derr, ErrTimeout) {
		t.Error("non-timeout error must not match ErrTimeout")
	}
	if resp.Command != "stub agent" {
		t.Errorf("Command = %q", resp.Command)
	}
}

func TestServeExec_UnknownBackend(t *testing.T) {
	var out bytes.Buffer
	err := ServeExec(context.Background(), "nope", strings.NewReader(`{}`), &out)
	if err == nil || !strings.Contains(err.Error(), "unknown backend") {
		t.Fatalf("err = %v, want unknown backend error", err)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should be written on error, got %q", out.String())
	}
}

func TestServeExec_BadInput(t *testing.T) {
	var out bytes.Buffer
	if err := ServeExec(context.Background(), NameStub, strings.NewReader(`not json`), &out); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestServeExec_KiroCLIBackendErrorInEnvelope(t *testing.T) {
	// A kiro-cli judge call with an unsupported role is a backend error, not
	// a transport error.
	out, err := serve(t, NameKiroCLI, Request{Role: Role("bogus")})
	if err != nil {
		t.Fatal(err)
	}
	if _, derr := DecodeExecResult(out); derr == nil {
		t.Fatal("expected error from envelope")
	}
}

func TestDecodeExecResult_RoundTrip(t *testing.T) {
	want := Response{
		Text: "t", Model: "m", Command: "c", Stderr: "e", ExitCode: 2, Duration: 42,
		Usage: Usage{InputTokens: 1, OutputTokens: 2, Source: UsageEstimated},
	}
	data, err := json.Marshal(ExecEnvelope{Response: want})
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeExecResult(append(data, '\n'))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDecodeExecResult_Timeout(t *testing.T) {
	data, _ := json.Marshal(ExecEnvelope{Error: "kiro-cli timeout after 1s", Timeout: true, Response: Response{Command: "c"}})
	resp, err := DecodeExecResult(data)
	if err == nil || !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want errors.Is ErrTimeout", err)
	}
	if err.Error() != "kiro-cli timeout after 1s" {
		t.Errorf("message = %q", err.Error())
	}
	if resp.Command != "c" {
		t.Errorf("response not carried: %+v", resp)
	}

	// Timeout flag with no message still yields a usable error.
	data, _ = json.Marshal(ExecEnvelope{Timeout: true})
	if _, err := DecodeExecResult(data); !errors.Is(err, ErrTimeout) || err.Error() == "" {
		t.Errorf("err = %v", err)
	}
}

func TestDecodeExecResult_Garbage(t *testing.T) {
	if _, err := DecodeExecResult([]byte("boom")); err == nil {
		t.Fatal("expected decode error")
	}
	if _, err := DecodeExecResult(nil); err == nil {
		t.Fatal("expected decode error on empty output")
	}
}

// TestInferenceDoesNotImportEval guards the import-cycle invariant stated in
// the package doc.
func TestInferenceDoesNotImportEval(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list unavailable: %v", err)
	}
	if strings.Contains(string(out), "internal/eval") {
		t.Errorf("internal/inference must not import internal/eval:\n%s", out)
	}
}
