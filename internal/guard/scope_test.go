package guard

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

const scopeTestdata = "testdata/scope"

// scopeFixtureBody returns the issue body of testdata/scope/input.yaml. The
// guard package cannot import internal/boundary (boundary imports guard), so
// the test reads the body key directly; the command test covers verification.
func scopeFixtureBody(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scopeTestdata, "input.yaml"))
	require.NoError(t, err)
	var doc struct {
		Body string `yaml:"body"`
	}
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.NotEmpty(t, doc.Body)
	return doc.Body
}

func scopeFixtureSpec(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scopeTestdata, name))
	require.NoError(t, err)
	return string(data)
}

// fixtureTaskIDs are the task IDs of the kiro-plan block in the spec fixtures.
var fixtureTaskIDs = []string{"retry-config", "retry-cap", "retry-docs", "validate-all"}

func TestScopeItems_ListsInScopeThenAcceptanceCriteria(t *testing.T) {
	items := ScopeItems(scopeFixtureBody(t))
	assert.Equal(t, []ScopeItem{
		{ID: "s-1", Text: "`internal/agent`: cap the attempts per issue"},
		{ID: "s-2", Text: "`docs/retries.md`: document the cap"},
		{ID: "s-3", Text: "`.kairon/config.yaml`: a `max_attempts` setting"},
		{ID: "ac-1", Text: "The third failed attempt stops the loop."},
		{ID: "ac-2", Text: "The docs name the setting."},
	}, items)
}

func TestScopeCheck_Fixtures(t *testing.T) {
	body := scopeFixtureBody(t)
	cases := []struct {
		spec string
		want []ScopeFinding
	}{
		{"pass.md", nil},
		{"uncovered-s2.md", []ScopeFinding{
			{Kind: ScopeUncovered, Item: "s-2", Text: "`docs/retries.md`: document the cap"},
		}},
		{"unknown-task.md", []ScopeFinding{
			{Kind: ScopeUnknownTask, Item: "s-2", Line: 8, Text: "retry-manual"},
		}},
		{"deferred-section.md", []ScopeFinding{
			{Kind: ScopeDeferredSection, Item: "Deferred work", Line: 15, Text: "- Per-repository caps move to a later issue."},
		}},
	}
	for _, c := range cases {
		t.Run(c.spec, func(t *testing.T) {
			got := CheckScope(body, scopeFixtureSpec(t, c.spec), fixtureTaskIDs)
			assert.Equal(t, c.want, got)
		})
	}
}

// TestScopeCheck_IgnoresDeferralLikeWords pins that the check never scans for
// deferral language: whether a word like defer, TODO or follow-up puts off work
// is judged by the validator, not matched here.
func TestScopeCheck_IgnoresDeferralLikeWords(t *testing.T) {
	spec := scopeFixtureSpec(t, "pass.md")
	require.Contains(t, spec, "`defer`")
	require.Contains(t, spec, "`TODO`")
	require.Contains(t, spec, "`follow-up`")
	spec += "\nTODO: a follow-up issue will handle the deferred future work.\n"
	assert.Empty(t, CheckScope(scopeFixtureBody(t), spec, fixtureTaskIDs))
}

func TestScopeCheck_ReportsEveryFinding(t *testing.T) {
	spec := "## Deferred work\n\nlater\n"
	got := CheckScope(scopeFixtureBody(t), spec, fixtureTaskIDs)
	var kinds []string
	var items []string
	for _, f := range got {
		kinds = append(kinds, f.Kind)
		items = append(items, f.Item)
	}
	assert.Equal(t, []string{
		ScopeUncovered, ScopeUncovered, ScopeUncovered, ScopeUncovered, ScopeUncovered, ScopeDeferredSection,
	}, kinds)
	assert.Equal(t, []string{"s-1", "s-2", "s-3", "ac-1", "ac-2", "Deferred work"}, items)
}

