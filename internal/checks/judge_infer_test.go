package checks

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/inference"
)

// recordingBackend records every request and answers via resp.
type recordingBackend struct {
	reqs []inference.Request
	resp func(req inference.Request) (inference.Response, error)
}

func (b *recordingBackend) Name() string                { return "recording" }
func (b *recordingBackend) Available() error            { return nil }
func (b *recordingBackend) StartupProbe() time.Duration { return 0 }

func (b *recordingBackend) Invoke(_ context.Context, req inference.Request) (inference.Response, error) {
	b.reqs = append(b.reqs, req)
	return b.resp(req)
}

const testYesReply = "===JSON_START===\n{\"reasoning\": \"looks right\", \"answer\": \"yes\"}\n===JSON_END==="

func replying(text string, err error) *recordingBackend {
	return &recordingBackend{resp: func(inference.Request) (inference.Response, error) {
		return inference.Response{Text: text, Model: "served"}, err
	}}
}

func TestInferenceJudgeRequest(t *testing.T) {
	stub := &inference.StubScript{Judge: inference.StubJudge{"yes"}}
	q := JudgeQuery{Criterion: "crit", Question: "Is it good?", Input: "in", Output: "out"}

	tests := []struct {
		name      string
		judge     func(b *recordingBackend) InferenceJudge
		query     JudgeQuery
		wantModel string
		wantStub  *inference.StubScript
		// wantFull also asserts Role, YesNo, Timeout and Prompt on the request.
		wantFull bool
	}{
		{
			name: "pinned-with-stub",
			judge: func(b *recordingBackend) InferenceJudge {
				return InferenceJudge{Backend: b, Model: "pinned", Stub: stub}
			},
			query:     q,
			wantModel: "pinned",
			wantStub:  stub,
			wantFull:  true,
		},
		{
			name:  "unpinned-no-stub",
			judge: func(b *recordingBackend) InferenceJudge { return InferenceJudge{Backend: b} },
			query: JudgeQuery{Question: "q"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := replying(testYesReply, nil)
			v, err := tt.judge(b).Func()(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			if !v.Yes || v.Reasoning != "looks right" {
				t.Errorf("verdict = %+v", v)
			}
			if len(b.reqs) != 1 {
				t.Fatalf("backend called %d times, want 1", len(b.reqs))
			}
			req := b.reqs[0]
			if req.Model != tt.wantModel || req.Stub != tt.wantStub {
				t.Errorf("request = %+v, want model %q and stub %p", req, tt.wantModel, tt.wantStub)
			}
			if tt.wantFull && (req.Role != inference.RoleJudge || !req.YesNo || req.Timeout != 2*time.Minute ||
				req.Prompt != BuildYesNoPrompt(tt.query)) {
				t.Errorf("request = %+v", req)
			}
		})
	}
}

func TestInferenceJudgeOnCall(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name         string
		text         string
		err          error
		wantErr      func(error) bool
		wantCallErr  bool
		wantParseErr bool
	}{
		{"success", testYesReply, nil, func(e error) bool { return e == nil }, false, false},
		{"backend error", "", boom, func(e error) bool { return errors.Is(e, boom) }, true, false},
		{"parse error", "no verdict here", nil, func(e error) bool { return errors.Is(e, ErrJudgeParse) }, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []JudgeCall
			j := InferenceJudge{Backend: replying(tt.text, tt.err), OnCall: func(c JudgeCall) { calls = append(calls, c) }}
			_, err := j.Func()(JudgeQuery{Criterion: "crit", Question: "q"})
			if !tt.wantErr(err) {
				t.Errorf("err = %v", err)
			}
			if len(calls) != 1 {
				t.Fatalf("OnCall called %d times, want 1", len(calls))
			}
			c := calls[0]
			if c.Criterion != "crit" || c.Request.Role != inference.RoleJudge || (c.Err != nil) != tt.wantCallErr || (c.ParseErr != nil) != tt.wantParseErr {
				t.Errorf("call = %+v", c)
			}
			if c.Wall < 0 {
				t.Errorf("Wall = %v", c.Wall)
			}
			if tt.err == nil && c.Response.Model != "served" {
				t.Errorf("Response.Model = %q", c.Response.Model)
			}
		})
	}
}

func TestInferenceJudgeDebug(t *testing.T) {
	var buf bytes.Buffer
	j := InferenceJudge{Backend: replying(testYesReply, nil), Debug: &buf}.Func()
	q := JudgeQuery{Criterion: "crit", Question: "Is it good?"}
	for range 2 {
		if _, err := j(q); err != nil {
			t.Fatal(err)
		}
	}
	out := buf.String()
	for _, want := range []string{
		"🔧 Debug: judge prompt (criterion=crit, judge check #1)\n",
		"🔧 Debug: judge prompt (criterion=crit, judge check #2)\n",
		"QUESTION: Is it good?",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("debug output missing %q:\n%s", want, out)
		}
	}
}
