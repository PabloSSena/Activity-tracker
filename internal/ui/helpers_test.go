package ui_test

import (
	"testing"
	"time"

	"github.com/user/activitytracker/internal/ui"
)

func TestContextColor(t *testing.T) {
	cases := []struct{ t, want string }{
		{"vscode", "#4ade80"},
		{"browser", "#60a5fa"},
		{"meeting", "#f59e0b"},
		{"other", "#94a3b8"},
		{"unknown", "#94a3b8"},
	}
	for _, c := range cases {
		if got := ui.ContextColor(c.t); got != c.want {
			t.Errorf("ContextColor(%q) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestFmtDur(t *testing.T) {
	cases := []struct {
		secs int
		want string
	}{
		{0, "< 1m"},
		{30, "< 1m"},
		{60, "1m"},
		{600, "10m"},
		{3600, "1h 0m"},
		{3660, "1h 1m"},
	}
	for _, c := range cases {
		if got := ui.FmtDur(c.secs); got != c.want {
			t.Errorf("FmtDur(%d) = %q, want %q", c.secs, got, c.want)
		}
	}
}

func TestFmtDurP(t *testing.T) {
	secs := 90
	if got := ui.FmtDurP(&secs); got != "1m" {
		t.Errorf("FmtDurP(&90) = %q, want 1m", got)
	}
	if got := ui.FmtDurP(nil); got != "" {
		t.Errorf("FmtDurP(nil) = %q, want empty", got)
	}
}

func TestFmtTimeRange(t *testing.T) {
	loc := time.Local
	start := time.Date(2026, 5, 1, 0, 34, 0, 0, loc)
	end := time.Date(2026, 5, 1, 0, 45, 0, 0, loc)
	got := ui.FmtTimeRange(start, &end)
	want := "00:34 – 00:45"
	if got != want {
		t.Errorf("FmtTimeRange = %q, want %q", got, want)
	}
	if got2 := ui.FmtTimeRange(start, nil); got2 == "" {
		t.Error("FmtTimeRange with nil end should not be empty")
	}
}

func TestFmtSidebarDate(t *testing.T) {
	got := ui.FmtSidebarDate("2026-05-03")
	want := "Sun, May 3"
	if got != want {
		t.Errorf("FmtSidebarDate = %q, want %q", got, want)
	}
	if got2 := ui.FmtSidebarDate("bad"); got2 != "bad" {
		t.Errorf("FmtSidebarDate(bad) = %q, want passthrough", got2)
	}
}

func TestFmtHeaderDate(t *testing.T) {
	got := ui.FmtHeaderDate("2026-05-03")
	want := "Sunday, May 3"
	if got != want {
		t.Errorf("FmtHeaderDate = %q, want %q", got, want)
	}
}
