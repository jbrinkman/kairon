package eval

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const mockCaseHeader = "name: mocked\nrequires_sandbox: true\ninput: x\n"

// writeMockScript writes a script under the test evals dir ("evals") and
// returns its path relative to that dir.
func writeMockScript(t *testing.T, rel, content string) string {
	t.Helper()
	writeCfgFile(t, filepath.Join("evals", rel), content)
	return rel
}

func TestMocksYAMLParsing(t *testing.T) {
	wsEnv(t)
	writeMockScript(t, "fixtures/mock-cli.sh", "#!/bin/sh\n")
	writeCfgFile(t, filepath.Join("evals", "cases", "agent-m", "c.yaml"),
		mockCaseHeader+"mocks:\n  - command: aws\n    script: fixtures/mock-cli.sh\n  - command: npm\n    script: fixtures/mock-cli.sh\n")
	got, err := loadCases("agent-m")
	if err != nil {
		t.Fatal(err)
	}
	want := []CaseMock{{Command: "aws", Script: "fixtures/mock-cli.sh"}, {Command: "npm", Script: "fixtures/mock-cli.sh"}}
	if len(got) != 1 || !reflect.DeepEqual(got[0].Mocks, want) {
		t.Fatalf("Mocks = %+v, want %+v", got, want)
	}
}

func TestMocksRequireSandboxNamesCaseAndFile(t *testing.T) {
	wsEnv(t)
	writeMockScript(t, "fixtures/mock-cli.sh", "#!/bin/sh\n")
	writeCfgFile(t, filepath.Join("evals", "cases", "agent-m", "nosandbox.yaml"),
		"name: nosb\ninput: x\nmocks:\n  - command: aws\n    script: fixtures/mock-cli.sh\n")
	_, err := loadCases("agent-m")
	if err == nil {
		t.Fatal("expected an error for mocks without requires_sandbox")
	}
	for _, want := range []string{`"nosb"`, "nosandbox.yaml", "requires_sandbox: true"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

func TestMocksValidation(t *testing.T) {
	wsEnv(t)
	writeMockScript(t, "fixtures/mock-cli.sh", "#!/bin/sh\necho hi\n")
	if err := os.MkdirAll("evals/fixtures/adir", 0o755); err != nil {
		t.Fatal(err)
	}
	const ok = "fixtures/mock-cli.sh"

	tests := []struct {
		name    string
		sandbox bool
		mocks   []CaseMock
		wantErr string // empty means valid
	}{
		{"valid", true, []CaseMock{{"aws", ok}}, ""},
		{"valid with punctuation", true, []CaseMock{{"python3.12", ok}, {"g++", ok}, {"a_b-c", ok}}, ""},
		{"valid multiple", true, []CaseMock{{"aws", ok}, {"npm", ok}}, ""},
		{"no mocks", false, nil, ""},
		{"requires sandbox", false, []CaseMock{{"aws", ok}}, "requires_sandbox: true"},
		{"gh reserved", true, []CaseMock{{"gh", ok}}, "reserved"},
		{"empty command", true, []CaseMock{{"", ok}}, "command is required"},
		{"command with slash", true, []CaseMock{{"a/b", ok}}, "invalid command"},
		{"command dotdot", true, []CaseMock{{"..", ok}}, "invalid command"},
		{"command leading dot", true, []CaseMock{{".aws", ok}}, "invalid command"},
		{"command leading dash", true, []CaseMock{{"-aws", ok}}, "invalid command"},
		{"command with space", true, []CaseMock{{"a b", ok}}, "invalid command"},
		{"command absolute", true, []CaseMock{{"/usr/bin/aws", ok}}, "invalid command"},
		{"duplicate command", true, []CaseMock{{"aws", ok}, {"aws", ok}}, "duplicate command"},
		{"empty script", true, []CaseMock{{"aws", ""}}, "script is required"},
		{"absolute script", true, []CaseMock{{"aws", "/etc/passwd"}}, "invalid script"},
		{"dotdot script", true, []CaseMock{{"aws", "../x.sh"}}, "invalid script"},
		{"nested dotdot script", true, []CaseMock{{"aws", "fixtures/../../x.sh"}}, "invalid script"},
		{"missing script", true, []CaseMock{{"aws", "fixtures/nope.sh"}}, "not found"},
		{"script is directory", true, []CaseMock{{"aws", "fixtures/adir"}}, "not a regular file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := TestCase{Name: "c", RequiresSandbox: tt.sandbox, Mocks: tt.mocks}
			err := validateSandboxFields(tc, "c.yaml")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			for _, want := range []string{tt.wantErr, `"c"`, "c.yaml"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err, want)
				}
			}
		})
	}
}

