package checks

import (
	"strings"
	"testing"
)

// envValue returns the value of name in a KEY=VALUE environment slice, and
// whether it is present.
func envValue(env []string, name string) (string, bool) {
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if k == name {
			return v, true
		}
	}
	return "", false
}

func TestHermeticGitEnv_StripsRepoSelectingVars(t *testing.T) {
	// These inherited variables let a git command resolve refs or objects
	// outside the selected directory, so a hermetic environment must drop them.
	// GIT_NAMESPACE changes ref resolution; GIT_ALTERNATE_OBJECT_DIRECTORIES
	// adds external object stores.
	stripped := []string{
		"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE",
		"GIT_OBJECT_DIRECTORY", "GIT_COMMON_DIR",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE",
		"GIT_CONFIG_GLOBAL_EXTRA", "GIT_AUTHOR_NAME", "GIT_COMMITTER_EMAIL",
	}
	for _, name := range stripped {
		t.Setenv(name, "poison")
	}
	// An unrelated variable the hermetic env must keep.
	t.Setenv("UNRELATED_ENV_MARKER", "kept")

	env := HermeticGitEnv()
	for _, name := range stripped {
		if v, ok := envValue(env, name); ok {
			t.Errorf("%s must be stripped, got %q", name, v)
		}
	}
	if v, ok := envValue(env, "UNRELATED_ENV_MARKER"); !ok || v != "kept" {
		t.Errorf("UNRELATED_ENV_MARKER = (%q, %v), want kept", v, ok)
	}
	// The config-disabling overrides are always appended.
	for name, want := range map[string]string{
		"GIT_CONFIG_GLOBAL":   "/dev/null",
		"GIT_CONFIG_SYSTEM":   "/dev/null",
		"GIT_CONFIG_NOSYSTEM": "1",
	} {
		if v, ok := envValue(env, name); !ok || v != want {
			t.Errorf("%s = (%q, %v), want %q", name, v, ok, want)
		}
	}
}
