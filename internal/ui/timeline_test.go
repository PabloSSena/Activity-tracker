package ui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/user/activitytracker/internal/storage"
	"github.com/user/activitytracker/internal/ui"
)

func mkSession(id int64, ctype, label string, startMin, durSec int) storage.Session {
	start := time.Date(2026, 5, 3, 9, startMin, 0, 0, time.Local)
	end := start.Add(time.Duration(durSec) * time.Second)
	d := durSec
	return storage.Session{
		ID:           id,
		ContextType:  ctype,
		ContextLabel: label,
		StartUTC:     start,
		EndUTC:       &end,
		DurationSecs: &d,
	}
}

func TestBuildTimelineItems_CollapsesConsecutiveShorts(t *testing.T) {
	sessions := []storage.Session{
		mkSession(1, "vscode", "a", 0, 1800), // long
		mkSession(2, "browser", "b", 30, 5),  // short
		mkSession(3, "browser", "c", 31, 8),  // short
		mkSession(4, "browser", "d", 32, 10), // short
		mkSession(5, "vscode", "e", 33, 600), // long
	}
	items := ui.BuildTimelineItems(sessions)
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	if items[0].Group {
		t.Error("first item should be single, not group")
	}
	if !items[1].Group || items[1].Count() != 3 {
		t.Errorf("middle item should be group of 3, got Group=%v Count=%d", items[1].Group, items[1].Count())
	}
	if items[2].Group {
		t.Error("last item should be single, not group")
	}
}

func TestBuildTimelineItems_SingleShortNotCollapsed(t *testing.T) {
	sessions := []storage.Session{
		mkSession(1, "vscode", "a", 0, 1800),
		mkSession(2, "browser", "b", 30, 5),
		mkSession(3, "vscode", "c", 31, 600),
	}
	items := ui.BuildTimelineItems(sessions)
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3 (no collapse)", len(items))
	}
	for i, it := range items {
		if it.Group {
			t.Errorf("item %d should not be a group", i)
		}
	}
}

func TestBuildTimelineItems_AllShorts(t *testing.T) {
	sessions := []storage.Session{
		mkSession(1, "browser", "a", 0, 5),
		mkSession(2, "browser", "b", 1, 5),
		mkSession(3, "vscode", "c", 2, 5),
	}
	items := ui.BuildTimelineItems(sessions)
	if len(items) != 1 || !items[0].Group || items[0].Count() != 3 {
		t.Errorf("expected one group of 3, got %+v", items)
	}
	if items[0].ChildrenIDs() != "1 2 3" {
		t.Errorf("ChildrenIDs = %q, want %q", items[0].ChildrenIDs(), "1 2 3")
	}
}

func TestReverseTimelineItems(t *testing.T) {
	in := []ui.TimelineItem{{Session: ptrSession(mkSession(1, "v", "a", 0, 100))},
		{Session: ptrSession(mkSession(2, "v", "b", 5, 100))}}
	out := ui.ReverseTimelineItems(in)
	if out[0].Session.ID != 2 || out[1].Session.ID != 1 {
		t.Errorf("reverse failed: got %d,%d", out[0].Session.ID, out[1].Session.ID)
	}
	if in[0].Session.ID != 1 {
		t.Error("input slice was mutated")
	}
}

func TestBuildDayStrip_Empty(t *testing.T) {
	if got := ui.BuildDayStrip(nil); got.HasContent() {
		t.Error("expected empty strip for nil sessions")
	}
}

func TestBuildDayStrip_PositionsAreProportional(t *testing.T) {
	// Strip now uses a fixed 24h axis (midnight→midnight).
	// Sessions at 09:00–10:00 and 10:00–11:00 in a 24h range:
	//   seg0 left = 9/24*100 = 37.5%, width = 1/24*100 ≈ 4.17%
	//   seg1 left = 10/24*100 ≈ 41.67%
	sessions := []storage.Session{
		mkSession(1, "vscode", "a", 0, 3600),   // 09:00–10:00
		mkSession(2, "browser", "b", 60, 3600), // 10:00–11:00
	}
	strip := ui.BuildDayStrip(sessions)
	if !strip.HasContent() || len(strip.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(strip.Segments))
	}
	// seg0 starts at 09:00 = 37.5% of 24h
	if strip.Segments[0].LeftPct < 37 || strip.Segments[0].LeftPct > 38 {
		t.Errorf("seg 0 left = %f, want ~37.5", strip.Segments[0].LeftPct)
	}
	// seg1 starts at 10:00 = 41.67% of 24h
	if strip.Segments[1].LeftPct < 41 || strip.Segments[1].LeftPct > 43 {
		t.Errorf("seg 1 left = %f, want ~41.67", strip.Segments[1].LeftPct)
	}
	// Fixed ticks: 0h, 6h, 12h, 18h, 24h
	if len(strip.Ticks) != 5 {
		t.Errorf("got %d ticks, want 5", len(strip.Ticks))
	}
	// Noon is always at 50%
	if !strip.HasNoon || strip.NoonPct < 49.9 || strip.NoonPct > 50.1 {
		t.Errorf("noon pct = %v / %f, want HasNoon=true at 50%%", strip.HasNoon, strip.NoonPct)
	}
}

func TestBuildDayStrip_LargeSpanSkipsTicks(t *testing.T) {
	// Full 24h strip always has exactly 5 fixed ticks (0h, 6h, 12h, 18h, 24h).
	sessions := []storage.Session{
		mkSession(1, "vscode", "a", 0, 14*3600),
	}
	strip := ui.BuildDayStrip(sessions)
	if len(strip.Ticks) != 5 {
		t.Errorf("expected 5 fixed ticks, got %d", len(strip.Ticks))
	}
}

func TestReportPage_RendersStripAndGroupedShorts(t *testing.T) {
	sessions := []storage.Session{
		mkSession(1, "vscode", "main work", 0, 1800),
		mkSession(2, "browser", "x", 30, 5),
		mkSession(3, "browser", "y", 31, 5),
		mkSession(4, "browser", "z", 32, 5),
		mkSession(5, "vscode", "more work", 33, 600),
	}
	srv := newTestServer(sessions)
	req := httptest.NewRequest(http.MethodGet, "/report?date=2026-05-03", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	body := w.Body.String()

	if !strings.Contains(body, "strip-bar") {
		t.Error("expected day strip bar in rendered HTML")
	}
	if !strings.Contains(body, "tl-group") {
		t.Error("expected collapsed group card in HTML")
	}
	if !strings.Contains(body, "3 short entries") {
		t.Error("expected '3 short entries' label for collapsed group")
	}
	// Reverse order: most recent label ("more work") should appear before earliest ("main work").
	// Reverse order: card with id="s-5" (most recent) must appear before id="s-1".
	idxRecent := strings.Index(body, `id="s-5"`)
	idxOldest := strings.Index(body, `id="s-1"`)
	if idxRecent < 0 || idxOldest < 0 || idxRecent >= idxOldest {
		t.Errorf("expected reverse card order: id=s-5 before id=s-1, got %d vs %d", idxRecent, idxOldest)
	}
}

func ptrSession(s storage.Session) *storage.Session { return &s }
