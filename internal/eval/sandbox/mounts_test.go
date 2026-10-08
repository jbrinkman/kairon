package sandbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/mount"
)

func realTempDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestNewHostConfigWithMounts_BindFlags(t *testing.T) {
	ws := realTempDir(t)
	kiro := filepath.Join(ws, ".kiro")
	eval := filepath.Join(ws, ".eval")
	for _, d := range []string{kiro, eval} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	hc, err := newHostConfigWithMounts(DefaultLimits(), []Mount{
		{HostPath: kiro, ContainerPath: "/workspace/.kiro", ReadOnly: true},
		{HostPath: ws, ContainerPath: "/workspace"},
		{HostPath: eval, ContainerPath: "/workspace/.eval"},
	}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hc.Mounts) != 3 {
		t.Fatalf("want 3 mounts, got %d", len(hc.Mounts))
	}
	want := []struct {
		src, dst string
		ro       bool
	}{
		{kiro, "/workspace/.kiro", true},
		{ws, "/workspace", false},
		{eval, "/workspace/.eval", false},
	}
	for i, w := range want {
		m := hc.Mounts[i]
		if m.Type != mount.TypeBind {
			t.Errorf("mount %d: type = %q, want bind", i, m.Type)
		}
		if m.Source != w.src || m.Target != w.dst || m.ReadOnly != w.ro {
			t.Errorf("mount %d: got src=%q dst=%q ro=%v; want src=%q dst=%q ro=%v",
				i, m.Source, m.Target, m.ReadOnly, w.src, w.dst, w.ro)
		}
		if m.BindOptions == nil || m.BindOptions.CreateMountpoint {
			t.Errorf("mount %d: CreateMountpoint must be explicitly false, got %+v", i, m.BindOptions)
		}
	}
	if len(hc.Binds) != 0 {
		t.Errorf("Binds must not be used, got %v", hc.Binds)
	}
}

