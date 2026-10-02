package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_EnableCopilotReviewDefaults(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		expectedValue bool
	}{
		{
			name: "enable_copilot_review not specified - should default to true",
			configContent: `repo: test/repo
label: test-label`,
			expectedValue: true,
		},
		{
			name: "enable_copilot_review explicitly set to true",
			configContent: `repo: test/repo
label: test-label
enable_copilot_review: true`,
			expectedValue: true,
		},
		{
			name: "enable_copilot_review explicitly set to false",
			configContent: `repo: test/repo
label: test-label
enable_copilot_review: false`,
			expectedValue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp config directory and file
			tmpDir := t.TempDir()
			configDir := tmpDir + string(os.PathSeparator) + ".kairon"
			if err := os.Mkdir(configDir, 0755); err != nil {
				t.Fatalf("Failed to create config dir: %v", err)
			}

			configFile := configDir + string(os.PathSeparator) + "config.yaml"
			if err := os.WriteFile(configFile, []byte(tt.configContent), 0644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			// Change to temp directory
			oldDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("Failed to get working directory: %v", err)
			}
			t.Cleanup(func() {
				if err := os.Chdir(oldDir); err != nil {
					t.Errorf("Failed to restore working directory: %v", err)
				}
			})
			if err := os.Chdir(tmpDir); err != nil {
				t.Fatalf("Failed to change directory: %v", err)
			}

			// Load config
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if cfg.EnableCopilotReview != tt.expectedValue {
				t.Errorf("EnableCopilotReview = %v, expected %v", cfg.EnableCopilotReview, tt.expectedValue)
			}
		})
	}
}

func TestLoad_AllDefaultValues(t *testing.T) {
	// Test that all default values are set correctly
	tmpDir := t.TempDir()
	configDir := tmpDir + string(os.PathSeparator) + ".kairon"
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configContent := `repo: test/repo`
	configFile := configDir + string(os.PathSeparator) + "config.yaml"
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Errorf("Failed to restore working directory: %v", err)
		}
	})
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify all default values
	if cfg.Label != "kairon" {
		t.Errorf("Label = %s, expected kairon", cfg.Label)
	}
	if cfg.PollInterval != 5*time.Minute {
		t.Errorf("PollInterval = %v, expected 5m0s", cfg.PollInterval)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, expected 3", cfg.MaxRetries)
	}
	if cfg.MaxActivityLines != 1000 {
		t.Errorf("MaxActivityLines = %d, expected 1000", cfg.MaxActivityLines)
	}
	if cfg.ConsoleLogging != false {
		t.Errorf("ConsoleLogging = %v, expected false", cfg.ConsoleLogging)
	}
	if cfg.Theme != "default" {
		t.Errorf("Theme = %s, expected default", cfg.Theme)
	}
	if cfg.EnableCopilotReview != true {
		t.Errorf("EnableCopilotReview = %v, expected true", cfg.EnableCopilotReview)
	}
}
