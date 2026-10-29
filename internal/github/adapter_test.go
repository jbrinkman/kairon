package github

import (
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/failure"
)

const (
	testRepo   = "o/r"
	testViewer = "kairon-bot"
)

// fakeGH is a scripted gh. It records every call and never starts a process.
type fakeGH struct {
	list      string         // output of every `gh api graphql` call when pages is empty
	listErr   error          // error from every `gh api graphql` call when pages is empty
	pages     []fakePage     // replies to `gh api graphql`, served in call order when non-empty
	view      map[int]string // issue number -> output of `gh issue view`
	events    map[int]string // issue number -> output of the events call
	eventsErr map[int]error  // issue number -> error from the events call
	viewer    string         // output of `gh api user`
	viewerErr error          // error from `gh api user`
	calls     []fakeCall
}

// fakePage is one scripted reply to a `gh api graphql` call.
type fakePage struct {
	out string
	err error
}

type fakeCall struct {
	stdin string
	args  []string
}

func (c fakeCall) joined() string { return strings.Join(c.args, " ") }

func (f *fakeGH) run(stdin string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, fakeCall{stdin: stdin, args: args})
	j := strings.Join(args, " ")
	switch {
	case strings.HasPrefix(j, "api graphql"):
		if len(f.pages) == 0 {
			return []byte(f.list), f.listErr
		}
		// The call was recorded above, so it is already counted.
		i := f.count("api graphql") - 1
		if i >= len(f.pages) {
			return nil, fmt.Errorf("unexpected extra gh api graphql call %d", i+1)
		}
		return []byte(f.pages[i].out), f.pages[i].err
	case strings.HasPrefix(j, "issue view"):
		var n int
		fmt.Sscanf(args[2], "%d", &n)
		return []byte(f.view[n]), nil
	case j == "api user --jq .login":
		return []byte(f.viewer + "\n"), f.viewerErr
	case strings.HasPrefix(j, "api repos/"):
		var n int
		fmt.Sscanf(strings.TrimPrefix(args[1], "repos/"+testRepo+"/issues/"), "%d", &n)
		return []byte(f.events[n]), f.eventsErr[n]
	case strings.HasPrefix(j, "issue comment"), strings.HasPrefix(j, "issue edit"):
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected gh call: %s", j)
}

func (f *fakeGH) count(prefix string) int {
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c.joined(), prefix) {
			n++
		}
	}
	return n
}

func markerComment(author, runID, at string) string {
	body := failure.Marker(runID) + "\\n\\nKairon punched out."
	return fmt.Sprintf(`{"author":{"login":%q},"body":"%s","createdAt":%q}`, author, body, at)
}

func plainComment(author, text, at string) string {
	return fmt.Sprintf(`{"author":{"login":%q},"body":%q,"createdAt":%q}`, author, text, at)
}

// issueJSON returns one issue in the GraphQL node shape `gh api graphql`
// returns for the issues connection.
func issueJSON(n int, labels []string, comments ...string) string {
	var ls []string
	for _, l := range labels {
		ls = append(ls, fmt.Sprintf(`{"name":%q}`, l))
	}
	return fmt.Sprintf(`{"number":%d,"title":"t%d","body":"b%d","labels":{"nodes":[%s]},"comments":{"nodes":[%s]}}`,
		n, n, n, strings.Join(ls, ","), strings.Join(comments, ","))
}

// pageOf returns one GraphQL page holding issues. hasNextPage is true exactly
// when endCursor is non-empty.
func pageOf(endCursor string, issues ...string) string {
	return fmt.Sprintf(`{"data":{"repository":{"issues":{"pageInfo":{"hasNextPage":%t,"endCursor":%q},"nodes":[%s]}}}}`,
		endCursor != "", endCursor, strings.Join(issues, ","))
}

// listOf returns a complete single page: no next page.
func listOf(issues ...string) string { return pageOf("", issues...) }

// afterValue returns the after= variable of a recorded call, or "" if the call
// has none.
func afterValue(c fakeCall) string {
	for i, a := range c.args {
		if a == "-f" && i+1 < len(c.args) && strings.HasPrefix(c.args[i+1], "after=") {
			return strings.TrimPrefix(c.args[i+1], "after=")
		}
	}
	return ""
}

