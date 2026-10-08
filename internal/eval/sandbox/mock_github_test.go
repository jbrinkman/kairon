package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGHEnv is a hermetic harness that runs the embedded fake gh with a shell
// on the host against a temp KAIRON_EVAL_DIR. No container, daemon or network
// is involved.
type fakeGHEnv struct {
	t        *testing.T
	shellCmd []string // e.g. ["/bin/sh"] or ["/usr/bin/busybox", "sh"]
	scratch  string   // working directory for the calls (body files live here)
	evalDir  string
	script   string
	path     string // PATH given to the script
}

type ghResult struct {
	stdout, stderr string
	exit           int
}

func newFakeGHEnv(t *testing.T, shellCmd []string) *fakeGHEnv {
	t.Helper()
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	require.NoError(t, WriteFakeGH(binDir))
	evalDir := filepath.Join(root, "eval")
	require.NoError(t, os.MkdirAll(evalDir, 0o755))
	scratch := filepath.Join(root, "work")
	require.NoError(t, os.MkdirAll(scratch, 0o755))
	return &fakeGHEnv{
		t: t, shellCmd: shellCmd, scratch: scratch, evalDir: evalDir,
		script: filepath.Join(binDir, "gh"),
		path:   "/usr/bin:/bin",
	}
}

func (e *fakeGHEnv) run(stdin string, args ...string) ghResult {
	e.t.Helper()
	argv := append(append([]string{}, e.shellCmd[1:]...), e.script)
	argv = append(argv, args...)
	cmd := exec.Command(e.shellCmd[0], argv...)
	cmd.Dir = e.scratch
	cmd.Env = []string{"KAIRON_EVAL_DIR=" + e.evalDir, "PATH=" + e.path, "HOME=" + e.scratch}
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	res := ghResult{}
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		require.True(e.t, errors.As(err, &ee), "run fake gh: %v", err)
		res.exit = ee.ExitCode()
	}
	res.stdout, res.stderr = out.String(), errb.String()
	return res
}

func (e *fakeGHEnv) logLines() []string {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(e.evalDir, "gh.log"))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(e.t, err)
	s := strings.TrimSuffix(string(data), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func (e *fakeGHEnv) writeFile(name, content string) {
	e.t.Helper()
	require.NoError(e.t, os.WriteFile(filepath.Join(e.scratch, name), []byte(content), 0o644))
}

func (e *fakeGHEnv) readEval(name string) []byte {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(e.evalDir, name))
	require.NoError(e.t, err)
	return data
}

func (e *fakeGHEnv) configureIssue(issue GHIssue) {
	e.t.Helper()
	doc, text, err := RenderGHIssue(issue)
	require.NoError(e.t, err)
	require.NoError(e.t, os.WriteFile(filepath.Join(e.evalDir, "gh-issue.json"), doc, 0o644))
	require.NoError(e.t, os.WriteFile(filepath.Join(e.evalDir, "gh-issue.txt"), []byte(text), 0o644))
}

// forEachShell runs fn as a subtest per available POSIX shell. /bin/sh is
// required; dash and busybox ash (the sandbox image's shell) run when installed.
func forEachShell(t *testing.T, fn func(t *testing.T, e *fakeGHEnv)) {
	t.Helper()
	candidates := []struct {
		name     string
		bin      string
		args     []string
		required bool
	}{
		{"sh", "sh", nil, true},
		{"dash", "dash", nil, false},
		{"busybox", "busybox", []string{"sh"}, false},
	}
	for _, c := range candidates {
		t.Run(c.name, func(t *testing.T) {
			bin, err := exec.LookPath(c.bin)
			if err != nil {
				if c.required {
					t.Skipf("%s not available: %v", c.bin, err)
				}
				t.Skipf("%s not installed", c.bin)
			}
			fn(t, newFakeGHEnv(t, append([]string{bin}, c.args...)))
		})
	}
}

