package utilities

import "time"

// Timestamp parsing functions

// MARK: ParseTimestamp
// Attempts to parse timestamp string using multiple common formats
func ParseTimestamp(timeStr string) time.Time {
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t
	}

	if t, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
		return t
	}

	formats := []string{
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		time.Kitchen,
		time.Stamp,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t
		}
	}

	return time.Now()
}

// Timestamp formatting functions

// MARK: FormatTimestamp
// Formats time as RFC3339 string
func FormatTimestamp(t time.Time) string {
	return t.Format(time.RFC3339)
}

// MARK: CurrentTimestamp
// Returns current time formatted as RFC3339 string
func CurrentTimestamp() string {
	return FormatTimestamp(time.Now())
}
