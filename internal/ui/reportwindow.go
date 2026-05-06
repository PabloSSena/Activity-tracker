package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/user/activitytracker/internal/report"
	"github.com/user/activitytracker/internal/storage"
)

// LiveSession holds the state of the currently running activity session.
type LiveSession struct {
	ContextType  string
	ContextLabel string
	StartUTC     time.Time
}

// ReportServer serves the daily activity report as a local HTTP page.
type ReportServer struct {
	store     storage.Storage
	generator *report.Generator
	formatter *report.Formatter
	port      int
	liveFn    func() *LiveSession
}

// NewReportServer creates a ReportServer. liveFn may be nil if live session
// display is not needed.
func NewReportServer(store storage.Storage, gen *report.Generator, fmtr *report.Formatter, liveFn func() *LiveSession) *ReportServer {
	return &ReportServer{store: store, generator: gen, formatter: fmtr, liveFn: liveFn}
}

// buildMux constructs and returns the HTTP mux with all registered routes.
func (s *ReportServer) buildMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/report", s.handleReport)
	mux.HandleFunc("/export", s.handleExport)
	mux.HandleFunc("/delete", s.handleDelete)
	mux.HandleFunc("/api/session/current", s.handleCurrentSession)
	mux.HandleFunc("/api/session/note", s.handleSessionNote)
	return mux
}

// ServeHTTP exposes the report server's mux for testing.
func (s *ReportServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.buildMux().ServeHTTP(w, r)
}

// Start begins listening on a random localhost port (non-blocking).
func (s *ReportServer) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("ui: listen: %w", err)
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	log.Printf("ui: report server → http://127.0.0.1:%d", s.port)

	srv := &http.Server{Handler: s.buildMux()}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("ui: http server: %v", err)
		}
	}()
	return nil
}

// OpenInBrowser opens the report page in the default system browser.
func (s *ReportServer) OpenInBrowser() error {
	return openBrowser(fmt.Sprintf("http://127.0.0.1:%d/", s.port))
}

func (s *ReportServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	days, _ := s.store.DaysWithData(r.Context())
	if len(days) > 0 {
		http.Redirect(w, r, "/report?date="+days[0], http.StatusSeeOther)
		return
	}
	s.renderPage(w, pageData{Days: days})
}

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

func (s *ReportServer) handleCurrentSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if s.liveFn == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"monitor not available"}`))
		return
	}
	sess := s.liveFn()
	if sess == nil {
		_, _ = w.Write([]byte(`{"active":false}`))
		return
	}
	elapsed := int64(time.Since(sess.StartUTC).Seconds())
	resp := struct {
		Active       bool   `json:"active"`
		ContextType  string `json:"context_type"`
		ContextLabel string `json:"context_label"`
		StartTime    string `json:"start_time"`
		ElapsedSecs  int64  `json:"elapsed_seconds"`
	}{
		Active:       true,
		ContextType:  sess.ContextType,
		ContextLabel: sess.ContextLabel,
		StartTime:    sess.StartUTC.UTC().Format(time.RFC3339),
		ElapsedSecs:  elapsed,
	}
	b, _ := json.Marshal(resp)
	_, _ = w.Write(b)
}

func (s *ReportServer) handleExport(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		http.Error(w, "missing date", http.StatusBadRequest)
		return
	}
	sessions, err := s.store.SessionsForDay(r.Context(), date)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	dr := s.generator.BuildReport(date, sessions)
	md := s.formatter.Format(dr)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.md"`, date))
	_, _ = w.Write([]byte(md))
}

