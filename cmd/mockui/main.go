// mockui starts the report UI with rich fake data so the multi-column layout
// can be previewed without needing a real tracking session.
package main

import (
	"context"
	"log"
	"time"

	"github.com/user/activitytracker/internal/report"
	"github.com/user/activitytracker/internal/storage"
	"github.com/user/activitytracker/internal/ui"
)

func main() {
	sessions := mockSessions()
	store := &mockStore{sessions: sessions}
	gen := report.NewGenerator(nil)
	fmtr := report.NewFormatter()
	srv := ui.NewReportServer(store, gen, fmtr, nil)

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		log.Fatal(err)
	}
	if err := srv.OpenInBrowser(); err != nil {
		log.Printf("could not open browser: %v", err)
	}
	log.Println("mock UI running — press Ctrl+C to stop")
	select {} // block forever
}

// mockSessions returns a realistic heavy day: 12 morning + 10 afternoon cards,
// plus a small cluster of short sessions to exercise the group-collapse UI.
func mockSessions() []storage.Session {
	day := time.Date(2026, 5, 7, 0, 0, 0, 0, time.Local)
	id := int64(0)
	var out []storage.Session

	add := func(ctype, label string, h, m, durMin int) {
		id++
		start := day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
		dur := durMin * 60
		end := start.Add(time.Duration(dur) * time.Second)
		out = append(out, storage.Session{
			ID: id, DateLocal: "2026-05-07",
			ContextType: ctype, ContextLabel: label,
			StartUTC: start, EndUTC: &end, DurationSecs: &dur,
		})
	}
	addShort := func(ctype, label string, h, m int) {
		id++
		start := day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
		dur := 20 // 20 seconds — short enough to collapse into a group
		end := start.Add(time.Duration(dur) * time.Second)
		out = append(out, storage.Session{
			ID: id, DateLocal: "2026-05-07",
			ContextType: ctype, ContextLabel: label,
			StartUTC: start, EndUTC: &end, DurationSecs: &dur,
		})
	}

	// ── Manhã (07:00–12:00) ────────────────────────────────────────────────
	add("vscode", "trackingSystem – CLI setup & config", 7, 0, 40)
	add("browser", "Gmail – E-mails matinais", 7, 40, 25)
	add("browser", "Slack – Mensagens da equipe", 8, 5, 12)
	add("vscode", "trackingSystem – módulo de autenticação", 8, 17, 48)
	add("browser", "GitHub – Code review PR #142", 9, 5, 28)
	add("browser", "Stack Overflow – debug HTTP 422 error", 9, 33, 14)
	add("vscode", "api-service – refactor controllers", 9, 47, 55)
	add("other", "Daily Standup", 10, 42, 30)
	add("browser", "Notion – Sprint planning", 11, 12, 18)
	add("vscode", "trackingSystem – fix nil pointer panic", 11, 30, 22)
	add("browser", "Hacker News", 11, 52, 5)

	// Three rapid-fire short sessions → should collapse into a group
	addShort("browser", "GitHub notif", 11, 57)
	addShort("browser", "Gmail notif", 11, 58)
	addShort("browser", "Slack notif", 11, 59)

	// ── Tarde (12:00–17:30) ────────────────────────────────────────────────
	add("browser", "YouTube – Go concurrency patterns", 12, 5, 38)
	add("vscode", "pix-api – webhook handler", 12, 43, 52)
	add("browser", "Figma – revisão dos mockups de UI", 13, 35, 22)
	add("other", "Reunião com cliente – demo Sprint 4", 13, 57, 45)
	add("vscode", "pix-web – validação do formulário", 14, 42, 48)
	add("browser", "Chrome DevTools – debug SSE connection", 15, 30, 25)
	add("vscode", "pix-api – fix SSE broadcast race condition", 15, 55, 42)
	add("browser", "GitHub – push + abrir PR #148", 16, 37, 14)
	add("browser", "Slack – review do feedback do cliente", 16, 51, 10)
	add("vscode", "trackingSystem – melhorias na UI multi-coluna", 17, 1, 28)

	return out
}

// ── mock storage ──────────────────────────────────────────────────────────

type mockStore struct{ sessions []storage.Session }

func (s *mockStore) InsertRawEvent(_ context.Context, _ storage.RawEvent) error { return nil }
func (s *mockStore) PurgeOldRawEvents(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (s *mockStore) OpenSession(_ context.Context, _ storage.Session) (int64, error) {
	return 0, nil
}
func (s *mockStore) CheckpointSession(_ context.Context, _ int64, _ int) error { return nil }
func (s *mockStore) CloseSession(_ context.Context, _ int64, _ time.Time, _, _ int) error {
	return nil
}
func (s *mockStore) RecoverCheckpoints(_ context.Context, _ int) error { return nil }
func (s *mockStore) SessionsForDay(_ context.Context, _ string) ([]storage.Session, error) {
	return s.sessions, nil
}
func (s *mockStore) DaysWithData(_ context.Context) ([]string, error) {
	return []string{"2026-05-07"}, nil
}
func (s *mockStore) DeleteDay(_ context.Context, _ string) error { return nil }
func (s *mockStore) SetSessionNote(_ context.Context, id int64, note string) error {
	for i := range s.sessions {
		if s.sessions[i].ID == id {
			s.sessions[i].Note = note
		}
	}
	return nil
}
func (s *mockStore) GetMeta(_ context.Context, _ string) (string, error) { return "", nil }
func (s *mockStore) SetMeta(_ context.Context, _, _ string) error         { return nil }
func (s *mockStore) Migrate(_ context.Context) error                      { return nil }
func (s *mockStore) Close() error                                          { return nil }
