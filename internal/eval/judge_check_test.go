package eval

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/inference"
)

// recordingBackend records every request and answers via resp.
type recordingBackend struct {
	mu   sync.Mutex
	reqs []inference.Request
	resp func(i int, req inference.Request) (inference.Response, error)
}

func (b *recordingBackend) Name() string                { return "recording" }
func (b *recordingBackend) Available() error            { return nil }
func (b *recordingBackend) StartupProbe() time.Duration { return 0 }

func (b *recordingBackend) Invoke(_ context.Context, req inference.Request) (inference.Response, error) {
	b.mu.Lock()
	i := len(b.reqs)
	b.reqs = append(b.reqs, req)
	b.mu.Unlock()
	return b.resp(i, req)
}

const yesReply = "===JSON_START===\n{\"reasoning\": \"looks right\", \"answer\": \"yes\"}\n===JSON_END==="

func yesBackend() *recordingBackend {
	return &recordingBackend{resp: func(int, inference.Request) (inference.Response, error) {
		return inference.Response{
			Text:  yesReply,
			Model: "served-model",
			Usage: inference.Usage{InputTokens: 1000, OutputTokens: 200, Source: inference.UsageReported},
		}, nil
	}}
}

// useRecording installs b as the backend with a pinned judge model and
// restores the default config afterwards.
func useRecording(t *testing.T, b inference.Backend) {
	t.Helper()
	resetConfig()
	t.Cleanup(resetConfig)
	cfg.backend = b
	cfg.pins = &runPins{Judge: "pinned-judge-model"}
}

func captureDebug(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := debugWriter
	debugWriter = &buf
	t.Cleanup(func() { debugWriter = orig })
	return &buf
}

func TestJudgeCheckRequestAndPrompt(t *testing.T) {
	b := yesBackend()
	useRecording(t, b)

	dir := t.TempDir()
	writeEvalFile(t, dir, ".eval/issue-body.md", "UNIQUE-FILE-CONTENT-123")
	stub := &inference.StubScript{Judge: inference.StubJudge{"yes"}}
	tc := TestCase{Name: "c", Input: "UNIQUE-CASE-INPUT", Stub: stub, Checks: []Check{
		{Criterion: "clarity", Type: CheckJudge, Question: "Is UNIQUE-QUESTION satisfied?", Files: []string{".eval/issue-body.md"}},
	}}
	cr := CaseResult{ActualOutput: "UNIQUE-AGENT-OUTPUT", WorkspaceDir: dir}
	scoreCase(checkedRubric(), tc, &cr)

	if len(b.reqs) != 1 {
		t.Fatalf("backend got %d requests, want 1", len(b.reqs))
	}
	req := b.reqs[0]
	if req.Role != inference.RoleJudge || !req.YesNo {
		t.Errorf("Role/YesNo = %v/%v, want judge/true", req.Role, req.YesNo)
	}
	if req.Stub != stub {
		t.Errorf("Stub = %p, want the case's stub %p", req.Stub, stub)
	}
	if req.Model != "pinned-judge-model" {
		t.Errorf("Model = %q, want the pinned judge model", req.Model)
	}
	if req.Timeout != 2*time.Minute {
		t.Errorf("Timeout = %v, want 2m", req.Timeout)
	}
	for _, want := range []string{
		"UNIQUE-QUESTION", "UNIQUE-CASE-INPUT", "UNIQUE-AGENT-OUTPUT", "UNIQUE-FILE-CONTENT-123",
		".eval/issue-body.md", "untrusted", "===JSON_START===", "===JSON_END===",
	} {
		if !strings.Contains(req.Prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, req.Prompt)
		}
	}
}

func TestJudgeCheckAccountingSuccess(t *testing.T) {
	useRecording(t, yesBackend())
	tc := TestCase{Name: "c", Input: "in", Checks: []Check{
		{Criterion: "clarity", Type: CheckJudge, Question: "q1"},
		{Criterion: "clarity", Type: CheckJudge, Question: "q2"},
	}}
	cr := CaseResult{ActualOutput: "out", WorkspaceDir: t.TempDir()}
	scoreCase(checkedRubric(), tc, &cr)

	if len(cr.Calls) != 2 {
		t.Fatalf("Calls = %d, want one per judge check (2)", len(cr.Calls))
	}
	for _, c := range cr.Calls {
		if c.Role != "judge" || c.Criterion != "clarity" || c.Error != "" {
			t.Errorf("record = %+v, want judge/clarity/no error", c)
		}
		if c.InputTokens != 1000 || c.OutputTokens != 200 || c.CostUSD <= 0 || c.Model != "served-model" {
			t.Errorf("record usage/model wrong: %+v", c)
		}
	}
	if cr.JudgeCost.TokensIn != 2000 || cr.JudgeCost.TokensOut != 400 || cr.JudgeCost.EstimatedUSD <= 0 {
		t.Errorf("JudgeCost = %+v, want the sum of both calls", cr.JudgeCost)
	}
	if s := scoreByName(t, cr, "clarity"); s.Score != 2 || s.MaxScore != 2 {
		t.Errorf("clarity = %d/%d, want 2/2", s.Score, s.MaxScore)
	}
}

