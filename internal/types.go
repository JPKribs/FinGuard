package internal

import (
	"log/slog"
	"sync"
	"time"
)

// MARK: Health.go

type HealthChecker struct {
	ready int64
	alive int64
}

type HealthStatus struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

// MARK: Health.go

type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

type Logger struct {
	*slog.Logger
	mu    sync.Mutex
	logs  []LogEntry
	OnLog func(level, msg string)
}
