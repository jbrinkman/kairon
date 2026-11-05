package github

import (
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// gqlFake is a scripted `gh api graphql`. It serves one reply per call, in
// order, records every call and never starts a process. A call past the last
// reply fails the test's expectations by returning an error.
type gqlFake struct {
	replies []gqlReply
	calls   [][]string
}

type gqlReply struct {
	out string
	err error
}

func (f *gqlFake) run(_ string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if len(f.calls) > len(f.replies) {
		return nil, fmt.Errorf("unexpected gh call #%d: %s", len(f.calls), strings.Join(args, " "))
	}
	r := f.replies[len(f.calls)-1]
	return []byte(r.out), r.err
}

// gqlNode renders one issue node in the GraphQL response shape.
func gqlNode(n int) string {
	return fmt.Sprintf(`{"number":%d,"title":"t%d","body":"b%d",`+
		`"labels":{"nodes":[{"name":"kairon"}]},`+
		`"comments":{"nodes":[{"author":{"login":"alice"},"body":"c%d","createdAt":"2026-01-01T10:00:00Z"},`+
		`{"author":null,"body":"ghost","createdAt":"2026-01-02T10:00:00Z"}]}}`, n, n, n, n)
}

// gqlPage renders a response page. hasNext is true when endCursor is non-empty.
func gqlPage(endCursor string, numbers ...int) string {
	var nodes []string
	for _, n := range numbers {
		nodes = append(nodes, gqlNode(n))
	}
	return fmt.Sprintf(`{"data":{"repository":{"issues":{"pageInfo":{"hasNextPage":%t,"endCursor":%q},"nodes":[%s]}}}}`,
		endCursor != "", endCursor, strings.Join(nodes, ","))
}

// fVars returns the -f key=value arguments of a call as a map, and the query.
func fVars(t *testing.T, args []string) map[string]string {
	t.Helper()
	vars := map[string]string{}
	for i := 0; i < len(args); i++ {
		if args[i] != "-f" {
			continue
		}
		if i+1 >= len(args) {
			t.Fatalf("dangling -f in %v", args)
		}
		k, v, ok := strings.Cut(args[i+1], "=")
		if !ok {
			t.Fatalf("-f argument %q has no '='", args[i+1])
		}
		vars[k] = v
		i++
	}
	return vars
}

func issueNumbers(issues []rawIssue) []int {
	var out []int
	for _, i := range issues {
		out = append(out, i.Number)
	}
	return out
}

func TestFetchOpenIssues_SinglePageConvertsNodes(t *testing.T) {
	g := &gqlFake{replies: []gqlReply{{out: gqlPage("", 4)}}}
	got, err := fetchOpenIssues(g.run, "o/r", "kairon", "")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 || len(g.calls) != 1 {
		t.Fatalf("got %d issues in %d calls, want 1 and 1", len(got), len(g.calls))
	}
	is := got[0]
	if is.Number != 4 || is.Title != "t4" || is.Body != "b4" {
		t.Errorf("issue = %+v", is.Issue)
	}
	if len(is.Labels) != 1 || is.Labels[0].Name != "kairon" {
		t.Errorf("labels = %+v", is.Labels)
	}
	if len(is.Comments) != 2 {
		t.Fatalf("comments = %d, want 2", len(is.Comments))
	}
	if is.Comments[0].Author.Login != "alice" || is.Comments[0].Body != "c4" || is.Comments[0].CreatedAt.IsZero() {
		t.Errorf("comment 0 = %+v", is.Comments[0])
	}
	if is.Comments[1].Author.Login != "" {
		t.Errorf("null author decoded to %q, want empty login", is.Comments[1].Author.Login)
	}
}

func TestFetchOpenIssues_EmptyRepositoryReturnsNoIssues(t *testing.T) {
	g := &gqlFake{replies: []gqlReply{{out: gqlPage("")}}}
	got, err := fetchOpenIssues(g.run, "o/r", "kairon", "")
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v; want no issues and no error", got, err)
	}
}