func TestJudgeCheckParseFailureKeepsRecordZeroCost(t *testing.T) {
	b := &recordingBackend{resp: func(int, inference.Request) (inference.Response, error) {
		return inference.Response{Text: "garbage", Usage: inference.Usage{InputTokens: 500, OutputTokens: 50, Source: inference.UsageReported}}, nil
	}}
	useRecording(t, b)
	tc := TestCase{Name: "c", Checks: []Check{{Criterion: "clarity", Type: CheckJudge, Question: "q"}}}
	cr := CaseResult{ActualOutput: "out", WorkspaceDir: t.TempDir()}
	scoreCase(checkedRubric(), tc, &cr)

	if len(cr.Calls) != 1 || cr.Calls[0].Role != "judge" || cr.Calls[0].Criterion != "clarity" {
		t.Fatalf("Calls = %+v, want one judge record", cr.Calls)
	}
	if cr.Calls[0].InputTokens != 500 || cr.Calls[0].CostUSD <= 0 {
		t.Errorf("record should carry the spent tokens: %+v", cr.Calls[0])
	}
	if cr.JudgeCost != (CostInfo{}) {
		t.Errorf("JudgeCost = %+v, want zero on parse failure", cr.JudgeCost)
	}
	s := scoreByName(t, cr, "clarity")
	if s.Score != 0 || len(s.Checks) != 1 || s.Checks[0].Passed || !strings.Contains(s.Checks[0].Detail, "judge parse error") {
		t.Errorf("score = %+v, want a failed check with a parse error", s)
	}
}

func TestJudgeCheckBackendErrorRecorded(t *testing.T) {
	b := &recordingBackend{resp: func(int, inference.Request) (inference.Response, error) {
		return inference.Response{}, errors.New("boom")
	}}
	useRecording(t, b)
	tc := TestCase{Name: "c", Checks: []Check{{Criterion: "clarity", Type: CheckJudge, Question: "q"}}}
	cr := CaseResult{ActualOutput: "out", WorkspaceDir: t.TempDir()}
	scoreCase(checkedRubric(), tc, &cr)

	if len(cr.Calls) != 1 || cr.Calls[0].Error != "boom" || cr.Calls[0].Model != "pinned-judge-model" {
		t.Fatalf("Calls = %+v, want one record with Error=boom and the pinned model", cr.Calls)
	}
	if cr.JudgeCost != (CostInfo{}) {
		t.Errorf("JudgeCost = %+v, want zero", cr.JudgeCost)
	}
	s := scoreByName(t, cr, "clarity")
	if s.Skipped || s.Score != 0 || !strings.Contains(s.Checks[0].Detail, "judge call failed: boom") {
		t.Errorf("score = %+v, want failed (not skipped) with 'judge call failed: boom'", s)
	}
}

func TestJudgeCheckMixedCriterionScoresOutOfTwoWithoutLegacyJudge(t *testing.T) {
	b := yesBackend()
	useRecording(t, b)
	dir := t.TempDir()
	writeEvalFile(t, dir, "a.txt", "x")
	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "clarity", Type: CheckFileExists, Path: "a.txt"},
		{Criterion: "clarity", Type: CheckJudge, Question: "q"},
	}}
	cr := CaseResult{ActualOutput: "## H\ntext", WorkspaceDir: dir}
	scoreCase(checkedRubric(), tc, &cr)

	s := scoreByName(t, cr, "clarity")
	if s.Score != 2 || s.MaxScore != 2 || !s.Deterministic || s.Skipped {
		t.Errorf("clarity = %+v, want 2/2 deterministic", s)
	}
	if len(b.reqs) != 1 || !b.reqs[0].YesNo {
		t.Fatalf("requests = %+v, want exactly one yes/no judge request and no legacy judge call", b.reqs)
	}
	if len(cr.Calls) != 1 {
		t.Errorf("Calls = %d, want 1", len(cr.Calls))
	}
}

