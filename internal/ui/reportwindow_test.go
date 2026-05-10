package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/user/activitytracker/internal/report"
	"github.com/user/activitytracker/internal/storage"
	"github.com/user/activitytracker/internal/ui"
)

// stubStore implements storage.Storage with canned data for one day.
type stubStore struct {
	days     []string
	sessions []storage.Session
}

func (s *stubStore) InsertRawEvent(_ context.Context, _ storage.RawEvent) error { return nil }
func (s *stubStore) PurgeOldRawEvents(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (s *stubStore) OpenSession(_ context.Context, _ storage.Session) (int64, error) { return 0, nil }
func (s *stubStore) CheckpointSession(_ context.Context, _ int64, _ int) error       { return nil }
func (s *stubStore) CloseSession(_ context.Context, _ int64, _ time.Time, _, _ int) error {
	return nil
}
func (s *stubStore) RecoverCheckpoints(_ context.Context, _ int) error { return nil }
func (s *stubStore) SessionsForDay(_ context.Context, _ string) ([]storage.Session, error) {
	return s.sessions, nil
}
func (s *stubStore) DaysWithData(_ context.Context) ([]string, error) { return s.days, nil }
func (s *stubStore) DeleteDay(_ context.Context, _ string) error       { return nil }
func (s *stubStore) SetSessionNote(_ context.Context, id int64, note string) error {
	for i := range s.sessions {
		if s.sessions[i].ID == id {
			s.sessions[i].Note = note
			return nil
		}
	}
	return nil
}
func (s *stubStore) GetMeta(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (s *stubStore) SetMeta(_ context.Context, _, _ string) error { return nil }
func (s *stubStore) Migrate(_ context.Context) error              { return nil }
func (s *stubStore) Close() error                                  { return nil }

func newTestServer(sessions []storage.Session) *ui.ReportServer {
	days := []string{}
	if len(sessions) > 0 {
		days = []string{"2026-05-03"}
	}
	store := &stubStore{days: days, sessions: sessions}
	gen := report.NewGenerator(nil)
	fmtr := report.NewFormatter()
	return ui.NewReportServer(store, gen, fmtr, nil)
}

func TestReportPage_DarkBackground(t *testing.T) {
	srv := newTestServer(nil)
	req := httptest.NewRequest(http.MethodGet, "/report?date=2026-05-03", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "#0b0d12") {
		t.Error("expected dark background color #0b0d12 in rendered page")
	}
}

func TestReportPage_TimelineCards(t *testing.T) {
	end := time.Date(2026, 5, 3, 10, 0, 0, 0, time.Local)
	dur := 3600
	sessions := []storage.Session{{
		ContextType:  "vscode",
		ContextLabel: "trackingSystem",
		StartUTC:     time.Date(2026, 5, 3, 9, 0, 0, 0, time.Local),
		EndUTC:       &end,
		DurationSecs: &dur,
	}}
	srv := newTestServer(sessions)
	req := httptest.NewRequest(http.MethodGet, "/report?date=2026-05-03", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	body := w.Body.String()

	if !strings.Contains(body, "tl-card") {
		t.Error("expected timeline card class in HTML")
	}
	if !strings.Contains(body, "trackingSystem") {
		t.Error("expected context label in HTML")
	}
	if !strings.Contains(body, "#4ade80") {
		t.Error("expected vscode color #4ade80 in HTML")
	}
	if !strings.Contains(body, "Sunday, May 3") {
		t.Error("expected formatted header date")
	}
}

func TestSessionNote_PostPersistsAndRendersInMarkdown(t *testing.T) {
	end := time.Date(2026, 5, 3, 10, 0, 0, 0, time.Local)
	dur := 3600
	sessions := []storage.Session{{
		ID:           42,
		ContextType:  "vscode",
		ContextLabel: "trackingSystem",
		StartUTC:     time.Date(2026, 5, 3, 9, 0, 0, 0, time.Local),
		EndUTC:       &end,
		DurationSecs: &dur,
	}}
	srv := newTestServer(sessions)

	// POST a note for session 42.
	req := httptest.NewRequest(http.MethodPost, "/api/session/note?id=42",
		strings.NewReader("Refactored the timeline grouping; ran into a template scope bug."))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/session/note: status=%d body=%s", w.Code, w.Body.String())
	}

	// Render the report page; the note should now appear in the card and in the markdown payload.
	pageReq := httptest.NewRequest(http.MethodGet, "/report?date=2026-05-03", nil)
	pageW := httptest.NewRecorder()
	srv.ServeHTTP(pageW, pageReq)
	body := pageW.Body.String()
	if !strings.Contains(body, "Refactored the timeline grouping") {
		t.Error("expected note text rendered in card HTML")
	}
	if !strings.Contains(body, "## My notes") {
		t.Error("expected '## My notes' section in markdown payload (mdText) for AI copy")
	}
	if !strings.Contains(body, "📝") {
		t.Error("expected note marker emoji in timeline table")
	}
}

func TestSessionNote_HighlightsCardAndStripSegment(t *testing.T) {
	end := time.Date(2026, 5, 3, 10, 0, 0, 0, time.Local)
	dur := 3600
	sessions := []storage.Session{{
		ID:           99,
		ContextType:  "vscode",
		ContextLabel: "myproject",
		StartUTC:     time.Date(2026, 5, 3, 9, 0, 0, 0, time.Local),
		EndUTC:       &end,
		DurationSecs: &dur,
		Note:         "Refactored timeline UI",
	}}
	srv := newTestServer(sessions)
	req := httptest.NewRequest(http.MethodGet, "/report?date=2026-05-03", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	body := w.Body.String()

	if !strings.Contains(body, `class="tl-card has-note"`) {
		t.Error("expected `has-note` class on the card whose session has a Note")
	}
	if !strings.Contains(body, `class="strip-seg has-note"`) {
		t.Error("expected `has-note` class on the strip segment of an annotated session")
	}
}

func TestSessionNote_RejectsNonPost(t *testing.T) {
	srv := newTestServer(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/session/note?id=1", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestSessionNote_RejectsInvalidID(t *testing.T) {
	srv := newTestServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/session/note?id=abc", strings.NewReader("x"))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestReportPage_SidebarEmpty(t *testing.T) {
	srv := newTestServer(nil)
	req := httptest.NewRequest(http.MethodGet, "/report?date=2026-05-03", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "No data yet") {
		t.Error("expected empty sidebar message")
	}
}
