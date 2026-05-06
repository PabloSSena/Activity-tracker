package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.up.sql
var migrationFS embed.FS

// DB is the SQLite-backed Storage implementation.
type DB struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at the given path.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("storage: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	return &DB{db: db}, nil
}

func (d *DB) Close() error { return d.db.Close() }

// Migrate runs all pending SQL migration files in version order.
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("storage: create migrations table: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("storage: read migrations dir: %w", err)
	}
	for i, entry := range entries {
		version := i + 1
		var exists int
		_ = d.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&exists)
		if exists > 0 {
			continue
		}
		sql, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("storage: read migration %s: %w", entry.Name(), err)
		}
		if _, err := d.db.ExecContext(ctx, string(sql)); err != nil {
			return fmt.Errorf("storage: apply migration %s: %w", entry.Name(), err)
		}
		if _, err := d.db.ExecContext(ctx,
			`INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`,
			version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("storage: record migration %d: %w", version, err)
		}
		log.Printf("storage: applied migration %s", entry.Name())
	}
	return nil
}

// InsertRawEvent persists a single poll observation.
func (d *DB) InsertRawEvent(ctx context.Context, e RawEvent) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO raw_events(timestamp_utc, window_title, process_name, context_type, context_label)
		 VALUES(?, ?, ?, ?, ?)`,
		e.TimestampUTC.UTC().Format(time.RFC3339),
		e.WindowTitle, e.ProcessName, e.ContextType, e.ContextLabel)
	if err != nil {
		return fmt.Errorf("storage: insert raw event: %w", err)
	}
	return nil
}

// PurgeOldRawEvents deletes raw_events older than before.
func (d *DB) PurgeOldRawEvents(ctx context.Context, before time.Time) (int64, error) {
	res, err := d.db.ExecContext(ctx,
		`DELETE FROM raw_events WHERE timestamp_utc < ?`,
		before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("storage: purge raw events: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// OpenSession creates a new in-progress session checkpoint.
func (d *DB) OpenSession(ctx context.Context, s Session) (int64, error) {
	res, err := d.db.ExecContext(ctx,
		`INSERT INTO sessions(date_local, context_type, context_label, start_utc, end_utc, duration_secs, is_checkpoint)
		 VALUES(?, ?, ?, ?, NULL, 0, 1)`,
		s.DateLocal, s.ContextType, s.ContextLabel,
		s.StartUTC.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("storage: open session: %w", err)
	}
	return res.LastInsertId()
}

// CheckpointSession updates the in-progress session's duration.
func (d *DB) CheckpointSession(ctx context.Context, id int64, durationSecs int) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE sessions SET duration_secs = ? WHERE id = ? AND is_checkpoint = 1`,
		durationSecs, id)
	if err != nil {
		return fmt.Errorf("storage: checkpoint session %d: %w", id, err)
	}
	return nil
}

// CloseSession finalises a session. Deletes it if below minDurationSecs.
func (d *DB) CloseSession(ctx context.Context, id int64, endUTC time.Time, durationSecs int, minDurationSecs int) error {
	if durationSecs < minDurationSecs {
		_, err := d.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
		return err
	}
	_, err := d.db.ExecContext(ctx,
		`UPDATE sessions SET end_utc = ?, duration_secs = ?, is_checkpoint = 0 WHERE id = ?`,
		endUTC.UTC().Format(time.RFC3339), durationSecs, id)
	if err != nil {
		return fmt.Errorf("storage: close session %d: %w", id, err)
	}
	return nil
}

// RecoverCheckpoints closes any sessions left open by a previous crash.
func (d *DB) RecoverCheckpoints(ctx context.Context, minDurationSecs int) error {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, start_utc, duration_secs FROM sessions WHERE is_checkpoint = 1`)
	if err != nil {
		return fmt.Errorf("storage: recover checkpoints query: %w", err)
	}
	defer rows.Close()

	type row struct {
		id           int64
		startUTC     string
		durationSecs int
	}
	var stale []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.startUTC, &r.durationSecs); err != nil {
			return fmt.Errorf("storage: recover checkpoints scan: %w", err)
		}
		stale = append(stale, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range stale {
		start, _ := time.Parse(time.RFC3339, r.startUTC)
		end := start.Add(time.Duration(r.durationSecs) * time.Second)
		if err := d.CloseSession(ctx, r.id, end, r.durationSecs, minDurationSecs); err != nil {
			log.Printf("storage: recover checkpoint %d: %v", r.id, err)
		}
	}
	return nil
}

// SessionsForDay returns completed sessions for a local date, ordered by start_utc.
func (d *DB) SessionsForDay(ctx context.Context, dateLocal string) ([]Session, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, date_local, context_type, context_label, start_utc, end_utc, duration_secs, note
		 FROM sessions
		 WHERE date_local = ?
		 ORDER BY start_utc ASC`,
		dateLocal)
	if err != nil {
		return nil, fmt.Errorf("storage: sessions for day: %w", err)
	}
	defer rows.Close()
	return scanSessions(rows)
}

// DaysWithData returns all dates with sessions, most recent first.
func (d *DB) DaysWithData(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT DISTINCT date_local FROM sessions ORDER BY date_local DESC`)
	if err != nil {
		return nil, fmt.Errorf("storage: days with data: %w", err)
	}
	defer rows.Close()
	var days []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	return days, rows.Err()
}

// DeleteDay permanently removes all sessions for a local date.
func (d *DB) DeleteDay(ctx context.Context, dateLocal string) error {
	_, err := d.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE date_local = ?`, dateLocal)
	if err != nil {
		return fmt.Errorf("storage: delete day %s: %w", dateLocal, err)
	}
	return nil
}

// GetMeta returns a metadata value by key.
func (d *DB) GetMeta(ctx context.Context, key string) (string, error) {
	var val string
	err := d.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("storage: get meta %s: %w", key, err)
	}
	return val, nil
}

// SetMeta upserts a metadata key-value pair.
func (d *DB) SetMeta(ctx context.Context, key, value string) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO meta(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	if err != nil {
		return fmt.Errorf("storage: set meta %s: %w", key, err)
	}
	return nil
}

func scanSessions(rows *sql.Rows) ([]Session, error) {
	var sessions []Session
	for rows.Next() {
		var s Session
		var startUTC string
		var endUTC sql.NullString
		var durationSecs sql.NullInt64
		if err := rows.Scan(&s.ID, &s.DateLocal, &s.ContextType, &s.ContextLabel,
			&startUTC, &endUTC, &durationSecs, &s.Note); err != nil {
			return nil, err
		}
		s.StartUTC, _ = time.Parse(time.RFC3339, startUTC)
		if endUTC.Valid {
			t, _ := time.Parse(time.RFC3339, endUTC.String)
			s.EndUTC = &t
		}
		if durationSecs.Valid {
			v := int(durationSecs.Int64)
			s.DurationSecs = &v
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// SetSessionNote upserts the user-authored annotation for a session.
// Empty string clears the note.
func (d *DB) SetSessionNote(ctx context.Context, id int64, note string) error {
	res, err := d.db.ExecContext(ctx,
		`UPDATE sessions SET note = ? WHERE id = ?`, note, id)
	if err != nil {
		return fmt.Errorf("storage: set session note %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("storage: session %d not found", id)
	}
	return nil
}