func numbers(issues []Issue) []int {
	var out []int
	for _, i := range issues {
		out = append(out, i.Number)
	}
	return out
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSelectIssues_ExcludesTerminalPending(t *testing.T) {
	g := &fakeGH{
		viewer: testViewer,
		list: listOf(
			issueJSON(1, []string{"kairon"}),
			issueJSON(2, []string{"kairon"}, markerComment(testViewer, "run1", "2026-01-01T10:00:00Z")),
		),
		events: map[int]string{2: ""},
	}
	sel, err := selectIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := numbers(sel.Eligible); !eqInts(got, []int{1}) {
		t.Errorf("Eligible = %v, want [1]", got)
	}
	if got := numbers(sel.TerminalPending); !eqInts(got, []int{2}) {
		t.Errorf("TerminalPending = %v, want [2]", got)
	}
	if n := g.count("api repos/o/r/issues/1/events"); n != 0 {
		t.Errorf("events fetched for an issue with no marker: %d calls", n)
	}
}

func TestSelectIssues_ExcludesDoneAndFailed(t *testing.T) {
	g := &fakeGH{
		viewer: testViewer,
		list: listOf(
			issueJSON(1, []string{"kairon"}),
			issueJSON(2, []string{"kairon", "kairon-done"}),
			issueJSON(3, []string{"kairon", "kairon-failed"}, markerComment(testViewer, "run1", "2026-01-01T10:00:00Z")),
		),
	}
	sel, err := selectIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := numbers(sel.Eligible); !eqInts(got, []int{1}) {
		t.Errorf("Eligible = %v, want [1]", got)
	}
	if len(sel.TerminalPending) != 0 {
		t.Errorf("TerminalPending = %v, want none", numbers(sel.TerminalPending))
	}
	if n := g.count("api repos/"); n != 0 {
		t.Errorf("events fetched for labeled issue: %d calls", n)
	}
}

func TestSelectIssues_RestartedIssueIsEligible(t *testing.T) {
	g := &fakeGH{
		viewer: testViewer,
		list: listOf(
			issueJSON(5, []string{"kairon"}, markerComment(testViewer, "run1", "2026-01-01T10:00:00Z")),
		),
		events: map[int]string{
			5: `{"label":"kairon-failed","at":"2026-01-01T09:00:00Z"}` + "\n" + // earlier removal, before marker
				`{"label":"other","at":"2026-01-02T00:00:00Z"}` + "\n" + // different label
				`{"label":"kairon-failed","at":"2026-01-01T11:00:00Z"}` + "\n",
		},
	}
	sel, err := selectIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := numbers(sel.Eligible); !eqInts(got, []int{5}) {
		t.Errorf("Eligible = %v, want [5]", got)
	}
	if len(sel.TerminalPending) != 0 {
		t.Errorf("TerminalPending = %v, want none", numbers(sel.TerminalPending))
	}
}

func TestSelectIssues_UnlabelOfOtherLabelStaysPending(t *testing.T) {
	g := &fakeGH{
		viewer: testViewer,
		list: listOf(
			issueJSON(5, []string{"kairon"}, markerComment(testViewer, "run1", "2026-01-01T10:00:00Z")),
		),
		events: map[int]string{5: `{"label":"other","at":"2026-01-02T00:00:00Z"}` + "\n"},
	}
	sel, err := selectIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sel.Eligible) != 0 || !eqInts(numbers(sel.TerminalPending), []int{5}) {
		t.Errorf("Eligible=%v Pending=%v, want pending [5]", numbers(sel.Eligible), numbers(sel.TerminalPending))
	}
}

func TestSelectIssues_IgnoresUntrustedMarker(t *testing.T) {
	g := &fakeGH{
		viewer: testViewer,
		list: listOf(
			issueJSON(7, []string{"kairon"}, markerComment("mallory", "run1", "2026-01-01T10:00:00Z")),
		),
	}
	sel, err := selectIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := numbers(sel.Eligible); !eqInts(got, []int{7}) {
		t.Errorf("Eligible = %v, want [7]", got)
	}
	if len(sel.TerminalPending) != 0 {
		t.Errorf("TerminalPending = %v, want none", numbers(sel.TerminalPending))
	}
	if n := g.count("api repos/"); n != 0 {
		t.Errorf("events fetched for an untrusted marker: %d calls", n)
	}
}

