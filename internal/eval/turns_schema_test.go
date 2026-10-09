package eval

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// loadTurnsCase writes one case file into a fresh t.TempDir() evals dir
// (a load error is fatal for the whole agent directory, so invalid cases
// cannot live in testdata) and loads the agent's cases.
func loadTurnsCase(t *testing.T, caseYAML string) ([]TestCase, error) {
	t.Helper()
	dir := chdirTemp(t)
	writeCfgFile(t, filepath.Join(dir, "evals", "cases", "a1", "c1.yaml"), caseYAML)
	if err := configure(RunOptions{EvalsDir: "evals"}); err != nil {
		t.Fatal(err)
	}
	return loadCases("a1")
}

func TestLoadCasesTurnsAndInputMutuallyExclusive(t *testing.T) {
	_, err := loadTurnsCase(t, `name: both-forms
agent: a1
input: hello
turns:
  - one
  - two
`)
	if err == nil {
		t.Fatal("expected a load error for a case with both turns and input")
	}
	for _, want := range []string{"both-forms", "turns and input are mutually exclusive"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestLoadCasesTurnsValid(t *testing.T) {
	cases, err := loadTurnsCase(t, `name: three-turns
agent: a1
turns:
  - one
  - two
  - three
stub:
  turns:
    - response: a
    - response: b
    - response: c
checks:
  - {criterion: c, type: output_contains, pattern: c, turn: 3}
`)
	if err != nil {
		t.Fatalf("loadCases: %v", err)
	}
	if len(cases) != 1 || !reflect.DeepEqual(cases[0].Turns, []string{"one", "two", "three"}) {
		t.Fatalf("cases = %+v", cases)
	}
}

func TestLoadCasesTurnsSchemaErrors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "empty-turns",
			yaml: "name: empty-turns\nagent: a1\nturns: []\n",
			want: "turns",
		},
		{
			name: "blank-turn",
			yaml: "name: blank-turn\nagent: a1\nturns:\n  - one\n  - \"  \"\n",
			want: "turns[1]",
		},
		{
			name: "stub-count-mismatch",
			yaml: `name: stub-count-mismatch
agent: a1
turns: [one, two, three]
stub:
  turns:
    - response: a
    - response: b
`,
			want: "stub.turns",
		},
		{
			name: "check-turn-too-high",
			yaml: `name: check-turn-too-high
agent: a1
turns: [one, two, three]
checks:
  - {criterion: c, type: output_contains, pattern: x, turn: 4}
`,
			want: "turn 4",
		},
		{
			name: "classic-turn-2",
			yaml: `name: classic-turn-2
agent: a1
input: hi
checks:
  - {criterion: c, type: output_contains, pattern: x, turn: 2}
`,
			want: "turn 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadTurnsCase(t, tt.yaml)
			if err == nil {
				t.Fatal("expected a load error")
			}
			if !strings.Contains(err.Error(), tt.name) {
				t.Errorf("error %q does not name the case %q", err, tt.name)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}

func TestLoadCasesClassicTurnOneLoads(t *testing.T) {
	cases, err := loadTurnsCase(t, `name: classic-turn-1
agent: a1
input: hi
checks:
  - {criterion: c, type: output_contains, pattern: x, turn: 1}
`)
	if err != nil || len(cases) != 1 {
		t.Fatalf("loadCases: %v, %d cases", err, len(cases))
	}
}

func TestLoadCasesClassicMultiEntryStubUnchanged(t *testing.T) {
	cases, err := loadTurnsCase(t, `name: classic-stub
agent: a1
input: hi
stub:
  turns:
    - response: a
    - response: b
`)
	if err != nil || len(cases) != 1 {
		t.Fatalf("loadCases: %v, %d cases", err, len(cases))
	}
	if cases[0].Turns != nil {
		t.Errorf("Turns = %v, want nil for a classic case", cases[0].Turns)
	}
}

func TestUserTurns(t *testing.T) {
	if got := (TestCase{Input: "solo"}).userTurns(); !reflect.DeepEqual(got, []string{"solo"}) {
		t.Errorf("classic userTurns = %q", got)
	}
	if got := (TestCase{Turns: []string{"a", "b"}}).userTurns(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("multi-turn userTurns = %q", got)
	}
}

// captureStdinKiroCLI installs a fake kiro-cli that appends each call's stdin
// to the returned file, separated by a form feed, and prints a judge reply.
func captureStdinKiroCLI(t *testing.T) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "stdin.log")
	t.Setenv("STDIN_LOG", log)
	installFakeKiroCLI(t, `cat >> "$STDIN_LOG"
printf '\f' >> "$STDIN_LOG"
printf '===JSON_START===\n{"score": 4, "reasoning": "fine", "pass": true}\n===JSON_END==='`)
	return log
}

