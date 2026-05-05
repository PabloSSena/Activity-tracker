<!--
SYNC IMPACT REPORT
==================
Version change: 1.1.1 → 1.2.0 (MINOR — UI architecture replaced: Fyne removed, browser-based localhost UI adopted)

Modified sections:
  - Technical Constraints: replaced "Fyne (cross-platform desktop)" with embedded HTTP server
    serving a browser UI on localhost. "No external runtime dependencies" clarified to mean
    the HTTP server is embedded in the binary (net/http from stdlib).
  - Principle V. Observability & Reliability: removed system tray / no-terminal-window constraint
    (no longer applicable with browser UI); added localhost HTTP server availability requirement.

Templates requiring updates:
  - .specify/templates/plan-template.md ✅ (Constitution Check section is generic; no update required)
  - .specify/templates/spec-template.md ✅ (generic; no update required)
  - .specify/templates/tasks-template.md ✅ (generic; no update required)

Deferred TODOs: none.
-->

# TrackingSystem Constitution

## Core Principles

### I. Simplicity First (NON-NEGOTIABLE)

Start with the simplest solution that satisfies the requirement.
MUST NOT add abstractions, layers, or configurability beyond what a current feature
demands. Complexity MUST be justified in the plan's Complexity Tracking table.
YAGNI applies at all times — no speculative generalization.

The single-binary constraint reinforces this: if a feature would require a separate
process, daemon, or external service, the design MUST be reconsidered first.

### II. Data Integrity & Privacy

All tracking data MUST be stored exclusively in local SQLite — no cloud, no telemetry,
no network calls of any kind during monitoring. This is non-negotiable.

Every write operation MUST be atomic or explicitly transactional.
Data MUST NOT be silently dropped or overwritten — deletes and updates MUST be
traceable (soft-delete or audit log pattern where applicable).

Retention policy: raw events are retained for 30 days; aggregated reports are kept
indefinitely. Users MUST be able to delete any day's data from the UI.

### III. Test-First (NON-NEGOTIABLE)

TDD is mandatory: tests are written and reviewed BEFORE implementation begins.
Red-Green-Refactor cycle MUST be enforced.
A feature is not considered started until at least one failing test exists.
Tests MUST cover the happy path and the primary failure mode of every user story.

Minimum coverage for `internal/report/`: **90%** — this is the critical path.
`internal/monitor/` packages MUST use integration tests with mocks for OS APIs
(not unit tests against live system calls).

### IV. Incremental & Independent Delivery

Every feature MUST be broken into user stories that are independently deliverable,
testable, and demonstrable as an MVP slice.
No story implementation may begin until foundational infrastructure (Phase 2) is done.
Each story MUST ship behind its own checkpoint validation before the next story begins.

### V. Observability & Reliability

No `panic` in production code — all errors MUST be handled and logged.
Errors MUST surface with actionable messages — not generic stack traces.
Every non-trivial operation MUST emit structured logs with enough context to diagnose
failures without a debugger.

Reliability non-negotiables:
- The app MUST serve its UI via an embedded HTTP server accessible at **localhost** — the user interacts through a browser, not a native window.
- The app MUST survive **PC sleep/wake cycles** without losing data or crashing.
- CPU usage MUST stay **below 1%** during normal monitoring.
- First run MUST work **without any configuration** (sensible defaults apply).

### VI. Monitoring Accuracy

Activity collection MUST be reliable and low-noise:
- Polling interval: **5 seconds** — balances accuracy against CPU budget.
- Minimum activity block to record: **2 minutes** — blocks below this threshold are
  discarded to avoid noise.
- Activity is grouped by **contiguous sessions** — same context without interruption.

Captured context per monitor:
- **VSCode**: active file, workspace/folder name, language identifier.
- **Teams**: call state (in-call / not-in-call), meeting name when available.
- **Active window title**: always recorded as fallback context.

Every monitor MUST implement the `Monitor` interface:
`Start()`, `Stop()`, `Events() <-chan Event`.
No global state — all dependencies MUST be injected via interfaces.

## Technical Constraints

- **Language**: Go 1.22+
- **UI**: Browser-based — an embedded HTTP server (Go `net/http` stdlib) serves the UI on `localhost`. The user opens the report by navigating to the local URL in any browser.
- **Storage**: SQLite via `modernc.org/sqlite` (pure Go, no CGO required)
- **Config**: TOML file in user home directory; zero-config first run with sensible defaults
- **Build & distribution**: Makefile + goreleaser; output is a single self-contained binary
- **Target platforms**: Windows (primary), Linux (secondary — X11/Xorg assumed)
- **Performance constraint**: CPU usage below 1% during normal operation
- **No external runtime dependencies** — everything embedded in the binary (HTTP server uses Go stdlib)

## Development Workflow

1. Each feature begins with `/speckit-specify` — spec approved before planning.
2. `/speckit-plan` produces the implementation plan — plan approved before tasks.
3. `/speckit-tasks` generates the ordered task list — reviewed before implementation.
4. `/speckit-implement` executes tasks in dependency order.
5. Git commits are made after each task or logical group (auto-commit hooks enabled).
6. No force-pushes to `main`. All work happens on feature branches.
7. Constitution amendments require updating this file, bumping the version,
   and propagating changes to affected templates before any new feature work begins.

## Governance

This constitution supersedes all other practices and informal agreements.
Amendments require: (1) updating this file, (2) version bump per semver rules,
(3) Sync Impact Report documenting affected artifacts, (4) team acknowledgement
before new features are started.

All plans MUST include a Constitution Check gate before Phase 0 research.
Complexity violations MUST be documented in the plan's Complexity Tracking table.
Principles marked NON-NEGOTIABLE MUST NOT be bypassed without a full constitution
amendment — workarounds are not acceptable substitutes.

Runtime development guidance: `CLAUDE.md` (project root).

**Version**: 1.2.0 | **Ratified**: 2026-05-01 | **Last Amended**: 2026-05-02
