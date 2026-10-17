package eval

import (
	"errors"
	"strings"
	"testing"
)

func TestLoadCasesJudgeCheck(t *testing.T) {
	checksEnv(t)
	writeChecksCase(t, "chk", "judge.yaml", `name: judge
input: x
checks:
  - criterion: clarity
    type: judge
    question: "Is the issue body testable?"
    files: [.eval/issue-body.md, notes.md]
`)
	got, err := loadCases("chk")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Checks) != 1 {
		t.Fatalf("loaded %+v", got)
	}
	c := got[0].Checks[0]
	if c.Type != CheckJudge || c.Question != "Is the issue body testable?" || len(c.Files) != 2 || c.Files[0] != ".eval/issue-body.md" {
		t.Errorf("judge check decoded wrong: %+v", c)
	}
}

func TestLoadCasesJudgeCheckRejected(t *testing.T) {
	tests := []struct {
		name, yaml, wantErr string
	}{
		{"missing question", "  - criterion: clarity\n    type: judge\n", "question is required"},
		{"duplicate file", "  - criterion: clarity\n    type: judge\n    question: q\n    files: [a, a]\n", "duplicate"},
		{"files on other type", "  - criterion: clarity\n    type: file_exists\n    path: a\n    files: [a]\n", "files is not valid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksEnv(t)
			writeChecksCase(t, "chk", "bad-case.yaml", "name: bad\ninput: x\nchecks:\n"+tt.yaml)
			_, err := loadCases("chk")
			if err == nil || !errors.Is(err, errInvalidChecks) {
				t.Fatalf("want errInvalidChecks, got %v", err)
			}
			for _, want := range []string{"bad-case.yaml", "#1", tt.wantErr} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestScoredUserTurns(t *testing.T) {
	if got := scoredUserTurns(TestCase{Input: "classic"}); got != nil {
		t.Errorf("classic case: scoredUserTurns = %v, want nil", got)
	}
	got := scoredUserTurns(TestCase{Turns: []string{"a", "b"}})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("scoredUserTurns = %v, want [a b]", got)
	}
}

// scoreCase must hand the judge the user messages actually sent (tc.Turns,
// which executeCase swaps for the simulated user's messages).
func TestCaseCheckInputCarriesUserTurns(t *testing.T) {
	cr := &CaseResult{
		ActualOutput: "r2",
		WorkspaceDir: t.TempDir(),
		TurnOutputs:  []string{"r1", "r2"},
		turnGHLogs:   make([]ghLogSnapshot, 2),
	}
	tc := TestCase{Name: "c", Turns: []string{"sent one", "sent two"}}
	in := caseCheckInput(tc, cr)
	if len(in.UserTurns) != 2 || in.UserTurns[0] != "sent one" || in.UserTurns[1] != "sent two" {
		t.Errorf("CheckInput.UserTurns = %v, want the messages from tc.Turns", in.UserTurns)
	}
	if len(in.Turns) != 2 {
		t.Errorf("CheckInput.Turns = %d entries, want 2", len(in.Turns))
	}
	if got := caseCheckInput(TestCase{Name: "classic", Input: "x"}, &CaseResult{}).UserTurns; got != nil {
		t.Errorf("classic case UserTurns = %v, want nil", got)
	}
}

// A hand-built case that bypasses the loader records the out-of-range failure.
func TestScoreCaseJudgeTurnOutOfRangeRecorded(t *testing.T) {
	chdirTemp(t)
	useRecording(t, yesBackend())
	rubric := Rubric{Agent: "a", Criteria: []Criterion{{Name: "crit", Scoring: "1-7"}}}
	tc := TestCase{Name: "c", Turns: []string{"u1", "u2"}, Checks: []Check{
		{Criterion: "crit", Type: CheckJudge, Question: "q", Turn: turnPtr(9)},
	}}
	cr := CaseResult{
		ActualOutput: "r2",
		WorkspaceDir: t.TempDir(),
		TurnOutputs:  []string{"r1", "r2"},
		turnGHLogs:   make([]ghLogSnapshot, 2),
	}
	scoreCase(rubric, tc, &cr)
	if len(cr.Scores) != 1 || len(cr.Scores[0].Checks) != 1 {
		t.Fatalf("scores = %+v", cr.Scores)
	}
	r := cr.Scores[0].Checks[0]
	if r.Passed || !strings.Contains(r.Detail, "turn 9 is out of range: the case has 2 turn(s)") {
		t.Errorf("check result = %+v, want a failure naming the turn count", r)
	}
	if len(cr.Calls) != 0 {
		t.Errorf("model called %d times for an out-of-range turn", len(cr.Calls))
	}
}
