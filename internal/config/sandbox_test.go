package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoad_SandboxConfigDefaults(t *testing.T) {
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

	expected := SandboxConfig{
		WorkspaceDir: "/workspace",
		CPUCores:     1.0,
		MemoryMB:     1024,
		Timeout:      5 * time.Minute,
	}

	if cfg.Sandbox != expected {
		t.Errorf("SandboxConfig = %+v, expected %+v", cfg.Sandbox, expected)
	}
}

func TestLoad_SandboxConfigParsing(t *testing.T) {
	tests := []struct {
		name           string
		configContent  string
		expectedConfig SandboxConfig
	}{
		{
			name: "complete sandbox config",
			configContent: `repo: test/repo
sandbox:
  workspace_dir: /app
  cpu_cores: 2.0
  memory_mb: 2048
  timeout: 10m`,
			expectedConfig: SandboxConfig{
				WorkspaceDir: "/app",
				CPUCores:     2.0,
				MemoryMB:     2048,
				Timeout:      10 * time.Minute,
			},
		},
		{
			name: "partial sandbox config with defaults",
			configContent: `repo: test/repo
sandbox:
  memory_mb: 512`,
			expectedConfig: SandboxConfig{
				WorkspaceDir: "/workspace",
				CPUCores:     1.0,
				MemoryMB:     512,
				Timeout:      5 * time.Minute,
			},
		},
		{
			name: "empty sandbox section uses defaults",
			configContent: `repo: test/repo
sandbox:`,
			expectedConfig: SandboxConfig{
				WorkspaceDir: "/workspace",
				CPUCores:     1.0,
				MemoryMB:     1024,
				Timeout:      5 * time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configDir := tmpDir + string(os.PathSeparator) + ".kairon"
			if err := os.Mkdir(configDir, 0755); err != nil {
				t.Fatalf("Failed to create config dir: %v", err)
			}

			configFile := configDir + string(os.PathSeparator) + "config.yaml"
			if err := os.WriteFile(configFile, []byte(tt.configContent), 0644); err != nil {
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

			if cfg.Sandbox != tt.expectedConfig {
				t.Errorf("SandboxConfig = %+v, expected %+v", cfg.Sandbox, tt.expectedConfig)
			}
		})
	}
}

func TestLoad_SandboxConfigValidation(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		expectError   string
	}{
		{
			name: "negative cpu_cores",
			configContent: `repo: test/repo
sandbox:
  cpu_cores: -1.0`,
			expectError: "sandbox.cpu_cores must be greater than 0",
		},
		{
			name: "zero cpu_cores",
			configContent: `repo: test/repo
sandbox:
  cpu_cores: 0`,
			expectError: "sandbox.cpu_cores must be greater than 0",
		},
		{
			name: "negative memory_mb",
			configContent: `repo: test/repo
sandbox:
  memory_mb: -512`,
			expectError: "sandbox.memory_mb must be greater than 0",
		},
		{
			name: "zero memory_mb",
			configContent: `repo: test/repo
sandbox:
  memory_mb: 0`,
			expectError: "sandbox.memory_mb must be greater than 0",
		},
		{
			name: "memory_mb below minimum",
			configContent: `repo: test/repo
sandbox:
  memory_mb: 128`,
			expectError: "sandbox.memory_mb must be at least 256",
		},
		{
			name: "negative timeout",
			configContent: `repo: test/repo
sandbox:
  timeout: -5m`,
			expectError: "sandbox.timeout must be greater than 0",
		},
		{
			name: "zero timeout",
			configContent: `repo: test/repo
sandbox:
  timeout: 0s`,
			expectError: "sandbox.timeout must be greater than 0",
		},
		{
			name: "empty workspace_dir",
			configContent: `repo: test/repo
sandbox:
  workspace_dir: ""`,
			expectError: "sandbox.workspace_dir cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configDir := tmpDir + string(os.PathSeparator) + ".kairon"
			if err := os.Mkdir(configDir, 0755); err != nil {
				t.Fatalf("Failed to create config dir: %v", err)
			}

			configFile := configDir + string(os.PathSeparator) + "config.yaml"
			if err := os.WriteFile(configFile, []byte(tt.configContent), 0644); err != nil {
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

			_, err = Load()
			if err == nil {
				t.Fatalf("Expected error containing %q, but got no error", tt.expectError)
			}
			if !containsError(err.Error(), tt.expectError) {
				t.Errorf("Expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestLoad_DurationParsing(t *testing.T) {
	tests := []struct {
		name            string
		timeoutValue    string
		expectedTimeout time.Duration
	}{
		{
			name:            "minutes format",
			timeoutValue:    "5m",
			expectedTimeout: 5 * time.Minute,
		},
		{
			name:            "seconds format",
			timeoutValue:    "300s",
			expectedTimeout: 300 * time.Second,
		},
		{
			name:            "hours format",
			timeoutValue:    "1h",
			expectedTimeout: time.Hour,
		},
		{
			name:            "mixed format",
			timeoutValue:    "1h30m",
			expectedTimeout: time.Hour + 30*time.Minute,
		},
		{
			name:            "nanoseconds",
			timeoutValue:    "1000000000ns",
			expectedTimeout: time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configDir := tmpDir + string(os.PathSeparator) + ".kairon"
			if err := os.Mkdir(configDir, 0755); err != nil {
				t.Fatalf("Failed to create config dir: %v", err)
			}

			configContent := `repo: test/repo
sandbox:
  timeout: ` + tt.timeoutValue
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

			if cfg.Sandbox.Timeout != tt.expectedTimeout {
				t.Errorf("Timeout = %v, expected %v", cfg.Sandbox.Timeout, tt.expectedTimeout)
			}
		})
	}
}

func containsError(actual, expected string) bool {
	return strings.Contains(actual, expected)
}

func TestValidateSandboxWorkspaceDir(t *testing.T) {
	cases := []struct {
		name    string
		dir     string
		wantErr string // substring; "" means no error
	}{
		{"valid default", "/workspace", ""},
		{"valid nested", "/srv/kairon/ws", ""},
		{"collides with tmp", "/tmp", "collides with the container's tmpfs mount at /tmp"},
		{"nested under tmp", "/tmp/ws", "collides with the container's tmpfs mount at /tmp"},
		{"collides with var tmp", "/var/tmp", "collides with the container's tmpfs mount at /var/tmp"},
		{"collides with home", "/home/sandbox", "collides with the container's tmpfs mount at /home/sandbox"},
		{"nested under home", "/home/sandbox/work", "collides with the container's tmpfs mount at /home/sandbox"},
		{"trailing slash normalized", "/tmp/", "collides with the container's tmpfs mount at /tmp"},
		{"relative rejected", "workspace", "must be an absolute path"},
		{"root rejected", "/", "must not be the container root"},
		{"tmp-prefixed sibling is fine", "/tmpfs-data", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSandboxWorkspaceDir(tc.dir)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSandboxWorkspaceDir(%q) = %v, want nil", tc.dir, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateSandboxWorkspaceDir(%q) = %v, want error containing %q", tc.dir, err, tc.wantErr)
			}
		})
	}
}
