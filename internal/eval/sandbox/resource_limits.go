package sandbox

import (
	"time"

	"github.com/docker/docker/api/types/container"
)

// ResourceLimits defines container resource constraints
type ResourceLimits struct {
	CPUQuota int64         // CPU quota in microseconds (1 core = 1000000)
	Memory   int64         // Memory limit in bytes
	Timeout  time.Duration // Execution timeout
}

// DefaultLimits returns the default resource limits
func DefaultLimits() ResourceLimits {
	return ResourceLimits{
		CPUQuota: 1000000,           // 1.0 core
		Memory:   512 * 1024 * 1024, // 512MB
		Timeout:  5 * time.Minute,   // 5 minutes
	}
}

// ApplyToHostConfig applies resource limits to Docker host config
func (rl ResourceLimits) ApplyToHostConfig(hostConfig *container.HostConfig) {
	if hostConfig.Resources.CPUQuota == 0 {
		hostConfig.Resources.CPUQuota = rl.CPUQuota
	}
	if hostConfig.Resources.CPUPeriod == 0 {
		hostConfig.Resources.CPUPeriod = 100000 // Standard period: 100ms
	}
	if hostConfig.Resources.Memory == 0 {
		hostConfig.Resources.Memory = rl.Memory
	}
}
