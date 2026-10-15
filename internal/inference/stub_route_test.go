package inference

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func routeReq(stub *StubScript, seq int) Request {
	return Request{Role: RoleJudge, Prompt: "which fact?", Stub: stub, Route: true, RouteSeq: seq}
}

// parseStubRoute extracts the delimited JSON and returns its fact.
func parseStubRoute(t *testing.T, text string) string {
	t.Helper()
	const start, end = "===JSON_START===", "===JSON_END==="
	i := strings.Index(text, start)
	j := strings.Index(text, end)
	if i < 0 || j < i {
		t.Fatalf("text lacks delimiters: %q", text)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(text[i+len(start):j]), &v); err != nil {
		t.Fatalf("bad JSON in %q: %v", text, err)
	}
	fact, ok := v["fact"].(string)
	if !ok {
		t.Fatalf("JSON has no string \"fact\" key: %q", text)
	}
	if v["reasoning"] != "stub route" {
		t.Errorf("reasoning = %v, want \"stub route\"", v["reasoning"])
	}
	return fact
}

func TestStubRoute_ReturnsRouteBySeq(t *testing.T) {
	b := newTestStub(t)
	s := &StubScript{Routes: []string{"f1", "none"}}
	for seq, want := range []string{"f1", "none"} {
		resp, err := b.Invoke(context.Background(), routeReq(s, seq))
		if err != nil {
			t.Fatalf("seq %d: %v", seq, err)
		}
		if got := parseStubRoute(t, resp.Text); got != want {
			t.Errorf("seq %d: fact = %q, want %q", seq, got, want)
		}
		if resp.Model != NameStub {
			t.Errorf("seq %d: Model = %q, want %q", seq, resp.Model, NameStub)
		}
		if resp.Usage.Source != UsageEstimated {
			t.Errorf("seq %d: Usage.Source = %q, want estimated", seq, resp.Usage.Source)
		}
	}
}

func TestStubRoute_ExhaustedIsError(t *testing.T) {
	b := newTestStub(t)
	s := &StubScript{Routes: []string{"f1"}}
	for _, seq := range []int{1, 5} {
		_, err := b.Invoke(context.Background(), routeReq(s, seq))
		want := "stub.routes[" + string(rune('0'+seq)) + "]"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("seq %d: err = %v, want containing %q", seq, err, want)
		}
	}
	if _, err := b.Invoke(context.Background(), routeReq(s, -1)); err == nil || !strings.Contains(err.Error(), "stub.routes[-1]") {
		t.Errorf("seq -1: err = %v, want containing stub.routes[-1]", err)
	}
}

func TestStubRoute_NoScript(t *testing.T) {
	b := newTestStub(t)
	for name, stub := range map[string]*StubScript{
		"nil stub":     nil,
		"empty routes": {Turns: []StubTurn{{Response: "x"}}},
	} {
		_, err := b.Invoke(context.Background(), routeReq(stub, 0))
		if err == nil || !strings.Contains(err.Error(), "stub.routes[0]") {
			t.Errorf("%s: err = %v, want containing 'stub.routes[0]'", name, err)
		}
	}
}

func TestStubRoute_WithYesNoIsError(t *testing.T) {
	b := newTestStub(t)
	req := routeReq(&StubScript{Routes: []string{"f1"}, Judge: StubJudge{"yes"}}, 0)
	req.YesNo = true
	if _, err := b.Invoke(context.Background(), req); err == nil {
		t.Fatal("Route together with YesNo must be an error")
	}
}

func TestStubRoute_Stateless(t *testing.T) {
	b := newTestStub(t)
	s := &StubScript{Routes: []string{"f1", "none"}}
	ask := func(seq int) string {
		resp, err := b.Invoke(context.Background(), routeReq(s, seq))
		if err != nil {
			t.Fatalf("seq %d: %v", seq, err)
		}
		return parseStubRoute(t, resp.Text)
	}
	// Out of order and repeated: same answers as in order.
	if got := ask(1); got != "none" {
		t.Errorf("seq 1 first = %q, want none", got)
	}
	if got := ask(0); got != "f1" {
		t.Errorf("seq 0 second = %q, want f1", got)
	}
	if got := ask(1); got != "none" {
		t.Errorf("seq 1 again = %q, want none", got)
	}
}

func TestStubRoute_DoesNotTouchJudgeCursor(t *testing.T) {
	b := newTestStub(t)
	s := &StubScript{Routes: []string{"f1"}, Judge: StubJudge{"yes", "no"}}
	if _, err := b.Invoke(context.Background(), routeReq(s, 0)); err != nil {
		t.Fatal(err)
	}
	if s.judgeNext != 0 {
		t.Errorf("route call advanced the yes/no cursor to %d", s.judgeNext)
	}
}

func TestStubRoute_NonRouteJudgeUnchanged(t *testing.T) {
	b := newTestStub(t)
	s := &StubScript{Routes: []string{"f1"}, Judge: StubJudge{"no"}}
	resp, err := b.Invoke(context.Background(), judgeReq(s, false))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != stubJudgeOutput {
		t.Errorf("legacy Text = %q, want legacy output", resp.Text)
	}
	resp, err = b.Invoke(context.Background(), judgeReq(s, true))
	if err != nil {
		t.Fatal(err)
	}
	if got := parseStubAnswer(t, resp.Text); got != "no" {
		t.Errorf("yes/no answer = %q, want no", got)
	}
}

func TestStubScript_RoutesYAML(t *testing.T) {
	var s StubScript
	if err := yaml.Unmarshal([]byte("routes: [f1, none]\n"), &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Routes) != 2 || s.Routes[0] != "f1" || s.Routes[1] != "none" {
		t.Errorf("Routes = %v, want [f1 none]", s.Routes)
	}
}

func TestCallRecordPurposeOmittedWhenEmpty(t *testing.T) {
	b, err := json.Marshal(CallRecord{Role: "judge"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "purpose") {
		t.Errorf("empty Purpose must be omitted: %s", b)
	}
	b, err = json.Marshal(CallRecord{Role: "judge", Purpose: "simulated_user"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"purpose":"simulated_user"`) {
		t.Errorf("Purpose missing: %s", b)
	}
}
