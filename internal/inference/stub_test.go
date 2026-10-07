package inference

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func newTestStub(t *testing.T) Backend {
	t.Helper()
	b, err := New("stub")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStub_AvailabilityAndProbe(t *testing.T) {
	// An empty PATH proves the stub never consults or spawns anything.
	t.Setenv("PATH", "")
	b := newTestStub(t)
	if err := b.Available(); err != nil {
		t.Errorf("Available() = %v, want nil", err)
	}
	if d := b.StartupProbe(); d != 0 {
		t.Errorf("StartupProbe() = %v, want 0", d)
	}
}

func TestStub_AgentReturnsScriptedResponseEstimated(t *testing.T) {
	t.Setenv("PATH", "")
	b := newTestStub(t)
	resp, err := b.Invoke(context.Background(), Request{
		Role:   RoleAgent,
		Agent:  "a",
		Prompt: strings.Repeat("p", 80),
		Stub:   &StubScript{Turns: []StubTurn{{Response: strings.Repeat("r", 40)}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != strings.Repeat("r", 40) {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.Usage.Source != UsageEstimated || resp.Usage.InputTokens != 20 || resp.Usage.OutputTokens != 10 {
		t.Errorf("Usage = %+v", resp.Usage)
	}
	if resp.Model != "" {
		t.Errorf("Model = %q, want empty", resp.Model)
	}
}

func TestStub_AgentReportedUsageAndModel(t *testing.T) {
	b := newTestStub(t)
	resp, err := b.Invoke(context.Background(), Request{
		Role:   RoleAgent,
		Prompt: "prompt",
		Stub: &StubScript{Turns: []StubTurn{{
			Response: "hello",
			Model:    "stub-model",
			Usage:    &StubUsage{InputTokens: 123, OutputTokens: 45},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := Usage{InputTokens: 123, OutputTokens: 45, Source: UsageReported}
	if resp.Usage != want {
		t.Errorf("Usage = %+v, want %+v", resp.Usage, want)
	}
	if resp.Model != "stub-model" {
		t.Errorf("Model = %q", resp.Model)
	}
}

func TestStub_AgentMissingTurn(t *testing.T) {
	b := newTestStub(t)
	cases := map[string]Request{
		"nil stub":     {Role: RoleAgent},
		"empty turns":  {Role: RoleAgent, Stub: &StubScript{}},
		"out of range": {Role: RoleAgent, Turn: 1, Stub: &StubScript{Turns: []StubTurn{{Response: "x"}}}},
		"negative":     {Role: RoleAgent, Turn: -1, Stub: &StubScript{Turns: []StubTurn{{Response: "x"}}}},
		"empty resp":   {Role: RoleAgent, Stub: &StubScript{Turns: []StubTurn{{}}}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			resp, err := b.Invoke(context.Background(), req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "case has no stub.turns[") {
				t.Errorf("unclear error: %v", err)
			}
			if resp.Text != "" {
				t.Errorf("Text = %q, want empty", resp.Text)
			}
		})
	}
	// Exact message for the common turn-0 case.
	_, err := b.Invoke(context.Background(), Request{Role: RoleAgent})
	if err.Error() != "case has no stub.turns[0].response" {
		t.Errorf("error = %q", err.Error())
	}
}

func TestStub_JudgeReturnsPassingDelimitedJSON(t *testing.T) {
	b := newTestStub(t)
	resp, err := b.Invoke(context.Background(), Request{Role: RoleJudge, Prompt: "judge me"})
	if err != nil {
		t.Fatal(err)
	}
	const startTag, endTag = "===JSON_START===", "===JSON_END==="
	s, e := strings.Index(resp.Text, startTag), strings.Index(resp.Text, endTag)
	if s == -1 || e == -1 || e <= s {
		t.Fatalf("delimiters not found in %q", resp.Text)
	}
	var j struct {
		Score     int    `json:"score"`
		Reasoning string `json:"reasoning"`
		Pass      bool   `json:"pass"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Text[s+len(startTag):e])), &j); err != nil {
		t.Fatalf("judge JSON invalid: %v", err)
	}
	if j.Score != 5 || !j.Pass {
		t.Errorf("judgment = %+v, want score 5 pass true", j)
	}
	if resp.Model != "stub" {
		t.Errorf("Model = %q", resp.Model)
	}
	if resp.Usage.Source != UsageEstimated {
		t.Errorf("Usage.Source = %q", resp.Usage.Source)
	}
}

func TestStub_UnsupportedRole(t *testing.T) {
	if _, err := newTestStub(t).Invoke(context.Background(), Request{Role: "weird"}); err == nil {
		t.Error("expected error for unsupported role")
	}
}

func TestStub_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newTestStub(t).Invoke(ctx, Request{Role: RoleJudge}); err == nil {
		t.Error("expected context error")
	}
}

func TestStub_IgnoresRequestModel(t *testing.T) {
	b := newTestStub(t)

	agent, err := b.Invoke(context.Background(), Request{
		Role: RoleAgent, Prompt: "p", Model: "claude-sonnet-5.5",
		Stub: &StubScript{Turns: []StubTurn{{Response: "r", Model: "stub-model"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Model != "stub-model" {
		t.Errorf("agent Model = %q, want scripted %q", agent.Model, "stub-model")
	}

	agentNoScriptModel, err := b.Invoke(context.Background(), Request{
		Role: RoleAgent, Prompt: "p", Model: "claude-sonnet-5.5",
		Stub: &StubScript{Turns: []StubTurn{{Response: "r"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if agentNoScriptModel.Model != "" {
		t.Errorf("agent Model = %q, want empty (Request.Model must be ignored)", agentNoScriptModel.Model)
	}

	judge, err := b.Invoke(context.Background(), Request{Role: RoleJudge, Prompt: "p", Model: "claude-sonnet-5.5"})
	if err != nil {
		t.Fatal(err)
	}
	if judge.Model != "stub" {
		t.Errorf("judge Model = %q, want %q", judge.Model, "stub")
	}
}
