package eval

import (
	"regexp"
	"strings"
	"testing"
)

// simulatedUserDocCase returns the documented simulated_user example: the
// ```yaml block of docs/evaluation.md that has a top-level simulated_user key.
func simulatedUserDocCase(t *testing.T) string {
	t.Helper()
	top := regexp.MustCompile(`(?m)^simulated_user:`)
	var found []string
	for _, block := range docYAMLBlocks(readEvaluationDoc(t)) {
		if top.MatchString(block) {
			found = append(found, block)
		}
	}
	if len(found) != 1 {
		t.Fatalf("docs/evaluation.md has %d YAML examples with a top-level simulated_user key, want exactly 1", len(found))
	}
	return found[0]
}

// The documented example must pass the strict case decoder (unknown fields are
// errors) and the full case validation, so the documentation cannot drift from
// the schema.
func TestSimulatedUserDocExampleLoads(t *testing.T) {
	cases, err := loadTurnsCase(t, simulatedUserDocCase(t))
	if err != nil {
		t.Fatalf("documented simulated_user example does not load: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(cases))
	}
	tc := cases[0]
	su := tc.SimulatedUser
	if su == nil {
		t.Fatal("example has no simulated_user")
	}

	// The example must show every field of the schema.
	if strings.TrimSpace(tc.Input) == "" {
		t.Error("example has no input")
	}
	if len(su.Facts) < 2 {
		t.Fatalf("example has %d facts, want at least 2", len(su.Facts))
	}
	var sawFalse, sawTrue bool
	for _, f := range su.Facts {
		if f.ID == "" || f.Answer == "" || f.Discoverable == nil {
			t.Errorf("example fact is incomplete: %+v", f)
			continue
		}
		if *f.Discoverable {
			sawTrue = true
		} else {
			sawFalse = true
		}
	}
	if !sawFalse || !sawTrue {
		t.Error("example should show both discoverable: false and discoverable: true")
	}
	if su.DefaultReply == "" {
		t.Error("example has no default_reply")
	}
	if su.StopWhen == "" || su.stopRE == nil {
		t.Errorf("example has no compiled stop_when: %q", su.StopWhen)
	}
	if su.MaxTurns == nil {
		t.Error("example has no max_turns")
	}
	if len(tc.Turns) == 0 {
		t.Error("example has no scripted turns after the simulated phase")
	}
	if tc.Stub == nil || len(tc.Stub.Routes) == 0 || len(tc.Stub.Turns) == 0 {
		t.Error("example has no stub.routes / stub.turns")
	}
	haveCheck := map[CheckType]bool{}
	for _, c := range tc.Checks {
		haveCheck[c.Type] = true
	}
	for _, ct := range []CheckType{CheckFactAsked, CheckFactNotAsked} {
		if !haveCheck[ct] {
			t.Errorf("example has no %s check", ct)
		}
	}
}

// Facts the docs state as rules must stay true in the code: the stated
// default and the load errors of the Load errors list.
func TestSimulatedUserDocStatesKeyRules(t *testing.T) {
	doc := readEvaluationDoc(t)
	for _, want := range []string{
		"## Simulated user (elicitation cases)",
		"`simulated_user.max_turns` | no (default `10`)",
		".eval/sim-log.jsonl",
		"purpose: simulated_user",
		"user_turns",
		"stub.routes",
		"fails closed",
		"index-based, not a cursor",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/evaluation.md does not contain %q", want)
		}
	}
	if got := (TestCase{SimulatedUser: &SimulatedUser{}}).maxTurns(); got != 10 {
		t.Errorf("default max_turns = %d, but docs/evaluation.md says 10", got)
	}
}
