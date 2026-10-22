package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/pflag"

	"github.com/jbrinkman/kairon/internal/provenance"
)

// plannerFakeStore is an in-memory provenance.IssueStore that records every
// call. No test in this file reaches the real gh.
type plannerFakeStore struct {
	mu      sync.Mutex
	calls   []string
	titles  map[int]string
	bodies  map[int]string
	nextNum int
}

func (f *plannerFakeStore) Revision(_ context.Context, n int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "Revision")
	return "rev", nil
}

func (f *plannerFakeStore) Create(_ context.Context, title, body string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "Create")
	f.nextNum++
	f.titles[f.nextNum] = title
	f.bodies[f.nextNum] = body
	return f.nextNum, nil
}

func (f *plannerFakeStore) Update(_ context.Context, n int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "Update")
	return nil
}

// plannerHarness replaces the store factory for one test and records what
// the command asked for.
type plannerHarness struct {
	store      *plannerFakeStore
	repos      []string
	labels     [][]string
	storeCalls int
}

func newPlannerHarness(t *testing.T) *plannerHarness {
	t.Helper()
	h := &plannerHarness{store: &plannerFakeStore{titles: map[int]string{}, bodies: map[int]string{}, nextNum: 400}}
	orig := plannerNewStore
	plannerNewStore = func(repo string, labels []string) provenance.IssueStore {
		h.storeCalls++
		h.repos = append(h.repos, repo)
		h.labels = append(h.labels, labels)
		return h.store
	}
	t.Cleanup(func() { plannerNewStore = orig })
	return h
}

// runPlanner executes `kairon planner <args...>` through the root command, so
// the test also proves the command tree is wired. It returns stdout, stderr
// and Execute's error, and restores flag state afterwards.
func runPlanner(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetIn(strings.NewReader(stdin))
	rootCmd.SetArgs(append([]string{"planner"}, args...))
	// Reset after every call, not only at test end: a test may run the
	// command twice.
	defer func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetIn(nil)
		rootCmd.SetArgs(nil)
		plannerSubmitCmd.Flags().VisitAll(func(f *pflag.Flag) {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		})
		plannerSubmitCmd.SilenceUsage = false
		plannerSubmitCmd.SilenceErrors = false
	}()
	err := rootCmd.Execute()
	return stdout.String(), stderr.String(), err
}

const plannerTestBody = `# %s

## Problem Statement
The export has no retry, so one transient error loses the run.

## Context
Export code lives in internal/export/export.go:10.

## Scope
### In Scope
- Retry failed export requests
### Out of Scope
- Changing the export format

## Acceptance Criteria
1. A failed request is retried up to 3 times. Verify: ` + "`go test ./internal/export -run TestRetry`" + `

## Dependencies
Dependencies: %s
`

// plannerDoc returns a valid interactive planner stage-output document.
func plannerDoc(title, deps string) string {
	body := fmt.Sprintf(plannerTestBody, title, deps)
	var b strings.Builder
	b.WriteString("schema_version: 1\nstage: planner\nresult: canonical\n")
	b.WriteString("source:\n  kind: document\n  ref: interactive-request\n")
	b.WriteString("generated: \"2026-01-02T03:04:05Z\"\nbody: |\n")
	for _, l := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		if l == "" {
			b.WriteString("\n")
		} else {
			b.WriteString("  " + l + "\n")
		}
	}
	return b.String()
}

func writePlannerFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "replies.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlannerSubmitIsRegisteredUnderPlanner(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"planner", "submit"})
	if err != nil || c == nil || c.Name() != "submit" {
		t.Fatalf("kairon planner submit is not registered (Find: %v)", err)
	}
	p, _, err := rootCmd.Find([]string{"planner"})
	if err != nil || p == nil || p.Name() != "planner" {
		t.Fatalf("kairon planner is not registered (Find: %v)", err)
	}
	for _, f := range []string{"file", "repo", "label", "known", "dry-run"} {
		if c.Flags().Lookup(f) == nil {
			t.Errorf("planner submit has no --%s flag", f)
		}
	}
}

func TestPlannerHelpListsSubmit(t *testing.T) {
	out, _, err := runPlanner(t, "", "--help")
	if err != nil {
		t.Fatalf("planner --help: %v", err)
	}
	if !strings.Contains(out, "submit") {
		t.Errorf("planner --help does not list submit:\n%s", out)
	}
}

