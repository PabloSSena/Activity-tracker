# Activity Tracker

A privacy-first, single-binary activity tracker for Windows and Linux. It monitors the active window, classifies what you're doing (coding, meetings, browsing), stores everything **locally** in SQLite, and serves a daily report through a browser UI on `localhost`.

No cloud. No telemetry. No network calls during monitoring.

## Features

- **Automatic context detection** — classifies activity into `vscode`, `meeting` (Teams/Zoom), `browser`, or generic window context.
- **VSCode workspace awareness** — discovers open workspaces and labels sessions by folder name.
- **Teams meeting detection** — extracts meeting names from window titles.
- **Browser-based daily report** — embedded HTTP server on `localhost` (random port), opened from the system tray.
- **Resilient to sleep/wake** — checkpoint recovery rebuilds sessions interrupted by power events.
- **Low overhead** — stays under 1% CPU during normal monitoring (5s poll interval).
- **Zero-config first run** — sensible defaults; optional TOML override.
- **Auto-start on login** — Windows registry / Linux desktop entry, toggleable.
- **30-day raw retention** — aggregated reports kept indefinitely; raw events purged after 30 days.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    cmd/activitytracker                   │
│                       (main entry)                       │
└──────────────────────────┬───────────────────────────────┘
                           │
       ┌───────────────────┼───────────────────┐
       ▼                   ▼                   ▼
┌─────────────┐    ┌──────────────┐    ┌──────────────┐
│  monitor/   │    │   storage/   │    │     ui/      │
│  ─ window   │───▶│   SQLite     │◀───│  HTTP server │
│  ─ idle     │    │  (~/.act…)   │    │  + systray   │
│  ─ classify │    └──────────────┘    └──────────────┘
│  ─ collect  │           ▲                    ▲
└─────────────┘           │                    │
                  ┌───────┴───────┐            │
                  │    report/    │────────────┘
                  │ group + format│
                  └───────────────┘
```

Key packages:

- [internal/monitor/window/](internal/monitor/window/) — OS-specific active-window polling (Windows / Linux X11).
- [internal/monitor/idle/](internal/monitor/idle/) — user idle detection.
- [internal/monitor/classifier/](internal/monitor/classifier/) — process + title → context type/label.
- [internal/monitor/collector/](internal/monitor/collector/) — turns polled events into sessions with checkpointing.
- [internal/storage/](internal/storage/) — SQLite schema, migrations, session/event persistence.
- [internal/report/](internal/report/) — grouping and rendering of daily reports.
- [internal/ui/](internal/ui/) — embedded HTTP server + system tray ("Open Report" / "Quit").
- [internal/vscode/](internal/vscode/) — discovers VSCode workspaces from local config.
- [internal/autostart/](internal/autostart/) — login-startup registration per OS.

## Requirements

- **Go 1.22+** to build from source.
- **Windows 10+** (primary target) or **Linux with X11** (Wayland not supported).
- No CGO required — SQLite is pure-Go via `modernc.org/sqlite`.

## Build & Run

```bash
make build      # binary lands in bin/activitytracker
make run        # build + run
make test       # unit tests for the critical packages
make lint       # go vet ./...
```

Or directly:

```bash
go build -o bin/activitytracker ./cmd/activitytracker
./bin/activitytracker
```

On startup the tracker:

1. Creates `~/.activitytracker/` (data dir).
2. Migrates the SQLite DB and recovers any interrupted sessions.
3. Purges raw events older than 30 days.
4. Registers itself for autostart (if enabled).
5. Starts the collector and the local HTTP report server.
6. Adds a system tray icon — click **Open Report** to view the dashboard in your browser.

## Configuration

Configuration is optional. To override defaults, create `~/.activitytracker/config.toml`:

```toml
[monitoring]
poll_interval_secs  = 5     # how often to sample the active window
min_session_secs    = 30    # discard sessions shorter than this
idle_timeout_mins   = 10    # close sessions after this much idle time
checkpoint_secs     = 60    # how often to checkpoint open sessions

[grouping]
browser_adjacency_mins = 15 # merge adjacent browser sessions within this gap

[autostart]
enabled = true
```

Missing fields fall back to defaults — see [internal/config/config.go](internal/config/config.go).

## Data & Privacy

- All data lives in `~/.activitytracker/data.db` (SQLite, mode `0700`).
- No data leaves your machine. The HTTP server binds to `127.0.0.1` only.
- Daily data can be deleted from the report UI.

## Project Conventions

This project uses [Spec Kit](https://github.com/github/spec-kit) for spec-driven development. The constitution at [.specify/memory/constitution.md](.specify/memory/constitution.md) governs:

- Simplicity-first, single-binary design.
- Test-first development (90% coverage required for `internal/report/`).
- Local-only storage as a non-negotiable.
- CPU budget below 1% during monitoring.

Feature specs and plans live under [specs/](specs/).

## Releases

Cross-platform release archives are produced by [GoReleaser](https://goreleaser.com/) using [.goreleaser.yaml](.goreleaser.yaml) — Windows and Linux `amd64` zips, with checksums, published as draft GitHub releases.

## License

TBD.
