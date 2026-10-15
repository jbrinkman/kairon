package eval

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// decodeChecks decodes a YAML list of checks the way a case file does.
func decodeChecks(t *testing.T, src string) []Check {
	t.Helper()
	var checks []Check
	if err := yaml.Unmarshal([]byte(src), &checks); err != nil {
		t.Fatalf("decode checks: %v", err)
	}
	return checks
}

func turnPtr(n int) *int { return &n }

func TestCheckTurnYAMLDecodes(t *testing.T) {
	checks := decodeChecks(t, "- {criterion: c, type: gh_log_contains, pattern: x, turn: 3}\n- {criterion: c, type: gh_log_contains, pattern: x}\n")
	if checks[0].Turn == nil || *checks[0].Turn != 3 {
		t.Errorf("turn: 3 decoded as %v, want 3", checks[0].Turn)
	}
	if checks[1].Turn != nil {
		t.Errorf("missing turn decoded as %v, want nil", *checks[1].Turn)
	}
}

func TestCheckTurnValidateAllowedTypes(t *testing.T) {
	for _, typ := range []string{"output_contains", "output_not_contains", "gh_log_contains", "gh_log_not_contains"} {
		t.Run(typ, func(t *testing.T) {
			checks := decodeChecks(t, "- {criterion: c, type: "+typ+", pattern: x, turn: 2}\n")
			if err := ValidateChecks("f.yaml", checks, "", nil); err != nil {
				t.Fatalf("turn should be valid for %s: %v", typ, err)
			}
		})
	}
}

func TestCheckTurnValidateRejectedTypes(t *testing.T) {
	cases := map[string]string{
		"file_exists":       "{criterion: c, type: file_exists, path: a.txt, turn: 1}",
		"file_absent":       "{criterion: c, type: file_absent, path: a.txt, turn: 1}",
		"file_contains":     "{criterion: c, type: file_contains, path: a.txt, pattern: x, turn: 1}",
		"file_not_contains": "{criterion: c, type: file_not_contains, path: a.txt, pattern: x, turn: 1}",
		"changed_files":     "{criterion: c, type: changed_files, allow: [a.txt], turn: 1}",
		"command":           "{criterion: c, type: command, run: 'true', turn: 1}",
	}
	for typ, src := range cases {
		t.Run(typ, func(t *testing.T) {
			err := ValidateChecks("f.yaml", decodeChecks(t, "- "+src+"\n"), "", nil)
			if err == nil {
				t.Fatalf("turn on %s must be a load error", typ)
			}
			want := "turn is not valid for type " + typ
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err, want)
			}
		})
	}
}

func TestCheckTurnValidateRejectsBelowOne(t *testing.T) {
	for _, n := range []string{"0", "-1"} {
		t.Run(n, func(t *testing.T) {
			checks := decodeChecks(t, "- {criterion: c, type: output_contains, pattern: x, turn: "+n+"}\n")
			err := ValidateChecks("f.yaml", checks, "", nil)
			if err == nil {
				t.Fatalf("turn: %s must be rejected", n)
			}
			if !strings.Contains(err.Error(), "turn") || !strings.Contains(err.Error(), ">= 1") {
				t.Errorf("error %q should say turn must be >= 1", err)
			}
		})
	}
}

// threeTurns is shared evidence: `gh issue create` only happens in turn 3.
func threeTurns() CheckInput {
	return CheckInput{
		Output: "out3",
		GHLog:  "gh issue create --title t\n",
		Turns: []TurnEvidence{
			{Output: "out1", GHLog: ""},
			{Output: "out2", GHLog: "gh issue list\n"},
			{Output: "out3", GHLog: "gh issue list\ngh issue create --title t\n"},
		},
	}
}

func TestCheckTurnEvalPerTurnGHLog(t *testing.T) {
	in := threeTurns()
	notCreate := Check{Criterion: "c", Type: CheckGHLogNotContains, Pattern: "issue create", Turn: turnPtr(1)}
	create3 := Check{Criterion: "c", Type: CheckGHLogContains, Pattern: "issue create", Turn: turnPtr(3)}
	rs := EvaluateChecks([]Check{notCreate, create3}, in)
	wantPass(t, rs[0])
	wantPass(t, rs[1])

	// The same pattern pinned to the wrong turn must fail (proves scoping).
	create1 := Check{Criterion: "c", Type: CheckGHLogContains, Pattern: "issue create", Turn: turnPtr(1)}
	wantFail(t, evalOne(t, create1, in), "not found in gh log")
	notCreate3 := Check{Criterion: "c", Type: CheckGHLogNotContains, Pattern: "issue create", Turn: turnPtr(3)}
	wantFail(t, evalOne(t, notCreate3, in), "matched in gh log")
}

