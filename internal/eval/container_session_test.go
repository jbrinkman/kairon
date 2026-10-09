package eval

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/inference"
)

// fakeSession is a containerSession standing in for one container kept open
// across the turns of a case.
type fakeSession struct {
	reqs   []inference.Request
	closed []bool // the failed argument of every Close call

	// failTurn, when non-zero, is the 1-based call number of Run that fails.
	failTurn int
}

func (f *fakeSession) Run(req inference.Request) (inference.Response, *ErrorContext, error) {
	f.reqs = append(f.reqs, req)
	n := len(f.reqs)
	if f.failTurn == n {
		return inference.Response{Stderr: "boom"}, &ErrorContext{Stderr: "boom"}, errors.New("exec failed")
	}
	return inference.Response{Text: "R" + string(rune('0'+n))}, nil, nil
}

func (f *fakeSession) Close(failed bool) { f.closed = append(f.closed, failed) }

// useFakeSession routes the session seam to f and fails the test if the
// one-shot container seam is used instead. It returns the number of opens.
func useFakeSession(t *testing.T, f *fakeSession) *[]inference.Request {
	t.Helper()
	opens := &[]inference.Request{}
	origOpen, origInvoke := openContainerSession, invokeInContainer
	openContainerSession = func(req inference.Request, _ *ContainerConfig, _ *caseWorkspace) (containerSession, error) {
		*opens = append(*opens, req)
		return f, nil
	}
	invokeInContainer = func(inference.Request, *ContainerConfig, *caseWorkspace) (inference.Response, *ErrorContext, error) {
		t.Error("one-shot invokeInContainer used for a multi-turn case")
		return inference.Response{}, nil, errors.New("unexpected one-shot")
	}
	t.Cleanup(func() { openContainerSession, invokeInContainer = origOpen, origInvoke })
	return opens
}

// sessionEnv prepares executeCase for container mode with a fake session:
// the case workspace is real (host side), the container is not.
func sessionEnv(t *testing.T) {
	t.Helper()
	execEnv(t, false)
	useAgentConfigs(t)
}

func containerTurnsCase(name string, turns ...string) TestCase {
	tc := TestCase{Name: name, Agent: "selftest", Turns: turns, Timeout: "45s"}
	// A check keeps the criterion off the judge.
	tc.Checks = []Check{{Criterion: "clarity", Type: "output_contains", Pattern: "R"}}
	return tc
}

func TestContainerSessionThreeTurnsOneOpenOneClose(t *testing.T) {
	sessionEnv(t)
	f := &fakeSession{}
	opens := useFakeSession(t, f)

	var out bytes.Buffer
	cr := executeCase(execRubric(), containerTurnsCase("three", "one", "two", "three"), testContainerConfig(), &out, false)

	if len(*opens) != 1 {
		t.Fatalf("opens = %d, want 1\n%s", len(*opens), out.String())
	}
	if len(f.reqs) != 3 {
		t.Fatalf("Run calls = %d, want 3\n%s", len(f.reqs), out.String())
	}
	for i, req := range f.reqs {
		if req.Turn != i {
			t.Errorf("Run %d: Turn = %d, want %d", i, req.Turn, i)
		}
	}
	if len(f.closed) != 1 || f.closed[0] {
		t.Errorf("Close calls = %v, want exactly one with failed=false", f.closed)
	}
	if got := strings.Join(cr.TurnOutputs, "|"); got != "R1|R2|R3" {
		t.Errorf("TurnOutputs = %q, want R1|R2|R3", got)
	}
	if cr.ActualOutput != "R3" {
		t.Errorf("ActualOutput = %q, want R3", cr.ActualOutput)
	}
}

func TestContainerSessionTurnFailureClosesFailedAndStops(t *testing.T) {
	sessionEnv(t)
	f := &fakeSession{failTurn: 2}
	useFakeSession(t, f)

	var out bytes.Buffer
	cr := executeCase(execRubric(), containerTurnsCase("fails", "one", "two", "three"), testContainerConfig(), &out, false)

	if len(f.reqs) != 2 {
		t.Fatalf("Run calls = %d, want 2 (turn 3 must not run)", len(f.reqs))
	}
	if len(f.closed) != 1 || !f.closed[0] {
		t.Errorf("Close calls = %v, want exactly one with failed=true", f.closed)
	}
	if cr.ActualOutput != "" || len(cr.TurnOutputs) != 1 {
		t.Errorf("ActualOutput=%q TurnOutputs=%q, want empty and 1 entry", cr.ActualOutput, cr.TurnOutputs)
	}
	if !strings.Contains(out.String(), "turn 2/3") {
		t.Errorf("output lacks 'turn 2/3':\n%s", out.String())
	}
}

