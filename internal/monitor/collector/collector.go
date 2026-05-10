package collector

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/user/activitytracker/internal/monitor"
	"github.com/user/activitytracker/internal/monitor/idle"
)

const sleepGapThreshold = 30 * time.Second

// Options configures collector behaviour.
type Options struct {
	MinSessionSecs  int
	CheckpointSecs  int
	IdleTimeoutMins int
}

// Store is the minimal storage interface the collector needs.
//
// CloseSession receives the explicit end time the collector decided on, which
// may be earlier than wall-clock now when trimming a trailing idle period.
type Store interface {
	OpenSession(contextType, label string) int64
	CloseSession(id int64, endUTC time.Time, durationSecs int)
}

// ActiveSession is a snapshot of the current in-progress activity session.
// Safe to read from any goroutine via CurrentSession.
type ActiveSession struct {
	ContextType  string
	ContextLabel string
	StartUTC     time.Time
}

// Collector aggregates monitor events into sessions.
type Collector struct {
	mon   monitor.Monitor
	idle  idle.Detector
	store Store
	opts  Options

	mu                sync.RWMutex
	activeID          int64
	activeContextType string
	activeLabel       string
	sessionStart      time.Time
}

// New creates a Collector.
func New(mon monitor.Monitor, idle idle.Detector, store Store, opts Options) *Collector {
	return &Collector{mon: mon, idle: idle, store: store, opts: opts}
}

// CurrentSession returns the current in-progress session, or nil if idle.
// Safe to call from any goroutine.
func (c *Collector) CurrentSession() *ActiveSession {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.activeID == 0 {
		return nil
	}
	return &ActiveSession{
		ContextType:  c.activeContextType,
		ContextLabel: c.activeLabel,
		StartUTC:     c.sessionStart,
	}
}

// Run starts the monitor and processes events until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) {
	if err := c.mon.Start(); err != nil {
		log.Printf("collector: monitor start: %v", err)
		return
	}
	defer c.mon.Stop()

	checkpointInterval := time.Duration(c.opts.CheckpointSecs) * time.Second
	idleTimeout := time.Duration(c.opts.IdleTimeoutMins) * time.Minute

	var lastPoll time.Time

	closeActive := func(endAt time.Time) {
		c.mu.Lock()
		id := c.activeID
		start := c.sessionStart
		c.activeID = 0
		c.activeLabel = ""
		c.activeContextType = ""
		c.mu.Unlock()
		if id == 0 {
			return
		}
		dur := int(endAt.Sub(start).Seconds())
		if dur < 0 {
			dur = 0
		}
		c.store.CloseSession(id, endAt, dur)
	}

	// idleAdjustedEnd returns when the session should logically end given the
	// reference wall time `now`: if the user has been idle past the threshold,
	// we credit them only up to when they actually went idle. Clamped so it
	// never precedes the session's own start.
	idleAdjustedEnd := func(now time.Time) time.Time {
		nowIdle := c.idle.IdleDuration()
		if nowIdle < idleTimeout {
			return now
		}
		c.mu.RLock()
		start := c.sessionStart
		c.mu.RUnlock()
		end := now.Add(-nowIdle)
		if end.Before(start) {
			end = start
		}
		return end
	}

	checkpoint := time.NewTicker(checkpointInterval)
	defer checkpoint.Stop()

	for {
		select {
		case <-ctx.Done():
			closeActive(idleAdjustedEnd(time.Now().UTC()))
			return

		case <-checkpoint.C:
			c.mu.RLock()
			hasActive := c.activeID != 0
			c.mu.RUnlock()
			if hasActive && c.idle.IdleDuration() >= idleTimeout {
				log.Printf("collector: idle timeout, closing session")
				closeActive(idleAdjustedEnd(time.Now().UTC()))
			}

		case evt, ok := <-c.mon.Events():
			if !ok {
				closeActive(idleAdjustedEnd(time.Now().UTC()))
				return
			}

			now := evt.Timestamp

			if !lastPoll.IsZero() && now.Sub(lastPoll) > sleepGapThreshold {
				log.Printf("collector: sleep/wake detected (gap %v)", now.Sub(lastPoll))
				closeActive(lastPoll.Add(5 * time.Second))
			}
			lastPoll = now

			nowIdle := c.idle.IdleDuration()

			c.mu.RLock()
			curID := c.activeID
			curLabel := c.activeLabel
			c.mu.RUnlock()

			if curID != 0 && nowIdle >= idleTimeout {
				log.Printf("collector: idle timeout on event, closing session")
				closeActive(idleAdjustedEnd(now))
				curLabel = ""
			}

			// While the user remains idle past the threshold, don't open a
			// new session — wait for actual interaction (idle drops below
			// timeout) before resuming tracking.
			if nowIdle >= idleTimeout {
				continue
			}

			if evt.ContextLabel != curLabel {
				closeActive(now)
				id := c.store.OpenSession(evt.ContextType, evt.ContextLabel)
				c.mu.Lock()
				c.activeID = id
				c.activeLabel = evt.ContextLabel
				c.activeContextType = evt.ContextType
				c.sessionStart = now
				c.mu.Unlock()
			}
		}
	}
}
