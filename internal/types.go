package internal

import (
	"log/slog"
	"sync"
	"time"
)

// MARK: HealthChecker
type HealthChecker struct {
	ready int64
	alive int64
}

// MARK: HealthStatus
type HealthStatus struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

// MARK: LogEntry
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// MARK: Logger
type Logger struct {
	*slog.Logger
	mu    sync.Mutex
	logs  []LogEntry
	OnLog func(level, msg string)
}
