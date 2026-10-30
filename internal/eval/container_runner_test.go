package eval

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/mount"
	"github.com/jbrinkman/kairon/internal/eval/sandbox"
	"github.com/jbrinkman/kairon/internal/inference"
)

// testWorkspaceDirs fabricates the host-side layout of a caseWorkspace
// without needing git.
func testWorkspaceDirs(t *testing.T) *caseWorkspace {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ws := &caseWorkspace{Dir: root, EvalDir: filepath.Join(root, ".eval"), KiroDir: filepath.Join(root, ".kiro"), BinDir: filepath.Join(root, ".bin")}
	for _, d := range []string{ws.EvalDir, ws.KiroDir, ws.BinDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return ws
}

// The runner must not install or copy anything into a running container.
// This source-level guard fails if the removed calls are reintroduced.
func TestRunnerHasNoRuntimeInstalls(t *testing.T) {
	src, err := os.ReadFile("runner.go")
	if err != nil {
		t.Fatal(err)
	}
	// Names are assembled from parts so that the repo-wide removal check for
	// the deleted legacy identifiers keeps finding no occurrence in Go files.
	bannedCalls := []string{"Setup" + "GitHubMocking", "Configure" + "MockGitHubPath", ".Copy" + "To("}
	for _, call := range bannedCalls {
		if strings.Contains(string(src), call) {
			t.Errorf("runner.go must not contain %q: nothing may be copied or installed into a running container", call)
		}
	}
}

func TestAgentExecerOnlyExecs(t *testing.T) {
	typ := reflect.TypeOf((*agentExecer)(nil)).Elem()
	if typ.NumMethod() != 1 || typ.Method(0).Name != "ExecWithStdin" {
		t.Fatalf("agentExecer must have only ExecWithStdin, has %d methods", typ.NumMethod())
	}
	// *sandbox.Container must keep satisfying it.
	var _ agentExecer = (*sandbox.Container)(nil)
}

func mountSummary(ms []sandbox.Mount) map[string]sandbox.Mount {
	out := map[string]sandbox.Mount{}
	for _, m := range ms {
		out[m.ContainerPath] = m
	}
	return out
}

// buildContainerMounts ignores its backend argument and returns the same
// ordered list for every backend, so one table over backend and workspace
// covers kiro-cli and the helper backends, default and custom workspaces.
func TestBuildContainerMountsKiroCLI(t *testing.T) {
	if containerHelperPath != "/usr/local/bin/kairon" {
		t.Errorf("containerHelperPath = %q, want /usr/local/bin/kairon", containerHelperPath)
	}
	for _, tc := range []struct {
		name         string
		backend      string
		workspaceDir string
	}{
		{"kiro-cli-default-workspace", inference.NameKiroCLI, "/workspace"},
		{"stub-default-workspace", inference.NameStub, "/workspace"},
		{"stub-custom-workspace", inference.NameStub, "/work/space"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := testWorkspaceDirs(t)
			cc := testContainerConfig()
			cc.WorkspaceDir = tc.workspaceDir
			bin := useFakeLinuxBinary(t)

			ms, err := buildContainerMounts(ws, cc, tc.backend)
			if err != nil {
				t.Fatal(err)
			}
			if len(ms) != 5 {
				t.Fatalf("mounts = %+v, want exactly 5 (the kairon binary is mounted for kiro-cli too)", ms)
			}
			want := []sandbox.Mount{
				{HostPath: ws.KiroDir, ContainerPath: path.Join(tc.workspaceDir, ".kiro"), ReadOnly: true},
				{HostPath: ws.Dir, ContainerPath: tc.workspaceDir},
				{HostPath: ws.EvalDir, ContainerPath: path.Join(tc.workspaceDir, ".eval")},
				{HostPath: ws.BinDir, ContainerPath: "/opt/kairon/bin", ReadOnly: true},
				{HostPath: bin, ContainerPath: "/usr/local/bin/kairon", ReadOnly: true},
			}
			if !reflect.DeepEqual(ms, want) {
				t.Errorf("mounts = %+v\nwant     %+v", ms, want)
			}
			if _, ok := mountSummary(ms)["/opt/kairon/kairon"]; ok {
				t.Error("the old /opt/kairon/kairon mount must not exist")
			}

			// The kairon binary is mounted read-only, exactly once, at a path
			// whose directory is on the container PATH, for every backend
			// (kiro-cli included), so a bare `kairon` resolves for agents.
			if kaironBinaryMounted {
				n := 0
				for _, m := range ms {
					if m.ContainerPath == containerHelperPath {
						n++
						if m.HostPath != bin || !m.ReadOnly {
							t.Errorf("kairon mount = %+v, want read-only %s", m, bin)
						}
					}
				}
				if n != 1 {
					t.Errorf("%d mounts at %s, want exactly 1: %+v", n, containerHelperPath, ms)
				}
				dir := path.Dir(containerHelperPath)
				onPath := false
				for _, d := range strings.Split(containerPath, ":") {
					if d == dir {
						onPath = true
					}
				}
				if !onPath {
					t.Errorf("%s is not a directory on containerPath %q", dir, containerPath)
				}
			}

			// Through the real host-config constructor: bind mounts only, plus the
			// read-only-rootfs tmpfs mounts for temp and $HOME (never the workspace).
			hc, err := sandbox.NewHostConfigWithMounts(cc.ResourceLimits, ms)
			if err != nil {
				t.Fatal(err)
			}
			if !hc.ReadonlyRootfs {
				t.Error("root filesystem must be read-only")
			}
			for _, p := range []string{"/tmp", "/var/tmp", "/home/sandbox"} {
				if _, ok := hc.Tmpfs[p]; !ok {
					t.Errorf("missing tmpfs %s: %v", p, hc.Tmpfs)
				}
			}
			if len(hc.Tmpfs) != 3 {
				t.Errorf("unexpected tmpfs: %v", hc.Tmpfs)
			}
			if _, ok := hc.Tmpfs[cc.WorkspaceDir]; ok {
				t.Errorf("workspace %s must not be a tmpfs", cc.WorkspaceDir)
			}
			for _, m := range hc.Mounts {
				if m.Type != mount.TypeBind {
					t.Errorf("mount %s is %s, want bind", m.Target, m.Type)
				}
			}
		})
	}
}

