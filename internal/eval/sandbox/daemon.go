package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/docker/docker/client"
)

// daemonPingTimeout bounds every daemon reachability probe.
const daemonPingTimeout = 2 * time.Second

// daemonResolveOnce ensures process-level Podman socket discovery runs at most
// once: the first caller that discovers a socket exports it through DOCKER_HOST,
// and every later client.FromEnv (constructors, the eval runner's pre-flight
// check) then reaches the same daemon without re-probing.
var daemonResolveOnce sync.Once

// pingFunc checks whether a container daemon is reachable. An empty host means
// "use the environment" (client.FromEnv, i.e. DOCKER_HOST or the default
// Docker socket); a non-empty host is an explicit endpoint such as
// unix:///path/to/podman.sock.
type pingFunc func(host string) error

// pingContainerDaemon pings the daemon through the Docker SDK (which also
// speaks Podman's Docker-compatible API) with API version negotiation and a 2s
// timeout. The podman binary is never used; only socket reachability matters.
func pingContainerDaemon(host string) error {
	opts := []client.Opt{client.WithAPIVersionNegotiation()}
	if host == "" {
		opts = append(opts, client.FromEnv)
	} else {
		opts = append(opts, client.WithHost(host))
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return fmt.Errorf("container client unavailable: %w", err)
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), daemonPingTimeout)
	defer cancel()
	_, err = cli.Ping(ctx)
	return err
}

// podmanSocketCandidates returns well-known Podman API socket paths for the
// given OS. It is pure (no filesystem or process-environment access) so it can
// be unit tested on any host.
func podmanSocketCandidates(goos string, getenv func(string) string, uid int) []string {
	var candidates []string
	switch goos {
	case "linux":
		if xdg := getenv("XDG_RUNTIME_DIR"); xdg != "" {
			candidates = append(candidates, filepath.Join(xdg, "podman", "podman.sock"))
		} else if uid > 0 {
			candidates = append(candidates, fmt.Sprintf("/run/user/%d/podman/podman.sock", uid))
		}
		candidates = append(candidates, "/run/podman/podman.sock")
	case "darwin":
		if tmp := getenv("TMPDIR"); tmp != "" {
			candidates = append(candidates, filepath.Join(tmp, "podman", "podman-machine-default-api.sock"))
		}
		if home := getenv("HOME"); home != "" {
			candidates = append(candidates,
				filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman.sock"),
				filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman-machine-default", "podman.sock"),
			)
		}
	}
	return candidates
}

// existingSockets filters candidates down to paths that exist on disk and are
// sockets, so that absent sockets do not cost a ping timeout.
func existingSockets(candidates []string) []string {
	var found []string
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && fi.Mode()&os.ModeSocket != 0 {
			found = append(found, p)
		}
	}
	return found
}

// discoverDaemonHost resolves a reachable container daemon endpoint. It returns
// ("", nil) when the environment/default endpoint already answers (nothing to
// do); ("unix://<path>", nil) when a Podman socket among candidates answers and
// should be exported through DOCKER_HOST; or ("", err) when nothing is
// reachable. An explicit DOCKER_HOST is never second-guessed: when it is set,
// the env ping result stands and no candidate is probed.
//
// It is pure with respect to its injected dependencies (ping, candidates,
// dockerHostSet) so it can be unit tested without touching the real host.
func discoverDaemonHost(ping pingFunc, candidates []string, dockerHostSet bool) (string, error) {
	envErr := ping("")
	if envErr == nil {
		return "", nil
	}
	if dockerHostSet {
		// An explicit endpoint was requested; do not probe alternatives.
		return "", envErr
	}

	lastErr := envErr
	for _, path := range candidates {
		host := "unix://" + path
		if err := ping(host); err == nil {
			return host, nil
		} else {
			lastErr = err
		}
	}
	return "", lastErr
}

// EnsureContainerDaemon makes a container daemon reachable for this process.
// If the environment/default endpoint already answers, it is a no-op. Otherwise
// it probes well-known Podman socket locations and, on the first that answers,
// exports it through DOCKER_HOST so every subsequent client.FromEnv (the
// sandbox constructors, the eval runner's pre-flight check) reaches the same
// daemon. Podman need not be on PATH. It returns a non-nil error, suitable to
// pass to DaemonNotRunningError, when no daemon is reachable.
//
// Discovery runs at most once per process; a later call re-pings the (possibly
// now-updated) environment so a daemon that came up afterwards is still seen.
func EnsureContainerDaemon() error {
	daemonResolveOnce.Do(func() {
		candidates := existingSockets(podmanSocketCandidates(runtime.GOOS, os.Getenv, os.Getuid()))
		host, err := discoverDaemonHost(pingContainerDaemon, candidates, os.Getenv("DOCKER_HOST") != "")
		if err == nil && host != "" {
			_ = os.Setenv("DOCKER_HOST", host)
		}
	})
	return pingContainerDaemon("")
}

// DaemonNotRunningError returns the user-facing error used when the container
// daemon cannot be reached. The message names both Podman and Docker because
// either runtime can serve the Docker-compatible API. The underlying error is
// wrapped with %w so callers can still inspect it with errors.Is/As.
func DaemonNotRunningError(err error) error {
	return fmt.Errorf("container daemon is not running. Start Podman or Docker and try again: %w", err)
}
