package eval

import (
	"reflect"
	"strings"
	"testing"
)

// simBase is a valid simulated-user case head. Tests append checks/stub
// blocks, or replace it wholesale, to build one invalid variation each.
const simBase = `name: %s
agent: a1
input: "I want a CSV export"
simulated_user:
  facts:
    - id: f1
      answer: "Comma separated."
      discoverable: false
    - id: f2
      answer: "Written in Go."
      discoverable: true
  default_reply: "Use your best judgement."
`

func simYAML(name, extra string) string {
	return strings.Replace(simBase, "%s", name, 1) + extra
}

func TestLoadCasesSimulatedUserValid(t *testing.T) {
	cases, err := loadTurnsCase(t, `name: sim-valid
agent: a1
input: "I want a CSV export"
simulated_user:
  facts:
    - id: f1
      answer: "  Comma separated.  "
      discoverable: false
    - id: f2
      answer: "Written in Go."
      discoverable: true
  default_reply: "Use your best judgement."
  stop_when: 'READY'
  max_turns: 4
turns:
  - "Looks good."
stub:
  routes: [f1, none, f2]
  turns:
    - response: a
    - response: b
    - response: c
    - response: d
    - response: e
checks:
  - {criterion: c, type: fact_asked, fact: f1}
  - {criterion: c, type: fact_not_asked, fact: f2}
  - {criterion: c, type: output_contains, pattern: e, turn: 5}
`)
	if err != nil {
		t.Fatalf("loadCases: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("cases = %d", len(cases))
	}
	tc := cases[0]
	su := tc.SimulatedUser
	if su == nil {
		t.Fatal("SimulatedUser not decoded")
	}
	if len(su.Facts) != 2 || su.Facts[0].ID != "f1" || su.Facts[0].Answer != "  Comma separated.  " {
		t.Errorf("facts = %+v", su.Facts)
	}
	if su.Facts[0].Discoverable == nil || *su.Facts[0].Discoverable || !*su.Facts[1].Discoverable {
		t.Errorf("discoverable not decoded: %+v", su.Facts)
	}
	if su.DefaultReply != "Use your best judgement." || su.StopWhen != "READY" {
		t.Errorf("default_reply/stop_when = %q / %q", su.DefaultReply, su.StopWhen)
	}
	if su.stopRE == nil || !su.stopRE.MatchString("xx READY yy") {
		t.Errorf("stop_when not compiled: %v", su.stopRE)
	}
	if got := tc.maxTurns(); got != 4 {
		t.Errorf("maxTurns = %d, want 4", got)
	}
	if !reflect.DeepEqual(tc.Turns, []string{"Looks good."}) || tc.Input == "" {
		t.Errorf("input/turns = %q / %v", tc.Input, tc.Turns)
	}
}

func TestLoadCasesSimulatedUserDefaults(t *testing.T) {
	cases, err := loadTurnsCase(t, simYAML("sim-defaults", ""))
	if err != nil {
		t.Fatalf("loadCases: %v", err)
	}
	tc := cases[0]
	if got := tc.maxTurns(); got != 10 {
		t.Errorf("default maxTurns = %d, want 10", got)
	}
	if tc.SimulatedUser.stopRE != nil {
		t.Error("stopRE set without stop_when")
	}
	// Without scripted turns a simulated case's only fixed message is input.
	if got := tc.userTurns(); !reflect.DeepEqual(got, []string{"I want a CSV export"}) {
		t.Errorf("userTurns = %q", got)
	}
}

func TestUserTurnsSimulated(t *testing.T) {
	tc := TestCase{Input: "open", Turns: []string{"a", "b"}, SimulatedUser: &SimulatedUser{}}
	if got := tc.userTurns(); !reflect.DeepEqual(got, []string{"open", "a", "b"}) {
		t.Errorf("userTurns = %q, want [open a b]", got)
	}
	if got := (TestCase{Turns: []string{"a", "b"}}).userTurns(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("non-simulated turns userTurns = %q", got)
	}
}

func TestMaxTurnsDefaultAndExplicit(t *testing.T) {
	if got := (TestCase{}).maxTurns(); got != 10 {
		t.Errorf("no simulated_user maxTurns = %d, want 10", got)
	}
	n := 7
	tc := TestCase{SimulatedUser: &SimulatedUser{MaxTurns: &n}}
	if got := tc.maxTurns(); got != 7 {
		t.Errorf("maxTurns = %d, want 7", got)
	}
}

func TestLoadCasesSimulatedUserSchemaErrors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "missing-discoverable",
			yaml: `name: missing-discoverable
agent: a1
input: hi
simulated_user:
  facts:
    - {id: f1, answer: a}
  default_reply: r
`,
			want: "discoverable",
		},
		{
			name: "no-facts",
			yaml: `name: no-facts
agent: a1
input: hi
simulated_user:
  facts: []
  default_reply: r
`,
			want: "facts must list at least one fact",
		},
		{
			name: "duplicate-id",
			yaml: `name: duplicate-id
agent: a1
input: hi
simulated_user:
  facts:
    - {id: f1, answer: a, discoverable: false}
    - {id: f1, answer: b, discoverable: false}
  default_reply: r
`,
			want: "duplicate id",
		},
		{
			name: "id-none",
			yaml: `name: id-none
agent: a1
input: hi
simulated_user:
  facts:
    - {id: None, answer: a, discoverable: false}
  default_reply: r
`,
			want: "reserved",
		},
		{
			name: "blank-id",
			yaml: `name: blank-id
agent: a1
input: hi
simulated_user:
  facts:
    - {id: "  ", answer: a, discoverable: false}
  default_reply: r
`,
			want: "facts[0].id",
		},
		{
			name: "whitespace-id",
			yaml: `name: whitespace-id
agent: a1
input: hi
simulated_user:
  facts:
    - {id: "f 1", answer: a, discoverable: false}
  default_reply: r
`,
			want: "whitespace",
		},
		{
			name: "leading-whitespace-id",
			yaml: `name: leading-whitespace-id
agent: a1
input: hi
simulated_user:
  facts:
    - {id: " f1", answer: a, discoverable: false}
  default_reply: r
`,
			want: "whitespace",
		},
		{
			name: "trailing-whitespace-id",
			yaml: `name: trailing-whitespace-id
agent: a1
input: hi
simulated_user:
  facts:
    - {id: "f1 ", answer: a, discoverable: false}
  default_reply: r
`,
			want: "whitespace",
		},
		{
			name: "blank-answer",
			yaml: `name: blank-answer
agent: a1
input: hi
simulated_user:
  facts:
    - {id: f1, answer: " ", discoverable: false}
  default_reply: r
`,
			want: "facts[0].answer",
		},
		{
			name: "blank-default-reply",
			yaml: `name: blank-default-reply
agent: a1
input: hi
simulated_user:
  facts:
    - {id: f1, answer: a, discoverable: false}
  default_reply: " "
`,
			want: "default_reply",
		},
		{
			name: "missing-default-reply",
			yaml: `name: missing-default-reply
agent: a1
input: hi
simulated_user:
  facts:
    - {id: f1, answer: a, discoverable: false}
`,
			want: "default_reply",
		},
		{
			name: "bad-stop-when",
			yaml: simYAML("bad-stop-when", "") + "  stop_when: '('\n",
			want: "stop_when",
		},
		{
			name: "max-turns-zero",
			yaml: simYAML("max-turns-zero", "") + "  max_turns: 0\n",
			want: "max_turns must be at least 1",
		},
		{
			name: "max-turns-negative",
			yaml: simYAML("max-turns-negative", "") + "  max_turns: -1\n",
			want: "max_turns must be at least 1",
		},
		{
			name: "missing-input",
			yaml: `name: missing-input
agent: a1
simulated_user:
  facts:
    - {id: f1, answer: a, discoverable: false}
  default_reply: r
`,
			want: "simulated_user requires input",
		},
		{
			name: "empty-turns",
			yaml: simYAML("empty-turns", "turns: []\n"),
			want: "turns must list at least one user message",
		},
		{
			name: "blank-scripted-turn",
			yaml: simYAML("blank-scripted-turn", "turns:\n  - one\n  - \"  \"\n"),
			want: "turns[1]",
		},
		{
			name: "fact-check-without-simulated-user",
			yaml: `name: fact-check-without-simulated-user
agent: a1
input: hi
checks:
  - {criterion: c, type: fact_asked, fact: f1}
`,
			want: "requires simulated_user",
		},
		{
			name: "fact-check-unknown-fact",
			yaml: simYAML("fact-check-unknown-fact", "checks:\n  - {criterion: c, type: fact_not_asked, fact: nope}\n"),
			want: `fact "nope" is not defined`,
		},
		{
			name: "routes-without-simulated-user",
			yaml: `name: routes-without-simulated-user
agent: a1
input: hi
stub:
  routes: [none]
  turns:
    - response: a
`,
			want: "stub.routes requires simulated_user",
		},
		{
			name: "route-unknown-fact",
			yaml: simYAML("route-unknown-fact", "stub:\n  routes: [f9]\n  turns:\n    - response: a\n    - response: b\n"),
			want: `stub.routes[0]`,
		},
		{
			name: "too-many-routes",
			yaml: simYAML("too-many-routes", "") + "  max_turns: 3\nstub:\n  routes: [f1, none, f2]\n  turns:\n    - response: a\n    - response: b\n    - response: c\n",
			want: "stub.routes has 3 entries",
		},
		{
			name: "stub-turns-too-few",
			yaml: simYAML("stub-turns-too-few", "turns: [one, two]\nstub:\n  turns:\n    - response: a\n    - response: b\n"),
			want: "stub.turns has 2 entries",
		},
		{
			name: "stub-turns-too-many",
			yaml: simYAML("stub-turns-too-many", "turns: [one]\nstub:\n  turns:\n"+strings.Repeat("    - response: a\n", 12)),
			want: "stub.turns has 12 entries",
		},
		{
			name: "check-turn-too-high",
			yaml: simYAML("check-turn-too-high", "turns: [one]\nchecks:\n  - {criterion: c, type: output_contains, pattern: x, turn: 12}\n"),
			want: "turn 12 is beyond",
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

func TestLoadCasesSimulatedUserCheckTurnBounds(t *testing.T) {
	// max_turns 3 + 2 scripted turns = 5 is the highest legal check turn.
	head := simYAML("sim-turn-bound", "")
	ok := head + "  max_turns: 3\nturns: [one, two]\nchecks:\n  - {criterion: c, type: output_contains, pattern: x, turn: 5}\n"
	if _, err := loadTurnsCase(t, ok); err != nil {
		t.Fatalf("turn 5 should load: %v", err)
	}
	bad := head + "  max_turns: 3\nturns: [one, two]\nchecks:\n  - {criterion: c, type: output_contains, pattern: x, turn: 6}\n"
	if _, err := loadTurnsCase(t, bad); err == nil || !strings.Contains(err.Error(), "turn 6") {
		t.Fatalf("turn 6 should fail, got %v", err)
	}
}

func TestLoadCasesSimulatedUserStubBounds(t *testing.T) {
	head := simYAML("sim-stub-bound", "") + "  max_turns: 3\nturns: [one]\nstub:\n  routes: [f1, none]\n  turns:\n"
	// 1+len(turns) = 2 .. max_turns+len(turns) = 4 entries are legal.
	for n := 2; n <= 4; n++ {
		if _, err := loadTurnsCase(t, head+strings.Repeat("    - response: a\n", n)); err != nil {
			t.Errorf("%d stub.turns should load: %v", n, err)
		}
	}
	if _, err := loadTurnsCase(t, head+strings.Repeat("    - response: a\n", 1)); err == nil {
		t.Error("1 stub.turns should fail")
	}
	if _, err := loadTurnsCase(t, head+strings.Repeat("    - response: a\n", 5)); err == nil {
		t.Error("5 stub.turns should fail")
	}
}

func TestLoadCasesSimulatedUserRoutesNoneAndCaseInsensitive(t *testing.T) {
	yaml := simYAML("sim-routes-ok", "stub:\n  routes: [NONE, f1]\n  turns:\n    - response: a\n    - response: b\n    - response: c\n")
	if _, err := loadTurnsCase(t, yaml); err != nil {
		t.Fatalf("routes with NONE and a fact id should load: %v", err)
	}
}

func TestLoadCasesTurnsAndInputStillExclusiveWithoutSimulatedUser(t *testing.T) {
	_, err := loadTurnsCase(t, "name: no-sim-both\nagent: a1\ninput: hello\nturns:\n  - one\n")
	if err == nil || !strings.Contains(err.Error(), "turns and input are mutually exclusive") {
		t.Fatalf("err = %v, want mutually exclusive", err)
	}
}