func TestSelectIssues_EventsLookupErrorFailsClosed(t *testing.T) {
	g := &fakeGH{
		viewer: testViewer,
		list: listOf(
			issueJSON(1, []string{"kairon"}),
			issueJSON(2, []string{"kairon"}, markerComment(testViewer, "run1", "2026-01-01T10:00:00Z")),
			issueJSON(3, []string{"kairon"}),
		),
		eventsErr: map[int]error{2: errors.New("events boom")},
	}
	sel, err := selectIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatalf("listing must not fail on one events error: %v", err)
	}
	if got := numbers(sel.Eligible); !eqInts(got, []int{1, 3}) {
		t.Errorf("Eligible = %v, want [1 3]", got)
	}
	if len(sel.TerminalPending) != 0 {
		t.Errorf("TerminalPending = %v, want none (unknown state is not reconciled)", numbers(sel.TerminalPending))
	}
}

func TestSelectIssues_ViewerLookupErrorFailsClosed(t *testing.T) {
	g := &fakeGH{
		viewerErr: errors.New("no auth"),
		list: listOf(
			issueJSON(1, []string{"kairon"}),
			issueJSON(2, []string{"kairon"}, markerComment(testViewer, "run1", "2026-01-01T10:00:00Z")),
		),
	}
	sel, err := selectIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := numbers(sel.Eligible); !eqInts(got, []int{1}) {
		t.Errorf("Eligible = %v, want [1]", got)
	}
}

func TestSelectIssues_NoViewerLookupWithoutMarkers(t *testing.T) {
	g := &fakeGH{
		list: listOf(issueJSON(1, []string{"kairon"}, plainComment("someone", "hello", "2026-01-01T10:00:00Z"))),
	}
	if _, err := selectIssues(g.run, testRepo, "kairon", ""); err != nil {
		t.Fatal(err)
	}
	if n := g.count("api user"); n != 0 {
		t.Errorf("viewer fetched %d times with no marker comments", n)
	}
}

// TestSelectIssues_PageArgs pins the exact first `gh` call: one `api graphql`
// call whose untrusted values (repo parts, label, assignee) travel as separate
// -f variables, and no `issue list`, --search or --limit anywhere.
func TestSelectIssues_PageArgs(t *testing.T) {
	g := &fakeGH{list: listOf()}
	if _, err := selectIssues(g.run, testRepo, "my-label", "alice"); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(g.calls))
	}
	want := []string{"api", "graphql", "-f", "query=" + openIssuesQuery,
		"-f", "owner=o", "-f", "name=r", "-f", "label=my-label", "-f", "assignee=alice"}
	got := g.calls[0].args
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("args = %v, want %v", got, want)
	}
	for _, a := range got {
		if a == "issue" || a == "list" || strings.HasPrefix(a, "--") {
			t.Errorf("arg %q must not appear in the page call", a)
		}
	}
}

// TestSelectIssues_MultiPageSelectsAllOldestFirst pins that more open labeled
// issues than one page holds are all selected, oldest first, across three pages.
func TestSelectIssues_MultiPageSelectsAllOldestFirst(t *testing.T) {
	const (
		total      = 2*listPageSize + 7
		doneIssue  = 10                // on page 1
		pendingIss = listPageSize + 10 // on page 2
	)
	nodes := make([]string, 0, total)
	for n := 1; n <= total; n++ {
		switch n {
		case doneIssue:
			nodes = append(nodes, issueJSON(n, []string{"kairon", "kairon-done"}))
		case pendingIss:
			nodes = append(nodes, issueJSON(n, []string{"kairon"}, markerComment(testViewer, "run1", "2026-01-01T10:00:00Z")))
		default:
			nodes = append(nodes, issueJSON(n, []string{"kairon"}))
		}
	}
	g := &fakeGH{
		viewer: testViewer,
		events: map[int]string{pendingIss: ""},
		pages: []fakePage{
			{out: pageOf("c1", nodes[:listPageSize]...)},
			{out: pageOf("c2", nodes[listPageSize:2*listPageSize]...)},
			{out: pageOf("", nodes[2*listPageSize:]...)},
		},
	}

	sel, err := selectIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatal(err)
	}

	var want []int
	for n := 1; n <= total; n++ {
		if n != doneIssue && n != pendingIss {
			want = append(want, n)
		}
	}
	got := numbers(sel.Eligible)
	if len(got) <= listPageSize {
		t.Fatalf("selected %d issues, want more than one page (%d)", len(got), listPageSize)
	}
	if !eqInts(got, want) {
		t.Errorf("Eligible = %v, want %v", got, want)
	}
	if got := numbers(sel.TerminalPending); !eqInts(got, []int{pendingIss}) {
		t.Errorf("TerminalPending = %v, want [%d]", got, pendingIss)
	}
	if n := g.count("api graphql"); n != 3 {
		t.Fatalf("api graphql calls = %d, want 3", n)
	}
	graphql := 0
	for _, c := range g.calls {
		if !strings.HasPrefix(c.joined(), "api graphql") {
			continue
		}
		if wantAfter := []string{"", "c1", "c2"}[graphql]; afterValue(c) != wantAfter {
			t.Errorf("page %d after = %q, want %q", graphql+1, afterValue(c), wantAfter)
		}
		graphql++
	}
}

