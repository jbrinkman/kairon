package stageoutput

import (
	"fmt"
	"strings"
	"time"
)

// Validate checks the structural rules of the document. It is shared by Parse
// and Marshal.
//
// It collects every problem it finds and returns them as a *ValidationError,
// which matches ErrInvalidDocument. Each problem names the offending field
// path, for example "unresolved[1].reason".
func (d *Document) Validate() error {
	if d == nil {
		return &ValidationError{Problems: []string{"document is nil"}}
	}

	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if d.SchemaVersion < 1 {
		add("schema_version: must be an integer >= 1, got %d", d.SchemaVersion)
	}
	if !d.Stage.Valid() {
		add("stage: %q is not one of planner, architect, krew-lead, builder, validator, documenter", d.Stage)
	}
	if !d.Result.Valid() {
		add("result: %q is not one of canonical, punch_out", d.Result)
	}
	if !d.Source.Kind.Valid() {
		add("source.kind: %q is not one of github_issue, document", d.Source.Kind)
	}
	if strings.TrimSpace(d.Source.Ref) == "" {
		add("source.ref: must not be empty")
	}
	if err := checkGenerated(d.Generated); err != nil {
		add("generated: %v", err)
	}

	// The variant rules apply only when result is a known value; otherwise the
	// result problem above is the one to fix.
	switch d.Result {
	case ResultCanonical:
		if strings.TrimSpace(d.Body) == "" {
			add("body: required (and not blank) when result is canonical")
		}
		if len(d.Unresolved) > 0 {
			add("unresolved: not allowed when result is canonical")
		}
	case ResultPunchOut:
		if d.Body != "" {
			add("body: not allowed when result is punch_out")
		}
		if len(d.Unresolved) == 0 {
			add("unresolved: required (at least one entry) when result is punch_out")
		}
		for i, u := range d.Unresolved {
			if strings.TrimSpace(u.Slot) == "" {
				add("unresolved[%d].slot: must not be empty", i)
			}
			if strings.TrimSpace(u.Reason) == "" {
				add("unresolved[%d].reason: must not be empty", i)
			}
		}
	}

	for i, inf := range d.Inferences {
		if strings.TrimSpace(inf.Slot) == "" {
			add("inferences[%d].slot: must not be empty", i)
		}
		if strings.TrimSpace(inf.Inferred) == "" {
			add("inferences[%d].inferred: must not be empty", i)
		}
		if strings.TrimSpace(inf.Evidence) == "" {
			add("inferences[%d].evidence: must not be empty", i)
		}
	}

	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

// checkGenerated requires an RFC 3339 timestamp with a zero UTC offset.
func checkGenerated(s string) error {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return fmt.Errorf("%q is not an RFC 3339 timestamp", s)
	}
	// RFC 3339 4.3 treats "-00:00" as "offset unknown", distinct from "Z" and
	// "+00:00" which assert UTC. Its numeric offset is zero, so the Zone check
	// below would accept it; reject it explicitly by its trailing offset text.
	if strings.HasSuffix(s, "-00:00") {
		return fmt.Errorf("%q is not in UTC (offset must be Z or +00:00)", s)
	}
	if _, offset := t.Zone(); offset != 0 {
		return fmt.Errorf("%q is not in UTC (offset must be Z or +00:00)", s)
	}
	return nil
}
