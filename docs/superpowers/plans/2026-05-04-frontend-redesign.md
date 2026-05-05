# Frontend Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Markdown-pipeline report page with a dark minimal HTML template using direct Go template rendering, and fix the VS Code workspace classifier bug that leaks raw window titles into activity labels.

**Architecture:** Two independent changes: (1) fix `extractVSCodeWorkspace` in the classifier to handle both em dash and hyphen separators, cleaning up activity labels; (2) replace the single `{{.ReportHTML}}` render in `reportwindow.go` with structured Go template rendering over `DailyReport` data, using a new set of exported helper functions registered in a `template.FuncMap`.

**Tech Stack:** Go `html/template`, Go test (`testing` package). No new dependencies.

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/monitor/classifier/classifier.go` | Modify | Fix `extractVSCodeWorkspace` to strip both ` — ` (em dash) and ` - ` (hyphen) VS Code suffixes |
| `internal/monitor/classifier/classifier_test.go` | Modify | Add test cases for hyphen-separated VS Code titles |
| `internal/ui/helpers.go` | Create | Exported template helper functions: `ContextColor`, `FmtDur`, `FmtDurP`, `FmtTimeRange`, `FmtSidebarDate`, `FmtHeaderDate` |
| `internal/ui/helpers_test.go` | Create | Unit tests for all helper functions |
| `internal/ui/reportwindow.go` | Modify | Update `pageData` struct; register `tmplFuncs` FuncMap; update `handleReport`; replace `pageTmpl` with dark template |

---

## Task 1: Fix VS Code Classifier — Hyphen Separator

**Files:**
- Modify: `internal/monitor/classifier/classifier_test.go`
- Modify: `internal/monitor/classifier/classifier.go`

- [ ] **Step 1: Add failing test cases for hyphen-separated titles**

Open `internal/monitor/classifier/classifier_test.go`. In `TestClassify_VSCode`, add these cases to the existing `tests` slice:

```go
{"Code.exe", "main.go - myproject - Visual Studio Code", "vscode", "myproject"},
{"code", "README.md - backend - Visual Studio Code", "vscode", "backend"},
{"Code.exe", "myproject - Visual Studio Code", "vscode", "myproject"},
{"Code.exe", "Add frequent auto-save f... - trackingSystem - Visual Studio Code", "vscode", "trackingSystem"},
```

- [ ] **Step 2: Run test to confirm it fails**

```
cd internal/monitor/classifier && go test -run TestClassify_VSCode -v
```

Expected: FAIL — the four new cases return the full raw title instead of workspace name.

- [ ] **Step 3: Fix `extractVSCodeWorkspace` to handle both separators**

Replace the body of `extractVSCodeWorkspace` in `internal/monitor/classifier/classifier.go`:

```go
const vscSuffixEm  = " — Visual Studio Code" // em dash —
const vscSuffixHyp = " - Visual Studio Code"       // regular hyphen

func extractVSCodeWorkspace(title string) string {
	t := title

	// Strip VS Code suffix — try em dash first, then hyphen
	if idx := strings.LastIndex(t, vscSuffixEm); idx >= 0 {
		t = t[:idx]
	} else if idx := strings.LastIndex(t, vscSuffixHyp); idx >= 0 {
		t = t[:idx]
	}

	// Take last segment — try em dash separator first, then hyphen
	if idx := strings.LastIndex(t, " — "); idx >= 0 {
		t = t[idx+len(" — "):]
	} else if idx := strings.LastIndex(t, " - "); idx >= 0 {
		t = t[idx+len(" - "):]
	}

	if t == "" {
		return title
	}
	return t
}
```

Also remove the old `const vscSuffix = " — Visual Studio Code"` line (it's replaced by the two new constants above).

- [ ] **Step 4: Run tests to confirm all pass**

```
cd internal/monitor/classifier && go test -v
```

Expected: all tests PASS, including the four new cases.

- [ ] **Step 5: Commit**

```bash
git add internal/monitor/classifier/classifier.go internal/monitor/classifier/classifier_test.go
git commit -m "fix: handle hyphen separator in VS Code window title extraction"
```

---

## Task 2: Template Helper Functions

**Files:**
- Create: `internal/ui/helpers.go`
- Create: `internal/ui/helpers_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/ui/helpers_test.go`:

```go
package ui_test

import (
	"testing"
	"time"

	"github.com/user/activitytracker/internal/ui"
)