func TestCheckTurnEvalPerTurnOutput(t *testing.T) {
	in := threeTurns()
	wantPass(t, evalOne(t, Check{Type: CheckOutputContains, Pattern: "out1", Turn: turnPtr(1)}, in))
	wantFail(t, evalOne(t, Check{Type: CheckOutputContains, Pattern: "out1", Turn: turnPtr(2)}, in), "not found in output")
	wantPass(t, evalOne(t, Check{Type: CheckOutputNotContains, Pattern: "out3", Turn: turnPtr(2)}, in))
	wantFail(t, evalOne(t, Check{Type: CheckOutputNotContains, Pattern: "out2", Turn: turnPtr(2)}, in), "matched in output")
}

func TestCheckTurnEvalDefaultsToLastTurn(t *testing.T) {
	// Top-level Output/GHLog deliberately differ from the last turn's evidence
	// so we can tell which one a no-turn check read.
	in := CheckInput{
		Output: "top-level",
		GHLog:  "top-level-log",
		Turns: []TurnEvidence{
			{Output: "first", GHLog: "first-log"},
			{Output: "last", GHLog: "last-log"},
		},
	}
	wantPass(t, evalOne(t, Check{Type: CheckOutputContains, Pattern: "^last$"}, in))
	wantPass(t, evalOne(t, Check{Type: CheckGHLogContains, Pattern: "^last-log$"}, in))
	wantPass(t, evalOne(t, Check{Type: CheckOutputNotContains, Pattern: "first"}, in))
}

func TestCheckTurnEvalEmptyTurnsUsesTopLevel(t *testing.T) {
	in := CheckInput{Output: "hello", GHLog: "gh issue create"}
	wantPass(t, evalOne(t, Check{Type: CheckOutputContains, Pattern: "hello"}, in))
	wantPass(t, evalOne(t, Check{Type: CheckGHLogContains, Pattern: "issue create"}, in))
	// With no per-turn evidence, an explicit turn falls back to the legacy fields.
	wantPass(t, evalOne(t, Check{Type: CheckOutputContains, Pattern: "hello", Turn: turnPtr(1)}, in))
}

func TestCheckTurnEvalOutOfRangeFailsWithDetail(t *testing.T) {
	in := threeTurns()
	for _, typ := range []CheckType{CheckOutputContains, CheckOutputNotContains, CheckGHLogContains, CheckGHLogNotContains} {
		t.Run(string(typ), func(t *testing.T) {
			// Hand-built (not loaded) check; validateCheck cannot know the turn count.
			r := evalOne(t, Check{Type: typ, Pattern: "zzz-never", Turn: turnPtr(4)}, in)
			wantFail(t, r, "turn 4")
			if !strings.Contains(r.Detail, "3") {
				t.Errorf("detail %q should mention the number of turns", r.Detail)
			}
		})
	}
}

func TestCheckTurnEvalOversizedGHLogIsPerTurn(t *testing.T) {
	in := CheckInput{
		GHLog: "tail",
		Turns: []TurnEvidence{
			{GHLog: "small"},
			{GHLog: "huge", GHLogOversized: true},
		},
	}
	wantPass(t, evalOne(t, Check{Type: CheckGHLogContains, Pattern: "small", Turn: turnPtr(1)}, in))
	wantFail(t, evalOne(t, Check{Type: CheckGHLogContains, Pattern: "huge", Turn: turnPtr(2)}, in), "10 MiB")
	// Default (last turn) is the oversized one.
	wantFail(t, evalOne(t, Check{Type: CheckGHLogNotContains, Pattern: "x"}, in), "10 MiB")
}

func TestCheckTurnLabel(t *testing.T) {
	with := checkLabel(2, Check{Type: CheckGHLogContains, Pattern: "issue create", Turn: turnPtr(3)})
	if want := "#2 gh_log_contains pattern=/issue create/ turn=3"; with != want {
		t.Errorf("label = %q, want %q", with, want)
	}
	without := checkLabel(2, Check{Type: CheckGHLogContains, Pattern: "issue create"})
	if want := "#2 gh_log_contains pattern=/issue create/"; without != want {
		t.Errorf("label = %q, want %q", without, want)
	}
	if strings.Contains(checkLabel(1, Check{Type: CheckFileExists, Path: "a.txt"}), "turn") {
		t.Error("label for a check without turn must not mention turn")
	}
}
