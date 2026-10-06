package sandbox

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pingContainerDaemon, podmanSocketCandidates, existingSockets, the
// daemonPingTimeout constant and the pingFunc type now live in daemon.go
// (production code) so the eval runner and sandbox constructors share one
// Podman-aware detection path. The test-only helpers below build on them.

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

// --- Production detection (daemon.go) unit tests ---

func TestDiscoverDaemonHost_DefaultEndpointAnswers(t *testing.T) {
	host, err := discoverDaemonHost(
		func(string) error { return nil },
		[]string{"/unused.sock"},
		false,
	)
	require.NoError(t, err)
	assert.Empty(t, host, "no DOCKER_HOST override when the default endpoint answers")
}

func TestDiscoverDaemonHost_SelectsFirstLivePodmanSocket(t *testing.T) {
	host, err := discoverDaemonHost(
		func(h string) error {
			if h == "unix:///run/podman/podman.sock" {
				return nil
			}
			return fmt.Errorf("unreachable: %s", h)
		},
		[]string{"/run/user/1000/podman/podman.sock", "/run/podman/podman.sock"},
		false,
	)
	require.NoError(t, err)
	assert.Equal(t, "unix:///run/podman/podman.sock", host)
}

func TestDiscoverDaemonHost_ExplicitDockerHostSkipsProbing(t *testing.T) {
	var probed []string
	host, err := discoverDaemonHost(
		func(h string) error {
			probed = append(probed, h)
			return fmt.Errorf("down")
		},
		[]string{"/run/podman/podman.sock"},
		true, // DOCKER_HOST is set
	)
	require.Error(t, err)
	assert.Empty(t, host)
	assert.Equal(t, []string{""}, probed, "an explicit DOCKER_HOST must not trigger candidate probing")
}

func TestDiscoverDaemonHost_NothingReachable(t *testing.T) {
	host, err := discoverDaemonHost(
		func(string) error { return fmt.Errorf("down") },
		[]string{"/run/podman/podman.sock"},
		false,
	)
	require.Error(t, err)
	assert.Empty(t, host)
}

func TestDaemonNotRunningError_NamesBothRuntimesAndWraps(t *testing.T) {
	cause := fmt.Errorf("boom")
	err := DaemonNotRunningError(cause)
	assert.Contains(t, err.Error(), "Podman")
	assert.Contains(t, err.Error(), "Docker")
	assert.ErrorIs(t, err, cause, "underlying error must stay unwrappable via errors.Is")
}