func TestPlannerSubmitDryRunValidDocumentCallsNoStore(t *testing.T) {
	h := newPlannerHarness(t)
	file := writePlannerFile(t, plannerDoc("Add retry to export", "none"))
	out, _, err := runPlanner(t, "", "submit", "--dry-run", "--file", file)
	if err != nil {
		t.Fatalf("dry run of a valid document failed: %v", err)
	}
	if len(h.store.calls) != 0 {
		t.Errorf("dry run called the store: %v", h.store.calls)
	}
	if h.storeCalls != 0 {
		t.Errorf("dry run built a store %d times, want 0", h.storeCalls)
	}
	if !strings.Contains(out, "dry run") || strings.Contains(out, "committed") {
		t.Errorf("dry-run output = %q, want a dry-run line and no 'committed'", out)
	}
}

func TestPlannerSubmitDryRunNeedsNoRepo(t *testing.T) {
	newPlannerHarness(t)
	if _, _, err := runPlanner(t, plannerDoc("Add retry", "none"), "submit", "--dry-run"); err != nil {
		t.Fatalf("dry run from stdin without --repo failed: %v", err)
	}
}

func TestPlannerSubmitRejectedDocumentPrintsExpectedAndActual(t *testing.T) {
	h := newPlannerHarness(t)
	file := writePlannerFile(t, "here is your issue, I hope you like it")
	out, errOut, err := runPlanner(t, "", "submit", "--dry-run", "--file", file)
	if err == nil {
		t.Fatal("a rejected document returned no error, so the exit code would be 0")
	}
	all := out + errOut
	for _, want := range []string{"the whole reply is one stage-output document", "Expected", "Actual", "here is your issue"} {
		if !strings.Contains(all, want) {
			t.Errorf("output missing %q:\n%s", want, all)
		}
	}
	if len(h.store.calls) != 0 {
		t.Errorf("store called on a rejection: %v", h.store.calls)
	}
}

func TestPlannerSubmitRejectedWithoutDryRunWritesNothing(t *testing.T) {
	h := newPlannerHarness(t)
	first := plannerDoc("First", "none")
	_, errOut, err := runPlanner(t, first+"---\nnot a document\n", "submit", "--repo", "o/r")
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(h.store.calls) != 0 {
		t.Errorf("a bad second reply left writes behind: %v", h.store.calls)
	}
	if !strings.Contains(errOut, "reply 2") {
		t.Errorf("stderr should name the rejected reply, got:\n%s", errOut)
	}
}

func TestPlannerSubmitCommitsAndPrintsOneLinePerDocument(t *testing.T) {
	h := newPlannerHarness(t)
	input := plannerDoc("Add the store", "none") + "---\n" + plannerDoc("Use the store", "#{issue-1}")
	out, _, err := runPlanner(t, input, "submit", "--repo", "owner/repo")
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	want := "committed issue-1 -> #401\ncommitted issue-2 -> #402\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if got := h.store.calls; !reflect.DeepEqual(got, []string{"Create", "Create"}) {
		t.Errorf("store calls = %v, want two Create", got)
	}
	if h.store.titles[401] != "Add the store" {
		t.Errorf("title = %q", h.store.titles[401])
	}
	if !strings.Contains(h.store.bodies[402], "Dependencies: #401") {
		t.Errorf("second body should depend on #401:\n%s", h.store.bodies[402])
	}
	if len(h.repos) != 1 || h.repos[0] != "owner/repo" {
		t.Errorf("store repo = %v, want owner/repo", h.repos)
	}
}

func TestPlannerSubmitReadsFileAndIgnoresStdin(t *testing.T) {
	newPlannerHarness(t)
	file := writePlannerFile(t, plannerDoc("From file", "none"))
	out, _, err := runPlanner(t, "garbage that would be rejected", "submit", "--repo", "o/r", "--file", file)
	if err != nil {
		t.Fatalf("submit --file failed: %v", err)
	}
	if !strings.Contains(out, "committed issue-1 -> #401") {
		t.Errorf("stdout = %q", out)
	}
}

func TestPlannerSubmitLabelIsOffUnlessGiven(t *testing.T) {
	h := newPlannerHarness(t)
	if _, _, err := runPlanner(t, plannerDoc("One", "none"), "submit", "--repo", "o/r"); err != nil {
		t.Fatal(err)
	}
	if len(h.labels) != 1 || len(h.labels[0]) != 0 {
		t.Fatalf("labels without --label = %v, want none", h.labels)
	}

	h2 := newPlannerHarness(t)
	if _, _, err := runPlanner(t, plannerDoc("Two", "none"), "submit", "--repo", "o/r", "--label", "kairon"); err != nil {
		t.Fatal(err)
	}
	if len(h2.labels) != 1 || !reflect.DeepEqual(h2.labels[0], []string{"kairon"}) {
		t.Fatalf("labels with --label kairon = %v", h2.labels)
	}
}

