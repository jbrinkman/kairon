package logging

import (
	"time"

	"github.com/charmbracelet/log"
)

// LogEntry represents a single log entry with timestamp, level, message, and metadata.
type LogEntry struct {
	Timestamp time.Time              // When the log entry was created
	Level     log.Level              // Log level (debug, info, warn, error)
	Message   string                 // Primary log message
	Metadata  map[string]interface{} // Structured key-value fields
}

// FileOutputConfig contains configuration for file-based log output.
type FileOutputConfig struct {
	LogDir        string // Directory for log files
	MaxFileSizeMB int    // Maximum file size before rotation (in MB)
}

// Constants for log levels
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Default configuration values
const (
	DefaultMaxBufferLines = 10000 // Default ring buffer size
	DefaultMaxFileSizeMB  = 100   // Default max file size before rotation
	DefaultLogDir         = ".kairon/logs"
	DefaultLevel          = LevelInfo
)
