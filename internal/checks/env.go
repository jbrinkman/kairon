package checks

import (
	"os"
	"strings"
)

// GitHubCredentialEnv names GitHub credential/host variables that never reach a
// check command (or an eval container), so a check cannot use the caller's
// GitHub identity.
var GitHubCredentialEnv = map[string]bool{
	"GH_TOKEN":                true,
	"GITHUB_TOKEN":            true,
	"GH_ENTERPRISE_TOKEN":     true,
	"GITHUB_ENTERPRISE_TOKEN": true,
	"GH_HOST":                 true,
}

// HermeticGitEnv returns the process environment with user and system git
// configuration disabled and repository-selecting variables removed.
func HermeticGitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch {
		case name == "GIT_DIR", name == "GIT_WORK_TREE", name == "GIT_INDEX_FILE",
			name == "GIT_OBJECT_DIRECTORY", name == "GIT_COMMON_DIR",
			strings.HasPrefix(name, "GIT_CONFIG"), strings.HasPrefix(name, "GIT_AUTHOR_"),
			strings.HasPrefix(name, "GIT_COMMITTER_"):
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
	)
}