func TestFetchOpenIssues_FollowsPagesAndSortsAscending(t *testing.T) {
	g := &gqlFake{replies: []gqlReply{
		{out: gqlPage("c1", 9, 3, 7)},
		{out: gqlPage("c2", 5, 1)},
		{out: gqlPage("", 8)},
	}}
	got, err := fetchOpenIssues(g.run, "o/r", "kairon", "")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if want := []int{1, 3, 5, 7, 8, 9}; !reflect.DeepEqual(issueNumbers(got), want) {
		t.Errorf("numbers = %v, want %v", issueNumbers(got), want)
	}
	if len(g.calls) != 3 {
		t.Errorf("gh calls = %d, want 3", len(g.calls))
	}
}

func TestFetchOpenIssues_LaterPageErrorReturnsNothing(t *testing.T) {
	rateLimit := fmt.Errorf("wrapped: %w", &exec.ExitError{Stderr: []byte("API rate limit exceeded")})
	tests := []struct {
		name      string
		page2     gqlReply
		rateLimit bool
	}{
		{"gh error", gqlReply{err: errors.New("boom")}, false},
		{"rate limit", gqlReply{err: rateLimit}, true},
		{"invalid JSON", gqlReply{out: "not json"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &gqlFake{replies: []gqlReply{{out: gqlPage("c1", 1, 2)}, tt.page2}}
			got, err := fetchOpenIssues(g.run, "o/r", "kairon", "")
			if err == nil {
				t.Fatal("err = nil, want an error")
			}
			if got != nil {
				t.Errorf("got %v, want a nil slice: pages that succeeded must not leak", issueNumbers(got))
			}
			if tt.rateLimit && !errors.Is(err, ErrRateLimited) {
				t.Errorf("err = %v, want errors.Is ErrRateLimited", err)
			}
			if !tt.rateLimit && errors.Is(err, ErrRateLimited) {
				t.Errorf("err = %v, must not be mapped to ErrRateLimited", err)
			}
			if !tt.rateLimit && !strings.Contains(err.Error(), "page 2") {
				t.Errorf("err = %q, want it to name page 2", err)
			}
		})
	}
}

func TestFetchOpenIssues_FirstPageRateLimitIsMapped(t *testing.T) {
	g := &gqlFake{replies: []gqlReply{{err: &exec.ExitError{Stderr: []byte("API rate limit exceeded")}}}}
	got, err := fetchOpenIssues(g.run, "o/r", "kairon", "")
	if got != nil || !errors.Is(err, ErrRateLimited) {
		t.Errorf("got %v, %v; want nil and ErrRateLimited", got, err)
	}
}

func TestFetchOpenIssues_PassesCursorAndVariables(t *testing.T) {
	const (
		repo     = `own"er/na$me`
		label    = `la"bel {x} search(`
		assignee = `@alice`
		cursor   = "Y3Vyc29yOjUw"
	)
	g := &gqlFake{replies: []gqlReply{{out: gqlPage(cursor, 1)}, {out: gqlPage("", 2)}}}
	if _, err := fetchOpenIssues(g.run, repo, label, assignee); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(g.calls) != 2 {
		t.Fatalf("gh calls = %d, want 2", len(g.calls))
	}
	for i, args := range g.calls {
		if len(args) < 2 || args[0] != "api" || args[1] != "graphql" {
			t.Fatalf("call %d args = %v, want it to start with api graphql", i+1, args)
		}
		v := fVars(t, args)
		if v["owner"] != "own\"er" || v["name"] != "na$me" || v["label"] != label || v["assignee"] != assignee {
			t.Errorf("call %d variables = %v", i+1, v)
		}
		q := v["query"]
		if q == "" {
			t.Fatalf("call %d has no query", i+1)
		}
		for _, secret := range []string{"own\"er", "na$me", "la\"bel", assignee, cursor, "o/r"} {
			if strings.Contains(q, secret) {
				t.Errorf("call %d query contains untrusted value %q", i+1, secret)
			}
		}
		for _, arg := range args {
			if arg == "--search" || arg == "--paginate" {
				t.Errorf("call %d uses %s", i+1, arg)
			}
		}
		if strings.Contains(q, "search(") {
			t.Errorf("call %d query uses the search field", i+1)
		}
		for _, want := range []string{"states: [OPEN]", "CREATED_AT", "ASC", "pageInfo", "labels(first: 100)", fmt.Sprintf("comments(first: %d)", listCommentCap), fmt.Sprintf("first: %d", listPageSize)} {
			if !strings.Contains(q, want) {
				t.Errorf("call %d query lacks %q", i+1, want)
			}
		}
	}
	if _, has := fVars(t, g.calls[0])["after"]; has {
		t.Errorf("page 1 carries after=%q, want none", fVars(t, g.calls[0])["after"])
	}
	if got := fVars(t, g.calls[1])["after"]; got != cursor {
		t.Errorf("page 2 after = %q, want %q", got, cursor)
	}
}

