package window

import (
	"context"
	"log"
	"time"

	"github.com/user/activitytracker/internal/monitor"
	"github.com/user/activitytracker/internal/monitor/classifier"
)

// Monitor polls the active foreground window on a fixed interval.
type Monitor struct {
	interval time.Duration
	events   chan monitor.Event
	stop     chan struct{}
}

// New creates a WindowMonitor with the given poll interval.
func New(interval time.Duration) *Monitor {
	return &Monitor{
		interval: interval,
		events:   make(chan monitor.Event, 16),
		stop:     make(chan struct{}),
	}
}

func (m *Monitor) Events() <-chan monitor.Event { return m.events }

func (m *Monitor) Start() error {
	go m.run()
	return nil
}

func (m *Monitor) Stop() error {
	close(m.stop)
	return nil
}

func (m *Monitor) run() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	defer close(m.events)
	for {
		select {
		case <-m.stop:
			return
		case t := <-ticker.C:
			title, proc, err := activeWindow()
			if err != nil {
				log.Printf("window: poll error: %v", err)
				continue
			}
			if title == "" {
				continue
			}
			ct, cl := classifier.Classify(proc, title)
			select {
			case m.events <- monitor.Event{
				Timestamp:    t.UTC(),
				ContextType:  ct,
				ContextLabel: cl,
				WindowTitle:  title,
				ProcessName:  proc,
			}:
			default:
			}
		}
	}
}

// activeWindow is implemented per-platform in window_windows.go / window_linux.go.
// Returns (windowTitle, processName, error).
func activeWindow() (string, string, error) {
	return platformActiveWindow(context.Background())
}