func TestNewHostConfigWithMounts_NoWorkspaceTmpfs(t *testing.T) {
	ws := realTempDir(t)
	hc, err := newHostConfigWithMounts(DefaultLimits(), []Mount{{HostPath: ws, ContainerPath: "/workspace"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := hc.Tmpfs["/workspace"]; ok {
		t.Error("host config must not have a tmpfs at /workspace")
	}
	for p := range hc.Tmpfs {
		if p == "/workspace" || strings.HasPrefix(p, "/workspace/") {
			t.Errorf("tmpfs entry %q must not be at or under the workspace", p)
		}
	}
}

func TestNewHostConfigWithMounts_ReadonlyRootfs(t *testing.T) {
	ws := realTempDir(t)
	for name, mounts := range map[string][]Mount{
		"no mounts": nil,
		"workspace": {{HostPath: ws, ContainerPath: "/workspace"}},
		"read-only": {{HostPath: ws, ContainerPath: "/workspace", ReadOnly: true}},
	} {
		for _, selinux := range []bool{false, true} {
			hc, err := newHostConfigWithMounts(DefaultLimits(), mounts, selinux)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if !hc.ReadonlyRootfs {
				t.Errorf("%s (selinux=%v): ReadonlyRootfs must be true", name, selinux)
			}
		}
	}
	hc, err := NewHostConfigWithMounts(DefaultLimits(), []Mount{{HostPath: ws, ContainerPath: "/workspace"}})
	if err != nil {
		t.Fatal(err)
	}
	if !hc.ReadonlyRootfs {
		t.Error("public NewHostConfigWithMounts: ReadonlyRootfs must be true")
	}
}

func TestNewHostConfigWithMounts_Tmpfs(t *testing.T) {
	ws := realTempDir(t)
	hc, err := newHostConfigWithMounts(DefaultLimits(), []Mount{{HostPath: ws, ContainerPath: "/workspace"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(hc.Tmpfs) != 3 {
		t.Fatalf("want exactly 3 tmpfs entries, got %v", hc.Tmpfs)
	}
	want := map[string][]string{
		"/tmp":          {"rw", "nosuid", "nodev", "mode=1777", "size=" + TmpfsTmpSize},
		"/var/tmp":      {"rw", "nosuid", "nodev", "mode=1777", "size=" + TmpfsVarTmpSize},
		"/home/sandbox": {"rw", "nosuid", "nodev", "uid=1000", "gid=1000", "mode=0755", "size=" + TmpfsHomeSize},
	}
	for path, opts := range want {
		got, ok := hc.Tmpfs[path]
		if !ok {
			t.Errorf("missing tmpfs entry for %s", path)
			continue
		}
		have := strings.Split(got, ",")
		for _, o := range opts {
			found := false
			for _, h := range have {
				if h == o {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("tmpfs %s options %q missing %q", path, got, o)
			}
		}
		if strings.Contains(got, "noexec") {
			t.Errorf("tmpfs %s must not set noexec: %q", path, got)
		}
	}
	// Only /home/sandbox is chowned; temp dirs are world-writable sticky.
	for _, p := range []string{"/tmp", "/var/tmp"} {
		if strings.Contains(hc.Tmpfs[p], "uid=") || strings.Contains(hc.Tmpfs[p], "gid=") {
			t.Errorf("tmpfs %s must not set uid/gid: %q", p, hc.Tmpfs[p])
		}
	}
}

func TestNewHostConfigWithMounts_PreservesLimitsAndNetwork(t *testing.T) {
	limits := ResourceLimits{CPUQuota: 250000, Memory: 128 * 1024 * 1024}
	hc, err := newHostConfigWithMounts(limits, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if hc.Resources.CPUQuota != limits.CPUQuota ||
		hc.Resources.CPUPeriod != 100000 ||
		hc.Resources.Memory != limits.Memory {
		t.Errorf("resources not applied: %+v (limits %+v)", hc.Resources, limits)
	}
	if hc.NetworkMode != "none" {
		t.Errorf("NetworkMode = %q, want none", hc.NetworkMode)
	}
}

func TestNewHostConfigWithMounts_RejectsBadHostPaths(t *testing.T) {
	missing := filepath.Join(realTempDir(t), "does-not-exist")
	cases := map[string]string{
		"relative":   "relative/path",
		"dot":        ".",
		"missing":    missing,
		"empty":      "",
		"fs root":    string(filepath.Separator),
		"root alias": filepath.Join(string(filepath.Separator), "."),
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			hc, err := newHostConfigWithMounts(DefaultLimits(), []Mount{{HostPath: p, ContainerPath: "/workspace"}}, false)
			if err == nil {
				t.Fatalf("expected error for host path %q, got config %+v", p, hc)
			}
			if hc != nil {
				t.Error("config must be nil on error")
			}
			if p != "" && !strings.Contains(err.Error(), p) {
				t.Errorf("error %q should name the path %q", err, p)
			}
		})
	}
}

func TestNewHostConfigWithMounts_RejectsBadContainerPaths(t *testing.T) {
	ws := realTempDir(t)
	for _, c := range []string{"", "workspace", "/", "//"} {
		if _, err := newHostConfigWithMounts(DefaultLimits(), []Mount{{HostPath: ws, ContainerPath: c}}, false); err == nil {
			t.Errorf("expected error for container path %q", c)
		}
	}
}

func TestNewHostConfigWithMounts_ResolvesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks unreliable on windows")
	}
	base := realTempDir(t)
	real := filepath.Join(base, "real")
	link := filepath.Join(base, "link")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	hc, err := newHostConfigWithMounts(DefaultLimits(), []Mount{{HostPath: link, ContainerPath: "/workspace"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if hc.Mounts[0].Source != real {
		t.Errorf("Source = %q, want symlink-resolved %q", hc.Mounts[0].Source, real)
	}
}

func TestNewHostConfigWithMounts_FirstBadMountRejectsAll(t *testing.T) {
	ws := realTempDir(t)
	missing := filepath.Join(ws, "nope")
	_, err := newHostConfigWithMounts(DefaultLimits(), []Mount{
		{HostPath: ws, ContainerPath: "/workspace"},
		{HostPath: missing, ContainerPath: "/workspace/.eval"},
	}, false)
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("want error naming %q, got %v", missing, err)
	}
}

func TestSELinuxEnforcingFromContent(t *testing.T) {
	cases := []struct {
		name    string
		content string
		err     error
		want    bool
	}{
		{"enforcing", "1", nil, true},
		{"enforcing newline", "1\n", nil, true},
		{"permissive", "0", nil, false},
		{"empty", "", nil, false},
		{"garbage", "yes", nil, false},
		{"read error", "1", errors.New("boom"), false},
		{"missing file", "", os.ErrNotExist, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := selinuxEnforcingFromContent([]byte(c.content), c.err); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestNewHostConfigWithMounts_SELinuxLabelDisable(t *testing.T) {
	ws := realTempDir(t)
	mounts := []Mount{{HostPath: ws, ContainerPath: "/workspace"}}

	off, err := newHostConfigWithMounts(DefaultLimits(), mounts, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(off.SecurityOpt) != 0 {
		t.Errorf("SecurityOpt must be empty when not enforcing, got %v", off.SecurityOpt)
	}

	on, err := newHostConfigWithMounts(DefaultLimits(), mounts, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(on.SecurityOpt) != 1 || on.SecurityOpt[0] != "label=disable" {
		t.Errorf("SecurityOpt = %v, want [label=disable]", on.SecurityOpt)
	}
}

func TestNewHostConfigWithMounts_PublicUsesHostProbe(t *testing.T) {
	ws := realTempDir(t)
	hc, err := NewHostConfigWithMounts(DefaultLimits(), []Mount{{HostPath: ws, ContainerPath: "/workspace"}})
	if err != nil {
		t.Fatal(err)
	}
	hasLabel := len(hc.SecurityOpt) == 1 && hc.SecurityOpt[0] == "label=disable"
	if hasLabel != hostSELinuxEnforcing() {
		t.Errorf("label=disable presence (%v) must match host probe (%v)", hasLabel, hostSELinuxEnforcing())
	}
}

func TestWithOpenUmask_Shape(t *testing.T) {
	got := WithOpenUmask([]string{"kiro-cli", "chat", "--flag"})
	want := []string{"sh", "-c", `umask 000; exec "$@"`, "kairon-exec", "kiro-cli", "chat", "--flag"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWithOpenUmask_DoesNotMutateInput(t *testing.T) {
	in := []string{"a", "b"}
	_ = WithOpenUmask(in)
	if len(in) != 2 || in[0] != "a" || in[1] != "b" {
		t.Errorf("input mutated: %q", in)
	}
}

func TestWithOpenUmask_PreservesArguments(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	// printf '%s\0' writes each argument NUL-terminated so we can compare
	// exactly, including embedded newlines.
	args := []string{
		"plain",
		"with space",
		`with "double" and 'single' quotes`,
		"multi\nline\narg",
		"$HOME `echo hi` $(echo hi) ; | & *",
		"",
		"-n",
	}
	cmd := append([]string{"printf", `%s\0`}, args...)
	argv := WithOpenUmask(cmd)
	out, err := exec.Command(argv[0], argv[1:]...).Output()
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	got := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	if len(got) != len(args) {
		t.Fatalf("got %d args %q, want %d %q", len(got), got, len(args), args)
	}
	for i := range args {
		if got[i] != args[i] {
			t.Errorf("arg %d = %q, want %q", i, got[i], args[i])
		}
	}
}

func TestWithOpenUmask_AppliesUmask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("umask not supported on windows")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := realTempDir(t)
	file := filepath.Join(dir, "f")
	argv := WithOpenUmask([]string{"sh", "-c", `touch "$1"`, "touch", file})
	if out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput(); err != nil {
		t.Fatalf("exec: %v: %s", err, out)
	}
	st, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm&0o066 != 0o066 {
		t.Errorf("file mode %o should be world read/write under umask 000", perm)
	}
}
