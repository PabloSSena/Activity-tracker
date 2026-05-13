package report_test

import (
	"strings"
	"testing"
	"time"

	igit "github.com/user/activitytracker/internal/git"
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

func TestFormatter_NotesSection(t *testing.T) {
	end := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	dur := 3600
	sessions := []storage.Session{
		{
			ContextType:  "vscode",
			ContextLabel: "myproject",
			StartUTC:     time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
			EndUTC:       &end,
			DurationSecs: &dur,
			Note:         "Built the new timeline UI",
		},
		{
			ContextType:  "browser",
			ContextLabel: "research",
			StartUTC:     time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
			EndUTC:       &end,
			DurationSecs: &dur,
		},
	}
	dr := report.DailyReport{Date: "2026-05-01", Sessions: sessions}
	out := report.NewFormatter().Format(dr)

	if !strings.Contains(out, "📝") {
		t.Error("expected 📝 marker on note-bearing row")
	}
	if !strings.Contains(out, "## My notes") {
		t.Error("expected '## My notes' section")
	}
	if !strings.Contains(out, "Built the new timeline UI") {
		t.Error("expected note text rendered")
	}
}

func TestFormatter_NoNotesSectionWhenAllEmpty(t *testing.T) {
	end := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	dur := 3600
	sessions := []storage.Session{{
		ContextType: "vscode", ContextLabel: "p", StartUTC: time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
		EndUTC: &end, DurationSecs: &dur,
	}}
	out := report.NewFormatter().Format(report.DailyReport{Date: "2026-05-01", Sessions: sessions})
	if strings.Contains(out, "## My notes") {
		t.Error("notes section should be omitted when no notes exist")
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

// ── git commits in formatter ──────────────────────────────────────────────────

func TestFormatter_GitCommitsSection(t *testing.T) {
	ts := time.Date(2026, 5, 12, 14, 23, 0, 0, time.Local)
	dr := report.DailyReport{
		Date: "2026-05-12",
		GitCommits: []igit.RepoCommits{
			{
				RepoName: "myproject",
				RepoPath: "/home/user/myproject",
				Commits: []igit.Commit{
					{Hash: "abc1234", Subject: "feat: add widget", Author: "Alice", Timestamp: ts},
					{Hash: "def5678", Subject: "fix: typo", Author: "Alice", Timestamp: ts.Add(-time.Hour)},
				},
			},
		},
	}
	out := report.NewFormatter().Format(dr)

	if !strings.Contains(out, "## Git commits") {
		t.Error("expected '## Git commits' section header")
	}
	if !strings.Contains(out, "myproject") {
		t.Error("expected repo name in output")
	}
	if !strings.Contains(out, "abc1234") {
		t.Error("expected commit hash in output")
	}
	if !strings.Contains(out, "feat: add widget") {
		t.Error("expected commit subject in output")
	}
}

func TestFormatter_NoGitSectionWhenEmpty(t *testing.T) {
	end := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	dur := 3600
	dr := report.DailyReport{
		Date: "2026-05-12",
		Sessions: []storage.Session{{
			ContextType: "vscode", ContextLabel: "p",
			StartUTC: time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC),
			EndUTC:   &end, DurationSecs: &dur,
		}},
	}
	out := report.NewFormatter().Format(dr)
	if strings.Contains(out, "## Git commits") {
		t.Error("'## Git commits' section should be absent when GitCommits is empty")
	}
}