func TestJudgeCheckDebugPrintsPrompt(t *testing.T) {
	useRecording(t, yesBackend())
	cfg.debug = true
	buf := captureDebug(t)

	dir := t.TempDir()
	writeEvalFile(t, dir, "spec.md", "DEBUG-FILE-CONTENT")
	tc := TestCase{Name: "c", Input: "DEBUG-INPUT", Checks: []Check{
		{Criterion: "clarity", Type: CheckJudge, Question: "DEBUG-QUESTION", Files: []string{"spec.md"}},
	}}
	cr := CaseResult{ActualOutput: "DEBUG-OUTPUT", WorkspaceDir: dir}
	scoreCase(checkedRubric(), tc, &cr)

	got := buf.String()
	for _, want := range []string{"Debug: judge prompt", "clarity", "DEBUG-QUESTION", "DEBUG-INPUT", "DEBUG-OUTPUT", "DEBUG-FILE-CONTENT"} {
		if !strings.Contains(got, want) {
			t.Errorf("debug output lacks %q:\n%s", want, got)
		}
	}
}

func TestJudgeCheckDebugOffWritesNothing(t *testing.T) {
	useRecording(t, yesBackend())
	buf := captureDebug(t)

	tc := TestCase{Name: "c", Checks: []Check{{Criterion: "clarity", Type: CheckJudge, Question: "q"}}}
	cr := CaseResult{ActualOutput: "out", WorkspaceDir: t.TempDir()}
	scoreCase(checkedRubric(), tc, &cr)

	if buf.Len() != 0 {
		t.Errorf("debug off but wrote: %q", buf.String())
	}
}

func TestJudgeCheckConfigureCarriesDebug(t *testing.T) {
	t.Cleanup(resetConfig)
	if err := configure(RunOptions{Backend: "stub", Debug: true}); err != nil {
		t.Fatal(err)
	}
	if !cfg.debug {
		t.Error("configure(Debug: true) did not set cfg.debug")
	}
	if err := configure(RunOptions{Backend: "stub"}); err != nil {
		t.Fatal(err)
	}
	if cfg.debug {
		t.Error("configure without Debug left cfg.debug set")
	}
}

func TestJudgeCheckEmptyOutputMakesNoJudgeCall(t *testing.T) {
	b := yesBackend()
	useRecording(t, b)
	tc := TestCase{Name: "c", Checks: []Check{
		{Criterion: "clarity", Type: CheckJudge, Question: "q1"},
		{Criterion: "clarity", Type: CheckJudge, Question: "q2"},
	}}
	cr := CaseResult{ActualOutput: "", WorkspaceDir: t.TempDir()}
	scoreCase(checkedRubric(), tc, &cr)

	if len(b.reqs) != 0 || len(cr.Calls) != 0 {
		t.Errorf("judge called for empty output: reqs=%d calls=%d", len(b.reqs), len(cr.Calls))
	}
	s := scoreByName(t, cr, "clarity")
	if s.Score != 0 || s.MaxScore != 2 || !strings.Contains(s.Reasoning, "checks not run") {
		t.Errorf("clarity = %+v, want 0/2 'checks not run'", s)
	}
}

func TestJudgeCheckPromptOmitsCaseExpectations(t *testing.T) {
	// Only input + output + files + question are shown (spec §2.2).
	b := yesBackend()
	useRecording(t, b)
	tc := TestCase{Name: "c", ExpectedOutput: "SECRET-EXPECTED", Checks: []Check{{Criterion: "clarity", Type: CheckJudge, Question: "q"}}}
	cr := CaseResult{ActualOutput: "out", WorkspaceDir: t.TempDir()}
	scoreCase(checkedRubric(), tc, &cr)
	if len(b.reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(b.reqs))
	}
	if strings.Contains(b.reqs[0].Prompt, "SECRET-EXPECTED") {
		t.Error("prompt leaks ExpectedOutput")
	}
}

func TestLoadCasesStubJudgeValidation(t *testing.T) {
	tests := []struct {
		name, stub string
	}{
		{"empty list", "stub:\n  judge: []\n"},
		{"blank scalar", "stub:\n  judge: \"\"\n"},
		{"blank entry", "stub:\n  judge: [yes, \"  \"]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksEnv(t)
			writeChecksCase(t, "chk", "bad-case.yaml", "name: bad\ninput: x\n"+tt.stub)
			_, err := loadCases("chk")
			if err == nil {
				t.Fatal("want a load error, got nil")
			}
			for _, want := range []string{"bad-case.yaml", "stub.judge"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestLoadCasesStubJudgeValid(t *testing.T) {
	for name, stub := range map[string]string{
		"scalar":   "stub:\n  judge: yes\n",
		"list":     "stub:\n  judge: [yes, no, garbage]\n",
		"no judge": "stub:\n  turns:\n    - response: hi\n",
	} {
		t.Run(name, func(t *testing.T) {
			checksEnv(t)
			writeChecksCase(t, "chk", "ok-case.yaml", "name: ok\ninput: x\n"+stub)
			if _, err := loadCases("chk"); err != nil {
				t.Fatalf("valid stub rejected: %v", err)
			}
		})
	}
}
