package storage

import (
	"context"
	"time"
)

// Storage is the contract for all persistence operations.
// All methods are safe for concurrent use.
// All time arguments must be in UTC.
type Storage interface {
	InsertRawEvent(ctx context.Context, e RawEvent) error
	PurgeOldRawEvents(ctx context.Context, before time.Time) (int64, error)

	OpenSession(ctx context.Context, s Session) (int64, error)
	CheckpointSession(ctx context.Context, id int64, durationSecs int) error
	CloseSession(ctx context.Context, id int64, endUTC time.Time, durationSecs int, minDurationSecs int) error
	RecoverCheckpoints(ctx context.Context, minDurationSecs int) error
	SessionsForDay(ctx context.Context, dateLocal string) ([]Session, error)
	DaysWithData(ctx context.Context) ([]string, error)
	DeleteDay(ctx context.Context, dateLocal string) error

	GetMeta(ctx context.Context, key string) (string, error)
	SetMeta(ctx context.Context, key, value string) error

	Migrate(ctx context.Context) error
	Close() error
}

// RawEvent is a single 5-second poll observation.
type RawEvent struct {
	TimestampUTC time.Time
	WindowTitle  string
	ProcessName  string
	ContextType  string
	ContextLabel string
}

// Session is an aggregated activity block.
type Session struct {
	ID           int64
	DateLocal    string
	ContextType  string
	ContextLabel string
	StartUTC     time.Time
	EndUTC       *time.Time
	DurationSecs *int
	IsCheckpoint bool
}
