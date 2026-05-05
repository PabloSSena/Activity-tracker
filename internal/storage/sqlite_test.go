package storage_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/user/activitytracker/internal/storage"
)

func openTestDB(t *testing.T) *storage.DB {
	t.Helper()
	f, err := os.CreateTemp("", "activitytracker-test-*.db")
	if err != nil {
		t.Fatalf("temp db: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := storage.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestSessionsForDay_OrderedByStartUTC(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	s1 := storage.Session{DateLocal: "2026-05-01", ContextType: "vscode", ContextLabel: "proj", StartUTC: now}
	s2 := storage.Session{DateLocal: "2026-05-01", ContextType: "browser", ContextLabel: "browser/research", StartUTC: now.Add(time.Hour)}

	id1, _ := db.OpenSession(ctx, s1)
	id2, _ := db.OpenSession(ctx, s2)
	db.CloseSession(ctx, id1, now.Add(time.Hour), 3600, 0)
	db.CloseSession(ctx, id2, now.Add(2*time.Hour), 3600, 0)

	sessions, err := db.SessionsForDay(ctx, "2026-05-01")
	if err != nil {
		t.Fatalf("SessionsForDay: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}
	if !sessions[0].StartUTC.Before(sessions[1].StartUTC) {
		t.Error("sessions not ordered by start_utc ascending")
	}
}

func TestDaysWithData_MostRecentFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	for i, date := range []string{"2026-05-01", "2026-05-02", "2026-05-03"} {
		s := storage.Session{DateLocal: date, ContextType: "other", ContextLabel: "x", StartUTC: now.Add(time.Duration(i) * time.Hour)}
		id, _ := db.OpenSession(ctx, s)
		db.CloseSession(ctx, id, now.Add(time.Duration(i+1)*time.Hour), 3600, 0)
	}

	days, err := db.DaysWithData(ctx)
	if err != nil {
		t.Fatalf("DaysWithData: %v", err)
	}
	if len(days) != 3 {
		t.Fatalf("got %d days, want 3", len(days))
	}
	if days[0] != "2026-05-03" {
		t.Errorf("first day = %q, want 2026-05-03 (most recent)", days[0])
	}
}

func TestDeleteDay_RemovesOnlyTargetDay(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	for i, date := range []string{"2026-05-01", "2026-05-02"} {
		s := storage.Session{DateLocal: date, ContextType: "other", ContextLabel: "x", StartUTC: now.Add(time.Duration(i) * time.Hour)}
		id, _ := db.OpenSession(ctx, s)
		db.CloseSession(ctx, id, now.Add(time.Duration(i+1)*time.Hour), 3600, 0)
	}

	if err := db.DeleteDay(ctx, "2026-05-01"); err != nil {
		t.Fatalf("DeleteDay: %v", err)
	}

	deleted, _ := db.SessionsForDay(ctx, "2026-05-01")
	if len(deleted) != 0 {
		t.Errorf("expected 0 sessions after delete, got %d", len(deleted))
	}

	remaining, _ := db.SessionsForDay(ctx, "2026-05-02")
	if len(remaining) != 1 {
		t.Errorf("expected 1 session on other day, got %d", len(remaining))
	}
}
