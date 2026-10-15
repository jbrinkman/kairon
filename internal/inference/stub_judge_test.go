package inference

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStubJudge_YAML(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		want    []string
		wantErr bool
	}{
		{"scalar", "judge: yes", []string{"yes"}, false},
		{"list", "judge: [yes, no]", []string{"yes", "no"}, false},
		{"block list", "judge:\n  - yes\n  - garbage\n", []string{"yes", "garbage"}, false},
		{"mapping rejected", "judge:\n  a: b\n", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s StubScript
			err := yaml.Unmarshal([]byte(tt.doc), &s)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got Judge=%v", s.Judge)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(s.Judge, ",") != strings.Join(tt.want, ",") || len(s.Judge) != len(tt.want) {
				t.Errorf("Judge = %v, want %v", s.Judge, tt.want)
			}
		})
	}
}

func TestNextJudgeAnswer_CyclesInOrder(t *testing.T) {
	s := &StubScript{Judge: StubJudge{"yes", "no"}}
	for i, want := range []string{"yes", "no", "yes", "no", "yes"} {
		got, ok := s.NextJudgeAnswer()
		if !ok || got != want {
			t.Fatalf("call %d = (%q, %v), want (%q, true)", i, got, ok, want)
		}
	}
}

func TestNextJudgeAnswer_EmptyJudge(t *testing.T) {
	s := &StubScript{}
	if got, ok := s.NextJudgeAnswer(); ok || got != "" {
		t.Errorf("NextJudgeAnswer() = (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestNextJudgeAnswer_ScriptsAreIndependent(t *testing.T) {
	a := &StubScript{Judge: StubJudge{"yes", "no"}}
	b := &StubScript{Judge: StubJudge{"yes", "no"}}
	a.NextJudgeAnswer()
	if got, _ := b.NextJudgeAnswer(); got != "yes" {
		t.Errorf("script b first answer = %q, want yes (cursor must not be shared)", got)
	}
}

func TestNextJudgeAnswer_Concurrent(t *testing.T) {
	s := &StubScript{Judge: StubJudge{"yes", "no"}}
	const n = 100
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[string]int{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, _ := s.NextJudgeAnswer()
			mu.Lock()
			counts[got]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if counts["yes"] != n/2 || counts["no"] != n/2 {
		t.Errorf("counts = %v, want %d of each", counts, n/2)
	}
}

func judgeReq(stub *StubScript, yesNo bool) Request {
	return Request{Role: RoleJudge, Prompt: "q?", Stub: stub, YesNo: yesNo}
}

func TestStub_YesNoJudge_CursorPersistsAcrossInvokes(t *testing.T) {
	b := newTestStub(t)
	s := &StubScript{Judge: StubJudge{"yes", "no"}}
	var answers []string
	for i := 0; i < 4; i++ {
		resp, err := b.Invoke(context.Background(), judgeReq(s, true))
		if err != nil {
			t.Fatal(err)
		}
		answers = append(answers, parseStubAnswer(t, resp.Text))
	}
	if got := strings.Join(answers, ","); got != "yes,no,yes,no" {
		t.Errorf("answers = %s, want yes,no,yes,no", got)
	}
}

// parseStubAnswer extracts and checks the delimited JSON, returning its answer.
func parseStubAnswer(t *testing.T, text string) string {
	t.Helper()
	const start, end = "===JSON_START===", "===JSON_END==="
	i := strings.Index(text, start)
	j := strings.Index(text, end)
	if i < 0 || j < i {
		t.Fatalf("text lacks delimiters: %q", text)
	}
	var v struct {
		Reasoning string `json:"reasoning"`
		Answer    string `json:"answer"`
	}
	if err := json.Unmarshal([]byte(text[i+len(start):j]), &v); err != nil {
		t.Fatalf("bad JSON in %q: %v", text, err)
	}
	if v.Reasoning == "" {
		t.Errorf("reasoning is empty in %q", text)
	}
	return v.Answer
}

func TestStub_YesNoJudge_Normalizes(t *testing.T) {
	b := newTestStub(t)
	for raw, want := range map[string]string{"yes": "yes", "NO": "no", " Yes ": "yes", "no\n": "no"} {
		resp, err := b.Invoke(context.Background(), judgeReq(&StubScript{Judge: StubJudge{raw}}, true))
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if got := parseStubAnswer(t, resp.Text); got != want {
			t.Errorf("%q: answer = %q, want %q", raw, got, want)
		}
		if resp.Model != NameStub {
			t.Errorf("Model = %q, want %q", resp.Model, NameStub)
		}
		if resp.Usage.Source != UsageEstimated {
			t.Errorf("Usage.Source = %q, want estimated", resp.Usage.Source)
		}
	}
}

func TestStub_YesNoJudge_OtherValueVerbatim(t *testing.T) {
	b := newTestStub(t)
	resp, err := b.Invoke(context.Background(), judgeReq(&StubScript{Judge: StubJudge{"garbage"}}, true))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "garbage" {
		t.Errorf("Text = %q, want verbatim garbage", resp.Text)
	}
	if strings.Contains(resp.Text, "===JSON_START===") {
		t.Errorf("garbage must not be wrapped in delimiters: %q", resp.Text)
	}
}

func TestStub_YesNoJudge_NoScript(t *testing.T) {
	b := newTestStub(t)
	for name, stub := range map[string]*StubScript{
		"nil stub":    nil,
		"empty judge": {Turns: []StubTurn{{Response: "x"}}},
	} {
		_, err := b.Invoke(context.Background(), judgeReq(stub, true))
		if err == nil || !strings.Contains(err.Error(), "no stub.judge") {
			t.Errorf("%s: err = %v, want containing 'no stub.judge'", name, err)
		}
	}
}

func TestStub_LegacyJudgeUnchangedWithoutYesNo(t *testing.T) {
	b := newTestStub(t)
	s := &StubScript{Judge: StubJudge{"no"}}
	resp, err := b.Invoke(context.Background(), judgeReq(s, false))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != stubJudgeOutput {
		t.Errorf("Text = %q, want legacy output", resp.Text)
	}
	if s.judgeNext != 0 {
		t.Errorf("legacy judge call advanced the yes/no cursor to %d", s.judgeNext)
	}
}
