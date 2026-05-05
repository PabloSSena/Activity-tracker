package monitor

import "time"

// Monitor is the contract for all activity data sources.
type Monitor interface {
	Start() error
	Stop() error
	Events() <-chan Event
}

// Event represents a single observation from a Monitor.
type Event struct {
	Timestamp    time.Time
	ContextType  string // "vscode" | "meeting" | "browser" | "other"
	ContextLabel string // workspace name, meeting name, "browser/research", or raw window title
	WindowTitle  string
	ProcessName  string
}