func TestScopeCheck_MissingDeferredSection(t *testing.T) {
	spec := regexp.MustCompile(`(?s)## Deferred work\n\nnone\n`).ReplaceAllString(scopeFixtureSpec(t, "pass.md"), "")
	got := CheckScope(scopeFixtureBody(t), spec, fixtureTaskIDs)
	assert.Equal(t, []ScopeFinding{{Kind: ScopeDeferredSection, Item: "Deferred work", Text: "missing"}}, got)
}

func TestScopeCheck_EmptyDeferredSection(t *testing.T) {
	spec := regexp.MustCompile(`(?s)## Deferred work\n\nnone\n`).ReplaceAllString(scopeFixtureSpec(t, "pass.md"), "## Deferred work\n\n")
	got := CheckScope(scopeFixtureBody(t), spec, fixtureTaskIDs)
	assert.Equal(t, []ScopeFinding{{Kind: ScopeDeferredSection, Item: "Deferred work", Line: 13, Text: "empty"}}, got)
}

func TestScopeCheck_DeferredNoneVariants(t *testing.T) {
	for _, none := range []string{"none", "None.", "**none**", "`none`"} {
		spec := regexp.MustCompile(`(?s)## Deferred work\n\nnone\n`).ReplaceAllString(scopeFixtureSpec(t, "pass.md"), "## Deferred work\n\n"+none+"\n")
		assert.Empty(t, CheckScope(scopeFixtureBody(t), spec, fixtureTaskIDs), none)
	}
}

func TestScopeCheck_RowWithoutTaskIsUncovered(t *testing.T) {
	spec := regexp.MustCompile("(?m)^(\\| S2 .*\\| )`retry-docs` \\|$").ReplaceAllString(scopeFixtureSpec(t, "pass.md"), "${1}the docs task |")
	got := CheckScope(scopeFixtureBody(t), spec, fixtureTaskIDs)
	assert.Equal(t, []ScopeFinding{{Kind: ScopeUncovered, Item: "s-2", Text: "`docs/retries.md`: document the cap"}}, got)
}

func TestScopeCheck_ParenthesizedNamesAreNotTasks(t *testing.T) {
	// pass.md names `TestRetryCap` in parentheses after `retry-cap`; it is not a
	// task ID, so no unknown_task finding is reported for it.
	spec := scopeFixtureSpec(t, "pass.md")
	require.Contains(t, spec, "`retry-cap` (`TestRetryCap`)")
	assert.Empty(t, CheckScope(scopeFixtureBody(t), spec, fixtureTaskIDs))
}

func TestScopeCheck_EscapedQuotesMatch(t *testing.T) {
	body := "## Scope\n\n### In Scope\n\n- the \"label | answer\" path\n\n## Acceptance Criteria\n\n1. It works. Verify: tests.\n"
	spec := "## Scope Coverage\n\n| # | Item | By |\n|---|---|---|\n" +
		"| S1 | \"the \\\"label \\| answer\\\" path\" | `t1` |\n" +
		"| AC1 | \"It works.  Verify: tests.\" | `t1` |\n\n## Deferred work\n\nnone\n"
	assert.Empty(t, CheckScope(body, spec, []string{"t1"}))
}

func TestScopeCheck_NoPlanMakesEveryTaskUnknown(t *testing.T) {
	got := CheckScope(scopeFixtureBody(t), scopeFixtureSpec(t, "pass.md"), nil)
	require.NotEmpty(t, got)
	for _, f := range got {
		assert.Equal(t, ScopeUnknownTask, f.Kind)
	}
}

func TestScopeCheck_TableOutsideScopeCoverageIsIgnored(t *testing.T) {
	spec := scopeFixtureSpec(t, "pass.md") + "\n## Notes\n\n| a | b |\n|---|---|\n| x | `not-a-task` |\n"
	assert.Empty(t, CheckScope(scopeFixtureBody(t), spec, fixtureTaskIDs))
}