func TestFetchOpenIssues_OmitsAssigneeWhenEmpty(t *testing.T) {
	g := &gqlFake{replies: []gqlReply{{out: gqlPage("", 1)}}}
	if _, err := fetchOpenIssues(g.run, "o/r", "kairon", ""); err != nil {
		t.Fatalf("err = %v", err)
	}
	v := fVars(t, g.calls[0])
	if _, has := v["assignee"]; has {
		t.Errorf("assignee sent as %q, want it omitted", v["assignee"])
	}
	if v["owner"] != "o" || v["name"] != "r" || v["label"] != "kairon" {
		t.Errorf("variables = %v", v)
	}
}

func TestFetchOpenIssues_RejectsBadPageState(t *testing.T) {
	tests := []struct {
		name      string
		repo      string
		replies   []gqlReply
		wantCalls int
	}{
		{
			name:      "hasNextPage with empty cursor",
			replies:   []gqlReply{{out: `{"data":{"repository":{"issues":{"pageInfo":{"hasNextPage":true,"endCursor":""},"nodes":[` + gqlNode(1) + `]}}}}`}},
			wantCalls: 1,
		},
		{
			name:      "repeated cursor",
			replies:   []gqlReply{{out: gqlPage("same", 1)}, {out: gqlPage("same", 2)}, {out: gqlPage("", 3)}},
			wantCalls: 2,
		},
		{
			name:      "errors array on page 1",
			replies:   []gqlReply{{out: `{"errors":[{"message":"boom"}],"data":{"repository":{"issues":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[` + gqlNode(1) + `]}}}}`}},
			wantCalls: 1,
		},
		{
			name:      "errors array on page 2",
			replies:   []gqlReply{{out: gqlPage("c1", 1)}, {out: `{"errors":[{"message":"boom"}],"data":null}`}},
			wantCalls: 2,
		},
		{
			name:      "null repository",
			replies:   []gqlReply{{out: `{"data":{"repository":null}}`}},
			wantCalls: 1,
		},
		{
			name:      "null issue node",
			replies:   []gqlReply{{out: `{"data":{"repository":{"issues":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[` + gqlNode(1) + `,null]}}}}`}},
			wantCalls: 1,
		},
		{
			name:      "missing issues connection",
			replies:   []gqlReply{{out: `{"data":{"repository":{}}}`}},
			wantCalls: 1,
		},
		{
			name:      "null data",
			replies:   []gqlReply{{out: `{"data":null}`}},
			wantCalls: 1,
		},
		{
			name:      "empty reply",
			replies:   []gqlReply{{out: ``}},
			wantCalls: 1,
		},
		{name: "repo without slash", repo: "noslash", wantCalls: 0},
		{name: "empty owner", repo: "/name", wantCalls: 0},
		{name: "empty name", repo: "owner/", wantCalls: 0},
		{name: "empty repo", repo: "", wantCalls: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.repo
			if repo == "" && tt.name != "empty repo" {
				repo = "o/r"
			}
			g := &gqlFake{replies: tt.replies}
			got, err := fetchOpenIssues(g.run, repo, "kairon", "")
			if err == nil {
				t.Fatal("err = nil, want an error")
			}
			if got != nil {
				t.Errorf("got %v, want a nil slice", issueNumbers(got))
			}
			if len(g.calls) != tt.wantCalls {
				t.Errorf("gh calls = %d, want %d", len(g.calls), tt.wantCalls)
			}
		})
	}
}
