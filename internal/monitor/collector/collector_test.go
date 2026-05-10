package collector_test

import (
	"context"
	"testing"
	"time"

	"github.com/user/activitytracker/internal/monitor"
	"github.com/user/activitytracker/internal/monitor/collector"
	"github.com/user/activitytracker/internal/monitor/idle"
)

// mockMonitor emits a fixed sequence of events then closes.
type mockMonitor struct {
	events []monitor.Event
	ch     chan monitor.Event
}

func newMockMonitor(events []monitor.Event) *mockMonitor {
	return &mockMonitor{events: events, ch: make(chan monitor.Event, len(events)+1)}
}
func (m *mockMonitor) Start() error {
	for _, e := range m.events {
		m.ch <- e
	}
	close(m.ch)
	return nil
}
func (m *mockMonitor) Stop() error  { return nil }
func (m *mockMonitor) Events() <-chan monitor.Event { return m.ch }

// mockIdle always reports zero idle time.
type mockIdle struct{ d time.Duration }
func (m *mockIdle) IdleDuration() time.Duration { return m.d }

// mockStorage records OpenSession/CloseSession calls.
type mockStorage struct {
	opened    []string // context labels of opened sessions
	closed    []int64
	deleted   []int64
	durations []int       // duration_secs passed to CloseSession, in call order
	endTimes  []time.Time // endUTC passed to CloseSession, in call order
}

func (m *mockStorage) OpenSession(contextType, label string) int64 {
	m.opened = append(m.opened, label)
	return int64(len(m.opened))
}
func (m *mockStorage) CloseSession(id int64, endUTC time.Time, dur int) {
	m.durations = append(m.durations, dur)
	m.endTimes = append(m.endTimes, endUTC)
	if dur >= 120 {
		m.closed = append(m.closed, id)
	} else {
		m.deleted = append(m.deleted, id)
	}
}

func TestCollector_SessionOpensOnFirstEvent(t *testing.T) {
	now := time.Now().UTC()
	events := []monitor.Event{
		{Timestamp: now, ContextType: "vscode", ContextLabel: "myproject"},
	}
	mon := newMockMonitor(events)
	idleDet := &mockIdle{}
	store := &mockStorage{}

	c := collector.New(mon, idleDet, store, collector.Options{
		MinSessionSecs:  120,
		CheckpointSecs:  60,
		IdleTimeoutMins: 10,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	c.Run(ctx)

	if len(store.opened) == 0 {
		t.Fatal("expected at least one session to be opened")
	}
	if store.opened[0] != "myproject" {
		t.Errorf("opened session label = %q, want myproject", store.opened[0])
	}
}

func TestCollector_NoiseFilterDiscardsShortSessions(t *testing.T) {
	now := time.Now().UTC()
	events := []monitor.Event{
		{Timestamp: now, ContextType: "vscode", ContextLabel: "myproject"},
		{Timestamp: now.Add(30 * time.Second), ContextType: "browser", ContextLabel: "browser/research"},
	}
	mon := newMockMonitor(events)
	store := &mockStorage{}

	c := collector.New(mon, &mockIdle{}, store, collector.Options{
		MinSessionSecs:  120,
		CheckpointSecs:  300,
		IdleTimeoutMins: 10,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	c.Run(ctx)

	if len(store.deleted) == 0 {
		t.Error("expected short session (30s < 120s) to be discarded")
	}
}

func TestCollector_IdleClosesSession(t *testing.T) {
	now := time.Now().UTC()
	events := []monitor.Event{
		{Timestamp: now, ContextType: "vscode", ContextLabel: "myproject"},
	}
	mon := newMockMonitor(events)
	// Report idle > 10 minutes immediately
	idleDet := &mockIdle{d: 11 * time.Minute}
	store := &mockStorage{}

	c := collector.New(mon, idleDet, store, collector.Options{
		MinSessionSecs:  0,
		CheckpointSecs:  300,
		IdleTimeoutMins: 10,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	c.Run(ctx)

	if len(store.closed) == 0 && len(store.deleted) == 0 {
		t.Error("expected idle to close the active session")
	}
}

func TestCollector_SleepWakeDetected(t *testing.T) {
	now := time.Now().UTC()
	// Gap of 60 seconds between two events (> 30s threshold = sleep/wake)
	events := []monitor.Event{
		{Timestamp: now, ContextType: "vscode", ContextLabel: "proj"},
		{Timestamp: now.Add(60 * time.Second), ContextType: "vscode", ContextLabel: "proj"},
	}
	mon := newMockMonitor(events)
	store := &mockStorage{}

	c := collector.New(mon, &mockIdle{}, store, collector.Options{
		MinSessionSecs:  0,
		CheckpointSecs:  300,
		IdleTimeoutMins: 10,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	c.Run(ctx)

	// A sleep gap should close the old session and open a new one
	if len(store.opened) < 2 {
		t.Errorf("expected 2 sessions (before + after sleep), got %d", len(store.opened))
	}
}

// Ensure the idle package is imported for interface check.
var _ idle.Detector = &mockIdle{}
