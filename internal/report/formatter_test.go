package report_test

import (
	"strings"
	"testing"
	"time"

	"github.com/user/activitytracker/internal/report"
	"github.com/user/activitytracker/internal/storage"
)

func TestFormatter_Header(t *testing.T) {
	dr := report.DailyReport{Date: "2026-05-01"}
	f := report.NewFormatter()
	out := f.Format(dr)
	if !strings.HasPrefix(out, "# Activity Report — 2026-05-01") {
		t.Errorf("missing header, got: %q", out[:min(50, len(out))])
	}
}

func TestFormatter_TimelineTable(t *testing.T) {
	end := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	dur := 3600
	sessions := []storage.Session{{
		ContextType:  "vscode",
		ContextLabel: "myproject",
		StartUTC:     time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
		EndUTC:       &end,
		DurationSecs: &dur,
	}}
	dr := report.DailyReport{Date: "2026-05-01", Sessions: sessions}
	f := report.NewFormatter()
	out := f.Format(dr)
	if !strings.Contains(out, "myproject") {
		t.Error("timeline table missing context label")
	}
	if !strings.Contains(out, "1h 0m") {
		t.Errorf("duration format missing: got %q", out)
	}
}

func TestFormatter_DurationFormat(t *testing.T) {
	f := report.NewFormatter()
	cases := []struct {
		secs int
		want string
	}{
		{0, "< 1m"},
		{30, "< 1m"},
		{60, "1m"},
		{90, "1m"},
		{3600, "1h 0m"},
		{3660, "1h 1m"},
		{7200, "2h 0m"},
	}
	for _, c := range cases {
		got := f.FormatDuration(c.secs)
		if got != c.want {
			t.Errorf("FormatDuration(%d) = %q, want %q", c.secs, got, c.want)
		}
	}
}

func TestFormatter_EmptyDay_EmptyStateMessage(t *testing.T) {
	dr := report.DailyReport{Date: "2026-05-01"}
	f := report.NewFormatter()
	out := f.Format(dr)
	if !strings.Contains(out, "No activity") {
		t.Errorf("expected empty state message, got: %q", out)
	}
}

func TestFormatter_EndsWithNewline(t *testing.T) {
	dr := report.DailyReport{Date: "2026-05-01"}
	f := report.NewFormatter()
	out := f.Format(dr)
	if !strings.HasSuffix(out, "\n") {
		t.Error("output does not end with newline")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