func TestMocksStagedIntoBinDir(t *testing.T) {
	wsEnv(t)
	script := "#!/bin/sh\necho mock \"$0\"\n\x00binary\xff tail\n"
	writeMockScript(t, "fixtures/mock-cli.sh", script)
	// A script that is not executable on disk must still be staged 0755.
	if err := os.Chmod("evals/fixtures/mock-cli.sh", 0o600); err != nil {
		t.Fatal(err)
	}
	writeMockScript(t, "fixtures/other.sh", "#!/bin/sh\nexit 3\n")

	tc := TestCase{Name: "m", RequiresSandbox: true, Mocks: []CaseMock{
		{Command: "aws", Script: "fixtures/mock-cli.sh"},
		{Command: "npm", Script: "fixtures/other.sh"},
	}}
	plain := newWS(t, TestCase{Name: "plain"})
	ws := newWS(t, tc)

	for cmd, want := range map[string]string{"aws": script, "npm": "#!/bin/sh\nexit 3\n"} {
		p := filepath.Join(ws.BinDir, cmd)
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("%s mode = %v, want 0755", cmd, info.Mode().Perm())
		}
		if got := readFileT(t, p); got != want {
			t.Errorf("%s content differs from script", cmd)
		}
	}

	// The fake gh is untouched by mocks.
	if got, want := readFileT(t, filepath.Join(ws.BinDir, "gh")), readFileT(t, filepath.Join(plain.BinDir, "gh")); got != want {
		t.Error("BinDir/gh changed when mocks are staged")
	}
	if got := binNames(t, ws.BinDir); !reflect.DeepEqual(got, []string{"aws", "gh", "npm"}) {
		t.Errorf("BinDir = %v", got)
	}
	// Mocks live outside the agent-writable workspace.
	if _, err := os.Stat(filepath.Join(ws.Dir, "aws")); !os.IsNotExist(err) {
		t.Errorf("mock leaked into the workspace: %v", err)
	}
}

func TestMocksAbsentLeavesOnlyGH(t *testing.T) {
	wsEnv(t)
	ws := newWS(t, TestCase{Name: "plain"})
	if got := binNames(t, ws.BinDir); !reflect.DeepEqual(got, []string{"gh"}) {
		t.Errorf("BinDir = %v, want only gh", got)
	}
}

func TestMocksStageRefusesUnsafeInput(t *testing.T) {
	wsEnv(t)
	writeMockScript(t, "fixtures/mock-cli.sh", "#!/bin/sh\n")
	for name, m := range map[string]CaseMock{
		"gh not overwritten": {Command: "gh", Script: "fixtures/mock-cli.sh"},
		"path traversal":     {Command: "../evil", Script: "fixtures/mock-cli.sh"},
		"script traversal":   {Command: "aws", Script: "../x"},
		"missing script":     {Command: "aws", Script: "fixtures/nope.sh"},
	} {
		t.Run(name, func(t *testing.T) {
			before := dirNames(t, os.Getenv(workspaceRootEnv))
			if _, err := newCaseWorkspace(TestCase{Name: "u", RequiresSandbox: true, Mocks: []CaseMock{m}}); err == nil {
				t.Fatal("expected an error")
			} else if !strings.Contains(err.Error(), `"u"`) {
				t.Errorf("error %q does not name the case", err)
			}
			if after := dirNames(t, os.Getenv(workspaceRootEnv)); !reflect.DeepEqual(before, after) {
				t.Errorf("failed staging left files behind: %v -> %v", before, after)
			}
		})
	}
}

func binNames(t *testing.T, dir string) []string {
	t.Helper()
	return dirNames(t, dir)
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}
