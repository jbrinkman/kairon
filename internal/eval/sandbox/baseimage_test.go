package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jbrinkman/kairon/internal/eval/dockerfile"
)

func strPtr(s string) *string { return &s }

func mustArgs(t *testing.T, ts ToolSet, platform string) map[string]*string {
	t.Helper()
	args, err := ts.BuildArgs(platform)
	require.NoError(t, err)
	return args
}

func TestBaseImageTag_HasImageNamePrefix(t *testing.T) {
	tag := BaseImageTag("linux/amd64", dockerfile.Base, mustArgs(t, DefaultToolSet, "linux/amd64"))
	assert.True(t, strings.HasPrefix(tag, ImageNamePrefix), "tag %q must start with %q so it is never pulled", tag, ImageNamePrefix)
	assert.Regexp(t, regexp.MustCompile(`^kairon-eval-base:linux-amd64-[0-9a-f]{12}$`), tag)
}

func TestBaseImageTag_ChangesWithInputs(t *testing.T) {
	base := BaseImageTag("linux/amd64", dockerfile.Base, mustArgs(t, DefaultToolSet, "linux/amd64"))

	ghBumped := ToolSet{KiroCLIVersion: DefaultToolSet.KiroCLIVersion, GHVersion: "9.9.9"}
	kiroBumped := ToolSet{KiroCLIVersion: "9.9.9", GHVersion: DefaultToolSet.GHVersion}

	tests := []struct {
		name string
		tag  string
	}{
		{"gh version", BaseImageTag("linux/amd64", dockerfile.Base, mustArgs(t, ghBumped, "linux/amd64"))},
		{"kiro-cli version", BaseImageTag("linux/amd64", dockerfile.Base, mustArgs(t, kiroBumped, "linux/amd64"))},
		{"platform", BaseImageTag("linux/arm64", dockerfile.Base, mustArgs(t, DefaultToolSet, "linux/arm64"))},
		{"dockerfile bytes", BaseImageTag("linux/amd64", dockerfile.Base+"\n# changed\n", mustArgs(t, DefaultToolSet, "linux/amd64"))},
	}
	seen := map[string]string{base: "baseline"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEqual(t, base, tt.tag)
			if prev, dup := seen[tt.tag]; dup {
				t.Fatalf("tag %q for %q collides with %q", tt.tag, tt.name, prev)
			}
			seen[tt.tag] = tt.name
		})
	}
}

// The AC 8 invariant: the tag is a pure function of the Dockerfile, the tool
// pins and the platform. Working directory, environment and the contents of
// any evals directory must not influence it.
func TestBaseImageTag_PureAcrossCwdEnvAndEvalsDir(t *testing.T) {
	args := mustArgs(t, DefaultToolSet, "linux/amd64")
	want := BaseImageTag("linux/amd64", dockerfile.Base, args)

	origWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	for i := 0; i < 3; i++ {
		dir := t.TempDir()
		// An evals directory whose contents differ on every iteration.
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "agents"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "agents", "a.json"), []byte(strings.Repeat("x", i+1)), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "cases"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "cases", "c.yaml"), []byte(strings.Repeat("y", i+7)), 0o644))
		require.NoError(t, os.Chdir(dir))
		t.Setenv("KAIRON_EVAL_WORKSPACE_ROOT", dir)
		t.Setenv("SOME_RANDOM_ENV", strings.Repeat("z", i+1))

		assert.Equal(t, want, BaseImageTag("linux/amd64", dockerfile.Base, args))
	}
}

func TestBaseImageTag_ArgOrderIndependent(t *testing.T) {
	a := map[string]*string{"A": strPtr("1"), "B": strPtr("2")}
	b := map[string]*string{"B": strPtr("2"), "A": strPtr("1")}
	assert.Equal(t, BaseImageTag("linux/amd64", "FROM x", a), BaseImageTag("linux/amd64", "FROM x", b))
}

func TestBaseImageTag_FieldBoundariesDoNotCollide(t *testing.T) {
	a := map[string]*string{"A": strPtr("bc")}
	b := map[string]*string{"Ab": strPtr("c")}
	assert.NotEqual(t, BaseImageTag("linux/amd64", "FROM x", a), BaseImageTag("linux/amd64", "FROM x", b))
}

func TestToolSetBuildArgs(t *testing.T) {
	tests := []struct {
		platform string
		kiroURL  string
		ghArch   string
	}{
		{"linux/amd64", "https://desktop-release.q.us-east-1.amazonaws.com/" + DefaultToolSet.KiroCLIVersion + "/kirocli-x86_64-linux-musl.zip", "amd64"},
		{"linux/arm64", "https://desktop-release.q.us-east-1.amazonaws.com/" + DefaultToolSet.KiroCLIVersion + "/kirocli-aarch64-linux-musl.zip", "arm64"},
	}
	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			args, err := DefaultToolSet.BuildArgs(tt.platform)
			require.NoError(t, err)
			require.Len(t, args, 3)
			assert.Equal(t, tt.kiroURL, *args["KIRO_CLI_URL"])
			assert.Equal(t, DefaultToolSet.GHVersion, *args["GH_VERSION"])
			assert.Equal(t, tt.ghArch, *args["GH_ARCH"])
			assert.NotContains(t, *args["KIRO_CLI_URL"], "/latest/")
		})
	}
}

