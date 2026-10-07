package sandbox

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// LinuxBinaryEnv names the environment variable that overrides the linux
// kairon binary copied into the sandbox container.
const LinuxBinaryEnv = "KAIRON_SANDBOX_BINARY"

const kaironModulePath = "github.com/jbrinkman/kairon"

// linuxBinarySource says where ResolveLinuxBinary gets its binary from.
type linuxBinarySource int

const (
	sourceOverride   linuxBinarySource = iota // KAIRON_SANDBOX_BINARY
	sourceExecutable                          // the running executable
	sourceBuild                               // cross-compile from the module root
)

// linuxBinaryInputs are the facts the decision depends on. Gathering them is
// impure (env, os.Executable, filesystem walk); deciding is not.
type linuxBinaryInputs struct {
	Platform   string // container platform, e.g. "linux/arm64"
	Override   string // value of KAIRON_SANDBOX_BINARY
	HostGOOS   string
	HostGOARCH string
	Executable string // path of the running executable ("" if unknown)
	ModuleRoot string // kairon module root ("" if not found)
}

// linuxBinaryPlan is the outcome of the decision.
type linuxBinaryPlan struct {
	Source     linuxBinarySource
	Path       string // binary path for override/executable sources
	Arch       string // GOARCH for the build source
	ModuleRoot string // build directory for the build source
}

// planLinuxBinary is the pure decision function behind ResolveLinuxBinary:
//  1. KAIRON_SANDBOX_BINARY wins when set.
//  2. The running executable is used only when it is a linux binary of the
//     container's architecture.
//  3. Otherwise cross-compile from the module root.
//  4. Otherwise fail with an error naming KAIRON_SANDBOX_BINARY.
func planLinuxBinary(in linuxBinaryInputs) (linuxBinaryPlan, error) {
	osName, arch, ok := strings.Cut(in.Platform, "/")
	if !ok || osName != "linux" || arch == "" || strings.Contains(arch, "/") {
		return linuxBinaryPlan{}, fmt.Errorf("unsupported sandbox platform %q: expected linux/<arch>", in.Platform)
	}

	if in.Override != "" {
		return linuxBinaryPlan{Source: sourceOverride, Path: in.Override, Arch: arch}, nil
	}

	if in.HostGOOS == "linux" && in.HostGOARCH == arch && in.Executable != "" {
		return linuxBinaryPlan{Source: sourceExecutable, Path: in.Executable, Arch: arch}, nil
	}

	if in.ModuleRoot != "" {
		return linuxBinaryPlan{Source: sourceBuild, Arch: arch, ModuleRoot: in.ModuleRoot}, nil
	}

	return linuxBinaryPlan{}, fmt.Errorf(
		"no linux/%s kairon binary available for the sandbox container: the running executable is %s/%s and the kairon source tree (go.mod for module %s) was not found from the current directory to cross-compile; set %s to a static linux/%s kairon binary, or run from a kairon source checkout with Go installed",
		arch, in.HostGOOS, in.HostGOARCH, kaironModulePath, LinuxBinaryEnv, arch)
}

var (
	linuxBinaryMu    sync.Mutex
	linuxBinaryCache = map[string]string{}
)

// ResolveLinuxBinary returns the path of a static linux kairon binary for the
// given container platform (e.g. "linux/arm64"). Successful results are
// memoised for the life of the process.
//
// Resolution order: KAIRON_SANDBOX_BINARY; the running executable when it is a
// linux binary of the same architecture; otherwise
// `CGO_ENABLED=0 GOOS=linux GOARCH=<arch> go build -trimpath ./cmd/kairon` from
// the kairon module root into os.UserCacheDir()/kairon/sandbox/.
func ResolveLinuxBinary(platform string) (string, error) {
	override := os.Getenv(LinuxBinaryEnv)
	key := platform + "\x00" + override

	linuxBinaryMu.Lock()
	defer linuxBinaryMu.Unlock()
	if p, ok := linuxBinaryCache[key]; ok {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		delete(linuxBinaryCache, key)
	}

	in := linuxBinaryInputs{
		Platform:   platform,
		Override:   override,
		HostGOOS:   runtime.GOOS,
		HostGOARCH: runtime.GOARCH,
	}
	if exe, err := os.Executable(); err == nil {
		if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
			exe = resolved
		}
		in.Executable = exe
	}
	if cwd, err := os.Getwd(); err == nil {
		in.ModuleRoot = findKaironModuleRoot(cwd)
	}

	plan, err := planLinuxBinary(in)
	if err != nil {
		return "", err
	}

	var path string
	switch plan.Source {
	case sourceOverride, sourceExecutable:
		if _, statErr := os.Stat(plan.Path); statErr != nil {
			if plan.Source == sourceOverride {
				return "", fmt.Errorf("%s=%q is not usable: %w", LinuxBinaryEnv, plan.Path, statErr)
			}
			return "", fmt.Errorf("running executable %q is not usable (%v); set %s to a static linux/%s kairon binary", plan.Path, statErr, LinuxBinaryEnv, plan.Arch)
		}
		path = plan.Path
	case sourceBuild:
		path, err = crossCompileKairon(plan.ModuleRoot, plan.Arch)
		if err != nil {
			return "", err
		}
	}

	linuxBinaryCache[key] = path
	return path, nil
}

// findKaironModuleRoot walks up from start looking for a go.mod that declares
// the kairon module. It returns "" when none is found.
func findKaironModuleRoot(start string) string {
	dir := start
	for {
		if declaresKaironModule(filepath.Join(dir, "go.mod")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func declaresKaironModule(goMod string) bool {
	f, err := os.Open(goMod)
	if err != nil {
		return false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"`) == kaironModulePath
		}
	}
	return false
}

// crossCompileKairon builds a static linux binary for arch from moduleRoot.
func crossCompileKairon(moduleRoot, arch string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locating cache directory for the sandbox binary: %w; set %s to a static linux/%s kairon binary", err, LinuxBinaryEnv, arch)
	}
	outDir := filepath.Join(cacheDir, "kairon", "sandbox")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w; set %s to a static linux/%s kairon binary", outDir, err, LinuxBinaryEnv, arch)
	}
	out := filepath.Join(outDir, "kairon-linux-"+arch)

	goBin, err := exec.LookPath("go")
	if err != nil {
		return "", fmt.Errorf("cannot cross-compile the sandbox kairon binary: go not found in PATH (%v); install Go or set %s to a static linux/%s kairon binary", err, LinuxBinaryEnv, arch)
	}

	cmd := exec.Command(goBin, "build", "-trimpath", "-o", out, "./cmd/kairon")
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("cross-compiling kairon for linux/%s failed: %w\n%s\nset %s to a prebuilt static linux/%s kairon binary to skip the build",
			arch, err, strings.TrimSpace(string(output)), LinuxBinaryEnv, arch)
	}
	return out, nil
}