func TestFakeGH_IssueCreateLogsAndCopiesBody(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		body := "# Heading\n\nline with trailing spaces  \n\ttabbed\nno final newline"
		e.writeFile("b.md", body)

		res := e.run("", "issue", "create", "--title", "t", "--body-file", "b.md")
		assert.Equal(t, 0, res.exit, res.stderr)
		assert.Equal(t, "https://github.com/fake-owner/fake-repo/issues/1\n", res.stdout)
		assert.Equal(t, []string{"gh issue create --title t --body-file b.md"}, e.logLines())
		assert.Equal(t, body, string(e.readEval("gh-body-1.md")), "gh-body-1.md must be byte-equal to b.md")

		res = e.run("", "issue", "create", "--title", "t2", "--body-file", "b.md")
		assert.Equal(t, 0, res.exit, res.stderr)
		assert.Equal(t, "https://github.com/fake-owner/fake-repo/issues/2\n", res.stdout)
		assert.Equal(t, body, string(e.readEval("gh-body-2.md")))
		assert.Len(t, e.logLines(), 2)
	})
}

func TestFakeGH_BodyFileForms(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		e.writeFile("a.md", "A")
		e.writeFile("c.md", "C")

		assert.Equal(t, 0, e.run("", "pr", "create", "-F", "a.md").exit)
		assert.Equal(t, 0, e.run("", "pr", "create", "--body-file=c.md").exit)
		assert.Equal(t, "A", string(e.readEval("gh-body-1.md")))
		assert.Equal(t, "C", string(e.readEval("gh-body-2.md")))
	})
}

func TestFakeGH_BodyFileStdin(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		res := e.run("from stdin\nsecond line\n", "issue", "create", "--title", "t", "--body-file", "-")
		assert.Equal(t, 0, res.exit, res.stderr)
		assert.Equal(t, "from stdin\nsecond line\n", string(e.readEval("gh-body-1.md")))
		assert.Equal(t, []string{"gh issue create --title t --body-file -"}, e.logLines())
	})
}

func TestFakeGH_MissingBodyFileFailsButIsLogged(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		res := e.run("", "issue", "create", "--title", "t", "--body-file", "nope.md")
		assert.Equal(t, 1, res.exit)
		assert.Contains(t, res.stderr, "open nope.md: no such file or directory")
		assert.Empty(t, res.stdout)
		assert.Equal(t, []string{"gh issue create --title t --body-file nope.md"}, e.logLines())
		_, err := os.Stat(filepath.Join(e.evalDir, "gh-body-1.md"))
		assert.True(t, os.IsNotExist(err), "a failed copy must not claim a body slot")
	})
}

func TestFakeGH_BodyFileMissingValue(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		res := e.run("", "issue", "create", "--body-file")
		assert.Equal(t, 1, res.exit)
		assert.Contains(t, res.stderr, "flag needs an argument")
		assert.Len(t, e.logLines(), 1)
	})
}

func TestFakeGH_PRCreateURL(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		res := e.run("", "pr", "create", "--title", "t", "--body", "b")
		assert.Equal(t, 0, res.exit, res.stderr)
		assert.Equal(t, "https://github.com/fake-owner/fake-repo/pull/1\n", res.stdout)
	})
}

func TestFakeGH_IssueViewJSONFields(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		e.configureIssue(GHIssue{Number: 42, Title: `Add "widget" <now>`, Body: "multi\nline body\twith tab", Labels: []string{"bug"}})

		res := e.run("", "issue", "view", "--json", "title,body")
		require.Equal(t, 0, res.exit, res.stderr)
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(res.stdout), &got), res.stdout)
		assert.Len(t, got, 2)
		assert.Equal(t, `Add "widget" <now>`, got["title"])
		assert.Equal(t, "multi\nline body\twith tab", got["body"])

		// Same result with the number, a URL, and --json=fields.
		for _, args := range [][]string{
			{"issue", "view", "42", "--json", "title,body"},
			{"issue", "view", "#42", "--json=title,body"},
			{"issue", "view", "https://github.com/o/r/issues/42", "--json", "title,body"},
		} {
			again := e.run("", args...)
			assert.Equal(t, 0, again.exit, "%v: %s", args, again.stderr)
			assert.Equal(t, res.stdout, again.stdout, "%v", args)
		}

		// Requested order is kept and nested fields work.
		res = e.run("", "issue", "view", "--json", "number,labels,author")
		require.Equal(t, 0, res.exit, res.stderr)
		var nested struct {
			Number int `json:"number"`
			Labels []struct {
				Name string `json:"name"`
			} `json:"labels"`
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
		}
		require.NoError(t, json.Unmarshal([]byte(res.stdout), &nested), res.stdout)
		assert.Equal(t, 42, nested.Number)
		require.Len(t, nested.Labels, 1)
		assert.Equal(t, "bug", nested.Labels[0].Name)
		assert.Equal(t, "fake-user", nested.Author.Login)
		assert.Less(t, strings.Index(res.stdout, `"number"`), strings.Index(res.stdout, `"labels"`))
	})
}

