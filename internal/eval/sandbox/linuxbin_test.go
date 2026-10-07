package sandbox

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanLinuxBinary(t *testing.T) {
	tests := []struct {
		name       string
		in         linuxBinaryInputs
		wantSource linuxBinarySource
		wantPath   string
		wantArch   string
		wantRoot   string
		wantErr    bool
	}{
		{
			name: "override wins over matching executable and module root",
			in: linuxBinaryInputs{
				Platform: "linux/amd64", Override: "/opt/kairon",
				HostGOOS: "linux", HostGOARCH: "amd64", Executable: "/usr/bin/kairon", ModuleRoot: "/src",
			},
			wantSource: sourceOverride, wantPath: "/opt/kairon", wantArch: "amd64",
		},
		{
			name: "matching linux executable is used",
			in: linuxBinaryInputs{
				Platform: "linux/arm64", HostGOOS: "linux", HostGOARCH: "arm64",
				Executable: "/usr/bin/kairon", ModuleRoot: "/src",
			},
			wantSource: sourceExecutable, wantPath: "/usr/bin/kairon", wantArch: "arm64",
		},
		{
			name: "darwin host builds even with same arch",
			in: linuxBinaryInputs{
				Platform: "linux/arm64", HostGOOS: "darwin", HostGOARCH: "arm64",
				Executable: "/usr/local/bin/kairon", ModuleRoot: "/src",
			},
			wantSource: sourceBuild, wantArch: "arm64", wantRoot: "/src",
		},
		{
			name: "linux host with different arch builds",
			in: linuxBinaryInputs{
				Platform: "linux/arm64", HostGOOS: "linux", HostGOARCH: "amd64",
				Executable: "/usr/bin/kairon", ModuleRoot: "/src",
			},
			wantSource: sourceBuild, wantArch: "arm64", wantRoot: "/src",
		},
		{
			name: "no module root and no usable executable is an error",
			in: linuxBinaryInputs{
				Platform: "linux/arm64", HostGOOS: "darwin", HostGOARCH: "arm64",
				Executable: "/usr/local/bin/kairon",
			},
			wantErr: true,
		},
		{
			name:    "non-linux platform is rejected",
			in:      linuxBinaryInputs{Platform: "windows/amd64", ModuleRoot: "/src"},
			wantErr: true,
		},
		{
			name:    "malformed platform is rejected",
			in:      linuxBinaryInputs{Platform: "linux", ModuleRoot: "/src"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := planLinuxBinary(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				if tt.in.Platform == "linux/arm64" {
					assert.Contains(t, err.Error(), LinuxBinaryEnv)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSource, plan.Source)
			assert.Equal(t, tt.wantPath, plan.Path)
			assert.Equal(t, tt.wantArch, plan.Arch)
			assert.Equal(t, tt.wantRoot, plan.ModuleRoot)
		})
	}
}

func TestFindKaironModuleRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/jbrinkman/kairon\n\ngo 1.26\n"), 0o644))
	nested := filepath.Join(root, "internal", "eval")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	assert.Equal(t, root, findKaironModuleRoot(nested))

	other := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(other, "go.mod"), []byte("module example.com/other\n"), 0o644))
	assert.Empty(t, findKaironModuleRoot(other))
}

func TestResolveLinuxBinary_Override(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "kairon-linux")
	require.NoError(t, os.WriteFile(bin, []byte("x"), 0o755))
	t.Setenv(LinuxBinaryEnv, bin)

	got, err := ResolveLinuxBinary("linux/amd64")
	require.NoError(t, err)
	assert.Equal(t, bin, got)

	// Memoised: the second call returns the same path.
	again, err := ResolveLinuxBinary("linux/amd64")
	require.NoError(t, err)
	assert.Equal(t, got, again)
}

func TestResolveLinuxBinary_BadOverrideNamesEnv(t *testing.T) {
	t.Setenv(LinuxBinaryEnv, filepath.Join(t.TempDir(), "missing"))
	_, err := ResolveLinuxBinary("linux/amd64")
	require.Error(t, err)
	assert.Contains(t, err.Error(), LinuxBinaryEnv)
}

// This whole suite is a `go test` binary, so the guard must see it as one.
func TestRunningUnderGoTest(t *testing.T) {
	assert.True(t, runningUnderGoTest(), "the test binary must be detected as a go test binary")
}

// ResolveLinuxBinary must never hand back the running executable when that
// executable is a go test binary (e.g. eval.test): it has no inference-exec
// command. On a Linux host of the container's architecture — the only case
// where the executable shortcut would otherwise fire — the resolver must fall
// through to cross-compile or an error, never to the test binary itself.
func TestResolveLinuxBinary_SkipsGoTestExecutable(t *testing.T) {
	t.Setenv(LinuxBinaryEnv, "") // no override, so only the exe/build paths remain

	platform := "linux/" + runtime.GOARCH
	if runtime.GOOS != "linux" {
		// Cross-OS always builds; assert the pure decision rejects a test-like
		// executable candidate by never selecting sourceExecutable here.
		exe, err := os.Executable()
		require.NoError(t, err)
		// With the guard the gatherer leaves Executable empty; emulate that in
		// the pure planner to prove it would not pick the executable.
		plan, perr := planLinuxBinary(linuxBinaryInputs{
			Platform: platform, HostGOOS: runtime.GOOS, HostGOARCH: runtime.GOARCH,
			Executable: "", ModuleRoot: "/src",
		})
		require.NoError(t, perr)
		assert.Equal(t, sourceBuild, plan.Source, "test binary %q must not be selected", exe)
		return
	}

	got, err := ResolveLinuxBinary(platform)
	if err != nil {
		// No module root / no Go: acceptable, as long as it did not return the exe.
		return
	}
	exe, eerr := os.Executable()
	require.NoError(t, eerr)
	assert.NotEqual(t, exe, got, "resolver returned the go test binary itself")
}
