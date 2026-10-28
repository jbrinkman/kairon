package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scopeFixture returns the absolute path of a kairon scope check fixture.
func scopeFixture(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "..", "internal", "guard", "testdata", "scope", name))
	require.NoError(t, err)
	return p
}

// runScopeCheck executes `kairon scope check <args...>` through the root
// command and restores the flag state afterwards.
func runScopeCheck(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cc, _, ferr := rootCmd.Find([]string{"scope", "check"})
	if ferr != nil || cc == nil || cc.Name() != "check" || cc.Parent().Name() != "scope" {
		t.Fatalf("kairon scope check is not registered on the root command (Find: %v)", ferr)
	}
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(append([]string{"scope", "check"}, args...))
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		cc.SilenceUsage = false
		cc.SilenceErrors = false
		cc.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	})
	err := rootCmd.Execute()
	return stdout.String(), err
}

type scopeOutput struct {
	Status   string `json:"status"`
	Findings []struct {
		Kind string `json:"kind"`
		Item string `json:"item"`
		Line int    `json:"line"`
		Text string `json:"text"`
	} `json:"findings"`
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return 1
}

func TestScopeCheck_PassPrintsPassStatus(t *testing.T) {
	out, err := runScopeCheck(t, "--input", scopeFixture(t, "input.yaml"), "--spec", scopeFixture(t, "pass.md"))
	require.NoError(t, err)
	assert.Equal(t, "{\"status\":\"pass\"}\n", out)
}

func TestScopeCheck_FindingsExitOne(t *testing.T) {
	cases := []struct {
		spec, kind, item string
	}{
		{"uncovered-s2.md", "uncovered", "s-2"},
		{"unknown-task.md", "unknown_task", "s-2"},
		{"deferred-section.md", "deferred_section", "Deferred work"},
	}
	for _, c := range cases {
		t.Run(c.spec, func(t *testing.T) {
			out, err := runScopeCheck(t, "--input", scopeFixture(t, "input.yaml"), "--spec", scopeFixture(t, c.spec))
			require.Error(t, err)
			assert.Equal(t, 1, exitCodeOf(err))
			var got scopeOutput
			require.NoError(t, json.Unmarshal([]byte(out), &got), out)
			assert.Equal(t, "fail", got.Status)
			require.Len(t, got.Findings, 1, out)
			assert.Equal(t, c.kind, got.Findings[0].Kind)
			assert.Equal(t, c.item, got.Findings[0].Item)
		})
	}
}

func TestScopeCheck_UnverifiedInputExitsThree(t *testing.T) {
	out, err := runScopeCheck(t, "--input", scopeFixture(t, "tampered.yaml"), "--spec", scopeFixture(t, "pass.md"))
	require.Error(t, err)
	assert.Equal(t, 3, exitCodeOf(err))
	assert.Empty(t, out, "no status is printed when the check cannot run")
}

func TestScopeCheck_MissingFilesExitThree(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	for _, args := range [][]string{
		{"--input", missing, "--spec", scopeFixture(t, "pass.md")},
		{"--input", scopeFixture(t, "input.yaml"), "--spec", missing},
	} {
		out, err := runScopeCheck(t, args...)
		require.Error(t, err)
		assert.Equal(t, 3, exitCodeOf(err), args)
		assert.Empty(t, out)
	}
}

func TestScopeCheck_MalformedPlanReportsUnknownTasks(t *testing.T) {
	data, err := os.ReadFile(scopeFixture(t, "pass.md"))
	require.NoError(t, err)
	spec := filepath.Join(t.TempDir(), "spec.md")
	broken := bytes.Replace(data, []byte("version: \"1.0\""), []byte("version: \"1.0\"\nbogus_key: 1"), 1)
	require.NoError(t, os.WriteFile(spec, broken, 0o644))

	out, err := runScopeCheck(t, "--input", scopeFixture(t, "input.yaml"), "--spec", spec)
	assert.Equal(t, 1, exitCodeOf(err))
	var got scopeOutput
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	require.NotEmpty(t, got.Findings)
	for _, f := range got.Findings {
		assert.Equal(t, "unknown_task", f.Kind)
	}
}