func TestFakeGH_IssueViewText(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		e.configureIssue(GHIssue{Number: 7, Title: "Add widget", Body: "Please add it.", State: "CLOSED", Author: "octocat", Labels: []string{"bug", "ui"}})

		res := e.run("", "issue", "view", "7")
		require.Equal(t, 0, res.exit, res.stderr)
		assert.Contains(t, res.stdout, "title:\tAdd widget\n")
		assert.Contains(t, res.stdout, "state:\tCLOSED\n")
		assert.Contains(t, res.stdout, "author:\toctocat\n")
		assert.Contains(t, res.stdout, "labels:\tbug, ui\n")
		assert.Contains(t, res.stdout, "number:\t7\n")
		assert.Contains(t, res.stdout, "--\nPlease add it.\n")

		// No number argument uses the configured issue.
		assert.Equal(t, res.stdout, e.run("", "issue", "view").stdout)
	})
}

func TestFakeGH_IssueViewFailures(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		// No gh_issue configured.
		res := e.run("", "issue", "view", "1", "--json", "title")
		assert.Equal(t, 1, res.exit)
		assert.Contains(t, res.stderr, "no issue configured for this case (gh_issue)")
		assert.Empty(t, res.stdout)

		e.configureIssue(GHIssue{Number: 42, Title: "T", Body: "B"})

		tests := []struct {
			name    string
			args    []string
			wantErr string
		}{
			{"different number", []string{"issue", "view", "7", "--json", "title"}, "could not resolve to an issue with the number 7"},
			{"different url", []string{"issue", "view", "https://github.com/o/r/issues/9"}, "could not resolve to an issue with the number 9"},
			{"non numeric", []string{"issue", "view", "abc"}, "invalid issue number"},
			{"unknown field", []string{"issue", "view", "--json", "title,bogus"}, `Unknown JSON field: "bogus"`},
			{"unsafe field", []string{"issue", "view", "--json", "ti.tle"}, `Unknown JSON field: "ti.tle"`},
			{"empty field list", []string{"issue", "view", "--json", ""}, "Specify one or more comma-separated fields"},
			{"json without value", []string{"issue", "view", "--json"}, "flag needs an argument"},
			{"jq", []string{"issue", "view", "--json", "title", "--jq", ".title"}, "--jq is not supported"},
			{"jq short", []string{"issue", "view", "--json", "title", "-q", ".title"}, "--jq is not supported"},
			{"jq equals", []string{"issue", "view", "--json", "title", "--jq=.title"}, "--jq is not supported"},
			{"template", []string{"issue", "view", "--json", "title", "--template", "{{.title}}"}, "--template is not supported"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				before := len(e.logLines())
				res := e.run("", tt.args...)
				assert.Equal(t, 1, res.exit)
				assert.Contains(t, res.stderr, tt.wantErr)
				assert.Empty(t, res.stdout, "failures must not print partial JSON")
				assert.Len(t, e.logLines(), before+1, "failed calls are still logged")
			})
		}
	})
}

func TestFakeGH_LogQuotingAndSingleLine(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		e.run("", "issue", "comment", "1", "--body", "two words")
		e.run("", "issue", "comment", "1", "--body", "it's")
		e.run("", "issue", "comment", "1", "--body", "line1\nline2\n\nline4")
		e.run("", "issue", "comment", "1", "--body", "")
		e.run("", "issue", "comment", "1", "--body", "a$b `c` \"d\" ;e")
		e.run("", "issue", "list", "--label", "a/b-c_d.e:f=g@h%i+j,k")

		want := []string{
			`gh issue comment 1 --body 'two words'`,
			`gh issue comment 1 --body 'it'\''s'`,
			`gh issue comment 1 --body 'line1 line2  line4'`,
			`gh issue comment 1 --body ''`,
			`gh issue comment 1 --body 'a$b ` + "`c`" + ` "d" ;e'`,
			`gh issue list --label a/b-c_d.e:f=g@h%i+j,k`,
		}
		assert.Equal(t, want, e.logLines())

		raw := e.readEval("gh.log")
		assert.Equal(t, len(want), strings.Count(string(raw), "\n"), "exactly one newline per call")
	})
}

