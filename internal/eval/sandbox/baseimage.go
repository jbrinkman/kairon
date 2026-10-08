package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/jbrinkman/kairon/internal/eval/dockerfile"
)

// baseImageRepo is the repository part of the base image tag. It begins with
// ImageNamePrefix so CreateWithPlatform treats the image as local-only and
// never tries to pull it.
const baseImageRepo = ImageNamePrefix + "-base"

// kiroCLIReleaseBaseURL is the kiro-cli release root; the version (or "latest")
// is appended as the first path segment.
const kiroCLIReleaseBaseURL = "https://desktop-release.q.us-east-1.amazonaws.com"

// ToolSet pins the tools baked into the base image. Bumping a field changes
// the build args, hence the image tag, hence triggers exactly one rebuild.
//
// URL verification (performed when these pins were chosen, HTTP 200 for each):
//   - kiro-cli: the URL is VERSIONED. Both
//     https://desktop-release.q.us-east-1.amazonaws.com/2.28.0/kirocli-x86_64-linux-musl.zip and
//     https://desktop-release.q.us-east-1.amazonaws.com/2.28.0/kirocli-aarch64-linux-musl.zip
//     exist (confirmed with `curl -I`), and 2.28.0 is the version reported by
//     .../latest/manifest.json. The tag therefore pins a real, immutable
//     artifact; "latest" is not used.
//   - gh: https://github.com/cli/cli/releases/download/v2.102.0/gh_2.102.0_linux_amd64.tar.gz
//     and ..._linux_arm64.tar.gz both exist (HTTP 200 after redirect).
type ToolSet struct {
	// KiroCLIVersion is the kiro-cli release, e.g. "2.28.0".
	KiroCLIVersion string
	// GHVersion is the GitHub CLI release without the leading "v", e.g. "2.102.0".
	GHVersion string
}

// DefaultToolSet holds the pinned tool versions used for the base image.
var DefaultToolSet = ToolSet{
	KiroCLIVersion: "2.28.0",
	GHVersion:      "2.102.0",
}

// kiroCLIVersionedURL returns the download URL of the kiro-cli zip for the
// platform at a specific version.
func kiroCLIVersionedURL(platform, version string) (string, error) {
	if version == "" {
		return "", fmt.Errorf("kiro-cli version must not be empty")
	}
	var file string
	switch platform {
	case "linux/amd64":
		file = "kirocli-x86_64-linux-musl.zip"
	case "linux/arm64":
		file = "kirocli-aarch64-linux-musl.zip"
	default:
		return "", fmt.Errorf("unsupported platform: %s", platform)
	}
	return fmt.Sprintf("%s/%s/%s", kiroCLIReleaseBaseURL, version, file), nil
}

// ghArch maps a Docker platform to the architecture suffix of gh release assets.
func ghArch(platform string) (string, error) {
	switch platform {
	case "linux/amd64":
		return "amd64", nil
	case "linux/arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported platform: %s", platform)
	}
}

// BuildArgs returns the Docker build args for the platform. It errors for an
// unsupported platform or an empty pin.
func (t ToolSet) BuildArgs(platform string) (map[string]*string, error) {
	if t.GHVersion == "" {
		return nil, fmt.Errorf("gh version must not be empty")
	}
	url, err := kiroCLIVersionedURL(platform, t.KiroCLIVersion)
	if err != nil {
		return nil, err
	}
	arch, err := ghArch(platform)
	if err != nil {
		return nil, err
	}
	gh := t.GHVersion
	return map[string]*string{
		"KIRO_CLI_URL": &url,
		"GH_VERSION":   &gh,
		"GH_ARCH":      &arch,
	}, nil
}

// BaseImageTag computes the base image tag. It is a pure function of the
// Dockerfile bytes, the platform and the build args: nothing about the
// working directory, environment, agents, skills or cases participates, which
// is what lets the image be reused across evals.
func BaseImageTag(platform string, dockerfileContent string, args map[string]*string) string {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	writeField := func(s string) {
		// Length-prefix each field so concatenations cannot collide.
		fmt.Fprintf(h, "%d:%s;", len(s), s)
	}
	writeField(dockerfileContent)
	writeField(platform)
	for _, k := range keys {
		v := ""
		if args[k] != nil {
			v = *args[k]
		}
		writeField(k)
		writeField(v)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	safePlatform := strings.ReplaceAll(platform, "/", "-")
	return fmt.Sprintf("%s:%s-%s", baseImageRepo, safePlatform, sum[:12])
}

// EnsureBaseImage returns the tag of the base image for the platform, building
// it only if no image with that tag exists. built reports whether a build ran.
// The image is persistent: nothing in this package removes it.
func (im *ImageManager) EnsureBaseImage(ctx context.Context, platform string) (tag string, built bool, err error) {
	return im.ensureBaseImage(ctx, platform, dockerfile.Base, DefaultToolSet)
}

func (im *ImageManager) ensureBaseImage(ctx context.Context, platform, dockerfileContent string, tools ToolSet) (string, bool, error) {
	args, err := tools.BuildArgs(platform)
	if err != nil {
		return "", false, fmt.Errorf("resolving base image tool pins: %w", err)
	}
	tag := BaseImageTag(platform, dockerfileContent, args)

	im.mu.Lock()
	defer im.mu.Unlock()

	if _, err := im.client.ImageInspect(ctx, tag); err == nil {
		if im.debugMode {
			fmt.Printf("🔧 Debug: Reusing base image %s\n", tag)
		}
		return tag, false, nil
	}

	c, err := NewContainerWithDebug("", im.debugMode)
	if err != nil {
		return "", false, fmt.Errorf("creating container client for base image build: %w", err)
	}
	defer c.Close()

	if err := c.buildImageWithArgs(ctx, dockerfileContent, tag, platform, args); err != nil {
		return "", false, fmt.Errorf("building base image %s: %w", tag, err)
	}
	return tag, true, nil
}

// ImageID returns the content ID of a local image tag. It lets callers (the
// gated reuse tests) prove that a later run reused the very same image rather
// than rebuilding an identically tagged one.
func (im *ImageManager) ImageID(ctx context.Context, tag string) (string, error) {
	info, err := im.client.ImageInspect(ctx, tag)
	if err != nil {
		return "", fmt.Errorf("inspecting image %s: %w", tag, err)
	}
	return info.ID, nil
}
