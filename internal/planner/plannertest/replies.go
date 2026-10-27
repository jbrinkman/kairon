// Package plannertest builds planner-automated replies for tests outside the
// planner package that run the real planner boundary. Production code never
// imports it.
package plannertest

import (
	"fmt"
	"strings"

	"github.com/jbrinkman/kairon/internal/stageoutput"
)

// ValidBody is a canonical issue body that passes the default guard chain and
// the dependencies-form guard. deps is the single Dependencies line, for
// example "Dependencies: none" or "Dependencies: #12".
func ValidBody(deps string) string {
	return "# Add a JSON Lines exporter\n\n" +
		"## Problem Statement\n\nRun history cannot be piped into jq.\n\n" +
		"## Context\n\nExporters live in internal/export.\n\n" +
		"## Scope\n\n### In Scope\n\n- A jsonl exporter.\n\n### Out of Scope\n\n- The CSV exporter.\n\n" +
		"## Acceptance Criteria\n\n" +
		"1. The exporter writes one JSON object per line.\n   Verify: run `kairon export --format jsonl`; every line parses with `jq -e .`.\n\n" +
		"## Dependencies\n\n" + deps + "\n"
}

// RejectedBody is a canonical body the structural guard rejects: it has no
// Context section.
func RejectedBody() string {
	return strings.Replace(ValidBody("Dependencies: none"), "## Context\n\nExporters live in internal/export.\n\n", "", 1)
}

// CanonicalReply is a planner-automated reply carrying body for repo#issue.
func CanonicalReply(repo string, issue int, body string) string {
	return marshal(&stageoutput.Document{
		SchemaVersion: 1,
		Stage:         stageoutput.StagePlanner,
		Result:        stageoutput.ResultCanonical,
		Source:        stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: fmt.Sprintf("%s#%d", repo, issue)},
		Generated:     "2026-10-27T12:00:00Z",
		Body:          body,
	})
}

// PunchOutReply is a planner-automated punch_out for repo#issue.
func PunchOutReply(repo string, issue int) string {
	return marshal(&stageoutput.Document{
		SchemaVersion: 1,
		Stage:         stageoutput.StagePlanner,
		Result:        stageoutput.ResultPunchOut,
		Source:        stageoutput.Source{Kind: stageoutput.SourceGitHubIssue, Ref: fmt.Sprintf("%s#%d", repo, issue)},
		Generated:     "2026-10-27T12:00:00Z",
		Unresolved: []stageoutput.Unresolved{
			{Slot: "performance_target", Reason: "the issue asks for faster export with no measurable target"},
		},
	})
}

func marshal(d *stageoutput.Document) string {
	raw, err := stageoutput.Marshal(d)
	if err != nil {
		panic(fmt.Sprintf("plannertest: marshal: %v", err))
	}
	return string(raw)
}
