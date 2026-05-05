CREATE TABLE IF NOT EXISTS raw_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp_utc TEXT    NOT NULL,
    window_title  TEXT    NOT NULL,
    process_name  TEXT    NOT NULL,
    context_type  TEXT    NOT NULL CHECK(context_type IN ('vscode','meeting','browser','other')),
    context_label TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_raw_events_timestamp ON raw_events(timestamp_utc);

CREATE TABLE IF NOT EXISTS sessions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    date_local    TEXT    NOT NULL,
    context_type  TEXT    NOT NULL CHECK(context_type IN ('vscode','meeting','browser','other')),
    context_label TEXT    NOT NULL,
    start_utc     TEXT    NOT NULL,
    end_utc       TEXT,
    duration_secs INTEGER,
    is_checkpoint INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_sessions_date_local    ON sessions(date_local);
CREATE INDEX IF NOT EXISTS idx_sessions_is_checkpoint ON sessions(is_checkpoint);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);
