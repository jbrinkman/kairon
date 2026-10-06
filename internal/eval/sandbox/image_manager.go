package sandbox

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// ImageManager handles evaluation-scoped image building and reuse
type ImageManager struct {
	mu           sync.Mutex
	client       *client.Client
	builtImages  map[string]string // sha256(dockerfile+platform) -> imageName
	evaluationID string
	debugMode    bool
}

// NewImageManager creates a new ImageManager for an evaluation run
func NewImageManager(evaluationID string, debugMode bool) (*ImageManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating Docker client: %w", err)
	}

	// Verify Docker is running
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		cli.Close()
		return nil, daemonNotRunningError(err)
	}

	return &ImageManager{
		client:       cli,
		builtImages:  make(map[string]string),
		evaluationID: evaluationID,
		debugMode:    debugMode,
	}, nil
}

// BuildForEvaluation builds an image once per evaluation run and returns the cached name
func (im *ImageManager) BuildForEvaluation(ctx context.Context, dockerfile, platform string) (string, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	// Create cache key from full dockerfile content hash and platform
	hash := sha256.Sum256([]byte(platform + ":" + dockerfile))
	cacheKey := fmt.Sprintf("%x", hash[:8])

	// Check if already built
	if imageName, exists := im.builtImages[cacheKey]; exists {
		if im.debugMode {
			fmt.Printf("🔧 Debug: Reusing cached image %s for platform %s\n", imageName, platform)
		}
		return imageName, nil
	}

	// Generate evaluation-scoped image name
	imageName := im.generateImageName(platform)

	if im.debugMode {
		fmt.Printf("🔧 Debug: Building new image %s for evaluation %s on platform %s\n",
			imageName, im.evaluationID, platform)
	}

	// Build the image directly (bypasses imageManager check to prevent recursion)
	container, err := NewContainerWithDebug("", im.debugMode)
	if err != nil {
		return "", fmt.Errorf("creating container for image build: %w", err)
	}
	defer container.Close()

	if err := container.buildImageDirect(ctx, dockerfile, imageName, platform); err != nil {
		return "", fmt.Errorf("building image %s: %w", imageName, err)
	}

	// Cache the built image
	im.builtImages[cacheKey] = imageName

	if im.debugMode {
		fmt.Printf("✅ Image %s built and cached for evaluation %s\n", imageName, im.evaluationID)
	}

	return imageName, nil
}

// generateImageName creates evaluation-scoped image name with platform support
func (im *ImageManager) generateImageName(platform string) string {
	safePlatform := strings.ReplaceAll(platform, "/", "-")

	if im.debugMode {
		return fmt.Sprintf("kairon-eval-debug:%s-%s", im.evaluationID, safePlatform)
	}
	return fmt.Sprintf("kairon-eval:%s-%s", im.evaluationID, safePlatform)
}

// Cleanup removes all built images for this evaluation (preserves in debug mode)
func (im *ImageManager) Cleanup(ctx context.Context) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	if im.debugMode {
		fmt.Printf("🔧 Debug: Preserving %d images for evaluation %s\n",
			len(im.builtImages), im.evaluationID)
		return nil
	}

	var errors []string
	for _, imageName := range im.builtImages {
		if _, err := im.client.ImageRemove(ctx, imageName, image.RemoveOptions{Force: false, PruneChildren: true}); err != nil {
			errors = append(errors, fmt.Sprintf("failed to remove image %s: %v", imageName, err))
		} else {
			fmt.Printf("🧹 Cleaned up image %s\n", imageName)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Close closes the Docker client connection
func (im *ImageManager) Close() error {
	return im.client.Close()
}