// TestSelectIssues_SecondPageFailureReturnsErrorAndEmptySelection pins that a
// failed later page yields an error and nothing from the pages that succeeded.
func TestSelectIssues_SecondPageFailureReturnsErrorAndEmptySelection(t *testing.T) {
	cases := map[string]fakePage{
		"page error":   {err: errors.New("boom")},
		"invalid JSON": {out: "not json"},
	}
	for name, second := range cases {
		t.Run(name, func(t *testing.T) {
			g := &fakeGH{pages: []fakePage{
				{out: pageOf("c1", issueJSON(1, []string{"kairon"}), issueJSON(2, []string{"kairon"}))},
				second,
			}}
			sel, err := selectIssues(g.run, testRepo, "kairon", "")
			if err == nil {
				t.Fatal("want error from failed second page")
			}
			if !reflect.DeepEqual(sel, Selection{}) {
				t.Errorf("Selection = %+v, want Selection{} (no partial result)", sel)
			}
			if n := g.count("api graphql"); n != 2 {
				t.Errorf("api graphql calls = %d, want 2", n)
			}
		})
	}
}

// TestSelectIssues_NeverUsesSearch pins that no gh call selectIssues makes
// reads the search index, in a scenario that also triggers the viewer, events
// and issue-view sub-calls.
func TestSelectIssues_NeverUsesSearch(t *testing.T) {
	view := `{"labels":[{"name":"kairon"}],"comments":[]}`
	g := &fakeGH{
		viewer: testViewer,
		events: map[int]string{2: ""},
		view:   map[int]string{3: view},
		pages: []fakePage{
			{out: pageOf("c1",
				issueJSON(1, []string{"kairon"}),
				issueJSON(2, []string{"kairon"}, markerComment(testViewer, "run1", "2026-01-01T10:00:00Z")))},
			{out: pageOf("", issueJSON(3, []string{"kairon"}, nonMarkerComments(listCommentCap)...))},
		},
	}
	sel, err := selectIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := numbers(sel.Eligible); !eqInts(got, []int{1, 3}) {
		t.Errorf("Eligible = %v, want [1 3]", got)
	}

	// Every sub-call must have run, or the assertions below prove too little.
	for prefix, want := range map[string]int{"api graphql": 2, "api user": 1, "api repos/": 1, "issue view": 1} {
		if n := g.count(prefix); n != want {
			t.Errorf("%q calls = %d, want %d", prefix, n, want)
		}
	}
	if n := g.count("issue list"); n != 0 {
		t.Errorf("issue list calls = %d, want 0", n)
	}
	for _, c := range g.calls {
		for _, a := range c.args {
			if strings.Contains(a, "--search") {
				t.Errorf("call %q has a --search argument", c.joined())
			}
			if strings.HasPrefix(a, "query=") && strings.Contains(a, "search(") {
				t.Errorf("call uses the GraphQL search field: %q", a)
			}
		}
	}
}

// TestSelectIssues_OutOfOrderPagesComeBackAscending pins that Eligible and
// TerminalPending are each ascending by number whatever order the pages and
// the issues within them arrive in.
func TestSelectIssues_OutOfOrderPagesComeBackAscending(t *testing.T) {
	pending := func(n int) string {
		return issueJSON(n, []string{"kairon"}, markerComment(testViewer, "run1", "2026-01-01T10:00:00Z"))
	}
	g := &fakeGH{
		viewer: testViewer,
		events: map[int]string{7: "", 4: ""},
		pages: []fakePage{
			{out: pageOf("c1", issueJSON(9, []string{"kairon"}), issueJSON(3, []string{"kairon"}), pending(7))},
			{out: pageOf("", issueJSON(5, []string{"kairon"}), issueJSON(1, []string{"kairon"}), pending(4))},
		},
	}
	sel, err := selectIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := numbers(sel.Eligible); !eqInts(got, []int{1, 3, 5, 9}) {
		t.Errorf("Eligible = %v, want [1 3 5 9]", got)
	}
	if got := numbers(sel.TerminalPending); !eqInts(got, []int{4, 7}) {
		t.Errorf("TerminalPending = %v, want [4 7]", got)
	}
}

