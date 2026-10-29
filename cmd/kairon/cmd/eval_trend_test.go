package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const trendFixtureEvalsDir = "../../../internal/eval/testdata/trend"

// runTrend runs trend for agent against the fixture, restoring the package
// flag vars afterwards.
func runTrend(t *testing.T, agent string, asJSON bool) (string, error) {
	t.Helper()
	origJSON, origEvals := trendJSON, evalEvalsDir
	t.Cleanup(func() {
		trendJSON, evalEvalsDir = origJSON, origEvals
	})
	trendJSON = asJSON
	evalEvalsDir = trendFixtureEvalsDir

	var out bytes.Buffer
	trendCmd.SetOut(&out)
	t.Cleanup(func() { trendCmd.SetOut(nil) })
	err := trendCmd.RunE(trendCmd, []string{agent})
	return out.String(), err
}

func TestTrendTextOutput(t *testing.T) {
	out, err := runTrend(t, "selftest", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), out)
	}
	wantRuns := []string{
		"260701-100000-aaaaaaa",
		"260702-100000-bbbbbbb",
		"260703-100000-ccccccc",
		"260704-100000-ddddddd",
	}
	incomplete := 0
	for i, line := range lines {
		if !strings.HasPrefix(line, wantRuns[i]) {
			t.Errorf("line %d = %q, want prefix %q", i, line, wantRuns[i])
		}
		if strings.Contains(line, "INCOMPLETE") {
			incomplete++
		}
	}
	if incomplete != 1 {
		t.Errorf("INCOMPLETE lines = %d, want 1:\n%s", incomplete, out)
	}
	if !strings.Contains(lines[2], "INCOMPLETE") {
		t.Errorf("third line %q should be INCOMPLETE", lines[2])
	}
}

func TestTrendJSONOutput(t *testing.T) {
	out, err := runTrend(t, "selftest", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("output is not a JSON array of objects: %v\n%s", err, out)
	}
	if len(rows) != 4 {
		t.Fatalf("got %d objects, want 4", len(rows))
	}
	if rows[0]["run"] != "260701-100000-aaaaaaa" {
		t.Errorf("first run = %v, want 260701-100000-aaaaaaa", rows[0]["run"])
	}
}

func TestTrendUnknownAgentErrorNamesAgent(t *testing.T) {
	out, err := runTrend(t, "nope", false)
	if err == nil {
		t.Fatal("expected an error for an agent with no results")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error %q does not name the agent", err.Error())
	}
	if out != "" {
		t.Errorf("nothing should be written on error, got %q", out)
	}
}

func TestTrendSilencesUsageOnError(t *testing.T) {
	t.Cleanup(func() { trendCmd.SilenceUsage = false })
	trendCmd.SilenceUsage = false
	if _, err := runTrend(t, "nope", false); err == nil {
		t.Fatal("expected an error")
	}
	if !trendCmd.SilenceUsage {
		t.Error("usage must be silenced once arguments are parsed")
	}
}

// trend receives the filters: a malformed one is refused before SilenceUsage,
// and a valid one that matches no case makes the run list empty (an error
// naming the filter).
func TestTrendReceivesFilters(t *testing.T) {
	origFilter, origSilence := evalFilter, trendCmd.SilenceUsage
	t.Cleanup(func() { evalFilter, trendCmd.SilenceUsage = origFilter, origSilence })

	evalFilter = []string{"speed"}
	trendCmd.SilenceUsage = false
	if _, err := runTrend(t, "selftest", false); err == nil {
		t.Fatal("trend with a malformed filter: expected an error")
	}
	if trendCmd.SilenceUsage {
		t.Error("usage must still be shown for a malformed filter (SilenceUsage was set)")
	}

	evalFilter = []string{"name==nope"}
	_, err := runTrend(t, "selftest", false)
	if err == nil || !strings.Contains(err.Error(), "name==nope") {
		t.Errorf("trend with a non-matching filter: err = %v, want an error naming the filter", err)
	}
}