// handleSessionNote upserts a note on a session via POST /api/session/note?id=<id>
// with the raw note text in the request body. Empty body clears the note.
func (s *ReportServer) handleSessionNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	const maxNoteBytes = 4096
	body, err := io.ReadAll(io.LimitReader(r.Body, maxNoteBytes+1))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if len(body) > maxNoteBytes {
		http.Error(w, "note too long", http.StatusRequestEntityTooLarge)
		return
	}
	note := strings.TrimSpace(string(body))
	if err := s.store.SetSessionNote(r.Context(), id, note); err != nil {
		log.Printf("ui: set session note %d: %v", id, err)
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *ReportServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		http.Error(w, "missing date", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteDay(r.Context(), date); err != nil {
		log.Printf("ui: delete day %s: %v", date, err)
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

type pageData struct {
	Days       []string
	Selected   string
	ReportMDJS template.JS
	IsToday    bool
	Report     *report.DailyReport
	GrandTotal int
}

func (s *ReportServer) renderPage(w http.ResponseWriter, pd pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTmpl.Execute(w, pd); err != nil {
		log.Printf("ui: template execute: %v", err)
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

var tmplFuncs = template.FuncMap{
	"contextColor":   ContextColor,
	"fmtDur":         FmtDur,
	"fmtDurP":        FmtDurP,
	"fmtTimeRange":   FmtTimeRange,
	"fmtSidebarDate": FmtSidebarDate,
	"fmtHeaderDate":  FmtHeaderDate,
	"changedFiles": func(dr *report.DailyReport, id int64) []string {
		if dr == nil {
			return nil
		}
		return dr.ChangedFiles[id]
	},
	"timelineItems": func(sessions []storage.Session) []TimelineItem {
		return ReverseTimelineItems(BuildTimelineItems(sessions))
	},
	"dayStrip": BuildDayStrip,
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
button.primary{background:#3b82f6;border-color:#3b82f6;color:#fff}
button.primary:hover{background:#2563eb;border-color:#2563eb;color:#fff}
#content{flex:1;overflow-y:auto;padding:24px 32px}
.inner{max-width:760px;margin:0 auto}
.section-label{font-size:11px;text-transform:uppercase;letter-spacing:.08em;color:#8b949e;margin:28px 0 10px}
.section-label:first-child{margin-top:0}
.strip{margin:0 0 24px;background:#161b22;border:1px solid #2a2d3a;border-radius:6px;padding:12px 14px 8px}
.strip-bar{position:relative;height:24px;background:#0f1117;border-radius:3px;overflow:hidden}
.strip-seg{position:absolute;top:0;bottom:0;cursor:pointer;opacity:.78;transition:opacity .15s,transform .15s;min-width:2px;border-radius:1px}
.strip-seg:hover{opacity:1;transform:scaleY(1.18);z-index:1}
.strip-ticks{position:relative;height:14px;margin-top:6px;font-family:monospace;font-size:10px;color:#6b7280}
.strip-tick{position:absolute;transform:translateX(-50%);white-space:nowrap;top:0}
.tl-card{background:#1c2030;border:1px solid #2a2d3a;border-radius:6px;padding:12px 16px;margin-bottom:4px;border-left-width:3px;scroll-margin-top:80px}
.tl-label{color:#e2e8f0;font-size:13px;line-height:1.5;word-break:break-word;margin-bottom:6px}
.tl-meta{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.tl-time{color:#8b949e;font-size:12px;font-family:monospace}
.tl-dur{background:#2a2d3a;color:#8b949e;font-size:11px;padding:2px 7px;border-radius:10px;font-family:monospace}
.tl-type{font-size:11px;font-weight:600}
.tl-files{margin-top:10px;padding-top:10px;border-top:1px solid #2a2d3a}
.tl-files-toggle{background:none;border:none;color:#8b949e;font-size:11px;cursor:pointer;padding:0;text-transform:uppercase;letter-spacing:.06em}
.tl-files-toggle:hover{color:#3b82f6}
.tl-files-list{margin-top:8px;font-family:monospace;font-size:11px;color:#94a3b8;line-height:1.7;word-break:break-all}
.tl-files-list div{padding:1px 0}
.tl-group{background:#171a26;border-style:dashed;border-left:3px solid #4b5260}
.tl-group-head{display:flex;align-items:center;gap:10px;cursor:pointer;user-select:none}
.tl-group-arrow{color:#6b7280;font-size:11px;width:10px;display:inline-block;transition:transform .15s}
.tl-group.expanded .tl-group-arrow{transform:rotate(90deg)}
.tl-group-title{color:#94a3b8;font-size:13px;font-style:italic;flex:1}
.tl-group-children{margin-top:10px;padding-top:10px;border-top:1px dashed #2a2d3a;display:none;flex-direction:column;gap:2px}
.tl-group.expanded .tl-group-children{display:flex}
.tl-group-child{display:flex;align-items:center;gap:10px;font-size:12px;color:#94a3b8;padding:3px 0;border-left:2px solid transparent;padding-left:6px;scroll-margin-top:80px}
.tl-group-child .tl-time{font-family:monospace;color:#6b7280;flex-shrink:0}
.tl-group-child .tl-dur{background:transparent;padding:0;color:#6b7280}
.tl-group-child .tl-type{font-size:10px;font-weight:600;flex-shrink:0}
.tl-group-child .tl-child-label{flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.flash{box-shadow:0 0 0 2px #3b82f6,0 0 16px rgba(59,130,246,.4);transition:box-shadow .4s}
.tl-note{margin-top:10px;padding:8px 10px;background:#1a2030;border-left:2px solid #3b82f6;border-radius:0 4px 4px 0;font-size:12px;color:#c8d3e0;line-height:1.5;white-space:pre-wrap;word-break:break-word;cursor:pointer}
.tl-note:hover{background:#1f2740}
.tl-note-empty{color:#6b7280;font-style:italic}
.tl-note-btn{background:none;border:none;color:#6b7280;font-size:11px;cursor:pointer;padding:4px 0;margin-top:6px;text-transform:uppercase;letter-spacing:.06em}
.tl-note-btn:hover{color:#3b82f6}
.tl-note-edit{margin-top:10px;display:flex;flex-direction:column;gap:6px}
.tl-note-edit textarea{width:100%;min-height:60px;background:#0f1117;border:1px solid #2a2d3a;border-radius:4px;color:#e2e8f0;font-family:inherit;font-size:12px;padding:8px;resize:vertical;line-height:1.5}
.tl-note-edit textarea:focus{outline:none;border-color:#3b82f6}
.tl-note-actions{display:flex;gap:6px;justify-content:flex-end}
.tl-note-actions button{padding:4px 10px;font-size:11px}
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
      <button onclick="copyReport(this)">Copy md</button>
      <button onclick="copyForAI(this)" class="primary">Copy for AI</button>
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
        {{$strip := dayStrip .Report.Sessions}}
        {{if $strip.HasContent}}
        <div class="strip">
          <div class="strip-bar">
            {{range $strip.Segments}}
            <div class="strip-seg" style="left:{{.LeftPct}}%;width:{{.WidthPct}}%;background:{{.Color}}"
                 title="{{.TimeRange}} · {{.ContextLabel}}"
                 onclick="jumpToSession({{.SessionID}})"></div>
            {{end}}
          </div>
          <div class="strip-ticks">
            {{range $strip.Ticks}}<span class="strip-tick" style="left:{{.LeftPct}}%">{{.Label}}</span>{{end}}
          </div>
        </div>
        {{end}}
        <div class="section-label">Timeline</div>
        {{range timelineItems .Report.Sessions}}
          {{if .Group}}
          <div class="tl-card tl-group" id="g-{{.FirstID}}" data-children="{{.ChildrenIDs}}">
            <div class="tl-group-head" onclick="toggleGroup(this)">
              <span class="tl-group-arrow">▶</span>
              <span class="tl-group-title">{{.Count}} short entries · click to expand</span>
              <span class="tl-time">{{.FmtRange}}</span>
              <span class="tl-dur">{{fmtDur .TotalSecs}}</span>
            </div>
            <div class="tl-group-children">
              {{range .Children}}{{if and .EndUTC .DurationSecs}}
              <div class="tl-group-child" id="s-{{.ID}}">
                <span class="tl-time">{{fmtTimeRange .StartUTC .EndUTC}}</span>
                <span class="tl-dur">{{fmtDurP .DurationSecs}}</span>
                <span class="tl-type" style="color:{{contextColor .ContextType}}">{{.ContextType}}</span>
                <span class="tl-child-label">{{.ContextLabel}}</span>
              </div>
              {{end}}{{end}}
            </div>
          </div>
          {{else}}{{with .Session}}{{if and .EndUTC .DurationSecs}}
          {{$files := changedFiles $.Report .ID}}
          <div class="tl-card" id="s-{{.ID}}" data-session-id="{{.ID}}" style="border-left-color:{{contextColor .ContextType}}">
            <div class="tl-label">{{.ContextLabel}}</div>
            <div class="tl-meta">
              <span class="tl-time">{{fmtTimeRange .StartUTC .EndUTC}}</span>
              <span class="tl-dur">{{fmtDurP .DurationSecs}}</span>
              <span class="tl-type" style="color:{{contextColor .ContextType}}">{{.ContextType}}</span>
            </div>
            {{if $files}}
            <div class="tl-files">
              <button class="tl-files-toggle" onclick="toggleFiles(this)">{{len $files}} file{{if ne (len $files) 1}}s{{end}} changed ▾</button>
              <div class="tl-files-list" style="display:none">
                {{range $files}}<div>{{.}}</div>{{end}}
              </div>
            </div>
            {{end}}
            <div class="tl-note-wrap">
              {{if .Note}}
              <div class="tl-note" onclick="editNote(this)" data-raw="{{.Note}}">{{.Note}}</div>
              {{else}}
              <button class="tl-note-btn" onclick="editNote(this)">+ add note</button>
              {{end}}
            </div>
          </div>
          {{end}}{{end}}{{end}}
        {{end}}
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
const mdText = {{if .ReportMDJS}}{{.ReportMDJS}}{{else}}null{{end}};
function copyReport(btn) {
  navigator.clipboard.writeText(mdText)
    .then(function(){ btn.textContent='Copied!'; setTimeout(function(){ btn.textContent='Copy md'; }, 1500); })
    .catch(function(e){ alert('Copy failed: ' + e); });
}
function copyForAI(btn) {
  var prompt = "Below is a log of my activity for one day, captured by an automated tracker. " +
    "Sessions annotated with the 📝 icon have a 'My notes' entry below — those are my own descriptions " +
    "of what I was doing in that block; treat them as authoritative over the inferred context. " +
    "Please summarize what I worked on, grouped by topic/project. " +
    "Highlight the main areas I spent time on, any meetings, and approximate time spent. " +
    "Keep the summary concise and use bullet points.\n\n---\n\n";
  navigator.clipboard.writeText(prompt + mdText)
    .then(function(){ btn.textContent='Copied!'; setTimeout(function(){ btn.textContent='Copy for AI'; }, 1500); })
    .catch(function(e){ alert('Copy failed: ' + e); });
}
function toggleFiles(btn) {
  var list = btn.nextElementSibling;
  var open = list.style.display === 'block';
  list.style.display = open ? 'none' : 'block';
  btn.textContent = btn.textContent.replace(open ? '▴' : '▾', open ? '▾' : '▴');
}
function toggleGroup(headEl) {
  headEl.parentElement.classList.toggle('expanded');
}
function editNote(triggerEl) {
  var card = triggerEl.closest('.tl-card');
  if (!card) return;
  var wrap = triggerEl.closest('.tl-note-wrap');
  var sessionId = card.getAttribute('data-session-id');
  var existing = '';
  var noteEl = wrap.querySelector('.tl-note');
  if (noteEl) existing = noteEl.getAttribute('data-raw') || noteEl.textContent;
  var editor = document.createElement('div');
  editor.className = 'tl-note-edit';
  editor.innerHTML = '<textarea placeholder="What were you working on? Anything worth remembering for the AI summary?"></textarea>' +
    '<div class="tl-note-actions">' +
      '<button onclick="cancelNote(this)">Cancel</button>' +
      '<button class="primary" onclick="saveNote(this,' + sessionId + ')">Save</button>' +
    '</div>';
  editor.querySelector('textarea').value = existing;
  wrap.replaceChildren(editor);
  editor.querySelector('textarea').focus();
}
function cancelNote(btn) {
  // Reload to restore the original note state without re-fetching the page server-side.
  window.location.reload();
}
function saveNote(btn, sessionId) {
  var editor = btn.closest('.tl-note-edit');
  var ta = editor.querySelector('textarea');
  var text = ta.value;
  btn.disabled = true; btn.textContent = 'Saving…';
  fetch('/api/session/note?id=' + sessionId, {method:'POST', body: text})
    .then(function(r){
      if (!r.ok) throw new Error('http ' + r.status);
      return r.text();
    })
    .then(function(){
      var wrap = editor.parentElement;
      var trimmed = text.trim();
      wrap.innerHTML = trimmed
        ? '<div class="tl-note" onclick="editNote(this)" data-raw="' + escAttr(trimmed) + '">' + escHtml(trimmed) + '</div>'
        : '<button class="tl-note-btn" onclick="editNote(this)">+ add note</button>';
    })
    .catch(function(e){
      btn.disabled = false; btn.textContent = 'Save';
      alert('Save failed: ' + e);
    });
}
function escHtml(s){return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');}
function escAttr(s){return escHtml(s).replace(/"/g,'&quot;');}
function jumpToSession(id) {
  var el = document.getElementById('s-' + id);
  if (!el) {
    var group = document.querySelector('.tl-group[data-children~="' + id + '"]');
    if (group) {
      group.classList.add('expanded');
      el = document.getElementById('s-' + id);
    }
  }
  if (!el) return;
  el.scrollIntoView({behavior:'smooth', block:'center'});
  el.classList.add('flash');
  setTimeout(function(){ el.classList.remove('flash'); }, 1200);
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
