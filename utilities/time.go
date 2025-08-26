package utilities

import "time"

// MARK: ParseTimestamp
func ParseTimestamp(timeStr string) time.Time {
	// Try parsing RFC3339 format first
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t
	}

	// Try parsing RFC3339Nano format
	if t, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
		return t
	}

	// Try parsing common timestamp formats
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

	// Fallback to current time if parsing fails
	return time.Now()
}

// MARK: FormatTimestamp
func FormatTimestamp(t time.Time) string {
	return t.Format(time.RFC3339)
}

// MARK: CurrentTimestamp
func CurrentTimestamp() string {
	return FormatTimestamp(time.Now())
}
