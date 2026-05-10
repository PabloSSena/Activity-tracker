package collector_test

import (
	"context"
	"testing"
	"time"

	"github.com/user/activitytracker/internal/monitor"
	"github.com/user/activitytracker/internal/monitor/collector"
)

type benchStore struct{ count int }

func (b *benchStore) OpenSession(_, _ string) int64 { b.count++; return int64(b.count) }
func (b *benchStore) CloseSession(_ int64, _ time.Time, _ int) {}

func BenchmarkCollector_EventThroughput(b *testing.B) {
	const eventCount = 1000
	events := make([]monitor.Event, eventCount)
	now := time.Now().UTC()
	for i := range events {
		events[i] = monitor.Event{
			Timestamp:    now.Add(time.Duration(i) * 5 * time.Second),
			ContextType:  "vscode",
			ContextLabel: "myproject",
		}
	}
	mon := newMockMonitor(events)
	store := &benchStore{}
	c := collector.New(mon, &mockIdle{}, store, collector.Options{
		MinSessionSecs:  0,
		CheckpointSecs:  300,
		IdleTimeoutMins: 10,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b.ResetTimer()
	c.Run(ctx)
}
