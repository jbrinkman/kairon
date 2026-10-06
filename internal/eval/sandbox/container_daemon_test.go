package sandbox

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// daemonPingTimeout bounds every daemon reachability probe.
const daemonPingTimeout = 2 * time.Second

// pingFunc checks whether a container daemon is reachable. An empty host means
// "use the environment" (client.FromEnv, i.e. DOCKER_HOST or the default
// Docker socket); a non-empty host is an explicit endpoint such as
// unix:///path/to/podman.sock.
type pingFunc func(host string) error

// pingContainerDaemon is the production pingFunc: it pings the daemon through
// the Docker SDK (which also speaks to Podman's Docker-compatible API) with
// API version negotiation and a 2s timeout. The podman binary is never used.
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

// existingSockets filters candidates down to paths that exist on disk, so that
// absent sockets do not cost a ping timeout.
func existingSockets(candidates []string) []string {
	var found []string
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && fi.Mode()&os.ModeSocket != 0 {
			found = append(found, p)
		}
	}
	return found
}

// ensureContainerDaemon makes a container daemon reachable for the current
// test. If DOCKER_HOST is set, or the default endpoint answers, nothing
// changes. Otherwise the candidate Podman sockets are probed in order and the
// first that answers is exported via t.Setenv("DOCKER_HOST", "unix://<path>")
// so production constructors (which use client.FromEnv) reach the same daemon.
// It returns a non-nil error when no daemon is reachable.
func ensureContainerDaemon(t testing.TB, ping pingFunc, candidates []string) error {
	t.Helper()

	envErr := ping("")
	if envErr == nil {
		return nil
	}
	if os.Getenv("DOCKER_HOST") != "" {
		// An explicit endpoint was requested; do not second-guess it.
		return envErr
	}

	lastErr := envErr
	for _, path := range candidates {
		host := "unix://" + path
		err := ping(host)
		if err == nil {
			t.Setenv("DOCKER_HOST", host)
			return nil
		}
		lastErr = err
	}
	return lastErr
}

// noContainerDaemonMessage is the skip message used when nothing is reachable.
func noContainerDaemonMessage(err error) string {
	return fmt.Sprintf("no container daemon reachable (tried Podman and Docker); start Podman or Docker to run this test: %v", err)
}

// skipIfNoContainerDaemon skips the test unless a Podman or Docker daemon is
// reachable. A Podman socket found at a well-known path is exported through
// DOCKER_HOST for the duration of the test. The podman binary is not required.
func skipIfNoContainerDaemon(t *testing.T) {
	t.Helper()
	candidates := existingSockets(podmanSocketCandidates(runtime.GOOS, os.Getenv, os.Getuid()))
	if err := ensureContainerDaemon(t, pingContainerDaemon, candidates); err != nil {
		t.Skip(noContainerDaemonMessage(err))
	}
}

func TestPodmanSocketCandidates_Linux(t *testing.T) {
	env := map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}
	got := podmanSocketCandidates("linux", func(k string) string { return env[k] }, 1000)
	assert.Equal(t, []string{
		"/run/user/1000/podman/podman.sock",
		"/run/podman/podman.sock",
	}, got)
}

func TestPodmanSocketCandidates_LinuxNoXDGFallsBackToUID(t *testing.T) {
	got := podmanSocketCandidates("linux", func(string) string { return "" }, 1234)
	assert.Equal(t, []string{
		"/run/user/1234/podman/podman.sock",
		"/run/podman/podman.sock",
	}, got)
}

func TestPodmanSocketCandidates_LinuxRoot(t *testing.T) {
	got := podmanSocketCandidates("linux", func(string) string { return "" }, 0)
	assert.Equal(t, []string{"/run/podman/podman.sock"}, got)
}

