package github

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// storeGH is a scripted gh for IssueStore tests. It records every call.
type storeGH struct {
	out   string
	err   error
	calls []fakeCall
}

func (f *storeGH) run(stdin string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, fakeCall{stdin: stdin, args: args})
	return []byte(f.out), f.err
}

func newStore(f *storeGH) IssueStore { return IssueStore{Repo: testRepo, run: f.run} }

func hasArgs(args []string, want ...string) bool {
	j := " " + strings.Join(args, " ") + " "
	return strings.Contains(j, " "+strings.Join(want, " ")+" ")
}

func TestIssueStoreRevisionReturnsUpdatedAt(t *testing.T) {
	f := &storeGH{out: `{"updatedAt":"2026-10-22T18:00:00Z"}` + "\n"}
	got, err := newStore(f).Revision(context.Background(), 7)
	if err != nil {
		t.Fatalf("Revision: %v", err)
	}
	if got != "2026-10-22T18:00:00Z" {
		t.Errorf("revision = %q", got)
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(f.calls))
	}
	c := f.calls[0]
	if !hasArgs(c.args, "issue", "view", "7") || !hasArgs(c.args, "--repo", testRepo) || !hasArgs(c.args, "--json", "updatedAt") {
		t.Errorf("args = %v", c.args)
	}
	if c.stdin != "" {
		t.Errorf("stdin = %q, want empty", c.stdin)
	}
}

func TestIssueStoreRevisionRejectsBadJSONAndEmpty(t *testing.T) {
	for _, out := range []string{"not json", `{}`, `{"updatedAt":""}`} {
		f := &storeGH{out: out}
		if _, err := newStore(f).Revision(context.Background(), 7); err == nil {
			t.Errorf("output %q: want error", out)
		}
	}
}

func TestIssueStoreCreateSendsBodyOnStdinAndParsesNumber(t *testing.T) {
	body := "line one\n--flag $(rm -rf /)\n"
	f := &storeGH{out: "https://github.com/o/r/issues/42\n"}
	n, err := newStore(f).Create(context.Background(), "A title", body)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n != 42 {
		t.Errorf("number = %d, want 42", n)
	}
	c := f.calls[0]
	if c.stdin != body {
		t.Errorf("stdin = %q, want body", c.stdin)
	}
	if !hasArgs(c.args, "issue", "create") || !hasArgs(c.args, "--repo", testRepo) ||
		!hasArgs(c.args, "--title", "A title") || !hasArgs(c.args, "--body-file", "-") {
		t.Errorf("args = %v", c.args)
	}
	for _, a := range c.args {
		if strings.Contains(a, "rm -rf") {
			t.Errorf("body leaked into argv: %v", c.args)
		}
	}
}

func TestIssueStoreCreateRejectsUnparsableURL(t *testing.T) {
	for _, out := range []string{"", "created!", "https://github.com/o/r/pull/3x"} {
		f := &storeGH{out: out}
		if _, err := newStore(f).Create(context.Background(), "t", "b"); err == nil {
			t.Errorf("output %q: want error", out)
		}
	}
}

func TestIssueStoreUpdateSendsBodyOnStdin(t *testing.T) {
	body := "new body\n"
	f := &storeGH{}
	if err := newStore(f).Update(context.Background(), 9, body); err != nil {
		t.Fatalf("Update: %v", err)
	}
	c := f.calls[0]
	if c.stdin != body {
		t.Errorf("stdin = %q, want body", c.stdin)
	}
	if !hasArgs(c.args, "issue", "edit", "9") || !hasArgs(c.args, "--repo", testRepo) || !hasArgs(c.args, "--body-file", "-") {
		t.Errorf("args = %v", c.args)
	}
}

func TestIssueStoreWrapsGHErrors(t *testing.T) {
	boom := errors.New("boom")
	f := &storeGH{err: boom}
	s := newStore(f)
	ctx := context.Background()
	if _, err := s.Revision(ctx, 1); !errors.Is(err, boom) || !strings.Contains(err.Error(), "gh issue view") {
		t.Errorf("Revision err = %v", err)
	}
	if _, err := s.Create(ctx, "t", "b"); !errors.Is(err, boom) || !strings.Contains(err.Error(), "gh issue create") {
		t.Errorf("Create err = %v", err)
	}
	if err := s.Update(ctx, 1, "b"); !errors.Is(err, boom) || !strings.Contains(err.Error(), "gh issue edit") {
		t.Errorf("Update err = %v", err)
	}
}

func TestIssueStoreRejectsInvalidTargetBeforeAnyCall(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		run  func(s IssueStore) error
		repo string
	}{
		{"revision empty repo", func(s IssueStore) error { _, err := s.Revision(ctx, 1); return err }, ""},
		{"revision bad number", func(s IssueStore) error { _, err := s.Revision(ctx, 0); return err }, testRepo},
		{"update empty repo", func(s IssueStore) error { return s.Update(ctx, 1, "b") }, "  "},
		{"update bad number", func(s IssueStore) error { return s.Update(ctx, -3, "b") }, testRepo},
		{"create empty repo", func(s IssueStore) error { _, err := s.Create(ctx, "t", "b"); return err }, ""},
	}
	for _, c := range cases {
		f := &storeGH{}
		if err := c.run(IssueStore{Repo: c.repo, run: f.run}); err == nil {
			t.Errorf("%s: want error", c.name)
		}
		if len(f.calls) != 0 {
			t.Errorf("%s: gh called %d times, want 0", c.name, len(f.calls))
		}
	}
}

func TestIssueStoreCreateRejectsEmptyTitleBeforeAnyCall(t *testing.T) {
	f := &storeGH{}
	if _, err := newStore(f).Create(context.Background(), "  ", "b"); err == nil {
		t.Fatal("want error")
	}
	if len(f.calls) != 0 {
		t.Errorf("gh called %d times", len(f.calls))
	}
}

func TestIssueStoreZeroRunUsesExecGH(t *testing.T) {
	// The zero value must select the real gh; only the selector is checked,
	// so no process is started.
	if (IssueStore{}).gh() == nil {
		t.Fatal("gh() returned nil")
	}
}
