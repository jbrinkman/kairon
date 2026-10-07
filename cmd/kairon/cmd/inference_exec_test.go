package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
)

// runInferenceExec runs the real root command with args and stdin, returning
// stdout and stderr separately.
func runInferenceExec(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errBuf bytes.Buffer
	origBackend := inferenceExecBackend
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetIn(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		inferenceExecBackend = origBackend
		_ = inferenceExecCmd.Flags().Set("backend", origBackend)
	})
	rootCmd.SetIn(strings.NewReader(stdin))
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errBuf)
	rootCmd.SetArgs(append([]string{"inference-exec"}, args...))
	err = rootCmd.Execute()
	return out.String(), errBuf.String(), err
}

func TestInferenceExecHidden(t *testing.T) {
	if !inferenceExecCmd.Hidden {
		t.Fatal("inference-exec must be Hidden")
	}
	if inferenceExecCmd.Flags().Lookup("backend") == nil {
		t.Fatal("--backend flag not registered")
	}

	var help bytes.Buffer
	rootCmd.SetOut(&help)
	t.Cleanup(func() { rootCmd.SetOut(nil) })
	if err := rootCmd.Help(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help.String(), "inference-exec") {
		t.Errorf("root help lists inference-exec:\n%s", help.String())
	}
}

func TestInferenceExecStub(t *testing.T) {
	const req = `{"Role":"agent","Agent":"a","Prompt":"p","Stub":{"turns":[{"response":"hello","model":"m1"}]}}`

	stdout, stderr, err := runInferenceExec(t, req, "--backend", inference.NameStub)
	if err != nil {
		t.Fatalf("inference-exec failed: %v (stderr %q)", err, stderr)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr: %q", stderr)
	}

	// Exactly one JSON envelope and nothing else on stdout.
	dec := json.NewDecoder(strings.NewReader(stdout))
	var env inference.ExecEnvelope
	if err := dec.Decode(&env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, stdout)
	}
	if dec.More() {
		t.Errorf("stdout has more than one JSON value:\n%s", stdout)
	}
	if rest := strings.TrimSpace(stdout[dec.InputOffset():]); rest != "" {
		t.Errorf("trailing output after envelope: %q", rest)
	}
	if env.Error != "" || env.Timeout {
		t.Errorf("envelope error = %q timeout = %v, want none", env.Error, env.Timeout)
	}
	if env.Response.Text != "hello" || env.Response.Model != "m1" {
		t.Errorf("response = %+v, want text hello / model m1", env.Response)
	}

	resp, err := inference.DecodeExecResult([]byte(stdout))
	if err != nil || resp.Text != "hello" {
		t.Errorf("DecodeExecResult = (%+v, %v), want hello", resp, err)
	}
}

func TestInferenceExecUnknownBackend(t *testing.T) {
	stdout, _, err := runInferenceExec(t, `{"Role":"agent"}`, "--backend", "nope")
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error %q does not mention the backend", err)
	}
	if stdout != "" {
		t.Errorf("stdout must be empty on error, got %q", stdout)
	}
}

func TestInferenceExecBadRequest(t *testing.T) {
	stdout, _, err := runInferenceExec(t, `not json`, "--backend", inference.NameStub)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if stdout != "" {
		t.Errorf("stdout must be empty on error, got %q", stdout)
	}
}