func TestFakeGH_UnsimulatedCommandFailsAndIsLogged(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		for _, args := range [][]string{
			{"repo", "delete", "owner/name", "--yes"},
			{"api", "-X", "POST", "/repos/o/r/issues"},
			{"issue", "delete", "1"},
			{"pr", "merge", "5"},
			{"auth", "login"},
			{"release", "create", "v1"},
			{},
		} {
			before := len(e.logLines())
			res := e.run("", args...)
			assert.Equal(t, 1, res.exit, "%v", args)
			assert.Contains(t, res.stderr, "kairon fake gh: ", "%v", args)
			assert.Contains(t, res.stderr, "is not simulated", "%v", args)
			assert.Empty(t, res.stdout, "%v", args)
			assert.Len(t, e.logLines(), before+1, "%v must be logged", args)
		}
		assert.Equal(t, "gh", e.logLines()[len(e.logLines())-1])
		assert.Equal(t, "gh repo delete owner/name --yes", e.logLines()[0])

		res := e.run("", "pr", "merge", "my branch")
		assert.Contains(t, res.stderr, `kairon fake gh: "pr merge 'my branch'" is not simulated`)
	})
}

func TestFakeGH_SimulatedCommandsSucceed(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		for _, args := range [][]string{
			{"issue", "list"},
			{"issue", "edit", "1", "--add-label", "x"},
			{"issue", "comment", "1", "--body", "hi"},
			{"issue", "close", "1"},
			{"issue", "reopen", "1"},
			{"pr", "list"},
			{"pr", "view", "1"},
			{"pr", "comment", "1", "--body", "hi"},
			{"pr", "edit", "1", "--title", "t"},
			{"auth", "status"},
			{"--version"},
		} {
			res := e.run("", args...)
			assert.Equal(t, 0, res.exit, "%v: %s", args, res.stderr)
		}
		assert.Contains(t, e.run("", "auth", "status").stdout, "Logged in")
		assert.Contains(t, e.run("", "--version").stdout, "gh version")
	})
}

func TestFakeGH_RequiresEvalDir(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		e.evalDir = filepath.Join(e.scratch, "does-not-exist")
		res := e.run("", "issue", "list")
		assert.Equal(t, 1, res.exit)
		assert.Contains(t, res.stderr, "KAIRON_EVAL_DIR is not a directory")
	})
}

// TestFakeGH_NeverInvokesAnotherGH puts a trap `gh` first on PATH and checks it
// is never executed, for simulated, failing and unsimulated calls alike.
func TestFakeGH_NeverInvokesAnotherGH(t *testing.T) {
	forEachShell(t, func(t *testing.T, e *fakeGHEnv) {
		trapDir := t.TempDir()
		marker := filepath.Join(trapDir, "trap-ran")
		trap := "#!/bin/sh\necho ran > " + marker + "\nexit 0\n"
		require.NoError(t, os.WriteFile(filepath.Join(trapDir, "gh"), []byte(trap), 0o755))
		e.path = trapDir + ":/usr/bin:/bin"

		e.configureIssue(GHIssue{Title: "T"})
		e.writeFile("b.md", "x")
		e.run("", "issue", "create", "--body-file", "b.md")
		e.run("", "issue", "view", "--json", "title")
		e.run("", "issue", "view", "--json", "nope")
		e.run("", "auth", "status")
		e.run("", "repo", "delete", "x")

		_, err := os.Stat(marker)
		assert.True(t, os.IsNotExist(err), "the fake must never execute another gh")
	})
}

func TestFakeGHScript_NoExternalGHInvocation(t *testing.T) {
	for i, line := range strings.Split(string(fakeGHScript), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		assert.NotRegexp(t, `(^|[;&|(]\s*|\$\(|command\s+|exec\s+)gh(\s|$)`, trimmed, "line %d invokes gh", i+1)
	}
}