func TestSelectIssues_ListErrors(t *testing.T) {
	g := &fakeGH{listErr: errors.New("boom")}
	if _, err := selectIssues(g.run, testRepo, "kairon", ""); err == nil {
		t.Error("want error from failed list")
	}
	g = &fakeGH{list: "not json"}
	if _, err := selectIssues(g.run, testRepo, "kairon", ""); err == nil {
		t.Error("want error from bad JSON")
	}
}

func TestListIssues_ReturnsEligibleOnly(t *testing.T) {
	g := &fakeGH{
		viewer: testViewer,
		list: listOf(
			issueJSON(1, []string{"kairon"}),
			issueJSON(2, []string{"kairon", "kairon-done"}),
			issueJSON(3, []string{"kairon", "kairon-failed"}),
			issueJSON(4, []string{"kairon"}, markerComment(testViewer, "r", "2026-01-01T10:00:00Z")),
		),
	}
	got, err := listIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatal(err)
	}
	if !eqInts(numbers(got), []int{1}) {
		t.Errorf("ListIssues = %v, want [1]", numbers(got))
	}
}

func TestParseUnlabeledEvents(t *testing.T) {
	out := []byte(`{"label":"kairon-failed","at":"2026-01-01T09:00:00Z"}
{"label":"kairon-failed","at":"2026-01-03T09:00:00Z"}
{"label":"kairon-failed ","at":"2026-01-09T09:00:00Z"}
{"label":null,"at":"2026-01-10T09:00:00Z"}
{"label":"x\ty\"z","at":"2026-01-08T09:00:00Z"}
`)
	got, err := parseUnlabeledEvents(out, "kairon-failed")
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 1, 3, 9, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("latest = %v, want %v", got, want)
	}
	// Label text is matched literally, never interpreted.
	got, err = parseUnlabeledEvents(out, "x\ty\"z")
	if err != nil || !got.Equal(time.Date(2026, 1, 8, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("literal match = %v, %v", got, err)
	}
	got, err = parseUnlabeledEvents(out, "absent")
	if err != nil || !got.IsZero() {
		t.Errorf("absent label = %v, %v; want zero, nil", got, err)
	}
	got, err = parseUnlabeledEvents(nil, "kairon-failed")
	if err != nil || !got.IsZero() {
		t.Errorf("empty = %v, %v; want zero, nil", got, err)
	}
	pretty := []byte("{\n  \"label\": \"kairon-failed\",\n  \"at\": \"2026-02-01T00:00:00Z\"\n}\n")
	if got, err = parseUnlabeledEvents(pretty, "kairon-failed"); err != nil || got.IsZero() {
		t.Errorf("pretty-printed output = %v, %v", got, err)
	}
	if _, err := parseUnlabeledEvents([]byte("garbage"), "kairon-failed"); err == nil {
		t.Error("want error for malformed output")
	}
}

func TestIssueState_ByViewerAndLabels(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cs := []rawComment{{Body: "a", CreatedAt: at}, {Body: "b", CreatedAt: at}}
	cs[0].Author.Login = testViewer
	cs[1].Author.Login = "other"
	st := issueState([]Label{{Name: "x"}, {Name: "y"}}, cs, testViewer)
	if strings.Join(st.Labels, ",") != "x,y" {
		t.Errorf("Labels = %v", st.Labels)
	}
	if len(st.Comments) != 2 || !st.Comments[0].ByViewer || st.Comments[1].ByViewer {
		t.Errorf("Comments = %+v", st.Comments)
	}
	if !st.Comments[0].CreatedAt.Equal(at) || st.Comments[0].Body != "a" {
		t.Errorf("comment fields not copied: %+v", st.Comments[0])
	}
	// An empty viewer trusts nobody.
	st = issueState(nil, cs, "")
	if st.Comments[0].ByViewer || st.Comments[1].ByViewer {
		t.Error("empty viewer must not match any author")
	}
}