func TestPlannerSubmitKnownSeedsTheChain(t *testing.T) {
	h := newPlannerHarness(t)
	doc := plannerDoc("Use the store", "#{issue-1}")
	out, _, err := runPlanner(t, doc, "submit", "--repo", "o/r", "--known", "issue-1=412")
	if err != nil {
		t.Fatalf("submit --known failed: %v", err)
	}
	if out != "committed issue-2 -> #401\n" {
		t.Errorf("stdout = %q, want the key to continue after the known ones", out)
	}
	if !strings.Contains(h.store.bodies[401], "Dependencies: #412") {
		t.Errorf("body should depend on #412:\n%s", h.store.bodies[401])
	}
}

func TestPlannerSubmitWithoutKnownRejectsPlaceholder(t *testing.T) {
	h := newPlannerHarness(t)
	_, errOut, err := runPlanner(t, plannerDoc("Use the store", "#{issue-1}"), "submit", "--repo", "o/r")
	if err == nil {
		t.Fatal("an unresolved placeholder was accepted")
	}
	if !strings.Contains(errOut, "issue-1") {
		t.Errorf("stderr should name the placeholder:\n%s", errOut)
	}
	if len(h.store.calls) != 0 {
		t.Errorf("store called: %v", h.store.calls)
	}
}

func TestPlannerSubmitDryRunHonorsKnown(t *testing.T) {
	h := newPlannerHarness(t)
	doc := plannerDoc("Use the store", "#{issue-1}")
	if _, _, err := runPlanner(t, doc, "submit", "--dry-run", "--known", "issue-1=412"); err != nil {
		t.Fatalf("dry run with --known failed: %v", err)
	}
	if _, _, err := runPlanner(t, doc, "submit", "--dry-run"); err == nil {
		t.Error("dry run without --known accepted an unresolved placeholder")
	}
	if len(h.store.calls) != 0 {
		t.Errorf("store called: %v", h.store.calls)
	}
}

func TestPlannerSubmitUsageErrors(t *testing.T) {
	cases := map[string][]string{
		"no repo":          {"submit"},
		"known no equals":  {"submit", "--dry-run", "--known", "issue-1"},
		"known bad number": {"submit", "--dry-run", "--known", "issue-1=abc"},
		"known zero":       {"submit", "--dry-run", "--known", "issue-1=0"},
		"known bad key":    {"submit", "--dry-run", "--known", "first=3"},
		"known duplicate":  {"submit", "--dry-run", "--known", "issue-1=3", "--known", "issue-1=4"},
		"missing file":     {"submit", "--dry-run", "--file", "/nonexistent/replies.yaml"},
		"positional args":  {"submit", "--dry-run", "extra"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			h := newPlannerHarness(t)
			if _, _, err := runPlanner(t, plannerDoc("x", "none"), args...); err == nil {
				t.Error("expected an error")
			}
			if len(h.store.calls) != 0 {
				t.Errorf("store called: %v", h.store.calls)
			}
		})
	}
}

func TestPlannerSubmitEmptyInputIsAnError(t *testing.T) {
	h := newPlannerHarness(t)
	for _, in := range []string{"", "\n\n", "---\n", "---\n\n---\n"} {
		if _, _, err := runPlanner(t, in, "submit", "--repo", "o/r"); err == nil {
			t.Errorf("input %q: expected an error", in)
		}
	}
	if len(h.store.calls) != 0 {
		t.Errorf("store called: %v", h.store.calls)
	}
}

func TestPlannerSplitReplies(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"one", "a\nb\n", []string{"a\nb"}},
		{"two", "a\n---\nb\n", []string{"a", "b"}},
		{"crlf", "a\r\n---\r\nb\r\n", []string{"a", "b"}},
		{"leading and trailing separators", "---\na\n---\n", []string{"a"}},
		{"empty segments dropped", "a\n---\n\n---\nb", []string{"a", "b"}},
		{"indented separator is content", "body: |\n  ---\n  x\n", []string{"body: |\n  ---\n  x"}},
		{"trailing spaces separator", "a\n---  \nb", []string{"a", "b"}},
		{"longer rule is content", "a\n----\nb", []string{"a\n----\nb"}},
		{"empty", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitReplies(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitReplies(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