func TestToolSetBuildArgs_Errors(t *testing.T) {
	for _, platform := range []string{"linux/386", "windows/amd64", "darwin/arm64", ""} {
		_, err := DefaultToolSet.BuildArgs(platform)
		require.Error(t, err, platform)
		assert.Contains(t, err.Error(), "unsupported platform")
	}

	_, err := ToolSet{GHVersion: "1.0.0"}.BuildArgs("linux/amd64")
	require.Error(t, err)
	_, err = ToolSet{KiroCLIVersion: "1.0.0"}.BuildArgs("linux/amd64")
	require.Error(t, err)
}

// Static assertions on the embedded Dockerfile: it is a tools-only image.
func TestBaseDockerfile_ToolsOnly(t *testing.T) {
	df := dockerfile.Base
	require.NotEmpty(t, df)

	var instructions []string
	for _, line := range strings.Split(df, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		instructions = append(instructions, trimmed)
	}
	body := strings.Join(instructions, "\n")

	assert.NotRegexp(t, regexp.MustCompile(`(?m)^(COPY|ADD)\s`), df, "no COPY/ADD allowed")
	for _, forbidden := range []string{".kiro", "agents", "skills", "cases", ".kairon"} {
		assert.NotContains(t, body, forbidden)
	}

	assert.Contains(t, body, "adduser -D -u 1000")
	assert.Contains(t, body, "sandbox")
	assert.Contains(t, body, "kiro-cli")
	assert.Contains(t, body, "gh")
	assert.Contains(t, body, "git")
	assert.Contains(t, body, "KIRO_CLI_URL")
	assert.Contains(t, body, "GH_VERSION")
	assert.Contains(t, body, "ca-certificates")
	assert.Contains(t, body, "kiro-cli --version && gh --version && git --version")

	assert.Equal(t, "USER sandbox", lastWithPrefix(instructions, "USER "), "USER sandbox must be the final USER")
	assert.True(t, strings.HasPrefix(instructions[len(instructions)-1], "CMD"))
	assert.Contains(t, body, "WORKDIR /workspace")
	assert.Contains(t, body, "chown sandbox:sandbox /workspace")

	// No project toolchains.
	toolchain := regexp.MustCompile(`(?i)\b(golang|go-?lang|nodejs|npm|yarn|python3?|pip3?|cargo|rustup|rust|openjdk\d*|maven|gradle|go-task)\b`)
	assert.Empty(t, toolchain.FindAllString(body, -1), "toolchains must not be installed")

	// apk package lists only contain the allowed packages.
	allowed := map[string]bool{"git": true, "bash": true, "ca-certificates": true, "curl": true, "unzip": true}
	for _, m := range regexp.MustCompile(`apk add[^;\n]*`).FindAllString(body, -1) {
		for _, f := range strings.Fields(m) {
			if f == "apk" || f == "add" || strings.HasPrefix(f, "--") || strings.HasPrefix(f, ".") {
				continue
			}
			assert.True(t, allowed[f], "unexpected apk package %q", f)
		}
	}
}

func lastWithPrefix(lines []string, prefix string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], prefix) {
			return lines[i]
		}
	}
	return ""
}

func TestKiroCLIVersionedURL(t *testing.T) {
	url, err := kiroCLIVersionedURL("linux/amd64", "1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "https://desktop-release.q.us-east-1.amazonaws.com/1.2.3/kirocli-x86_64-linux-musl.zip", url)

	_, err = kiroCLIVersionedURL("linux/riscv64", "1.2.3")
	require.Error(t, err)
	_, err = kiroCLIVersionedURL("linux/amd64", "")
	require.Error(t, err)
}

// Gated: builds the real base image (downloads kiro-cli and gh), so it only
// runs with a container daemon and KAIRON_EVAL_SANDBOX_SELFTEST=1.
func TestEnsureBaseImage_ReusesExistingImage(t *testing.T) {
	if os.Getenv("KAIRON_EVAL_SANDBOX_SELFTEST") != "1" {
		t.Skip("set KAIRON_EVAL_SANDBOX_SELFTEST=1 (see `task eval:selftest:sandbox`) to build the base image")
	}
	skipIfNoContainerDaemon(t)

	platform, err := DetectHostArchitecture()
	require.NoError(t, err)

	im, err := NewImageManager("base-image-test", false)
	require.NoError(t, err)
	defer im.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	tag1, _, err := im.EnsureBaseImage(ctx, platform)
	require.NoError(t, err)
	info1, err := im.client.ImageInspect(ctx, tag1)
	require.NoError(t, err)

	tag2, built2, err := im.EnsureBaseImage(ctx, platform)
	require.NoError(t, err)
	info2, err := im.client.ImageInspect(ctx, tag2)
	require.NoError(t, err)

	assert.False(t, built2, "second call must reuse the image")
	assert.Equal(t, tag1, tag2)
	assert.Equal(t, info1.ID, info2.ID)
}
