# Activity Tracker

A privacy-first, single-binary activity tracker for Windows and Linux. It monitors the active window, classifies what you're doing (coding, meetings, browsing), stores everything **locally** in SQLite, and serves a daily report through a browser UI on `localhost`.

No cloud. No telemetry. No network calls during monitoring.

## Features

- **Automatic context detection** — classifies activity into `vscode`, `meeting` (Teams/Zoom), `browser`, or generic window context.
- **VSCode workspace awareness** — discovers open workspaces and labels sessions by folder name.
- **Teams/Zoom meeting detection** — extracts meeting names from window titles.
- **Browser-based daily report** — embedded HTTP server on `localhost` (random port), opened from the system tray.
- **Resilient to sleep/wake** — checkpoint recovery rebuilds sessions interrupted by power events.
- **Low overhead** — stays under 1% CPU during normal monitoring (5s poll interval).
- **Zero-config first run** — sensible defaults; optional TOML override.
- **Auto-start on login** — Windows registry / Linux desktop entry, toggleable.
- **30-day raw retention** — aggregated reports kept indefinitely; raw events purged after 30 days.
- **Session notes** — add context notes to any recorded session from the report UI.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    cmd/activitytracker                   │
│                       (main entry)                       │
└──────────────────────┬───────────────────────────────────┘
                       │
     ┌─────────────────┼─────────────────┐
     ▼                 ▼                 ▼
┌─────────────┐ ┌──────────────┐ ┌──────────────┐
│  monitor/   │ │   storage/   │ │     ui/      │
│  ─ window   │─▶   SQLite     │◀─  HTTP server │
│  ─ idle     │ │  (~/.act…)   │ │  + systray   │
│  ─ classify │ └──────────────┘ └──────────────┘
│  ─ collect  │         ▲                ▲
└─────────────┘         │                │
                ┌───────┴───────┐        │
                │    report/    │────────┘
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

---

## Installation

### Requirements

| | Minimum |
|---|---|
| **Go** | 1.22+ |
| **OS** | Windows 10+ or Linux with X11 (Wayland not supported) |
| **CGO** | Not required — SQLite is pure-Go via `modernc.org/sqlite` |

> **Linux users:** you must install `xdotool` before building or running the tracker, as it is used for active-window detection on X11.
>
> ```bash
> # Debian / Ubuntu
> sudo apt install xdotool
>
> # Fedora / RHEL
> sudo dnf install xdotool
>
> # Arch
> sudo pacman -S xdotool
> ```

### Install Go

Download and install Go 1.22+ from [go.dev/dl](https://go.dev/dl/). Confirm the installation:

```bash
go version
# go version go1.22.x ...
```

### Clone and build

```bash
git clone https://github.com/your-org/activitytracker.git
cd activitytracker
```

**Linux / macOS:**

```bash
make build
# binary lands in bin/activitytracker
```

**Windows (PowerShell):**

```powershell
.\make.ps1 build
# binary lands in bin\activitytracker.exe
```

Or build directly with Go (works on both platforms):

```bash
# Linux / macOS
go build -o bin/activitytracker ./cmd/activitytracker

# Windows
go build -o bin\activitytracker.exe .\cmd\activitytracker
```

### Run

```bash
# Linux / macOS
./bin/activitytracker

# Windows
.\bin\activitytracker.exe
```

On first launch the tracker:

1. Creates `~/.activitytracker/` (data directory).
2. Initialises and migrates the SQLite database.
3. Recovers any sessions interrupted by a previous crash or sleep event.
4. Purges raw events older than 30 days.
5. Registers itself for autostart on login (if enabled).
6. Starts the activity collector and the local HTTP report server.
7. Adds a system tray icon — click **Open Report** to view the dashboard in your browser.

---

## Available Make Targets

| Target | Description |
|---|---|
| `make build` | Compile the binary into `bin/` |
| `make run` | Build and immediately run |
| `make test` | Run unit tests |
| `make lint` | Run `go vet ./...` |
| `make clean` | Remove build artefacts |

On Windows substitute `make` with `.\make.ps1`.

---

## Configuration

Configuration is optional. All fields have sensible defaults. To override, create `~/.activitytracker/config.toml`:

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

---

## Data & Privacy

- All data lives in `~/.activitytracker/data.db` (SQLite, mode `0700`).
- No data leaves your machine. The HTTP server binds to `127.0.0.1` only.
- Daily data can be deleted from the report UI.

---

## Development

```bash
make test    # unit tests
make lint    # static analysis
```

This project uses [Spec Kit](https://github.com/github/spec-kit) for spec-driven development. Feature specs and plans live under [specs/](specs/). The project constitution at [.specify/memory/constitution.md](.specify/memory/constitution.md) governs design constraints (single-binary, local-only storage, <1% CPU budget, test-first for `internal/report/`).

### Releases

Cross-platform release archives are produced by [GoReleaser](https://goreleaser.com/) using [.goreleaser.yaml](.goreleaser.yaml) — Windows and Linux `amd64` zips, with checksums, published as draft GitHub releases.

---

## License

TBD.
