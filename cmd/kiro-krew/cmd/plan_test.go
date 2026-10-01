package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn while capturing everything written to os.Stdout and
// returns it. The plan command prints its JSON result to stdout, so this lets
// the test assert on that machine-readable output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = orig
	return <-done
}

func writeSpec(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

// TestPlanParse_MalformedArtifactEmitsValidationFailed verifies that a bad plan
// artifact is reported via the machine-readable validation_failed status (which
// krew-lead consumes to trigger an architect retry) rather than a hard CLI error.
func TestPlanParse_MalformedArtifactEmitsValidationFailed(t *testing.T) {
	cases := map[string]string{
		"malformed YAML": "# Spec\n```kiro-plan\nversion: \"1.0\"\ntasks:\n  - id: task-1\n    agent: builder\n    dependencies: [\n```",
		"unclosed block": "# Spec\n```kiro-plan\nversion: \"1.0\"\ntasks: []\n",
		"multiple blocks": "# Spec\n```kiro-plan\nversion: \"1.0\"\ntasks: []\n```\n\n```kiro-plan\nversion: \"1.0\"\ntasks: []\n```",
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			specPath := writeSpec(t, body)

			var runErr error
			out := captureStdout(t, func() {
				runErr = planParseCmd.RunE(planParseCmd, []string{specPath})
			})

			if runErr != nil {
				t.Fatalf("expected no CLI error (JSON result instead), got: %v", runErr)
			}
			if !strings.Contains(out, `"status": "validation_failed"`) {
				t.Errorf("expected validation_failed status, got:\n%s", out)
			}
		})
	}
}

// TestPlanParse_MissingFileIsHardError verifies that a genuine file read failure
// remains a hard CLI error, distinct from the validation_failed path.
func TestPlanParse_MissingFileIsHardError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.md")

	var runErr error
	_ = captureStdout(t, func() {
		runErr = planParseCmd.RunE(planParseCmd, []string{missing})
	})

	if runErr == nil {
		t.Fatal("expected a hard error for a missing spec file, got nil")
	}
	if !strings.Contains(runErr.Error(), "read spec file") {
		t.Errorf("expected a file-read error, got: %v", runErr)
	}
}
