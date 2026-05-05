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
	if !strings.Contains(body, "#0f1117") {
		t.Error("expected dark background color #0f1117 in rendered page")
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
