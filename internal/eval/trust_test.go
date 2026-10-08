package eval

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jbrinkman/kairon/internal/inference"
)

func writeAgentConfig(t *testing.T, dir, agent, body string) {
	t.Helper()
	writeCfgFile(t, filepath.Join(dir, "agents", agent+".json"), body)
}

func TestResolveTrustSetPrecedence(t *testing.T) {
	chdirTemp(t)
	dir := t.TempDir()
	cfg.evalsDir = dir
	writeAgentConfig(t, dir, "builder", `{"allowedTools":["read","write","shell"]}`)
	writeAgentConfig(t, dir, "noTools", `{"model":"m"}`)

	t.Run("allowedTools", func(t *testing.T) {
		got, err := resolveTrustSet("builder")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"fs_read", "fs_write", "execute_bash"}; !reflect.DeepEqual(got.Tools, want) {
			t.Errorf("tools = %v, want %v", got.Tools, want)
		}
	})

	t.Run("override beats allowedTools", func(t *testing.T) {
		cfg.pins = &runPins{TrustOverrides: map[string][]string{"builder": {"read"}}}
		t.Cleanup(func() { cfg.pins = nil })
		got, err := resolveTrustSet("builder")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"fs_read"}; !reflect.DeepEqual(got.Tools, want) {
			t.Errorf("tools = %v, want %v", got.Tools, want)
		}
	})

	t.Run("empty override trusts nothing", func(t *testing.T) {
		cfg.pins = &runPins{TrustOverrides: map[string][]string{"builder": {}}}
		t.Cleanup(func() { cfg.pins = nil })
		got, err := resolveTrustSet("builder")
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || len(got.Tools) != 0 {
			t.Errorf("got %+v, want non-nil empty set", got)
		}
	})

	t.Run("no allowedTools is an empty non-nil set", func(t *testing.T) {
		got, err := resolveTrustSet("noTools")
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || len(got.Tools) != 0 {
			t.Errorf("got %+v, want non-nil empty set", got)
		}
	})
}

func TestResolveTrustSetMissingConfigListsPaths(t *testing.T) {
	chdirTemp(t)
	cfg.evalsDir = t.TempDir()
	_, err := resolveTrustSet("ghost")
	if err == nil {
		t.Fatal("want error for a missing agent config")
	}
	for _, want := range []string{"ghost", filepath.Join(cfg.evalsDir, "agents", "ghost.json"), filepath.Join(".kiro", "agents", "ghost.json")} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestResolveTrustSetFallsBackToRepoAgents(t *testing.T) {
	chdirTemp(t)
	cfg.evalsDir = t.TempDir()
	writeCfgFile(t, filepath.Join(".kiro", "agents", "documenter.json"), `{"allowedTools":["read","write"]}`)
	got, err := resolveTrustSet("documenter")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"fs_read", "fs_write"}; !reflect.DeepEqual(got.Tools, want) {
		t.Errorf("tools = %v, want %v", got.Tools, want)
	}
}

func TestResolveTrustSetBadJSON(t *testing.T) {
	chdirTemp(t)
	cfg.evalsDir = t.TempDir()
	writeAgentConfig(t, cfg.evalsDir, "broken", `{not json`)
	if _, err := resolveTrustSet("broken"); err == nil {
		t.Fatal("want parse error")
	}
}

func TestContainerInvokeAgentTrustsAllowedToolsOnly(t *testing.T) {
	chdirTemp(t)
	useAgentConfigs(t)
	x := &fakeExecer{kiroStdout: "ok"}
	useFakeContainer(t, x)

	_, _, rec, _, err := invokeAgent("builder", "p", testContainerConfig(), callOpts{})
	if err != nil {
		t.Fatal(err)
	}
	argv := strings.Join(x.cmds[0], " ")
	if !strings.Contains(argv, "--trust-tools=fs_read,fs_write") || strings.Contains(argv, "--trust-all-tools") {
		t.Errorf("argv = %q", argv)
	}
	if rec.TrustedTools == nil || !reflect.DeepEqual(*rec.TrustedTools, []string{"fs_read", "fs_write"}) {
		t.Errorf("TrustedTools = %v", rec.TrustedTools)
	}
}

func TestContainerInvokeAgentUsesOverride(t *testing.T) {
	chdirTemp(t)
	useAgentConfigs(t)
	cfg.pins = &runPins{TrustOverrides: map[string][]string{"builder": {}}}
	x := &fakeExecer{kiroStdout: "ok"}
	useFakeContainer(t, x)

	_, _, rec, _, err := invokeAgent("builder", "p", testContainerConfig(), callOpts{})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, a := range x.cmds[0] {
		found = found || a == "--trust-tools="
	}
	if !found {
		t.Errorf("argv = %v, want a bare --trust-tools=", x.cmds[0])
	}
	if rec.TrustedTools == nil || len(*rec.TrustedTools) != 0 {
		t.Errorf("TrustedTools = %v, want restricted-to-nothing", rec.TrustedTools)
	}
}

func TestContainerInvokeAgentTrustResolutionErrorFailsCall(t *testing.T) {
	chdirTemp(t)
	cfg.evalsDir = t.TempDir()
	x := &fakeExecer{kiroStdout: "ok"}
	useFakeContainer(t, x)

	_, _, rec, _, err := invokeAgent("ghost", "p", testContainerConfig(), callOpts{})
	if err == nil || !strings.HasPrefix(err.Error(), "tool trust: ") {
		t.Fatalf("err = %v, want tool trust error", err)
	}
	if len(x.cmds) != 0 {
		t.Errorf("agent ran despite trust failure: %v", x.cmds)
	}
	if rec.Error == "" {
		t.Error("failed call must still be recorded")
	}
}

func TestNewCallRecordCopiesTrustAndDenials(t *testing.T) {
	req := inference.Request{Role: inference.RoleAgent, ToolTrust: inference.NewToolTrust([]string{"read"})}
	resp := inference.Response{ToolDenials: []inference.ToolDenial{{Tool: "fs_write", Reason: inference.ReasonToolNotTrusted}}}
	rec := newCallRecord(req, resp, nil, 0, CostInfo{})
	if rec.TrustedTools == nil || !reflect.DeepEqual(*rec.TrustedTools, []string{"fs_read"}) {
		t.Errorf("TrustedTools = %v", rec.TrustedTools)
	}
	if !reflect.DeepEqual(rec.ToolDenials, resp.ToolDenials) {
		t.Errorf("ToolDenials = %v", rec.ToolDenials)
	}

	native := newCallRecord(inference.Request{Role: inference.RoleAgent}, inference.Response{}, nil, 0, CostInfo{})
	if native.TrustedTools != nil || native.ToolDenials != nil {
		t.Errorf("native record must omit trust: %+v", native)
	}
}

func TestPinRunCarriesTrustOverrides(t *testing.T) {
	setupPinProject(t, "", "")
	writeProjectFile(t, ".kairon/config.yaml", "evals:\n  trust_tools:\n    architect: [read]\n")
	if err := configure(RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := pinRun("architect", RunOptions{}, false); err != nil {
		t.Fatal(err)
	}
	got, ok := cfg.pins.trustOverride("architect")
	if !ok || !reflect.DeepEqual(got, []string{"read"}) {
		t.Errorf("override = %v, %v", got, ok)
	}
	if _, ok := cfg.pins.trustOverride("builder"); ok {
		t.Error("builder must have no override")
	}
	var nilPins *runPins
	if _, ok := nilPins.trustOverride("architect"); ok {
		t.Error("nil pins must have no override")
	}
}
