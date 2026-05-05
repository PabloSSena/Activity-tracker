package report_test

import (
	"testing"
	"time"

	"github.com/user/activitytracker/internal/report"
	"github.com/user/activitytracker/internal/storage"
)

func makeSession(ct, cl string, startH, startM, durationMins int) storage.Session {
	base := time.Date(2026, 5, 1, startH, startM, 0, 0, time.UTC)
	end := base.Add(time.Duration(durationMins) * time.Minute)
	dur := durationMins * 60
	return storage.Session{
		ID:           1,
		DateLocal:    "2026-05-01",
		ContextType:  ct,
		ContextLabel: cl,
		StartUTC:     base,
		EndUTC:       &end,
		DurationSecs: &dur,
	}
}

func TestGenerator_EmptyDay(t *testing.T) {
	g := report.NewGenerator(nil)
	dr := g.BuildReport("2026-05-01", nil)
	if dr.Date != "2026-05-01" {
		t.Errorf("Date = %q, want 2026-05-01", dr.Date)
	}
	if len(dr.Sessions) != 0 {
		t.Errorf("Sessions len = %d, want 0", len(dr.Sessions))
	}
}

func TestGenerator_SingleSession(t *testing.T) {
	sessions := []storage.Session{makeSession("vscode", "myproject", 9, 0, 60)}
	g := report.NewGenerator(nil)
	dr := g.BuildReport("2026-05-01", sessions)
	if len(dr.Sessions) != 1 {
		t.Fatalf("Sessions len = %d, want 1", len(dr.Sessions))
	}
	if dr.Sessions[0].ContextLabel != "myproject" {
		t.Errorf("label = %q, want myproject", dr.Sessions[0].ContextLabel)
	}
}

func TestGenerator_MultiSession_OrderPreserved(t *testing.T) {
	sessions := []storage.Session{
		makeSession("vscode", "proj", 9, 0, 30),
		makeSession("browser", "browser/research", 9, 30, 10),
		makeSession("vscode", "proj", 9, 40, 20),
	}
	g := report.NewGenerator(nil)
	dr := g.BuildReport("2026-05-01", sessions)
	if len(dr.Sessions) != 3 {
		t.Fatalf("Sessions len = %d, want 3", len(dr.Sessions))
	}
	// Chronological order preserved
	for i := 1; i < len(dr.Sessions); i++ {
		if dr.Sessions[i].StartUTC.Before(dr.Sessions[i-1].StartUTC) {
			t.Errorf("sessions not in chronological order at index %d", i)
		}
	}
}

func TestGenerator_TotalsCorrect(t *testing.T) {
	sessions := []storage.Session{
		makeSession("vscode", "proj", 9, 0, 60),
		makeSession("vscode", "proj", 10, 0, 30),
	}
	g := report.NewGenerator(nil)
	dr := g.BuildReport("2026-05-01", sessions)
	total := dr.Totals["proj"]
	if total != 90*60 {
		t.Errorf("total secs = %d, want %d", total, 90*60)
	}
}