func TestContainerSessionPerTurnRequestSetup(t *testing.T) {
	sessionEnv(t)
	f := &fakeSession{}
	useFakeSession(t, f)

	cConfig := testContainerConfig()
	executeCase(execRubric(), containerTurnsCase("per-turn", "one", "two", "three"), cConfig, &bytes.Buffer{}, false)

	if len(f.reqs) != 3 {
		t.Fatalf("Run calls = %d, want 3", len(f.reqs))
	}
	for i, req := range f.reqs {
		if req.Timeout != 45*time.Second {
			t.Errorf("turn %d: Timeout = %v, want the case timeout 45s", i, req.Timeout)
		}
		if req.WorkDir != cConfig.WorkspaceDir {
			t.Errorf("turn %d: WorkDir = %q, want %q", i, req.WorkDir, cConfig.WorkspaceDir)
		}
		if req.AgentConfigDir != "" {
			t.Errorf("turn %d: AgentConfigDir = %q, want empty (host path)", i, req.AgentConfigDir)
		}
		if req.ToolTrust == nil {
			t.Errorf("turn %d: ToolTrust not resolved", i)
		}
		if want := []string{"one", "two", "three"}[i]; req.Prompt != want {
			t.Errorf("turn %d: Prompt = %q, want %q", i, req.Prompt, want)
		}
	}
}

func TestContainerSessionTrustFailureFailsClosed(t *testing.T) {
	sessionEnv(t)
	f := &fakeSession{}
	useFakeSession(t, f)

	// An agent without a resolvable config must fail the turn, not run it
	// with the container's default trust.
	rubric := execRubric()
	rubric.Agent = "no-such-agent"
	var out bytes.Buffer
	cr := executeCase(rubric, containerTurnsCase("untrusted", "one", "two"), testContainerConfig(), &out, false)

	if len(f.reqs) != 0 {
		t.Errorf("Run calls = %d, want 0 when trust resolution fails", len(f.reqs))
	}
	if cr.ActualOutput != "" || !strings.Contains(out.String(), "tool trust") {
		t.Errorf("want a tool trust failure, got ActualOutput=%q\n%s", cr.ActualOutput, out.String())
	}
	for _, failed := range f.closed {
		if !failed {
			t.Errorf("Close(failed=false) after a failed turn")
		}
	}
}

func TestContainerSessionOneTurnAndClassicUseOneShot(t *testing.T) {
	sessionEnv(t)
	opened := 0
	var oneShot []inference.Request
	origOpen, origInvoke := openContainerSession, invokeInContainer
	openContainerSession = func(inference.Request, *ContainerConfig, *caseWorkspace) (containerSession, error) {
		opened++
		return &fakeSession{}, nil
	}
	invokeInContainer = func(req inference.Request, _ *ContainerConfig, _ *caseWorkspace) (inference.Response, *ErrorContext, error) {
		oneShot = append(oneShot, req)
		return inference.Response{Text: "R"}, nil, nil
	}
	t.Cleanup(func() { openContainerSession, invokeInContainer = origOpen, origInvoke })

	classic := TestCase{Name: "classic", Agent: "selftest", Input: "do it"}
	classic.Checks = []Check{{Criterion: "clarity", Type: "output_contains", Pattern: "R"}}
	oneTurn := containerTurnsCase("one-turn", "only")

	for _, tc := range []TestCase{classic, oneTurn} {
		executeCase(execRubric(), tc, testContainerConfig(), &bytes.Buffer{}, false)
	}
	if opened != 0 {
		t.Errorf("sessions opened = %d, want 0 for one-turn and classic cases", opened)
	}
	if len(oneShot) != 2 {
		t.Errorf("one-shot calls = %d, want 2", len(oneShot))
	}
}

func TestContainerSessionNativeMultiTurnOpensNothing(t *testing.T) {
	execEnv(t, false)
	opened := 0
	orig := openContainerSession
	openContainerSession = func(inference.Request, *ContainerConfig, *caseWorkspace) (containerSession, error) {
		opened++
		return &fakeSession{}, nil
	}
	t.Cleanup(func() { openContainerSession = orig })

	tc := turnsCase("native", []string{"a", "b"}, inference.StubTurn{Response: "R1"}, inference.StubTurn{Response: "R2"})
	tc.Checks = []Check{{Criterion: "clarity", Type: "output_contains", Pattern: "R2"}}
	cr := executeCase(execRubric(), tc, nil, &bytes.Buffer{}, false)
	if opened != 0 || cr.ActualOutput != "R2" {
		t.Errorf("opened=%d ActualOutput=%q, want 0 and R2", opened, cr.ActualOutput)
	}
}

func TestContainerSessionOpenFailureFailsTurnOne(t *testing.T) {
	sessionEnv(t)
	orig := openContainerSession
	openContainerSession = func(inference.Request, *ContainerConfig, *caseWorkspace) (containerSession, error) {
		return nil, errors.New("creating container: no daemon")
	}
	t.Cleanup(func() { openContainerSession = orig })

	var out bytes.Buffer
	cr := executeCase(execRubric(), containerTurnsCase("noopen", "one", "two"), testContainerConfig(), &out, false)
	if cr.ActualOutput != "" || len(cr.TurnOutputs) != 0 || len(cr.Calls) != 1 {
		t.Errorf("ActualOutput=%q TurnOutputs=%q Calls=%d, want empty, none, 1 failed call", cr.ActualOutput, cr.TurnOutputs, len(cr.Calls))
	}
	if !strings.Contains(out.String(), "turn 1/2") || !strings.Contains(out.String(), "no daemon") {
		t.Errorf("output lacks turn 1/2 open error:\n%s", out.String())
	}
}