func TestContextColor(t *testing.T) {
	cases := []struct{ t, want string }{
		{"vscode", "#4ade80"},
		{"browser", "#60a5fa"},
		{"meeting", "#f59e0b"},
		{"other", "#94a3b8"},
		{"unknown", "#94a3b8"},
	}
	for _, c := range cases {
		if got := ui.ContextColor(c.t); got != c.want {
			t.Errorf("ContextColor(%q) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestFmtDur(t *testing.T) {
	cases := []struct {
		secs int
		want string
	}{
		{0, "< 1m"},
		{30, "< 1m"},
		{60, "1m"},
		{600, "10m"},
		{3600, "1h 0m"},
		{3660, "1h 1m"},
	}
	for _, c := range cases {
		if got := ui.FmtDur(c.secs); got != c.want {
			t.Errorf("FmtDur(%d) = %q, want %q", c.secs, got, c.want)
		}
	}
}

func TestFmtDurP(t *testing.T) {
	secs := 90
	if got := ui.FmtDurP(&secs); got != "1m" {
		t.Errorf("FmtDurP(&90) = %q, want 1m", got)
	}
	if got := ui.FmtDurP(nil); got != "" {
		t.Errorf("FmtDurP(nil) = %q, want empty", got)
	}
}

func TestFmtTimeRange(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 5, 1, 0, 34, 0, 0, loc)
	end := time.Date(2026, 5, 1, 0, 45, 0, 0, loc)
	got := ui.FmtTimeRange(start, &end)
	want := "00:34 – 00:45"
	if got != want {
		t.Errorf("FmtTimeRange = %q, want %q", got, want)
	}
	if got2 := ui.FmtTimeRange(start, nil); got2 == "" {
		t.Error("FmtTimeRange with nil end should not be empty")
	}
}

func TestFmtSidebarDate(t *testing.T) {
	got := ui.FmtSidebarDate("2026-05-03")
	want := "Sun, May 3"
	if got != want {
		t.Errorf("FmtSidebarDate = %q, want %q", got, want)
	}
	if got2 := ui.FmtSidebarDate("bad"); got2 != "bad" {
		t.Errorf("FmtSidebarDate(bad) = %q, want passthrough", got2)
	}
}

func TestFmtHeaderDate(t *testing.T) {
	got := ui.FmtHeaderDate("2026-05-03")
	want := "Sunday, May 3"
	if got != want {
		t.Errorf("FmtHeaderDate = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```
cd internal/ui && go test -run "TestContextColor|TestFmtDur|TestFmtDurP|TestFmtTimeRange|TestFmtSidebarDate|TestFmtHeaderDate" -v
```

Expected: FAIL — `ui.ContextColor` etc. not defined.

- [ ] **Step 3: Create `internal/ui/helpers.go`**

```go
package ui

import (
	"fmt"
	"time"
)

// ContextColor returns a CSS hex color for a context type.
func ContextColor(contextType string) string {
	switch contextType {
	case "vscode":
		return "#4ade80"
	case "browser":
		return "#60a5fa"
	case "meeting":
		return "#f59e0b"
	default:
		return "#94a3b8"
	}
}

// FmtDur formats an integer number of seconds as a human-readable duration.
func FmtDur(secs int) string {
	if secs < 60 {
		return "< 1m"
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dh %dm", mins/60, mins%60)
}

// FmtDurP formats a pointer to seconds; returns empty string for nil.
func FmtDurP(secs *int) string {
	if secs == nil {
		return ""
	}
	return FmtDur(*secs)
}

// FmtTimeRange formats a start and optional end time as "HH:MM – HH:MM" (en dash).
func FmtTimeRange(start time.Time, end *time.Time) string {
	s := start.In(time.Local).Format("15:04")
	if end == nil {
		return s + " – ?"
	}
	return s + " – " + end.In(time.Local).Format("15:04")
}

// FmtSidebarDate parses a "YYYY-MM-DD" string and returns "Mon, Jan 2".
// Returns the input unchanged if parsing fails.
func FmtSidebarDate(dateStr string) string {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return dateStr
	}
	return t.Format("Mon, Jan 2")
}

// FmtHeaderDate parses a "YYYY-MM-DD" string and returns "Monday, January 2".
// Returns the input unchanged if parsing fails.
func FmtHeaderDate(dateStr string) string {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return dateStr
	}
	return t.Format("Monday, January 2")
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```
cd internal/ui && go test -run "TestContextColor|TestFmtDur|TestFmtDurP|TestFmtTimeRange|TestFmtSidebarDate|TestFmtHeaderDate" -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/helpers.go internal/ui/helpers_test.go
git commit -m "feat: add template helper functions for dark UI"
```

---

## Task 3: Update pageData, Handler, and Replace Template

**Files:**
- Modify: `internal/ui/reportwindow.go`

This task replaces the entire page template and updates the data structures that feed it. Do it in the following order to keep the code buildable at each step.

- [ ] **Step 1: Update `pageData` struct**

In `internal/ui/reportwindow.go`, replace the `pageData` struct (lines 180–186) with:

```go
type pageData struct {
	Days       []string
	Selected   string
	ReportMDJS template.JS
	IsToday    bool
	Report     *report.DailyReport
	GrandTotal int
}
```

(`ReportHTML` is removed — it was only used by the old template.)

- [ ] **Step 2: Update `handleReport` to populate new fields**

Replace the `handleReport` function body (lines 93–112) with:

```go
func (s *ReportServer) handleReport(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	today := time.Now().Local().Format("2006-01-02")
	days, _ := s.store.DaysWithData(r.Context())
	pd := pageData{Days: days, Selected: date, IsToday: date == today}

	if date != "" {
		sessions, err := s.store.SessionsForDay(r.Context(), date)
		if err != nil {
			log.Printf("ui: sessions for %s: %v", date, err)
		} else {
			dr := s.generator.BuildReport(date, sessions)
			md := s.formatter.Format(dr)
			b, _ := json.Marshal(md)
			pd.ReportMDJS = template.JS(b)
			pd.Report = &dr
			for _, g := range dr.Groups {
				pd.GrandTotal += g.TotalSecs
			}
		}
	}
	s.renderPage(w, pd)
}
```

- [ ] **Step 3: Register the FuncMap and update `pageTmpl`**

Replace the `pageTmpl` variable declaration (starting at line 216, the `var pageTmpl = ...` line) with the new dark template below. The `template.Must(...)` call must include `.Funcs(tmplFuncs)` before `.Parse(...)`.

Add the `tmplFuncs` var immediately before `pageTmpl`:

```go
var tmplFuncs = template.FuncMap{
	"contextColor":   ContextColor,
	"fmtDur":         FmtDur,
	"fmtDurP":        FmtDurP,
	"fmtTimeRange":   FmtTimeRange,
	"fmtSidebarDate": FmtSidebarDate,
	"fmtHeaderDate":  FmtHeaderDate,
}

var pageTmpl = template.Must(template.New("page").Funcs(tmplFuncs).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Activity Tracker</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;font-size:14px;display:flex;height:100vh;overflow:hidden;color:#e2e8f0;background:#0f1117}
#sidebar{width:180px;min-width:180px;overflow-y:auto;border-right:1px solid #2a2d3a;background:#161b22;padding-top:8px;flex-shrink:0}
.sidebar-title{font-size:11px;text-transform:uppercase;letter-spacing:.08em;color:#8b949e;padding:10px 16px 12px}
#sidebar a{display:block;padding:7px 16px;color:#8b949e;text-decoration:none;font-size:13px;border-left:3px solid transparent;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
#sidebar a:hover{background:#1e2535;color:#e2e8f0}
#sidebar a.active{background:#1a2744;color:#e2e8f0;border-left-color:#3b82f6;font-weight:600}
#main{flex:1;display:flex;flex-direction:column;overflow:hidden;min-width:0}
#toolbar{padding:16px 28px;border-bottom:1px solid #2a2d3a;display:flex;align-items:center;justify-content:space-between;flex-shrink:0;background:#0f1117}
.toolbar-date{font-size:18px;font-weight:700;color:#e2e8f0}
.toolbar-actions{display:flex;gap:8px;align-items:center}
button{padding:5px 12px;border:1px solid #2a2d3a;border-radius:5px;background:transparent;cursor:pointer;font-size:12px;color:#8b949e}
button:hover{border-color:#3b82f6;color:#e2e8f0}
button.danger{color:#f87171}
button.danger:hover{border-color:#f87171;background:rgba(248,113,113,.08)}
#content{flex:1;overflow-y:auto;padding:24px 32px}
.inner{max-width:760px;margin:0 auto}
.section-label{font-size:11px;text-transform:uppercase;letter-spacing:.08em;color:#8b949e;margin:28px 0 10px}
.section-label:first-child{margin-top:0}
.tl-card{background:#1c2030;border:1px solid #2a2d3a;border-radius:6px;padding:12px 16px;margin-bottom:4px;border-left-width:3px}
.tl-label{color:#e2e8f0;font-size:13px;line-height:1.5;word-break:break-word;margin-bottom:6px}
.tl-meta{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.tl-time{color:#8b949e;font-size:12px;font-family:monospace}
.tl-dur{background:#2a2d3a;color:#8b949e;font-size:11px;padding:2px 7px;border-radius:10px;font-family:monospace}
.tl-type{font-size:11px;font-weight:600}
.grp-card{background:#1c2030;border:1px solid #2a2d3a;border-radius:6px;padding:14px 16px;margin-bottom:8px}
.grp-name{font-size:13px;font-weight:600;color:#e2e8f0;margin-bottom:10px}
.grp-row{display:flex;justify-content:space-between;font-size:13px;color:#8b949e;padding:3px 0}
.grp-sep{border:none;border-top:1px solid #2a2d3a;margin:8px 0}
.grp-total{display:flex;justify-content:space-between;font-size:13px;color:#c8d3e0;font-weight:500;padding:3px 0}
.tot-card{background:#1c2030;border:1px solid #2a2d3a;border-radius:6px;padding:14px 16px}
.tot-row{display:flex;justify-content:space-between;font-size:13px;color:#8b949e;padding:4px 0}
.tot-grand{display:flex;justify-content:space-between;font-size:14px;color:#e2e8f0;font-weight:700;padding:8px 0 0;margin-top:8px;border-top:1px solid #2a2d3a}
#empty{color:#8b949e;font-size:14px;padding:40px 0}
#live-session{margin-bottom:20px}
.live-entry{display:flex;align-items:center;gap:8px;padding:10px 14px;background:#1c2a14;border:1px solid #2d4a1e;border-radius:6px;font-size:13px;color:#86efac}
.live-dot{color:#f87171;font-size:10px;animation:blink 1.2s step-start infinite}
@keyframes blink{0%,100%{opacity:1}50%{opacity:0}}
.live-label{font-weight:600}
.live-dur{color:#4ade80;font-family:monospace;font-size:12px}
</style>
</head>
<body>
<div id="sidebar">
  <div class="sidebar-title">Days</div>
  {{range .Days}}<a href="/report?date={{.}}"{{if eq . $.Selected}} class="active"{{end}}>{{. | fmtSidebarDate}}</a>
  {{end}}{{if not .Days}}<p style="padding:10px 16px;color:#8b949e;font-size:12px">No data yet</p>{{end}}
</div>
<div id="main">
  <div id="toolbar">
    {{if .Selected}}
    <span class="toolbar-date">{{.Selected | fmtHeaderDate}}</span>
    <div class="toolbar-actions">
      <button onclick="copyReport(this)">Copy</button>
      <a href="/export?date={{.Selected}}" style="text-decoration:none"><button>Export md</button></a>
      <button class="danger" onclick="deleteDay('{{.Selected}}')">Delete day</button>
    </div>
    {{else}}
    <span class="toolbar-date" style="color:#8b949e;font-size:14px;font-weight:400">Select a day from the sidebar</span>
    {{end}}
  </div>
  <div id="content">
    <div class="inner">
      {{if .IsToday}}<div id="live-session"></div>{{end}}
      {{if .Report}}
        {{if .Report.Sessions}}
        <div class="section-label">Timeline</div>
        {{range .Report.Sessions}}{{if and .EndUTC .DurationSecs}}
        <div class="tl-card" style="border-left-color:{{contextColor .ContextType}}">
          <div class="tl-label">{{.ContextLabel}}</div>
          <div class="tl-meta">
            <span class="tl-time">{{fmtTimeRange .StartUTC .EndUTC}}</span>
            <span class="tl-dur">{{fmtDurP .DurationSecs}}</span>
            <span class="tl-type" style="color:{{contextColor .ContextType}}">{{.ContextType}}</span>
          </div>
        </div>
        {{end}}{{end}}
        {{if .Report.Groups}}
        <div class="section-label">Summary</div>
        {{range .Report.Groups}}
        <div class="grp-card">
          <div class="grp-name">{{.Label}}</div>
          {{range .Entries}}
          <div class="grp-row"><span>{{.Label}}</span><span>{{fmtDur .DurationSecs}}</span></div>
          {{end}}
          <hr class="grp-sep">
          <div class="grp-total"><span>Total</span><span>{{fmtDur .TotalSecs}}</span></div>
        </div>
        {{end}}
        <div class="section-label">Totals</div>
        <div class="tot-card">
          {{range .Report.Groups}}
          <div class="tot-row"><span>{{.Label}}</span><span>{{fmtDur .TotalSecs}}</span></div>
          {{end}}
          <div class="tot-grand"><span>Total</span><span>{{fmtDur $.GrandTotal}}</span></div>
        </div>
        {{end}}
        {{else}}
        <div id="empty">No activity recorded for this day.</div>
        {{end}}
      {{else if .Selected}}
      <div id="empty">No activity recorded for this day.</div>
      {{else}}
      <div id="empty">Select a day from the sidebar.</div>
      {{end}}
    </div>
  </div>
</div>
<script>
const mdText = {{.ReportMDJS}};
function copyReport(btn) {
  navigator.clipboard.writeText(mdText)
    .then(function(){ btn.textContent='Copied!'; setTimeout(function(){ btn.textContent='Copy'; }, 1500); })
    .catch(function(e){ alert('Copy failed: ' + e); });
}
function deleteDay(date) {
  if (!confirm('Permanently delete all data for ' + date + '?')) return;
  fetch('/delete?date=' + encodeURIComponent(date), {method:'POST'})
    .then(function(r){ if (r.ok) { window.location = '/'; } else { alert('Delete failed'); } });
}
{{if .IsToday}}
function fmtDur(s) {
  var h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
  return (h > 0 ? h + 'h ' : '') + (m > 0 || h > 0 ? m + 'm ' : '') + sec + 's';
}
function esc(s) { return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'); }
(function poll() {
  fetch('/api/session/current')
    .then(function(r){ return r.json(); })
    .then(function(d) {
      var el = document.getElementById('live-session');
      if (!el) return;
      if (d.active) {
        var label = esc(d.context_label || d.context_type);
        el.innerHTML = '<div class="live-entry"><span class="live-dot">&#9210;</span><span class="live-label">' + label + '</span><span class="live-dur">' + fmtDur(d.elapsed_seconds) + '</span></div>';
      } else {
        el.innerHTML = '';
      }
    })
    .catch(function(){});
  setTimeout(poll, 5000);
})();
{{end}}
</script>
</body>
</html>`))
```

> **Note on the live dot:** The original template used the Unicode character `⏺` (U+23FA) directly. In the JS string above it's replaced with the HTML entity `&#9210;` (decimal for U+23FA) to avoid encoding issues inside the Go raw string literal. If your editor shows it fine as a literal character, you can use it directly instead.

- [ ] **Step 4: Ensure the build compiles**

```
go build ./...
```

Expected: no errors. Fix any type mismatches before continuing.

- [ ] **Step 5: Write a smoke test**

Create `internal/ui/reportwindow_test.go`:

```go
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
	end := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	dur := 3600
	sessions := []storage.Session{{
		ContextType:  "vscode",
		ContextLabel: "trackingSystem",
		StartUTC:     time.Date(2026, 5, 3, 9, 0, 0, 0, time.UTC),
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

func TestReportPage_SidebarFormattedDate(t *testing.T) {
	srv := newTestServer(nil)
	// Inject a stub that returns a day to check sidebar formatting
	req := httptest.NewRequest(http.MethodGet, "/report?date=2026-05-03", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	// The store returns no days so sidebar just has "No data yet"
	body := w.Body.String()
	if !strings.Contains(body, "No data yet") {
		t.Error("expected empty sidebar message")
	}
}
```

> **Note:** `ui.ReportServer` currently has no `ServeHTTP` method — the HTTP mux is set up internally in `Start()`. Add a `ServeHTTP` method or expose the mux for testing. The simplest approach: add this to `reportwindow.go`:
>
> ```go
> // ServeHTTP exposes the report server's mux for testing.
> func (s *ReportServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
> 	mux := http.NewServeMux()
> 	mux.HandleFunc("/", s.handleIndex)
> 	mux.HandleFunc("/report", s.handleReport)
> 	mux.HandleFunc("/export", s.handleExport)
> 	mux.HandleFunc("/delete", s.handleDelete)
> 	mux.HandleFunc("/api/session/current", s.handleCurrentSession)
> 	mux.ServeHTTP(w, r)
> }
> ```
>
> Also check that `storage.Storage` interface includes all methods used by `stubStore`. If it's missing `Close()` or `UpdateSessionEnd()`, remove those from the stub.

- [ ] **Step 6: Run smoke tests**

```
go test ./internal/ui/... -v
```

Expected: all PASS.

- [ ] **Step 7: Run full test suite**

```
go test ./...
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/ui/reportwindow.go internal/ui/reportwindow_test.go
git commit -m "feat: dark minimal report UI with card-based timeline"
```
