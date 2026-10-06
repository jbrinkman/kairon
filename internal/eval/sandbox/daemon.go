package sandbox

import "fmt"

// daemonNotRunningError returns the user-facing error used when the container
// daemon cannot be reached. The message names both Podman and Docker because
// either runtime can serve the Docker-compatible API. The underlying error is
// wrapped with %w so callers can still inspect it with errors.Is/As.
func daemonNotRunningError(err error) error {
	return fmt.Errorf("container daemon is not running. Start Podman or Docker and try again: %w", err)
}
