package eval

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// caseFilterLabel is the label recorded as case_filter for a run's selection.
// With no filters it is exactly the --case value (what runs recorded before
// filters existed). With filters it joins `case=<testcase>` (only when --case
// is given) and each filter with " AND ".
func caseFilterLabel(testcase string, filters []Filter) string {
	if len(filters) == 0 {
		return testcase
	}
	label := FormatFilters(filters)
	if testcase != "" {
		label = "case=" + testcase + " AND " + label
	}
	return label
}

// selectionScope describes the agent/--case scope of a selection for the
// no-match message, e.g. "(agent: alpha, case: a1)". It is "" when the
// selection is not narrowed by either.
func selectionScope(agent, testcase string) string {
	var parts []string
	if agent != "" {
		parts = append(parts, "agent: "+agent)
	}
	if testcase != "" {
		parts = append(parts, "case: "+testcase)
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// noMatchWarning is the non-strict message for a selection that matched no
// case. runSingleTestCase and selectForRun both use it.
func noMatchWarning(filters []Filter, agent, testcase string) string {
	return fmt.Sprintf("⚠️  No eval cases match --filter %s%s\n", FormatFilters(filters), selectionScope(agent, testcase))
}

// noMatchError is the --strict error for a selection that matched no case.
func noMatchError(filters []Filter, agent, testcase string) error {
	return fmt.Errorf("❌ no eval cases match --filter %s%s (--strict)", FormatFilters(filters), selectionScope(agent, testcase))
}

// selectForRun is the pre-flight selection of a run (or --list). It returns
// whether the run should proceed.
//
// With no filters it proceeds untouched. Otherwise it narrows the in-scope
// agents to those that have at least one case matching --case and the
// filters, and records them in cfg.scopeAgents so that pinning only covers
// agents that will run. When nothing at all matches, a non-strict call writes
// a warning to w and reports false (the caller returns nil); strict returns
// an error naming every filter.
//
// An agent whose cases cannot be loaded is left in scope: the run reports the
// load problem itself. A --case that names no case of the agent also proceeds,
// so the existing "not found" error surfaces rather than the filter message.
func selectForRun(w io.Writer, agent, testcase string, filters []Filter, strict bool, opts RunOptions) (bool, error) {
	if len(filters) == 0 {
		return true, nil
	}
	cfg.scopeAgents = nil

	if testcase != "" && agent == "" {
		// The existing "agent name required" error is reported by the run.
		return true, nil
	}

	// A whole-suite selection (no agent argument) draws its agents from the
	// rubrics. agentsInScope swallows a loadRubrics error so a downstream,
	// more specific error can surface — but that downstream path runs only
	// after this pre-flight, so an unloadable rubrics directory would reach
	// the no-match rule below and report a clean "no cases match" (or exit 0),
	// never running a case. Surface the rubric-load error here first.
	if agent == "" && !opts.Perf {
		if _, err := loadRubrics(""); err != nil {
			return false, err
		}
	}

	var scope []string
	total := 0
	loadFailed := false
	for _, name := range agentsInScope(agent, opts) {
		cases, err := loadCases(name)
		if err != nil {
			loadFailed = true
			scope = append(scope, name)
			continue
		}
		if testcase != "" {
			var named []TestCase
			for _, tc := range cases {
				if tc.Name == testcase {
					named = append(named, tc)
				}
			}
			if len(named) == 0 {
				return true, nil
			}
			cases = named
		}
		if n := len(filterCases(cases, filters)); n > 0 {
			total += n
			scope = append(scope, name)
		}
	}

	if total == 0 && !loadFailed {
		if strict {
			return false, noMatchError(filters, agent, testcase)
		}
		fmt.Fprint(w, noMatchWarning(filters, agent, testcase))
		return false, nil
	}
	cfg.scopeAgents = scope
	return true, nil
}

// printTestCaseList writes the cases of agent to w. A case with metadata gets
// an extra line with its keys sorted. With filters only matching cases are
// listed and the header names the filters.
func printTestCaseList(w io.Writer, agent string, filters []Filter) error {
	cases, err := loadCases(agent)
	if err != nil {
		return fmt.Errorf("failed to load test cases for %s: %w", agent, err)
	}

	if len(cases) == 0 {
		fmt.Fprintf(w, "📋 No test cases found for agent '%s'\n", agent)
		return nil
	}

	header := fmt.Sprintf("📋 Test cases for agent '%s'", agent)
	if len(filters) > 0 {
		header += fmt.Sprintf(" (filter: %s)", FormatFilters(filters))
		cases = filterCases(cases, filters)
	}
	fmt.Fprintln(w, header+":")
	for _, tc := range cases {
		fmt.Fprintf(w, "   • %s - %s\n", tc.Name, tc.Description)
		if len(tc.Metadata) > 0 {
			fmt.Fprintf(w, "       metadata: %s\n", listMetadata(tc.Metadata))
		}
	}
	return nil
}

// listMetadata renders metadata as `key=value` pairs sorted by key.
func listMetadata(m CaseMetadata) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = k + "=" + m[k]
	}
	return strings.Join(pairs, ", ")
}

// derefString is *p, or "" for nil (a result recorded before case_filter
// existed reads as unfiltered).
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