func TestWriteFakeGH(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "bin")
	require.NoError(t, WriteFakeGH(dir))

	info, err := os.Stat(filepath.Join(dir, "gh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), dirInfo.Mode().Perm())

	content, err := os.ReadFile(filepath.Join(dir, "gh"))
	require.NoError(t, err)
	assert.Equal(t, fakeGHScript, content)
	assert.True(t, strings.HasPrefix(string(content), "#!/bin/sh\n"))

	// Rewriting over an existing, less-permissive file fixes the mode.
	require.NoError(t, os.Chmod(filepath.Join(dir, "gh"), 0o600))
	require.NoError(t, WriteFakeGH(dir))
	info, err = os.Stat(filepath.Join(dir, "gh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestRenderGHIssue(t *testing.T) {
	doc, text, err := RenderGHIssue(GHIssue{
		Title:  "Add \"widget\" & <more>",
		Body:   "line one\nline two\ttab \u2028 end",
		Labels: []string{"bug", "ui"},
	})
	require.NoError(t, err)

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(doc, &got), string(doc))
	assert.Len(t, got, 7)
	assert.JSONEq(t, `1`, string(got["number"]))
	assert.JSONEq(t, `"OPEN"`, string(got["state"]))
	assert.JSONEq(t, `{"login":"fake-user"}`, string(got["author"]))
	assert.JSONEq(t, `[{"name":"bug"},{"name":"ui"}]`, string(got["labels"]))
	assert.JSONEq(t, `"https://github.com/fake-owner/fake-repo/issues/1"`, string(got["url"]))
	var title, body string
	require.NoError(t, json.Unmarshal(got["title"], &title))
	require.NoError(t, json.Unmarshal(got["body"], &body))
	assert.Equal(t, "Add \"widget\" & <more>", title)
	assert.Equal(t, "line one\nline two\ttab \u2028 end", body)

	// Exactly one top-level field per line: braces on their own lines, every
	// other line is a two-space-indented `"name": value` with commas between.
	lines := strings.Split(strings.TrimSuffix(string(doc), "\n"), "\n")
	require.Len(t, lines, len(got)+2)
	assert.Equal(t, "{", lines[0])
	assert.Equal(t, "}", lines[len(lines)-1])
	for i, l := range lines[1 : len(lines)-1] {
		assert.Regexp(t, `^  "[a-z]+": `, l)
		if i < len(got)-1 {
			assert.True(t, strings.HasSuffix(l, ","), "line %q needs a trailing comma", l)
		} else {
			assert.False(t, strings.HasSuffix(l, ","), "last field must not have a comma")
		}
	}

	assert.Contains(t, text, "title:\tAdd \"widget\" & <more>\n")
	assert.Contains(t, text, "state:\tOPEN\n")
	assert.Contains(t, text, "author:\tfake-user\n")
	assert.Contains(t, text, "labels:\tbug, ui\n")
	assert.Contains(t, text, "number:\t1\n")
	assert.Contains(t, text, "--\nline one\nline two\ttab")

	// Deterministic.
	doc2, text2, err := RenderGHIssue(GHIssue{Title: "Add \"widget\" & <more>", Body: "line one\nline two\ttab \u2028 end", Labels: []string{"bug", "ui"}})
	require.NoError(t, err)
	assert.Equal(t, doc, doc2)
	assert.Equal(t, text, text2)
}

func TestRenderGHIssue_ExplicitFieldsAndEmptyLabels(t *testing.T) {
	doc, _, err := RenderGHIssue(GHIssue{Number: 42, Title: "T", State: "CLOSED", Author: "octocat"})
	require.NoError(t, err)
	var got struct {
		Number int                 `json:"number"`
		State  string              `json:"state"`
		Labels []map[string]string `json:"labels"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
	}
	require.NoError(t, json.Unmarshal(doc, &got))
	assert.Equal(t, 42, got.Number)
	assert.Equal(t, "CLOSED", got.State)
	assert.Equal(t, "octocat", got.Author.Login)
	assert.NotNil(t, got.Labels, "labels renders as [] rather than null")
	assert.Contains(t, string(doc), `"labels": []`)
}

func TestRenderGHIssue_Errors(t *testing.T) {
	_, _, err := RenderGHIssue(GHIssue{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")

	_, _, err = RenderGHIssue(GHIssue{Title: "  "})
	require.Error(t, err)

	_, _, err = RenderGHIssue(GHIssue{Title: "T", Number: -3})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "number")
}

// TestFakeGH_ConcurrentCreatesGetUniqueNumbers fires many `gh issue/pr create`
// calls at once against one eval dir and asserts every returned number is
// unique. Before claim_seq, the number came from a `wc -l` of gh.log read
// after a separate append, so two concurrent creates could return the same
// number. Runs only on /bin/sh (the always-present shell); each call is its
// own subprocess sharing e.evalDir.
func TestFakeGH_ConcurrentCreatesGetUniqueNumbers(t *testing.T) {
	e := newFakeGHEnv(t, []string{"/bin/sh"})

	const n = 24
	results := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			kind := "issue"
			if i%2 == 1 {
				kind = "pr"
			}
			res := e.run("", kind, "create", "--title", "t", "--body", "b")
			results[i] = strings.TrimSpace(res.stdout)
		}(i)
	}
	wg.Wait()

	// Compare the NUMERIC suffix, not the full URL: issue and pr share one
	// counter, so /issues/1 and /pull/1 would be a collision (same number) even
	// though the URLs differ. A regression that reused a number across an issue
	// and a pr must be caught here.
	seen := make(map[int]int, n)
	var nums []int
	for i, url := range results {
		if url == "" {
			t.Fatalf("call %d produced no URL (exit/err?)", i)
		}
		idx := strings.LastIndex(url, "/")
		num, err := strconv.Atoi(url[idx+1:])
		if err != nil {
			t.Fatalf("call %d URL %q has no numeric suffix: %v", i, url, err)
		}
		if prev, dup := seen[num]; dup {
			t.Fatalf("duplicate create number %d from calls %d (%s) and %d (%s): the shared counter reused a number under concurrency",
				num, prev, results[prev], i, url)
		}
		seen[num] = i
		nums = append(nums, num)
	}
	// Every call was logged, and the shared counter yields exactly 1..n.
	assert.Len(t, e.logLines(), n)
	sort.Ints(nums)
	for i := 0; i < n; i++ {
		assert.Equal(t, i+1, nums[i], "create numbers should be exactly 1..%d, got %v", n, nums)
	}
}

// TestFakeGH_BodyFileUnwritableEvalDirFailsFast verifies that when $EVALDIR
// cannot be written (here: made read-only), a `gh ... --body-file` call fails
// with a write error instead of looping forever in the claim helper and
// hanging until the case timeout. The claim loop distinguishes a name
// collision (retry) from an unwritable dir (bail), so this returns quickly.
func TestFakeGH_BodyFileUnwritableEvalDirFailsFast(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root bypasses directory write permissions")
	}
	e := newFakeGHEnv(t, []string{"/bin/sh"})
	e.writeFile("b.md", "body")

	// Pre-create a writable gh.log, then make .eval read-only. The call's log
	// append (to the existing writable file) still succeeds, so execution
	// reaches the claim loop — but no new gh-body-<n>.md / gh-seq-<n> can be
	// created. This is the scenario where the old unbounded loop hung; the
	// bounded loop must instead fail fast with a write error.
	require.NoError(t, os.WriteFile(filepath.Join(e.evalDir, "gh.log"), nil, 0o644))
	require.NoError(t, os.Chmod(e.evalDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(e.evalDir, 0o755) }) // let t.TempDir clean up

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	argv := append(append([]string{}, e.shellCmd[1:]...), e.script,
		"issue", "create", "--title", "t", "--body-file", "b.md")
	cmd := exec.CommandContext(ctx, e.shellCmd[0], argv...)
	cmd.Dir = e.scratch
	cmd.Env = []string{"KAIRON_EVAL_DIR=" + e.evalDir, "PATH=" + e.path, "HOME=" + e.scratch}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()

	require.NotEqual(t, context.DeadlineExceeded, ctx.Err(),
		"gh --body-file hung on an unwritable .eval instead of failing fast")
	require.Error(t, runErr, "expected non-zero exit; stdout=%q stderr=%q", out.String(), errb.String())
	assert.Empty(t, out.String(), "a failed body capture must not print a success URL")
	assert.Contains(t, errb.String(), "cannot write", "should report a write failure: %q", errb.String())
}
