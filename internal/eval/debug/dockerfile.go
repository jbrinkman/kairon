package debug

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SaveDockerfile saves the Dockerfile to debug artifacts directory with timestamp
func SaveDockerfile(dockerfileContent, containerID string) error {
	artifactDir := filepath.Join(".kairon", "evals", "tmp", "dockerfiles")

	// Ensure directory exists
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return fmt.Errorf("creating dockerfiles directory: %w", err)
	}

	// Generate filename with timestamp and container short ID
	timestamp := time.Now().Format("20060102-150405")
	shortID := containerID
	if len(shortID) > 7 {
		shortID = shortID[:7]
	}
	filename := fmt.Sprintf("dockerfile-%s-%s", timestamp, shortID)

	filePath := filepath.Join(artifactDir, filename)

	// Write Dockerfile content
	if err := os.WriteFile(filePath, []byte(dockerfileContent), 0644); err != nil {
		return fmt.Errorf("writing dockerfile to %s: %w", filePath, err)
	}

	// Prune old artifacts
	_ = CleanOldDockerfiles()

	fmt.Printf("🔧 Debug: Dockerfile saved to %s\n", filePath)
	return nil
}

// CleanOldDockerfiles removes old Dockerfile artifacts (keep last 50 or 7 days)
func CleanOldDockerfiles() error {
	artifactDir := filepath.Join(".kairon", "evals", "tmp", "dockerfiles")

	entries, err := os.ReadDir(artifactDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to clean
		}
		return fmt.Errorf("reading dockerfiles directory: %w", err)
	}

	// Simple cleanup: if more than 50 files, remove oldest
	if len(entries) > 50 {
		// Remove first (oldest) files
		for i := 0; i < len(entries)-50; i++ {
			filePath := filepath.Join(artifactDir, entries[i].Name())
			if err := os.Remove(filePath); err != nil {
				fmt.Printf("⚠️ Warning: Failed to remove old dockerfile %s: %v\n", filePath, err)
			}
		}
	}

	return nil
}
