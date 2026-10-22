package github

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// storeGH is a scripted gh for IssueStore tests. It records the one call it
// receives and never starts a process.
type storeGH struct {
	out   string
	err   error
	calls []fakeCall
}

func (f *storeGH) run(stdin string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, fakeCall{stdin: stdin, args: args})
	return []byte(f.out), f.err
}

func (f *storeGH) only(t *testing.T) fakeCall {
	t.Helper()
	if len(f.calls) != 1 {
		t.Fatalf("gh calls = %d, want 1: %v", len(f.calls), f.calls)
	}
	return f.calls[0]
}

func TestIssueStoreRevisionArgsAndParse(t *testing.T) {
	gh := &storeGH{out: `{"updatedAt":"2026-10-22T18:00:00Z"}` + "\n"}
	s := IssueStore{Repo: testRepo, Labels: []string{"kairon"}, run: gh.run}

	rev, err := s.Revision(context.Background(), 7)
	if err != nil {
		t.Fatalf("Revision: %v", err)
	}
	if rev != "2026-10-22T18:00:00Z" {
		t.Errorf("revision = %q", rev)
	}
	c := gh.only(t)
	want := []string{"issue", "view", "7", "--repo", testRepo, "--json", "updatedAt"}
	if !reflect.DeepEqual(c.args, want) {
		t.Errorf("args = %v, want %v", c.args, want)
	}
	if c.stdin != "" {
		t.Errorf("stdin = %q, want empty", c.stdin)
	}
}

func TestIssueStoreRevisionRejectsBadOutput(t *testing.T) {
	for name, out := range map[string]string{
		"not json":  "nope",
		"no field":  `{}`,
		"empty val": `{"updatedAt":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			gh := &storeGH{out: out}
			_, err := IssueStore{Repo: testRepo, run: gh.run}.Revision(context.Background(), 7)
			if err == nil || !strings.Contains(err.Error(), "7") {
				t.Fatalf("err = %v, want error naming issue 7", err)
			}
		})
	}
}

func TestIssueStoreCreateArgsStdinAndLabels(t *testing.T) {
	gh := &storeGH{out: "https://github.com/o/r/issues/412\n"}
	s := IssueStore{Repo: testRepo, Labels: []string{"kairon", "planned"}, run: gh.run}
	body := "line one\n--not-a-flag $(rm -rf /) `x`\n"

	n, err := s.Create(context.Background(), "A title", body)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n != 412 {
		t.Errorf("number = %d, want 412", n)
	}
	c := gh.only(t)
	want := []string{"issue", "create", "--repo", testRepo, "--title", "A title", "--body-file", "-",
		"--label", "kairon", "--label", "planned"}
	if !reflect.DeepEqual(c.args, want) {
		t.Errorf("args = %v, want %v", c.args, want)
	}
	if c.stdin != body {
		t.Errorf("stdin = %q, want body", c.stdin)
	}
	for _, a := range c.args {
		if strings.Contains(a, "line one") || strings.Contains(a, "rm -rf") {
			t.Errorf("body leaked into argument %q", a)
		}
	}
}

func TestIssueStoreCreateNoLabels(t *testing.T) {
	for name, labels := range map[string][]string{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			gh := &storeGH{out: "https://github.com/o/r/issues/5\n"}
			if _, err := (IssueStore{Repo: testRepo, Labels: labels, run: gh.run}).Create(context.Background(), "T", "B"); err != nil {
				t.Fatalf("Create: %v", err)
			}
			for _, a := range gh.only(t).args {
				if a == "--label" {
					t.Errorf("unexpected --label in %v", gh.calls[0].args)
				}
			}
		})
	}
}

func TestIssueStoreCreateParsesNumber(t *testing.T) {
	for out, want := range map[string]int{
		"https://github.com/o/r/issues/9":                                 9,
		"\nCreating issue in o/r\n\nhttps://github.com/o/r/issues/1234\n": 1234,
	} {
		gh := &storeGH{out: out}
		n, err := IssueStore{Repo: testRepo, run: gh.run}.Create(context.Background(), "T", "B")
		if err != nil || n != want {
			t.Errorf("Create(%q) = %d, %v; want %d", out, n, err, want)
		}
	}
	for _, out := range []string{"", "created!", "https://github.com/o/r/issues/abc"} {
		gh := &storeGH{out: out}
		if _, err := (IssueStore{Repo: testRepo, run: gh.run}).Create(context.Background(), "T", "B"); err == nil {
			t.Errorf("Create with output %q: want error", out)
		}
	}
}

func TestIssueStoreUpdateArgsAndStdin(t *testing.T) {
	gh := &storeGH{}
	// Labels must not be applied on update.
	s := IssueStore{Repo: testRepo, Labels: []string{"kairon"}, run: gh.run}
	body := "new body\n--title x\n"

	if err := s.Update(context.Background(), 12, body); err != nil {
		t.Fatalf("Update: %v", err)
	}
	c := gh.only(t)
	want := []string{"issue", "edit", "12", "--repo", testRepo, "--body-file", "-"}
	if !reflect.DeepEqual(c.args, want) {
		t.Errorf("args = %v, want %v", c.args, want)
	}
	if c.stdin != body {
		t.Errorf("stdin = %q, want body", c.stdin)
	}
}

func TestIssueStoreWrapsErrors(t *testing.T) {
	boom := errors.New("boom")
	gh := &storeGH{err: boom}
	s := IssueStore{Repo: testRepo, run: gh.run}
	ctx := context.Background()

	_, revErr := s.Revision(ctx, 7)
	_, createErr := s.Create(ctx, "T", "B")
	updErr := s.Update(ctx, 12, "B")

	cases := []struct {
		name string
		err  error
		want []string
	}{
		{"revision", revErr, []string{"revision", "#7"}},
		{"create", createErr, []string{"create"}},
		{"update", updErr, []string{"update", "#12"}},
	}
	for _, tc := range cases {
		if !errors.Is(tc.err, boom) {
			t.Errorf("%s: err = %v, want wrapping boom", tc.name, tc.err)
			continue
		}
		for _, w := range tc.want {
			if !strings.Contains(tc.err.Error(), w) {
				t.Errorf("%s: err %q missing %q", tc.name, tc.err, w)
			}
		}
	}
}

func TestIssueStoreValidatesTarget(t *testing.T) {
	gh := &storeGH{out: "https://github.com/o/r/issues/1"}
	ctx := context.Background()
	if _, err := (IssueStore{run: gh.run}).Revision(ctx, 1); err == nil {
		t.Error("Revision with empty repo: want error")
	}
	if _, err := (IssueStore{Repo: testRepo, run: gh.run}).Revision(ctx, 0); err == nil {
		t.Error("Revision with issue 0: want error")
	}
	if _, err := (IssueStore{run: gh.run}).Create(ctx, "T", "B"); err == nil {
		t.Error("Create with empty repo: want error")
	}
	if _, err := (IssueStore{Repo: testRepo, run: gh.run}).Create(ctx, "  ", "B"); err == nil {
		t.Error("Create with blank title: want error")
	}
	if err := (IssueStore{Repo: testRepo, run: gh.run}).Update(ctx, -1, "B"); err == nil {
		t.Error("Update with issue -1: want error")
	}
	if len(gh.calls) != 0 {
		t.Errorf("gh was called %d times despite invalid input", len(gh.calls))
	}
}
