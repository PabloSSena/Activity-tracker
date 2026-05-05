package ui

import (
	"fmt"
	"time"
)

// ContextColor returns a CSS hex color for a context type.
func ContextColor(contextType string) string {
	switch contextType {
	case "vscode":
		return "#4ade80"
	case "browser":
		return "#60a5fa"
	case "meeting":
		return "#f59e0b"
	default:
		return "#94a3b8"
	}
}

// FmtDur formats an integer number of seconds as a human-readable duration.
func FmtDur(secs int) string {
	if secs < 60 {
		return "< 1m"
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dh %dm", mins/60, mins%60)
}

// FmtDurP formats a pointer to seconds; returns empty string for nil.
func FmtDurP(secs *int) string {
	if secs == nil {
		return ""
	}
	return FmtDur(*secs)
}

// FmtTimeRange formats a start and optional end time as "HH:MM – HH:MM" (en dash).
func FmtTimeRange(start time.Time, end *time.Time) string {
	s := start.In(time.Local).Format("15:04")
	if end == nil {
		return s + " – ?"
	}
	return s + " – " + end.In(time.Local).Format("15:04")
}

// FmtSidebarDate parses a "YYYY-MM-DD" string and returns "Mon, Jan 2".
// Returns the input unchanged if parsing fails.
func FmtSidebarDate(dateStr string) string {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return dateStr
	}
	return t.Format("Mon, Jan 2")
}

// FmtHeaderDate parses a "YYYY-MM-DD" string and returns the date formatted as
// "<Weekday>, <Month> <Day>" (e.g. "Sunday, May 3").
// Returns the input unchanged if parsing fails.
func FmtHeaderDate(dateStr string) string {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return dateStr
	}
	return t.Format("Monday, January 2")
}