// nonMarkerComments returns n comments, none of which is a punch-out marker,
// so an issue built from them looks eligible from the issues page alone.
func nonMarkerComments(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf(`{"author":{"login":"someone"},"body":"c%d","createdAt":"2026-01-01T00:00:00Z"}`, i)
	}
	return out
}

// TestSelectIssues_RereadsIssueAtCommentCap pins that an issue whose issues-page
// comment list is at the cap is re-read in full (via CLI.State) so a
// punch-out marker posted beyond the first 100 comments is not missed. Without
// the re-read the issue misclassifies as Eligible and the watcher re-spawns a
// terminal-pending run.
func TestSelectIssues_RereadsIssueAtCommentCap(t *testing.T) {
	// Issue 2's list page holds 100 non-marker comments (the marker is newer,
	// past the cap). Its full view carries the trusted marker, so State sees a
	// terminal-pending issue.
	view := `{"labels":[{"name":"kairon"}],"comments":[` +
		markerComment(testViewer, "run1", "2026-01-02T10:00:00Z") + `]}`
	g := &fakeGH{
		viewer: testViewer,
		list: listOf(
			issueJSON(1, []string{"kairon"}),
			issueJSON(2, []string{"kairon"}, nonMarkerComments(100)...),
		),
		view:   map[int]string{2: view},
		events: map[int]string{2: ""},
	}

	sel, err := selectIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := numbers(sel.Eligible); !eqInts(got, []int{1}) {
		t.Errorf("Eligible = %v, want [1] (issue 2 must not be eligible)", got)
	}
	if got := numbers(sel.TerminalPending); !eqInts(got, []int{2}) {
		t.Errorf("TerminalPending = %v, want [2] (marker is past the list cap)", got)
	}
}

// TestSelectIssues_CommentCapRereadFailsClosed pins that if the full re-read
// errors for a capped issue, the issue is skipped this poll rather than spawned.
func TestSelectIssues_CommentCapRereadFailsClosed(t *testing.T) {
	g := &fakeGH{
		viewer: testViewer,
		list: listOf(
			issueJSON(2, []string{"kairon"}, nonMarkerComments(100)...),
		),
		// No view output for issue 2: the re-read fails to parse, so State errors.
		view: map[int]string{},
	}

	sel, err := selectIssues(g.run, testRepo, "kairon", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sel.Eligible) != 0 {
		t.Errorf("Eligible = %v, want empty (a failed re-read must fail closed)", numbers(sel.Eligible))
	}
	if len(sel.TerminalPending) != 0 {
		t.Errorf("TerminalPending = %v, want empty", numbers(sel.TerminalPending))
	}
}

func TestCLI_State(t *testing.T) {
	view := `{"labels":[{"name":"kairon"}],"comments":[` +
		markerComment(testViewer, "run1", "2026-01-01T10:00:00Z") + `]}`
	g := &fakeGH{
		viewer: testViewer,
		view:   map[int]string{9: view},
		events: map[int]string{9: `{"label":"kairon-failed","at":"2026-01-01T12:00:00Z"}` + "\n"},
	}
	st, err := CLI{run: g.run}.State(testRepo, 9, "kairon-failed")
	if err != nil {
		t.Fatal(err)
	}
	if !st.FailLabelRemovedAt.Equal(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("FailLabelRemovedAt = %v", st.FailLabelRemovedAt)
	}
	if sel := st.Classify("kairon-done", "kairon-failed"); sel != failure.SelectEligible {
		t.Errorf("Classify = %v, want eligible", sel)
	}
	want := "issue view 9 --repo o/r --json labels,comments"
	if g.calls[0].joined() != want {
		t.Errorf("first call = %q, want %q", g.calls[0].joined(), want)
	}
}

func TestCLI_State_NoEventsWithoutTrustedMarker(t *testing.T) {
	g := &fakeGH{
		viewer: testViewer,
		view:   map[int]string{9: `{"labels":[],"comments":[` + markerComment("mallory", "r", "2026-01-01T10:00:00Z") + `]}`},
	}
	if _, err := (CLI{run: g.run}).State(testRepo, 9, "kairon-failed"); err != nil {
		t.Fatal(err)
	}
	if n := g.count("api repos/"); n != 0 {
		t.Errorf("events fetched: %d", n)
	}
}