// prepareBaseImage resolves the kairon binary first, so a missing binary
// fails before any image manager is created or any case starts.
func TestPrepareBaseImageFailsFastWithoutKaironBinary(t *testing.T) {
	orig := resolveLinuxBinary
	var gotPlatform string
	resolveLinuxBinary = func(p string) (string, error) {
		gotPlatform = p
		return "", errors.New("no binary: set KAIRON_SANDBOX_BINARY")
	}
	t.Cleanup(func() { resolveLinuxBinary = orig })

	cc := testContainerConfig()
	cc.Platform = "linux/arm64"
	im, err := prepareBaseImage(cc)
	if err == nil {
		t.Fatal("expected an error when the kairon binary cannot be resolved")
	}
	if im != nil {
		t.Errorf("image manager = %v, want nil (must not be created)", im)
	}
	for _, want := range []string{"KAIRON_SANDBOX_BINARY", "linux/arm64"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to contain %q", err, want)
		}
	}
	if gotPlatform != "linux/arm64" {
		t.Errorf("resolver platform = %q, want linux/arm64", gotPlatform)
	}
	if cc.ImageManager != nil || cc.CachedImageName != "" {
		t.Errorf("config must be untouched on failure: %+v", cc)
	}
}

func TestPrepareBaseImageNativeRunSkipsPreflight(t *testing.T) {
	orig := resolveLinuxBinary
	resolveLinuxBinary = func(string) (string, error) {
		t.Error("native run must not resolve the kairon binary")
		return "", errors.New("unexpected")
	}
	t.Cleanup(func() { resolveLinuxBinary = orig })
	im, err := prepareBaseImage(nil)
	if im != nil || err != nil {
		t.Errorf("prepareBaseImage(nil) = %v, %v; want nil, nil", im, err)
	}
}

