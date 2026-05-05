package report_test

import (
	"testing"
	"time"

	"github.com/user/activitytracker/internal/report"
	"github.com/user/activitytracker/internal/storage"
)

func makeSess(ct, cl string, startH, startM, durMins int) storage.Session {
	base := time.Date(2026, 5, 1, startH, startM, 0, 0, time.UTC)
	end := base.Add(time.Duration(durMins) * time.Minute)
	dur := durMins * 60
	return storage.Session{ContextType: ct, ContextLabel: cl, StartUTC: base, EndUTC: &end, DurationSecs: &dur}
}

func TestGrouper_SandwichBrowserMerged(t *testing.T) {
	sessions := []storage.Session{
		makeSess("vscode", "myproject", 9, 0, 30),
		makeSess("browser", "browser/research", 9, 30, 7),
		makeSess("vscode", "myproject", 9, 37, 20),
	}
	g := report.NewGrouper(15)
	groups := g.Group(sessions)

	// Should produce one group "myproject"
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1 (browser should merge into myproject)", len(groups))
	}
	if groups[0].Label != "myproject" {
		t.Errorf("group label = %q, want myproject", groups[0].Label)
	}
	// Should have a research/ai-assist entry
	foundResearch := false
	for _, e := range groups[0].Entries {
		if e.Label == "research/ai-assist" {
			foundResearch = true
		}
	}
	if !foundResearch {
		t.Error("expected research/ai-assist entry in myproject group")
	}
}

func TestGrouper_LongBrowserNotMerged(t *testing.T) {
	sessions := []storage.Session{
		makeSess("vscode", "myproject", 9, 0, 30),
		makeSess("browser", "browser/research", 9, 30, 20), // > 15 min
		makeSess("vscode", "myproject", 9, 50, 20),
	}
	g := report.NewGrouper(15)
	groups := g.Group(sessions)

	// Browser should remain standalone
	labels := make(map[string]bool)
	for _, gr := range groups {
		labels[gr.Label] = true
	}
	if !labels["browser/research"] {
		t.Error("expected browser/research as standalone group when > 15 min")
	}
}

func TestGrouper_NonAdjacentBrowserNotMerged(t *testing.T) {
	sessions := []storage.Session{
		makeSess("vscode", "projA", 9, 0, 30),
		makeSess("other", "Slack", 9, 30, 5),
		makeSess("browser", "browser/research", 9, 35, 7),
		makeSess("vscode", "projA", 9, 42, 20),
	}
	g := report.NewGrouper(15)
	groups := g.Group(sessions)

	labels := make(map[string]bool)
	for _, gr := range groups {
		labels[gr.Label] = true
	}
	if !labels["browser/research"] {
		t.Error("expected browser/research standalone when not directly sandwiched")
	}
}

func TestGrouper_DifferentWorkspacesNotMerged(t *testing.T) {
	sessions := []storage.Session{
		makeSess("vscode", "projA", 9, 0, 30),
		makeSess("browser", "browser/research", 9, 30, 5),
		makeSess("vscode", "projB", 9, 35, 20), // different workspace
	}
	g := report.NewGrouper(15)
	groups := g.Group(sessions)

	labels := make(map[string]bool)
	for _, gr := range groups {
		labels[gr.Label] = true
	}
	if !labels["browser/research"] {
		t.Error("expected browser/research standalone when workspaces differ")
	}
}
