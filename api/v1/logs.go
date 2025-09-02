package v1

import (
	"net/http"

	"github.com/JPKribs/FinGuard/internal"
	"github.com/JPKribs/FinGuard/utilities"
)

// MARK: handleLogs
// Get all logs in memory for the WebUI.
func (a *APIServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	limit, offset := a.parsePaginationParams(r)
	level := r.URL.Query().Get("level")

	allLogs := a.logger.GetLogs(level)

	total := len(allLogs)
	start := offset
	if start >= total {
		allLogs = []internal.LogEntry{}
	} else {
		end := start + limit
		if end > total {
			end = total
		}
		allLogs = allLogs[start:end]
	}

	response := LogResponse{
		Logs:   convertLogEntries(allLogs),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}

	a.respondWithSuccess(w, "Logs retrieved", response)
}

// MARK: convertLogEntries
// Prepare the Log Entries for usage on the WebUI.
func convertLogEntries(internalLogs []internal.LogEntry) []LogEntry {
	logs := make([]LogEntry, len(internalLogs))
	for i, log := range internalLogs {
		logs[i] = LogEntry{
			Timestamp: utilities.ParseTimestamp(log.Timestamp),
			Level:     log.Level,
			Message:   log.Message,
			Context:   log.Context,
		}
	}
	return logs
}
