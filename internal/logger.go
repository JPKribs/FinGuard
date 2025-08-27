package internal

import (
	"log/slog"
	"os"
	"strings"

	"github.com/JPKribs/FinGuard/utilities"
)

const maxLogs = 500

// MARK: NewLogger
func NewLogger(level string) *Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)

	return &Logger{
		Logger: logger,
		logs:   make([]LogEntry, 0, maxLogs),
	}
}

// MARK: addToMemory
func (l *Logger) addToMemory(level, msg string, context map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
		Timestamp: utilities.CurrentTimestamp(),
		Level:     strings.ToUpper(level),
		Message:   msg,
		Context:   context,
	}

	if len(l.logs) >= maxLogs {
		l.logs = l.logs[1:]
	}
	l.logs = append(l.logs, entry)

	if l.OnLog != nil {
		l.OnLog(level, msg)
	}
}

// MARK: convertArgsToContext
func convertArgsToContext(args []any) map[string]interface{} {
	if len(args) == 0 {
		return nil
	}

	context := make(map[string]interface{})
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			if key, ok := args[i].(string); ok {
				context[key] = args[i+1]
			}
		}
	}

	if len(context) == 0 {
		return nil
	}
	return context
}

// MARK: GetLogs
func (l *Logger) GetLogs(level string) []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level == "" {
		return append([]LogEntry(nil), l.logs...)
	}

	filtered := make([]LogEntry, 0)
	for _, log := range l.logs {
		if strings.EqualFold(log.Level, level) {
			filtered = append(filtered, log)
		}
	}
	return filtered
}

// MARK: Debug
func (l *Logger) Debug(msg string, args ...any) {
	context := convertArgsToContext(args)
	l.addToMemory("DEBUG", msg, context)
	l.Logger.Debug(msg, args...)
}

// MARK: Info
func (l *Logger) Info(msg string, args ...any) {
	context := convertArgsToContext(args)
	l.addToMemory("INFO", msg, context)
	l.Logger.Info(msg, args...)
}

// MARK: Warn
func (l *Logger) Warn(msg string, args ...any) {
	context := convertArgsToContext(args)
	l.addToMemory("WARN", msg, context)
	l.Logger.Warn(msg, args...)
}

// MARK: Error
func (l *Logger) Error(msg string, args ...any) {
	context := convertArgsToContext(args)
	l.addToMemory("ERROR", msg, context)
	l.Logger.Error(msg, args...)
}