// TestScopeItems_SameAcceptanceCriteriaAsStructural runs the structural guard
// and the scope listing on the same body and requires the same
// acceptance-criteria count, so the two can never disagree about what an item is.
func TestScopeItems_SameAcceptanceCriteriaAsStructural(t *testing.T) {
	body := scopeFixtureBody(t) + "\n## Acceptance Criteria\n\n- An extra criterion with no marker.\n  - a nested note\n"
	res := NewStructural(nil).Admit(structuralInput(canonicalRaw(t, stageoutput.StagePlanner, body)))
	require.False(t, res.Pass)
	m := regexp.MustCompile(`\(of (\d+)\)`).FindStringSubmatch(res.Failure.Actual)
	require.NotNil(t, m, res.Failure.Actual)

	var acs int
	for _, it := range ScopeItems(body) {
		if regexp.MustCompile(`^ac-\d+$`).MatchString(it.ID) {
			acs++
		}
	}
	assert.Equal(t, m[1], strconv.Itoa(acs))
}

// TestScopeCheck_ItemQuoteMustBeWhole pins that a row covers an item only when
// its item cell quotes the item whole: a longer item that starts with a
// shorter one, or a task cell, does not cover the shorter item.
func TestScopeCheck_ItemQuoteMustBeWhole(t *testing.T) {
	body := "## In Scope\n\n- Add retry caps\n- Add retry caps and record attempts\n- retry-cap\n"
	spec := "## Scope Coverage\n\n" +
		"| # | Item (quoted from the issue) | Delivered by |\n" +
		"|---|---|---|\n" +
		"| S2 | In Scope: \"Add retry caps and record attempts\" | `retry-cap` |\n\n" +
		"## Deferred work\n\nnone\n"
	got := CheckScope(body, spec, []string{"retry-cap"})
	assert.Equal(t, []ScopeFinding{
		{Kind: ScopeUncovered, Item: "s-1", Text: "Add retry caps"},
		{Kind: ScopeUncovered, Item: "s-3", Text: "retry-cap"},
	}, got)
}

// TestScopeCheck_BlankDeliveredByCell pins that an empty last cell stays the
// Delivered by cell: the item cell's code spans are never read as tasks.
func TestScopeCheck_BlankDeliveredByCell(t *testing.T) {
	body := "## In Scope\n\n- Document `retry-cap`\n"
	spec := "## Scope Coverage\n\n" +
		"| # | Item (quoted from the issue) | Delivered by |\n" +
		"|---|---|---|\n" +
		"| S1 | In Scope: \"Document `retry-cap`\" | |\n\n" +
		"## Deferred work\n\nnone\n"
	want := []ScopeFinding{{Kind: ScopeUncovered, Item: "s-1", Text: "Document `retry-cap`"}}
	assert.Equal(t, want, CheckScope(body, spec, []string{"retry-cap"}))
	assert.Equal(t, want, CheckScope(body, spec, []string{"retry-docs"}))
}

// TestScopeCheck_EveryDeferredSectionIsChecked pins that a second Deferred work
// section cannot hide deferred work behind a first one that says none.
func TestScopeCheck_EveryDeferredSectionIsChecked(t *testing.T) {
	spec := scopeFixtureSpec(t, "pass.md") + "\n## Deferred work\n\n- Per-repo caps later.\n"
	got := CheckScope(scopeFixtureBody(t), spec, fixtureTaskIDs)
	line := len(scanLines(spec)) - 1
	require.Equal(t, "- Per-repo caps later.", scanLines(spec)[line-1].text)
	assert.Equal(t, []ScopeFinding{
		{Kind: ScopeDeferredSection, Item: "Deferred work", Line: line, Text: "- Per-repo caps later."},
	}, got)
}

// TestScopeCheck_MisshapedTaskIDIsUnknown pins that every backticked value
// outside parentheses is a task reference: a mistyped ID is reported even when
// a valid ID in the same cell covers the item.
func TestScopeCheck_MisshapedTaskIDIsUnknown(t *testing.T) {
	spec := strings.Replace(scopeFixtureSpec(t, "pass.md"),
		"| `retry-cap` (`TestRetryCap`) |", "| `retry-cap`, `retry_cap` (`TestRetryCap`) |", 1)
	got := CheckScope(scopeFixtureBody(t), spec, fixtureTaskIDs)
	assert.Equal(t, []ScopeFinding{{Kind: ScopeUnknownTask, Item: "s-1", Line: 7, Text: "retry_cap"}}, got)
}
