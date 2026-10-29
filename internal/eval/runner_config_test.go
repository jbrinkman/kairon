package eval

import (
	"testing"
	"time"

	"github.com/jbrinkman/kairon/internal/config"
	"github.com/jbrinkman/kairon/internal/eval/sandbox"
)

func TestCreateContainerConfig_WithSandboxConfig(t *testing.T) {
	tests := []struct {
		name           string
		sandboxCfg     *config.SandboxConfig
		resourceLimits map[string]string
		debug          bool
		expectConfig   ContainerConfig
	}{
		{
			name: "complete sandbox config",
			sandboxCfg: &config.SandboxConfig{
				WorkspaceDir: "/app",
				CPUCores:     2.0,
				MemoryMB:     2048,
				Timeout:      10 * time.Minute,
			},
			resourceLimits: nil,
			expectConfig: ContainerConfig{
				WorkspaceDir: "/app",
				Platform:     "", // Will be set by platform detection
				Environment: map[string]string{
					"KIRO_CLI_DISABLE_TELEMETRY": "1",
				},
				ResourceLimits: sandbox.ResourceLimits{
					CPUQuota: 2000000,            // 2.0 cores * 1,000,000 microseconds
					Memory:   2048 * 1024 * 1024, // 2048 MB in bytes
					Timeout:  10 * time.Minute,
				},
			},
		},
		{
			name: "partial sandbox config",
			sandboxCfg: &config.SandboxConfig{
				MemoryMB: 512,
			},
			resourceLimits: nil,
			expectConfig: ContainerConfig{
				WorkspaceDir: "/workspace", // Default
				Platform:     "",           // Will be set by platform detection
				Environment: map[string]string{
					"KIRO_CLI_DISABLE_TELEMETRY": "1",
				},
				ResourceLimits: sandbox.ResourceLimits{
					CPUQuota: 1000000,           // Default 1.0 cores
					Memory:   512 * 1024 * 1024, // 512 MB in bytes
					Timeout:  5 * time.Minute,   // Default timeout
				},
			},
		},
		{
			name: "minimal sandbox config",
			sandboxCfg: &config.SandboxConfig{
				CPUCores: 0.5,
			},
			resourceLimits: nil,
			expectConfig: ContainerConfig{
				WorkspaceDir: "/workspace", // Default
				Platform:     "",           // Will be set by platform detection
				Environment: map[string]string{
					"KIRO_CLI_DISABLE_TELEMETRY": "1",
				},
				ResourceLimits: sandbox.ResourceLimits{
					CPUQuota: 500000,             // 0.5 cores * 1,000,000 microseconds
					Memory:   1024 * 1024 * 1024, // Default 1024 MB
					Timeout:  5 * time.Minute,    // Default timeout
				},
			},
		},
		{
			// A nil sandbox config yields every default.
			name:           "nil sandbox config",
			sandboxCfg:     nil,
			resourceLimits: nil,
			expectConfig: ContainerConfig{
				WorkspaceDir: "/workspace",
				Platform:     "", // Will be set by platform detection
				Environment: map[string]string{
					"KIRO_CLI_DISABLE_TELEMETRY": "1",
				},
				ResourceLimits: sandbox.ResourceLimits{
					CPUQuota: 1000000,            // 1.0 cores * 1,000,000 microseconds
					Memory:   1024 * 1024 * 1024, // 1024 MB in bytes
					Timeout:  5 * time.Minute,
				},
			},
		},
		{
			// Empty string values in the sandbox config fall back to defaults.
			name: "empty workspace dir",
			sandboxCfg: &config.SandboxConfig{
				WorkspaceDir: "", // Empty - should use default
				CPUCores:     2.0,
				MemoryMB:     1024,
				Timeout:      5 * time.Minute,
			},
			resourceLimits: nil,
			expectConfig: ContainerConfig{
				WorkspaceDir: "/workspace", // Default
				Platform:     "",           // Will be set by platform detection
				Environment: map[string]string{
					"KIRO_CLI_DISABLE_TELEMETRY": "1",
				},
				ResourceLimits: sandbox.ResourceLimits{
					CPUQuota: 2000000,            // 2.0 cores * 1,000,000 microseconds
					Memory:   1024 * 1024 * 1024, // 1024 MB in bytes
					Timeout:  5 * time.Minute,
				},
			},
		},
		{
			// Zero values in the sandbox config fall back to defaults.
			name: "zero cpu memory timeout",
			sandboxCfg: &config.SandboxConfig{
				WorkspaceDir: "/custom",
				CPUCores:     0, // Zero - should use default
				MemoryMB:     0, // Zero - should use default
				Timeout:      0, // Zero - should use default
			},
			resourceLimits: nil,
			expectConfig: ContainerConfig{
				WorkspaceDir: "/custom",
				Platform:     "", // Will be set by platform detection
				Environment: map[string]string{
					"KIRO_CLI_DISABLE_TELEMETRY": "1",
				},
				ResourceLimits: sandbox.ResourceLimits{
					CPUQuota: 1000000,            // Default 1.0 cores
					Memory:   1024 * 1024 * 1024, // Default 1024 MB
					Timeout:  5 * time.Minute,    // Default timeout
				},
			},
		},
		{
			// The debug flag is propagated to the container config.
			name:           "debug enabled",
			sandboxCfg:     nil,
			resourceLimits: nil,
			debug:          true,
			expectConfig: ContainerConfig{
				WorkspaceDir: "/workspace",
				Platform:     "", // Will be set by platform detection
				Environment: map[string]string{
					"KIRO_CLI_DISABLE_TELEMETRY": "1",
				},
				ResourceLimits: sandbox.ResourceLimits{
					CPUQuota: 1000000,            // 1.0 cores * 1,000,000 microseconds
					Memory:   1024 * 1024 * 1024, // 1024 MB in bytes
					Timeout:  5 * time.Minute,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createContainerConfig(tt.sandboxCfg, tt.resourceLimits, tt.debug)

			// Compare all fields except Platform (which is dynamically detected)
			if result.WorkspaceDir != tt.expectConfig.WorkspaceDir {
				t.Errorf("WorkspaceDir = %s, expected %s", result.WorkspaceDir, tt.expectConfig.WorkspaceDir)
			}
			if result.ResourceLimits.CPUQuota != tt.expectConfig.ResourceLimits.CPUQuota {
				t.Errorf("CPUQuota = %d, expected %d", result.ResourceLimits.CPUQuota, tt.expectConfig.ResourceLimits.CPUQuota)
			}
			if result.ResourceLimits.Memory != tt.expectConfig.ResourceLimits.Memory {
				t.Errorf("Memory = %d, expected %d", result.ResourceLimits.Memory, tt.expectConfig.ResourceLimits.Memory)
			}
			if result.ResourceLimits.Timeout != tt.expectConfig.ResourceLimits.Timeout {
				t.Errorf("Timeout = %v, expected %v", result.ResourceLimits.Timeout, tt.expectConfig.ResourceLimits.Timeout)
			}
			// Verify the debug flag is propagated (true only when requested)
			if result.Debug != tt.debug {
				t.Errorf("Debug = %v, expected %v", result.Debug, tt.debug)
			}
			// Verify platform is set (non-empty)
			if result.Platform == "" {
				t.Error("Platform should be detected and set")
			}
			// Verify environment is set correctly
			if len(result.Environment) != len(tt.expectConfig.Environment) {
				t.Errorf("Environment length = %d, expected %d", len(result.Environment), len(tt.expectConfig.Environment))
			}
			for k, v := range tt.expectConfig.Environment {
				if result.Environment[k] != v {
					t.Errorf("Environment[%s] = %s, expected %s", k, result.Environment[k], v)
				}
			}
		})
	}
}

func TestCreateContainerConfig_ResourceLimitOverrides(t *testing.T) {
	sandboxCfg := &config.SandboxConfig{
		WorkspaceDir: "/app",
		CPUCores:     2.0,
		MemoryMB:     2048,
		Timeout:      10 * time.Minute,
	}

	tests := []struct {
		name           string
		resourceLimits map[string]string
		expectCPU      int64
		expectMemory   int64
		expectTimeout  time.Duration
	}{
		{
			name: "cpu override",
			resourceLimits: map[string]string{
				"cpu": "4.0",
			},
			expectCPU:     4000000,            // 4.0 * 1,000,000
			expectMemory:  2048 * 1024 * 1024, // Original value
			expectTimeout: 10 * time.Minute,   // Original value
		},
		{
			name: "memory override",
			resourceLimits: map[string]string{
				"memory": "4294967296", // 4GB in bytes
			},
			expectCPU:     2000000,          // Original value
			expectMemory:  4294967296,       // 4GB
			expectTimeout: 10 * time.Minute, // Original value
		},
		{
			name: "timeout override",
			resourceLimits: map[string]string{
				"timeout": "30m",
			},
			expectCPU:     2000000,            // Original value
			expectMemory:  2048 * 1024 * 1024, // Original value
			expectTimeout: 30 * time.Minute,
		},
		{
			name: "all overrides",
			resourceLimits: map[string]string{
				"cpu":     "0.5",
				"memory":  "536870912", // 512MB in bytes
				"timeout": "2m",
			},
			expectCPU:     500000,    // 0.5 * 1,000,000
			expectMemory:  536870912, // 512MB
			expectTimeout: 2 * time.Minute,
		},
		{
			name: "invalid values ignored",
			resourceLimits: map[string]string{
				"cpu":     "invalid",
				"memory":  "not-a-number",
				"timeout": "bad-duration",
			},
			expectCPU:     2000000, // Original values maintained
			expectMemory:  2048 * 1024 * 1024,
			expectTimeout: 10 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createContainerConfig(sandboxCfg, tt.resourceLimits, false)

			if result.ResourceLimits.CPUQuota != tt.expectCPU {
				t.Errorf("CPUQuota = %d, expected %d", result.ResourceLimits.CPUQuota, tt.expectCPU)
			}
			if result.ResourceLimits.Memory != tt.expectMemory {
				t.Errorf("Memory = %d, expected %d", result.ResourceLimits.Memory, tt.expectMemory)
			}
			if result.ResourceLimits.Timeout != tt.expectTimeout {
				t.Errorf("Timeout = %v, expected %v", result.ResourceLimits.Timeout, tt.expectTimeout)
			}
		})
	}
}