func TestPodmanSocketCandidates_Darwin(t *testing.T) {
	env := map[string]string{"TMPDIR": "/var/folders/ab/xyz/T/", "HOME": "/Users/alice"}
	got := podmanSocketCandidates("darwin", func(k string) string { return env[k] }, 501)
	assert.Equal(t, []string{
		"/var/folders/ab/xyz/T/podman/podman-machine-default-api.sock",
		"/Users/alice/.local/share/containers/podman/machine/podman.sock",
		"/Users/alice/.local/share/containers/podman/machine/podman-machine-default/podman.sock",
	}, got)
}

func TestPodmanSocketCandidates_UnknownOS(t *testing.T) {
	assert.Empty(t, podmanSocketCandidates("plan9", func(string) string { return "x" }, 1))
}

func TestExistingSockets_FiltersMissingAndNonSockets(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	require.NoError(t, os.WriteFile(regular, []byte("x"), 0o600))
	assert.Empty(t, existingSockets([]string{filepath.Join(dir, "missing"), regular}))
}

// startFakePodmanSocket serves a minimal Docker-compatible /_ping endpoint on a
// unix socket and returns its path. A short /tmp path is used because unix
// socket paths are limited to ~104 bytes on macOS.
func startFakePodmanSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "kpm")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	path := filepath.Join(dir, "podman.sock")
	ln, err := net.Listen("unix", path)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Api-Version", "1.41")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("OK"))
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return path
}

// noDefaultDaemon simulates a host where the default/env endpoint is dead but
// explicit endpoints are pinged for real, keeping tests independent of the host.
func noDefaultDaemon(host string) error {
	if host == "" {
		return fmt.Errorf("default endpoint unreachable")
	}
	return pingContainerDaemon(host)
}

func TestPodmanDiscovery_SelectsFakePodmanSocket(t *testing.T) {
	sock := startFakePodmanSocket(t)
	t.Setenv("DOCKER_HOST", "") // treated as unset; restored at test end

	// Dead candidate first proves ordering/fallthrough; the live one must win.
	dead := filepath.Join(t.TempDir(), "dead.sock")
	candidates := existingSockets([]string{dead, sock})
	require.Equal(t, []string{sock}, candidates)

	err := ensureContainerDaemon(t, noDefaultDaemon, candidates)
	require.NoError(t, err)
	assert.Equal(t, "unix://"+sock, os.Getenv("DOCKER_HOST"))

	// The production client path (FromEnv) now reaches the fake Podman daemon.
	assert.NoError(t, pingContainerDaemon(""))
}

func TestPodmanDiscovery_DeadCandidateIsNotSelected(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	dead := filepath.Join(t.TempDir(), "dead.sock")

	err := ensureContainerDaemon(t, noDefaultDaemon, []string{dead})
	require.Error(t, err)
	assert.Empty(t, os.Getenv("DOCKER_HOST"))
}

func TestPodmanDiscovery_DefaultEndpointWins(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	var pinged []string
	ping := func(host string) error {
		pinged = append(pinged, host)
		return nil
	}
	require.NoError(t, ensureContainerDaemon(t, ping, []string{"/unused.sock"}))
	assert.Equal(t, []string{""}, pinged)
	assert.Empty(t, os.Getenv("DOCKER_HOST"))
}

func TestPodmanDiscovery_ExplicitDockerHostIsNotOverridden(t *testing.T) {
	sock := startFakePodmanSocket(t)
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-kairon.sock")

	err := ensureContainerDaemon(t, noDefaultDaemon, []string{sock})
	require.Error(t, err)
	assert.Equal(t, "unix:///nonexistent-kairon.sock", os.Getenv("DOCKER_HOST"))
}

func TestSkipIfNoContainerDaemon_MessageNamesPodmanAndDocker(t *testing.T) {
	msg := noContainerDaemonMessage(fmt.Errorf("boom"))
	assert.Contains(t, msg, "Podman")
	assert.Contains(t, msg, "Docker")
	assert.True(t, strings.Contains(msg, "boom"), "underlying error should be included")
}