func TestCLI_State_NoEventsWhenFailLabelPresent(t *testing.T) {
	g := &fakeGH{
		viewer: testViewer,
		view: map[int]string{9: `{"labels":[{"name":"kairon-failed"}],"comments":[` +
			markerComment(testViewer, "r", "2026-01-01T10:00:00Z") + `]}`},
	}
	if _, err := (CLI{run: g.run}).State(testRepo, 9, "kairon-failed"); err != nil {
		t.Fatal(err)
	}
	if n := g.count("api repos/"); n != 0 {
		t.Errorf("events fetched: %d", n)
	}
}

func TestCLI_State_EventsErrorIsReturned(t *testing.T) {
	g := &fakeGH{
		viewer:    testViewer,
		view:      map[int]string{9: `{"labels":[],"comments":[` + markerComment(testViewer, "r", "2026-01-01T10:00:00Z") + `]}`},
		eventsErr: map[int]error{9: errors.New("boom")},
	}
	if _, err := (CLI{run: g.run}).State(testRepo, 9, "kairon-failed"); err == nil {
		t.Error("want error when the events lookup fails")
	}
}

func TestCLI_EventsCallNeverCarriesLabelText(t *testing.T) {
	label := `evil"; system("x") | .label`
	g := &fakeGH{
		viewer: testViewer,
		view:   map[int]string{9: `{"labels":[],"comments":[` + markerComment(testViewer, "r", "2026-01-01T10:00:00Z") + `]}`},
		events: map[int]string{9: ""},
	}
	if _, err := (CLI{run: g.run}).State(testRepo, 9, label); err != nil {
		t.Fatal(err)
	}
	for _, c := range g.calls {
		if strings.Contains(c.joined(), label) || strings.Contains(c.stdin, label) {
			t.Errorf("label text leaked into gh call %q", c.joined())
		}
	}
}

func TestCLI_PostComment_BodyOnStdin(t *testing.T) {
	g := &fakeGH{}
	body := "<!-- kairon:punch-out run=abc -->\n$(rm -rf /) `x` \"q\" --flag"
	if err := (CLI{run: g.run}).PostComment(testRepo, 12, body); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) != 1 {
		t.Fatalf("calls = %d", len(g.calls))
	}
	c := g.calls[0]
	if c.stdin != body {
		t.Errorf("stdin = %q, want body", c.stdin)
	}
	if want := "issue comment 12 --repo o/r --body-file -"; c.joined() != want {
		t.Errorf("args = %q, want %q", c.joined(), want)
	}
}

func TestCLI_AddLabel(t *testing.T) {
	g := &fakeGH{}
	if err := (CLI{run: g.run}).AddLabel(testRepo, 12, "kairon-failed"); err != nil {
		t.Fatal(err)
	}
	if want := "issue edit 12 --repo o/r --add-label kairon-failed"; g.calls[0].joined() != want {
		t.Errorf("args = %q, want %q", g.calls[0].joined(), want)
	}
}

func TestCLI_RemoveLabel(t *testing.T) {
	g := &fakeGH{}
	if err := (CLI{run: g.run}).RemoveLabel(testRepo, 12, "kairon-failed"); err != nil {
		t.Fatal(err)
	}
	if want := "issue edit 12 --repo o/r --remove-label kairon-failed"; len(g.calls) != 1 || g.calls[0].joined() != want {
		t.Errorf("calls = %v, want one call %q", g.calls, want)
	}
}

func TestCLI_RejectsInvalidArguments(t *testing.T) {
	g := &fakeGH{}
	c := CLI{run: g.run}
	if err := c.PostComment("", 1, "x"); err == nil {
		t.Error("empty repo accepted")
	}
	if err := c.PostComment(testRepo, 0, "x"); err == nil {
		t.Error("issue 0 accepted")
	}
	if err := c.AddLabel(testRepo, 1, ""); err == nil {
		t.Error("empty label accepted")
	}
	if err := c.RemoveLabel(testRepo, 1, " "); err == nil {
		t.Error("empty label accepted for removal")
	}
	if _, err := c.State(testRepo, -1, "l"); err == nil {
		t.Error("negative issue accepted")
	}
	if len(g.calls) != 0 {
		t.Errorf("gh called %d times for invalid input", len(g.calls))
	}
}

func TestSelectIssues_RateLimitIsMapped(t *testing.T) {
	g := &fakeGH{listErr: fmt.Errorf("wrapped: %w", &exec.ExitError{Stderr: []byte("API rate limit exceeded")})}
	if _, err := selectIssues(g.run, testRepo, "kairon", ""); !errors.Is(err, ErrRateLimited) {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
}
