# Frontend Redesign — Activity Tracker Report Page

**Date:** 2026-05-04  
**Status:** Approved  

## Overview

Full visual redesign of the activity tracker report page. The current UI renders report sections as Markdown tables converted to HTML, which produces confusing pipe-delimited output and uncontrolled text truncation. This redesign replaces the Markdown pipeline with direct Go template rendering, enabling proper card-based layouts, clean typography, and a dark minimal aesthetic.

## Goals

- Make the Timeline readable at a glance — vertical cards, no tables, no pipes
- Fix the "..." in VS Code activity titles (caused by raw window title leaking into the Detail column)
- Full dark minimal visual redesign: clean layout, consistent spacing, proper color hierarchy
- Sidebar dates formatted as human-readable strings, not raw ISO dates

## Non-Goals

- No changes to the data collection pipeline (monitor, classifier, storage)
- No changes to the markdown Export feature (still exports the existing markdown format)
- No changes to the `/api/session/current` live session API

## Architecture

### Current flow (to be replaced for rendering)

```
DailyReport → report.Formatter.Format() → Markdown string → goldmark → template.HTML → rendered in #report div
```

### New flow

```
DailyReport → passed directly as pageData field → Go HTML template renders each section
```

The `pageData` struct gains a `Report *report.DailyReport` field. The formatter is no longer called for page rendering (still used for markdown export). The `pageTmpl` in `reportwindow.go` is updated to render Timeline, Summary, and Totals from the structured data.

## Design

### Layout

Two-column layout:
- **Sidebar:** 180px fixed width, left-aligned, vertically scrollable date list
- **Main content:** Remaining width, scrollable, with a max-width of 860px centered

### Color Palette

| Token | Value | Usage |
|---|---|---|
| `bg-page` | `#0f1117` | Page background |
| `bg-sidebar` | `#161b22` | Sidebar background |
| `bg-card` | `#1c2030` | Card/surface background |
| `border` | `#2a2d3a` | Borders and dividers |
| `text-primary` | `#e2e8f0` | Main content text |
| `text-muted` | `#8b949e` | Secondary labels, timestamps |
| `accent` | `#3b82f6` | Active date indicator, links |
| `color-vscode` | `#4ade80` | VS Code context type |
| `color-browser` | `#60a5fa` | Browser context type |
| `color-other` | `#94a3b8` | Other context type |
| `color-meeting` | `#f59e0b` | Meeting context type |

### Typography

- Font stack: `-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`
- Base size: 14px
- Metadata/labels: 12px
- Section headers: 11px uppercase, letter-spacing 0.08em

### Sidebar

- Background: `bg-sidebar`
- Each date rendered as `Mon, May 3` (Go format: `"Mon, Jan 2"`)
- Selected date: left border `3px solid accent`, background slightly lighter, text `text-primary`
- Unselected: `text-muted`, hover lightens background slightly
- No text truncation (dates are always short)

### Header / Toolbar

- Date displayed as `Sunday, May 3` (Go format: `"Monday, January 2"`)
- Action buttons (Copy, Export md, Delete day) right-aligned, small outlined style
- Delete button: muted red (`#f87171`), only colored on hover

### Timeline Section

Section header: `TIMELINE` — 11px uppercase, `text-muted`, letter-spacing.

Each session entry rendered as a card row:
- **Left accent bar:** 3px vertical stripe in the context type color
- **Content:**
  - Top line: `ContextLabel` — full text, `white-space: normal`, wraps naturally. For VS Code sessions this is the workspace name only (already extracted by the classifier). The raw window title (Detail column) is **not rendered** — this eliminates the "Add frequent auto-save f... - trackingSystem - Visual Studio Code" problem entirely.
  - Bottom line: time range (`00:34 – 00:45`) in `text-muted` + duration pill (`10m`) + context type tag in matching color
- Cards separated by `1px border` divider
- Background: `bg-card`

### Summary Section

Section header: `SUMMARY` — same style as Timeline header.

Each context group is a card:
- Group label (`trackingSystem`, `browser/research`) as card header, `text-primary`, 13px semi-bold
- Each entry type row: label left, duration right-aligned, `text-muted`
- Hairline separator before the total row
- Total row: `Total: 2h 6m`, slightly brighter text

### Totals Section

Section header: `TOTALS`.

Single card with rows:
- Each row: context label left, total duration right-aligned
- Last row bold: `Total — 3h 47m`
- No table borders, rows separated by spacing only

### Live Session Indicator

Kept in its current position (top of main content, above the date header). Restyled to match the dark theme:
- Background: `#1c2a14` (dark green tint)
- Border: `#2d4a1e`
- Animated red dot kept, restyled to match

## Implementation Plan

### Files to change

See "Files to Change (Complete List)" section below.

| File | Change |
|---|---|
| `internal/ui/reportwindow.go` | Update `pageData` struct, add `Report` field; update handler to pass `DailyReport` directly; replace entire `pageTmpl` with new dark template |
| `internal/report/formatter.go` | No change — still used for markdown export |
| `internal/report/generator.go` | No change |

### Template structure (within `pageTmpl`)

The new template replaces the single `{{.ReportHTML}}` render with three explicit sections:

```
{{range .Report.Sessions}}  → Timeline cards
{{range .Report.Groups}}    → Summary cards  
{{range .Report.Totals}}    → Totals rows
```

Helper template functions needed:
- `formatTimeRange(start, end time.Time) string` — "00:34 – 00:45"
- `formatDuration(secs int) string` — "10m", "1h 23m" (already exists as `fmtDur`)
- `contextColor(contextType string) string` — returns CSS color value
- `formatSidebarDate(dateStr string) string` — "Mon, May 3"
- `formatHeaderDate(dateStr string) string` — "Sunday, May 3"

These are registered as `template.FuncMap` entries in `reportwindow.go`.

## Classifier Bug Fix (included in scope)

The "Add frequent auto-save f... - trackingSystem - Visual Studio Code" label is caused by a bug in `internal/monitor/classifier/classifier.go`. The `extractVSCodeWorkspace` function searches for ` — Visual Studio Code` (em dash `—`) but some VS Code window titles use ` - Visual Studio Code` (regular hyphen). When the em dash isn't found, the full raw title is returned as-is, including VS Code's own tab-level "..." truncation.

**Fix:** Update `extractVSCodeWorkspace` to strip both ` — Visual Studio Code` and ` - Visual Studio Code` suffixes, and similarly support both ` — ` and ` - ` as workspace separators. This makes the label clean (e.g., `trackingSystem`) regardless of which separator VS Code uses.

This fix is included in the implementation scope alongside the UI redesign.

## Files to Change (Complete List)

| File | Change |
|---|---|
| `internal/ui/reportwindow.go` | Update `pageData` struct with `Report *report.DailyReport`; update `handleReport` handler; replace `pageTmpl` with new dark template; add template FuncMap helpers |
| `internal/monitor/classifier/classifier.go` | Fix `extractVSCodeWorkspace` to handle both em dash and regular hyphen separators |

## Open Questions

None — all design decisions resolved during brainstorming.