func TestBuildContainerMountsErrors(t *testing.T) {
	ws := testWorkspaceDirs(t)
	cc := testContainerConfig()

	if _, err := buildContainerMounts(nil, cc, inference.NameKiroCLI); err == nil {
		t.Error("nil workspace must be an error")
	}

	t.Run("no linux binary", func(t *testing.T) {
		orig := resolveLinuxBinary
		resolveLinuxBinary = func(string) (string, error) { return "", errors.New("set KAIRON_SANDBOX_BINARY") }
		t.Cleanup(func() { resolveLinuxBinary = orig })
		_, err := buildContainerMounts(ws, cc, inference.NameStub)
		if err == nil || !strings.Contains(err.Error(), "KAIRON_SANDBOX_BINARY") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("binary not readable by the container user", func(t *testing.T) {
		bin := filepath.Join(t.TempDir(), "kairon-linux")
		if err := os.WriteFile(bin, []byte("x"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(bin, 0o700); err != nil {
			t.Fatal(err)
		}
		orig := resolveLinuxBinary
		resolveLinuxBinary = func(string) (string, error) { return bin, nil }
		t.Cleanup(func() { resolveLinuxBinary = orig })
		for _, backend := range []string{inference.NameStub, inference.NameKiroCLI} {
			_, err := buildContainerMounts(ws, cc, backend)
			if err == nil || !strings.Contains(err.Error(), bin) {
				t.Fatalf("%s: err = %v, want one naming %s", backend, err, bin)
			}
			if strings.Contains(err.Error(), "helper") {
				t.Errorf("%s: err = %v, want backend-neutral wording (no \"helper\")", backend, err)
			}
		}
	})
}

func TestContainerConfigUserEnvAndWorkdir(t *testing.T) {
	t.Run("default-workspace", func(t *testing.T) {
		cc := testContainerConfig()
		cc.Environment = map[string]string{"B": "2", "A": "1"}
		cfgC := newContainerConfig("kairon-eval-base:x", cc)

		if cfgC.Image != "kairon-eval-base:x" {
			t.Errorf("image = %q", cfgC.Image)
		}
		if cfgC.User != "sandbox" {
			t.Errorf("User = %q, want sandbox", cfgC.User)
		}
		if cfgC.WorkingDir != "/workspace" {
			t.Errorf("WorkingDir = %q", cfgC.WorkingDir)
		}
		env := strings.Join(cfgC.Env, "\n")
		for _, want := range []string{
			"HOME=/home/sandbox",
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=safe.directory",
			"GIT_CONFIG_VALUE_0=*",
			"KIRO_CLI_DISABLE_TELEMETRY=1",
			"A=1",
			"B=2",
			"KAIRON_EVAL_DIR=/workspace/.eval",
			"GH_PROMPT_DISABLED=1",
			"GH_NO_UPDATE_NOTIFIER=1",
		} {
			if !strings.Contains("\n"+env+"\n", "\n"+want+"\n") {
				t.Errorf("env missing %q: %v", want, cfgC.Env)
			}
		}
		// Deterministic order for the configured variables.
		if strings.Index(env, "A=1") > strings.Index(env, "B=2") {
			t.Errorf("configured env not sorted: %v", cfgC.Env)
		}
		p, ok := envValue(cfgC.Env, "PATH")
		if !ok || !strings.HasPrefix(p, "/opt/kairon/bin:") {
			t.Errorf("PATH = %q, want it to start with /opt/kairon/bin:", p)
		}
	})

	t.Run("custom-workspace", func(t *testing.T) {
		cc := testContainerConfig()
		cc.WorkspaceDir = "/work/space"
		if v, _ := envValue(containerEnv(cc), "KAIRON_EVAL_DIR"); v != "/work/space/.eval" {
			t.Errorf("KAIRON_EVAL_DIR with custom workspace = %q", v)
		}
	})
}

// TestContainerConfigKeepAliveIsEffectivelyUnbounded pins the F2 fix: the
// container's keep-alive command must not impose a ~1h wall. A multi-turn
// case opens one container for the whole case (one Run per turn), so a fixed
// `sleep 3600` could exit mid-case even though no single per-turn timeout was
// exceeded. The container is always torn down explicitly by session.Close, so
// the keep-alive just has to outlast any real case. BusyBox sleep (alpine base
// image) takes a numeric argument only — `sleep infinity` is rejected — so the
// value is a large integer.
func TestContainerConfigKeepAliveIsEffectivelyUnbounded(t *testing.T) {
	cfgC := newContainerConfig("kairon-eval-base:x", testContainerConfig())

	if len(cfgC.Cmd) != 2 || cfgC.Cmd[0] != "sleep" {
		t.Fatalf("Cmd = %v, want [sleep <seconds>]", cfgC.Cmd)
	}
	secs, err := strconv.Atoi(cfgC.Cmd[1])
	if err != nil {
		t.Fatalf("keep-alive sleep argument %q is not a plain integer (BusyBox sleep needs one): %v", cfgC.Cmd[1], err)
	}
	// Must dwarf any plausible sum-of-per-turn-timeouts case. The old fixed
	// 3600 (one hour) is the regression this guards against.
	if secs < 2147483647 {
		t.Errorf("keep-alive sleep = %d s, want an effectively-unbounded value (>= 2147483647); a short wall can exit a long multi-turn case mid-run", secs)
	}
}

func TestContainerExecsAreWrappedAndPromptStaysOnStdin(t *testing.T) {
	t.Run("kiro-cli-exec", func(t *testing.T) {
		chdirTemp(t)
		useAgentConfigs(t)
		pinAgentModel("builder", "m1")
		prompt := nastyPrompt()
		x := &fakeExecer{kiroStdout: "\x1b[1mhello\x1b[0m"}
		useFakeContainer(t, x)

		out, cost, rec, ec, err := invokeAgent("builder", prompt, testContainerConfig(), callOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if out != "hello" {
			t.Errorf("output = %q, want ANSI stripped", out)
		}
		if len(x.rawCmds) != 1 {
			t.Fatalf("execs = %d", len(x.rawCmds))
		}
		raw := x.rawCmds[0]
		if len(raw) < 6 || raw[0] != "sh" || raw[1] != "-c" || raw[2] != `umask 000; exec "$@"` || raw[3] != "kairon-exec" {
			t.Fatalf("exec not wrapped with the open umask: %q", raw[:min(len(raw), 5)])
		}
		if raw[4] != "kiro-cli" {
			t.Errorf("wrapped program = %q", raw[4])
		}
		assertNoPromptInArgv(t, x.rawCmds)
		assertNoPromptInArgv(t, x.cmds)
		if len(x.stdins) != 1 || !strings.Contains(x.stdins[0], "PROMPT-MARKER") {
			t.Fatal("prompt must be on stdin")
		}
		if x.stdins[0] != prompt {
			t.Fatalf("stdin did not carry the exact prompt (got %d bytes)", len(x.stdins[0]))
		}
		if ec != nil {
			t.Errorf("ec = %+v", ec)
		}
		if cost.UsageSource != "estimated" || cost.Model != "m1" || cost.TokensIn != len(prompt)/4 {
			t.Errorf("cost = %+v", cost)
		}
		if rec.Model != "m1" || !rec.Estimated || rec.Role != "agent" {
			t.Errorf("rec = %+v", rec)
		}
	})

	// The helper exec is wrapped too.
	t.Run("helper-exec", func(t *testing.T) {
		chdirTemp(t)
		useStubBackend(t)
		useAgentConfigs(t)
		useFakeLinuxBinary(t)
		prompt := nastyPrompt()
		h := &fakeExecer{}
		useFakeContainer(t, h)
		stub := &inference.StubScript{Turns: []inference.StubTurn{{Response: "scripted"}}}
		out, _, _, _, err := invokeAgent("selftest", prompt, testContainerConfig(), callOpts{Stub: stub})
		if err != nil {
			t.Fatal(err)
		}
		if out != "scripted" {
			t.Errorf("output = %q", out)
		}
		if _, ok := unwrapOpenUmask(h.rawCmds[0]); !ok {
			t.Errorf("helper exec not wrapped: %q", h.rawCmds[0])
		}
		assertNoPromptInArgv(t, h.cmds)
		assertNoPromptInArgv(t, h.rawCmds)
		if len(h.stdins) != 1 || !strings.Contains(h.stdins[0], "PROMPT-MARKER") {
			t.Errorf("the JSON request on stdin must carry the prompt")
		}
		wantCmd := "/usr/local/bin/kairon inference-exec --backend stub"
		if got := strings.Join(h.cmds[0], " "); got != wantCmd {
			t.Errorf("cmd = %q, want %q", got, wantCmd)
		}
	})
}

func TestContainerHostExecDeadline(t *testing.T) {
	near := func(t *testing.T, got, want time.Duration) {
		t.Helper()
		if got > want || got < want-3*time.Second {
			t.Errorf("deadline = %v, want just under %v", got, want)
		}
	}

	t.Run("kiro-cli deadline equals the timeout", func(t *testing.T) {
		chdirTemp(t)
		useAgentConfigs(t)
		x := &fakeExecer{kiroStdout: "ok"}
		useFakeContainer(t, x)
		cc := testContainerConfig()
		cc.ResourceLimits.Timeout = 5 * time.Minute
		if _, _, _, _, err := invokeAgent("builder", "p", cc, callOpts{Timeout: 20 * time.Second}); err != nil {
			t.Fatal(err)
		}
		near(t, x.deadlines[0], 20*time.Second)
	})

	t.Run("kiro-cli without a case timeout uses the sandbox limit", func(t *testing.T) {
		chdirTemp(t)
		useAgentConfigs(t)
		x := &fakeExecer{kiroStdout: "ok"}
		useFakeContainer(t, x)
		cc := testContainerConfig()
		cc.ResourceLimits.Timeout = 40 * time.Second
		if _, _, _, _, err := invokeAgent("builder", "p", cc, callOpts{}); err != nil {
			t.Fatal(err)
		}
		near(t, x.deadlines[0], 40*time.Second)
	})

	t.Run("helper deadline is the timeout plus 10s and the request carries the timeout", func(t *testing.T) {
		chdirTemp(t)
		useStubBackend(t)
		useAgentConfigs(t)
		useFakeLinuxBinary(t)
		x := &fakeExecer{}
		useFakeContainer(t, x)
		stub := &inference.StubScript{Turns: []inference.StubTurn{{Response: "r"}}}
		if _, _, _, _, err := invokeAgent("selftest", "p", testContainerConfig(), callOpts{Stub: stub, Timeout: 20 * time.Second}); err != nil {
			t.Fatal(err)
		}
		near(t, x.deadlines[0], 30*time.Second)

		var sent inference.Request
		if err := json.Unmarshal([]byte(x.stdins[0]), &sent); err != nil {
			t.Fatal(err)
		}
		if sent.Timeout != 20*time.Second {
			t.Errorf("in-container timeout = %v, want the effective 20s (not the host deadline)", sent.Timeout)
		}
	})
}

func TestContainerTimeoutNamesEffectiveTimeout(t *testing.T) {
	t.Run("kiro-cli host deadline", func(t *testing.T) {
		chdirTemp(t)
		useAgentConfigs(t)
		cc := testContainerConfig()
		cc.ResourceLimits.Timeout = 5 * time.Minute // the sandbox default must not be named
		useFakeContainer(t, &fakeExecer{block: true})

		start := time.Now()
		_, _, _, ec, err := invokeAgent("builder", "p", cc, callOpts{Timeout: time.Second})
		if err == nil {
			t.Fatal("expected a timeout")
		}
		if !errors.Is(err, inference.ErrTimeout) {
			t.Errorf("err does not satisfy ErrTimeout: %v", err)
		}
		if !strings.Contains(err.Error(), "timeout after 1s") || strings.Contains(err.Error(), "5m0s") {
			t.Errorf("message = %q, want it to name 1s and not the sandbox default", err.Error())
		}
		if ec == nil || !strings.HasPrefix(ec.Stderr, "timeout after 1s") {
			t.Errorf("ec = %+v", ec)
		}
		if time.Since(start) > 3*time.Second {
			t.Errorf("took %v, want about 1s", time.Since(start))
		}
	})

	t.Run("helper-reported timeout", func(t *testing.T) {
		chdirTemp(t)
		useStubBackend(t)
		useAgentConfigs(t)
		useFakeLinuxBinary(t)
		cc := testContainerConfig()
		cc.ResourceLimits.Timeout = 5 * time.Minute
		env := `{"response":{},"error":"stub timeout","timeout":true}`
		useFakeContainer(t, &fakeExecer{helperStdout: &env})

		_, _, _, ec, err := invokeAgent("selftest", "p", cc, callOpts{Timeout: time.Second})
		if !errors.Is(err, inference.ErrTimeout) {
			t.Fatalf("err = %v, want ErrTimeout", err)
		}
		if ec == nil || !strings.HasPrefix(ec.Stderr, "timeout after 1s") {
			t.Errorf("ec = %+v", ec)
		}
	})

	t.Run("sandbox-limit-timeout", func(t *testing.T) {
		chdirTemp(t)
		useAgentConfigs(t)
		cc := testContainerConfig()
		cc.ResourceLimits.Timeout = 30 * time.Millisecond
		x := &fakeExecer{block: true}
		useFakeContainer(t, x)

		_, _, rec, ec, err := invokeAgent("builder", "p", cc, callOpts{})
		if err == nil {
			t.Fatal("expected a timeout")
		}
		if !errors.Is(err, inference.ErrTimeout) {
			t.Errorf("err does not satisfy ErrTimeout: %v", err)
		}
		if !strings.Contains(err.Error(), "Container execution timeout after 30ms") || !strings.Contains(err.Error(), "--resource-limit timeout=") {
			t.Errorf("message = %q", err.Error())
		}
		if ec == nil || !strings.HasPrefix(ec.Stderr, "timeout after 30ms") {
			t.Errorf("ec = %+v", ec)
		}
		if ec != nil && (ec.Platform != "linux/arm64" || ec.WorkingDir != "/workspace") {
			t.Errorf("container context missing: %+v", ec)
		}
		if rec.Error == "" {
			t.Errorf("failed call not recorded: %+v", rec)
		}
	})
}

func TestContainerRequestWorkDirIsContainerPath(t *testing.T) {
	chdirTemp(t)
	useStubBackend(t)
	useAgentConfigs(t)
	useFakeLinuxBinary(t)
	x := &fakeExecer{}
	useFakeContainer(t, x)
	cc := testContainerConfig()
	cc.WorkspaceDir = "/work/space"
	stub := &inference.StubScript{Turns: []inference.StubTurn{{Response: "r"}}}
	if _, _, _, _, err := invokeAgent("selftest", "p", cc, callOpts{Stub: stub}); err != nil {
		t.Fatal(err)
	}
	var sent inference.Request
	if err := json.Unmarshal([]byte(x.stdins[0]), &sent); err != nil {
		t.Fatal(err)
	}
	if sent.WorkDir != "/work/space" {
		t.Errorf("WorkDir on the wire = %q, want the container workspace path", sent.WorkDir)
	}
}

func envValue(env []string, key string) (string, bool) {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v, true
		}
	}
	return "", false
}

func TestContainerEnvDropsGitHubCredentialsAndHarnessVars(t *testing.T) {
	cc := testContainerConfig()
	cc.Environment = map[string]string{
		"GH_TOKEN": "a", "GITHUB_TOKEN": "b", "GH_ENTERPRISE_TOKEN": "c",
		"GITHUB_ENTERPRISE_TOKEN": "d", "GH_HOST": "e",
		"PATH": "/evil", "KAIRON_EVAL_DIR": "/evil", "KEEP": "1",
	}
	env := containerEnv(cc)
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GH_HOST"} {
		if v, ok := envValue(env, k); ok {
			t.Errorf("%s=%s reached the container env", k, v)
		}
	}
	if v, _ := envValue(env, "KEEP"); v != "1" {
		t.Errorf("unrelated variable dropped: %v", env)
	}
	n := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") || strings.HasPrefix(kv, "KAIRON_EVAL_DIR=") {
			n++
			if strings.Contains(kv, "/evil") {
				t.Errorf("caller override leaked: %s", kv)
			}
		}
	}
	if n != 2 {
		t.Errorf("PATH/KAIRON_EVAL_DIR appear %d times, want once each: %v", n, env)
	}
}
