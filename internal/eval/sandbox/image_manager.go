package sandbox

import (
	"fmt"
	"sync"

	"github.com/docker/docker/client"
)

// ImageManager builds and reuses the persistent base image (see EnsureBaseImage).
// It never removes images: the base image is cached across runs.
type ImageManager struct {
	mu           sync.Mutex
	client       *client.Client
	evaluationID string
	debugMode    bool
}

// NewImageManager creates a new ImageManager. evaluationID is informational
// (it labels debug output); it does not participate in image tags.
func NewImageManager(evaluationID string, debugMode bool) (*ImageManager, error) {
	// Resolve a reachable container daemon (Docker or Podman) first; this may
	// export DOCKER_HOST for a discovered Podman socket so the client below
	// connects to it.
	if err := EnsureContainerDaemon(); err != nil {
		return nil, DaemonNotRunningError(err)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating Docker client: %w", err)
	}

	return &ImageManager{
		client:       cli,
		evaluationID: evaluationID,
		debugMode:    debugMode,
	}, nil
}

// Close closes the Docker client connection
func (im *ImageManager) Close() error {
	return im.client.Close()
}