func readStdinCalls(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\f"), "\f")
}

func TestScoreLLMJudgeTurnsInput(t *testing.T) {
	chdirTemp(t)
	log := captureStdinKiroCLI(t)

	tc := TestCase{Turns: []string{"first question", "second answer"}}
	if _, _, _, skipped, _ := scoreLLMJudge(Criterion{Name: "clarity", Description: "d"}, tc, "LAST-OUTPUT"); skipped {
		t.Fatal("judge skipped")
	}
	got := readStdinCalls(t, log)
	if len(got) != 1 {
		t.Fatalf("judge calls = %d", len(got))
	}
	for _, want := range []string{
		"INPUT:\nTurn 1 (user): first question\n\nTurn 2 (user): second answer\n\nACTUAL OUTPUT TO EVALUATE:\nLAST-OUTPUT",
	} {
		if !strings.Contains(got[0], want) {
			t.Errorf("judge prompt missing %q:\n%s", want, got[0])
		}
	}
}

func TestScoreLLMJudgeClassicPromptUnchanged(t *testing.T) {
	chdirTemp(t)
	log := captureStdinKiroCLI(t)

	tc := TestCase{Input: "plain input", Context: []string{"fact"}, ExpectedOutput: "exp"}
	scoreLLMJudge(Criterion{Name: "clarity", Description: "d"}, tc, "OUT")
	got := readStdinCalls(t, log)

	want := `Evaluate this output against the criterion.
Wrap your JSON response between ===JSON_START=== and ===JSON_END=== delimiters.

{"score": <number 1-5>, "reasoning": "<explanation>", "pass": <boolean>}

CRITERION: clarity
DESCRIPTION: d

SCORING SCALE:
1 = Does not meet the criterion at all
2 = Minimally addresses the criterion with major gaps
3 = Partially meets the criterion with notable room for improvement
4 = Mostly meets the criterion with minor gaps
5 = Fully satisfies the criterion

CONTEXT FACTS (use for hallucination detection — deduct for contradictions):
fact

EXPECTED OUTPUT (compare similarity and completeness):
exp

INPUT:
plain input

ACTUAL OUTPUT TO EVALUATE:
OUT`
	if len(got) != 1 || got[0] != want {
		t.Errorf("classic judge prompt changed:\n got: %q\nwant: %q", got, want)
	}
}

func TestInvestigateParallelExecutionTurnsCaseSendsFirstTurn(t *testing.T) {
	dir := chdirTemp(t)
	for _, n := range []string{"c1", "c2"} {
		writeCfgFile(t, filepath.Join(dir, "evals", "cases", "a1", n+".yaml"),
			"name: "+n+"\nagent: a1\nturns: [\"FIRST-"+n+"\", \"second\"]\n")
	}
	if err := configure(RunOptions{EvalsDir: "evals"}); err != nil {
		t.Fatal(err)
	}
	log := captureStdinKiroCLI(t)

	if _, err := InvestigateParallelExecution("a1"); err != nil {
		t.Fatalf("InvestigateParallelExecution: %v", err)
	}
	all := strings.Join(readStdinCalls(t, log), "\n")
	for _, want := range []string{"FIRST-c1", "FIRST-c2"} {
		if !strings.Contains(all, want) {
			t.Errorf("agent never received %q; prompts:\n%s", want, all)
		}
	}
}
