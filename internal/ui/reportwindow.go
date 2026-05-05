package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
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
const mdText = {{if .ReportMDJS}}{{.ReportMDJS}}{{else}}null{{end}};
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
