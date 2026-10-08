package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
)

// Mount describes one host directory (or file) bind-mounted into the sandbox
// container. Everything the agent sees under the workspace is provided this
// way; nothing is copied into a running container.
type Mount struct {
	HostPath      string // absolute host path; must exist
	ContainerPath string // absolute path inside the container
	ReadOnly      bool
}

// Writable tmpfs mounts layered over the read-only root filesystem. Tmpfs
// memory counts against the container memory limit, so each size is capped.
const (
	// TmpfsTmpPath is the container's scratch directory.
	TmpfsTmpPath = "/tmp"
	// TmpfsVarTmpPath is the container's persistent-scratch directory.
	TmpfsVarTmpPath = "/var/tmp"
	// TmpfsHomePath is the sandbox user's $HOME. kiro-cli keeps state there,
	// so it must be writable; it is ephemeral per container.
	TmpfsHomePath = "/home/sandbox"

	// TmpfsTmpSize is the size cap for /tmp.
	TmpfsTmpSize = "256m"
	// TmpfsVarTmpSize is the size cap for /var/tmp.
	TmpfsVarTmpSize = "64m"
	// TmpfsHomeSize is the size cap for /home/sandbox.
	TmpfsHomeSize = "256m"

	// SandboxUID and SandboxGID own /home/sandbox inside the container.
	SandboxUID = 1000
	SandboxGID = 1000
)

// tmpfsMounts returns the tmpfs map for the read-only root filesystem. noexec
// is deliberately not set: kiro-cli and tools may exec from temp locations.
func tmpfsMounts() map[string]string {
	return map[string]string{
		TmpfsTmpPath:    "rw,nosuid,nodev,mode=1777,size=" + TmpfsTmpSize,
		TmpfsVarTmpPath: "rw,nosuid,nodev,mode=1777,size=" + TmpfsVarTmpSize,
		TmpfsHomePath: fmt.Sprintf("rw,nosuid,nodev,uid=%d,gid=%d,mode=0755,size=%s",
			SandboxUID, SandboxGID, TmpfsHomeSize),
	}
}

// selinuxEnforcePath is the kernel interface reporting SELinux enforcing mode.
const selinuxEnforcePath = "/sys/fs/selinux/enforce"

// selinuxEnforcingFromContent is the pure decision: SELinux is enforcing when
// the enforce file holds "1". Missing, unreadable or any other content means
// not enforcing.
func selinuxEnforcingFromContent(content []byte, readErr error) bool {
	if readErr != nil {
		return false
	}
	return strings.TrimSpace(string(content)) == "1"
}

// hostSELinuxEnforcing probes the host for SELinux enforcing mode.
func hostSELinuxEnforcing() bool {
	data, err := os.ReadFile(selinuxEnforcePath)
	return selinuxEnforcingFromContent(data, err)
}

// NewHostConfigWithMounts creates a host config with resource limits applied
// and the given bind mounts. The root filesystem is read-only; small tmpfs
// mounts cover /tmp, /var/tmp and /home/sandbox. It sets no tmpfs at the
// workspace path: the workspace is a host-built directory.
//
// Structured mounts (not Binds strings) are used with CreateMountpoint=false
// so a missing host source fails fast instead of being silently created as a
// root-owned directory. Every HostPath is validated (absolute, exists, not the
// filesystem root) and symlink-resolved before use, so a failure happens before
// any container is created.
//
// On SELinux-enforcing hosts, label=disable is added to SecurityOpt so the
// user's directories are not relabeled.
func NewHostConfigWithMounts(limits ResourceLimits, mounts []Mount) (*container.HostConfig, error) {
	return newHostConfigWithMounts(limits, mounts, hostSELinuxEnforcing())
}

// newHostConfigWithMounts is NewHostConfigWithMounts with the SELinux decision
// injected so it can be tested deterministically.
func newHostConfigWithMounts(limits ResourceLimits, mounts []Mount, selinuxEnforcing bool) (*container.HostConfig, error) {
	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			CPUQuota:  limits.CPUQuota,
			CPUPeriod: 100000,
			Memory:    limits.Memory,
		},
		NetworkMode: "none", // Disable network access for security
		// Only the bind mounts below and the tmpfs entries are writable.
		ReadonlyRootfs: true,
		Tmpfs:          tmpfsMounts(),
	}

	for _, m := range mounts {
		hostPath, err := validateMountHostPath(m.HostPath)
		if err != nil {
			return nil, err
		}
		if err := validateMountContainerPath(m.ContainerPath); err != nil {
			return nil, err
		}
		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type:        mount.TypeBind,
			Source:      hostPath,
			Target:      filepath.ToSlash(filepath.Clean(m.ContainerPath)),
			ReadOnly:    m.ReadOnly,
			BindOptions: &mount.BindOptions{CreateMountpoint: false},
		})
	}

	if selinuxEnforcing {
		hostConfig.SecurityOpt = append(hostConfig.SecurityOpt, "label=disable")
	}
	return hostConfig, nil
}

// validateMountHostPath requires an absolute, existing, non-root host path and
// returns it with symlinks resolved (Docker Desktop and Podman machine only
// share real paths).
func validateMountHostPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("mount host path is empty")
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("mount host path %q must be absolute", p)
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("mount host path %q does not exist", p)
		}
		return "", fmt.Errorf("resolving mount host path %q: %w", p, err)
	}
	if !filepath.IsAbs(resolved) {
		return "", fmt.Errorf("mount host path %q resolved to non-absolute %q", p, resolved)
	}
	if filepath.Dir(resolved) == resolved {
		return "", fmt.Errorf("mount host path %q must not be the filesystem root", p)
	}
	return resolved, nil
}

// validateMountContainerPath requires an absolute, non-root container path.
func validateMountContainerPath(p string) error {
	if p == "" {
		return fmt.Errorf("mount container path is empty")
	}
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("mount container path %q must be absolute", p)
	}
	if filepath.Clean(p) == "/" {
		return fmt.Errorf("mount container path %q must not be the container root", p)
	}
	return nil
}

// WithOpenUmask wraps cmd so it runs under umask 000. Files and directories the
// agent creates in the mounted workspace are then world-accessible, so the host
// user can read them for scoring and delete them afterwards even when the
// container uid maps to a foreign host uid. It is a process wrapper, not a
// content install; "$@" preserves every argument verbatim.
func WithOpenUmask(cmd []string) []string {
	wrapped := make([]string, 0, len(cmd)+4)
	wrapped = append(wrapped, "sh", "-c", `umask 000; exec "$@"`, "kairon-exec")
	return append(wrapped, cmd...)
}
