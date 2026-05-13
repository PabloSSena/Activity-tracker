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
	pd := pageData{Days: days, Selected: date, IsToday: date == today, Today: today}

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
	Today      string
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
	"timelineByPeriod": TimelineByPeriod,
	"dayStrip":         BuildDayStrip,
	"focusStats":       BuildFocusStats,
	"sessionFocus":     classifySession,
	"fmtCommitTime": func(t time.Time) string {
		return t.In(time.Local).Format("15:04")
	},
}

var pageTmpl = template.Must(template.New("page").Funcs(tmplFuncs).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Activity Tracker</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#0b0d12;--surface:#14171f;--raised:#1a1e28;
  --hairline:#1f2330;--border:#2a2f3d;
  --text:#e8eaed;--text-2:#9aa0aa;--text-3:#5c6370;
  --accent:#14b8a6;--accent-soft:rgba(20,184,166,.10);--accent-bg:rgba(20,184,166,.05);--accent-border:rgba(20,184,166,.35);--accent-ring:rgba(20,184,166,.18);
  --danger:#f87171;--live:#4ade80;
  --ctx-vscode:#4ade80;--ctx-browser:#60a5fa;--ctx-meeting:#f59e0b;--ctx-other:#94a3b8;
}
body{font-family:-apple-system,BlinkMacSystemFont,"Inter","Segoe UI",system-ui,sans-serif;font-size:14px;line-height:1.5;display:flex;height:100vh;overflow:hidden;color:var(--text);background:var(--bg)}
::selection{background:var(--accent-soft);color:var(--text)}

/* Sidebar */
#sidebar{width:200px;min-width:200px;overflow-y:auto;border-right:1px solid var(--hairline);background:var(--surface);padding:8px 0 24px;flex-shrink:0}
.sidebar-title{font-size:10px;text-transform:uppercase;letter-spacing:.12em;font-weight:600;color:var(--text-3);padding:14px 18px 10px}
#sidebar a{position:relative;display:block;padding:8px 18px;color:var(--text-2);text-decoration:none;font-size:13px;font-weight:500;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;transition:background .12s,color .12s}
#sidebar a:hover{background:rgba(255,255,255,.03);color:var(--text)}
#sidebar a.active{background:var(--accent-soft);color:var(--text);font-weight:600;box-shadow:inset 2px 0 0 var(--accent)}
#sidebar a.today::before{content:"";display:inline-block;width:5px;height:5px;border-radius:50%;background:var(--accent);margin-right:8px;vertical-align:middle}

/* Main / toolbar */
#main{flex:1;display:flex;flex-direction:column;overflow:hidden;min-width:0}
#toolbar{padding:18px 36px;border-bottom:1px solid var(--hairline);display:flex;align-items:center;justify-content:space-between;flex-shrink:0;background:var(--bg)}
.toolbar-date{font-size:22px;font-weight:700;letter-spacing:-.01em;color:var(--text)}
.toolbar-empty{font-size:14px;font-weight:400;color:var(--text-2)}
.toolbar-actions{display:flex;gap:6px;align-items:center}

/* Buttons */
button{padding:6px 12px;border:1px solid var(--border);border-radius:6px;background:transparent;cursor:pointer;font-size:12px;font-weight:500;color:var(--text-2);font-family:inherit;transition:border-color .12s,color .12s,background .12s}
button:hover{border-color:var(--accent);color:var(--text)}
button:focus-visible{outline:none;box-shadow:0 0 0 3px var(--accent-ring);border-color:var(--accent)}
button.primary{background:var(--accent);border-color:var(--accent);color:#0b0d12;font-weight:600;padding:6px 14px}
button.primary:hover{background:#0d9488;border-color:#0d9488;color:#0b0d12}
button.danger{color:var(--danger)}
button.danger:hover{border-color:var(--danger);background:rgba(248,113,113,.08);color:var(--danger)}
button.kebab{padding:0;width:30px;height:30px;display:inline-flex;align-items:center;justify-content:center;font-size:18px;line-height:1}

/* Dropdown menus */
.menu{position:relative}
.menu-content{display:none;position:absolute;top:calc(100% + 6px);right:0;min-width:180px;background:var(--raised);border:1px solid var(--border);border-radius:6px;box-shadow:0 8px 24px rgba(0,0,0,.4);padding:4px;z-index:50}
.menu.open .menu-content{display:block}
.menu-item{display:block;width:100%;text-align:left;padding:8px 10px;background:transparent;border:none;border-radius:4px;color:var(--text);font-size:13px;cursor:pointer;text-decoration:none;font-family:inherit;font-weight:500}
.menu-item:hover{background:rgba(255,255,255,.04);color:var(--text);border:none}
.menu-item:focus-visible{outline:none;background:rgba(255,255,255,.04);box-shadow:none}
.menu-item.danger{color:var(--danger)}
.menu-item.danger:hover{background:rgba(248,113,113,.08);color:var(--danger);border:none}

/* Content shell */
#content{flex:1;overflow-y:auto;padding:28px 36px 60px}
.inner{max-width:1200px;margin:0 auto}
.section-label{font-size:11px;text-transform:uppercase;letter-spacing:.1em;font-weight:600;color:var(--text-2);margin:40px 0 12px}
.section-label:first-child{margin-top:0}

/* Day strip */
.strip{margin:0 0 28px;background:var(--surface);border:1px solid var(--hairline);border-radius:8px;padding:14px 16px 10px}
.strip-bar{position:relative;height:26px;background:var(--bg);border-radius:4px;overflow:hidden}
.strip-seg{position:absolute;top:0;bottom:0;cursor:pointer;opacity:.85;transition:opacity .15s,transform .15s;min-width:2px;border-radius:2px}
.strip-seg:hover{opacity:1;transform:scaleY(1.15);z-index:1}
.strip-seg.has-note{box-shadow:inset 0 0 0 2px var(--accent)}
.strip-ticks{position:relative;height:14px;margin-top:8px;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:10px;color:var(--text-3)}
.strip-tick{position:absolute;transform:translateX(-50%);white-space:nowrap;top:0}

/* Period items — 2-column layout */
.period-items{columns:2 360px;column-gap:16px}

/* Period sections */
.period-section{--period-color:var(--border)}
.period-morning{--period-color:#f59e0b}
.period-afternoon{--period-color:#f97316}
.period-evening{--period-color:#8b5cf6}
.period-header-row{display:flex;align-items:center;gap:14px;margin:28px 0 14px}
.period-section:first-child .period-header-row{margin-top:0}
.period-icon{width:40px;height:40px;border-radius:10px;display:flex;align-items:center;justify-content:center;font-size:20px;flex-shrink:0}
.period-morning .period-icon{background:rgba(245,158,11,.15)}
.period-afternoon .period-icon{background:rgba(249,115,22,.15)}
.period-evening .period-icon{background:rgba(139,92,246,.15)}
.period-name{font-size:20px;font-weight:700;color:var(--period-color);line-height:1.2}
.period-stats{font-size:12px;color:var(--text-3);margin-top:3px}
.strip-noon{position:absolute;top:0;bottom:0;width:1px;background:rgba(255,255,255,.22);z-index:2;pointer-events:none}

/* Timeline cards */
.tl-card{background:var(--surface);border:1px solid var(--border);border-left:3px solid var(--period-color,var(--border));border-radius:8px;padding:14px 18px;margin-bottom:8px;scroll-margin-top:80px;transition:background .15s,border-color .15s;break-inside:avoid}
.tl-card:hover{background:#161a23}
.tl-card.has-note{background:var(--accent-bg);border-color:var(--accent-border)}
.tl-card.has-note:hover{background:rgba(20,184,166,.08)}
.tl-label{display:flex;align-items:center;gap:8px;color:var(--text);font-size:14px;font-weight:500;line-height:1.4;word-break:break-word;margin-bottom:8px}
.tl-dot{width:7px;height:7px;border-radius:50%;flex-shrink:0;display:inline-block}
.tl-meta{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.tl-time{color:var(--text-2);font-size:12px;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.tl-dur{background:rgba(255,255,255,.06);color:var(--text-2);font-size:11px;padding:2px 8px;border-radius:10px;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.tl-chip{font-size:10px;font-weight:600;text-transform:lowercase;letter-spacing:.02em;padding:2px 8px;border-radius:10px}
.tl-note-mark{font-size:12px;line-height:1;display:none}
.tl-card.has-note .tl-note-mark{display:inline}
.tl-files{margin-top:12px;padding-top:10px;border-top:1px solid var(--hairline)}
.tl-files-toggle{background:none;border:none;color:var(--text-2);font-size:11px;cursor:pointer;padding:0;text-transform:uppercase;letter-spacing:.08em;font-weight:600}
.tl-files-toggle:hover{color:var(--accent);border:none;background:transparent}
.tl-files-list{margin-top:8px;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:11px;color:var(--text-2);line-height:1.7;word-break:break-all}
.tl-files-list div{padding:1px 0}

/* Group cards */
.tl-group{background:#13161e;border:1px solid var(--hairline);border-left:3px solid var(--period-color,var(--text-3))}
.tl-group.has-children-note{border-left-color:var(--accent)}
.tl-group-head{display:flex;align-items:center;gap:10px;cursor:pointer;user-select:none}
.tl-group-arrow{color:var(--text-3);font-size:10px;width:10px;display:inline-block;transition:transform .2s ease}
.tl-group.expanded .tl-group-arrow{transform:rotate(90deg)}
.tl-group-title{color:var(--text-2);font-size:13px;flex:1}
.tl-group-children{margin-top:12px;padding-top:10px;border-top:1px solid var(--hairline);display:none;flex-direction:column;gap:4px}
.tl-group.expanded .tl-group-children{display:flex}
.tl-group-child{display:flex;align-items:center;gap:10px;font-size:12px;color:var(--text-2);padding:4px 0 4px 12px;scroll-margin-top:80px}
.tl-group-child .tl-time{flex-shrink:0;color:var(--text-3)}
.tl-group-child .tl-dur{background:transparent;padding:0;color:var(--text-3)}
.tl-group-child .tl-chip{font-size:10px;flex-shrink:0}
.tl-group-child .tl-child-label{flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}

/* Notes */
.tl-note-wrap{margin-top:12px}
.tl-note{padding:10px 12px;background:rgba(20,184,166,.06);border-left:2px solid var(--accent);border-radius:0 6px 6px 0;font-size:13px;color:var(--text);line-height:1.55;white-space:pre-wrap;word-break:break-word;cursor:pointer;transition:background .12s}
.tl-note:hover{background:rgba(20,184,166,.10)}
.tl-note-btn{background:none;border:1px solid var(--border);color:var(--text-2);font-size:11px;cursor:pointer;padding:4px 10px;border-radius:12px;font-weight:500;letter-spacing:.01em}
.tl-note-btn:hover{border-color:var(--accent);color:var(--accent);background:transparent}
.tl-note-edit{display:flex;flex-direction:column;gap:8px}
.tl-note-edit textarea{width:100%;min-height:72px;background:var(--bg);border:1px solid var(--border);border-radius:6px;color:var(--text);font-family:inherit;font-size:13px;padding:10px;resize:vertical;line-height:1.5;transition:border-color .12s,box-shadow .12s}
.tl-note-edit textarea:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 3px var(--accent-ring)}
.tl-note-actions{display:flex;gap:6px;justify-content:flex-end}
.tl-note-actions button{padding:5px 12px;font-size:12px}

/* Focus classification dots on timeline cards */
.fc-dot{width:7px;height:7px;border-radius:50%;display:inline-block;flex-shrink:0;cursor:help;margin-left:2px}
.fc-productive{background:#4ade80}
.fc-distraction{background:#f87171}
.fc-neutral{background:var(--border)}

/* Focus card */
.focus-card{background:var(--surface);border:1px solid var(--hairline);border-radius:8px;padding:16px 18px}
.focus-bar{height:10px;border-radius:5px;overflow:hidden;display:flex;margin-bottom:16px;background:var(--raised);gap:2px}
.focus-seg-p{background:#4ade80;border-radius:5px 0 0 5px;transition:width .4s}
.focus-seg-d{background:#f87171;transition:width .4s}
.focus-seg-n{flex:1;border-radius:0 5px 5px 0}
.focus-row{display:flex;align-items:center;gap:8px;padding:5px 0;font-size:13px}
.focus-dot{width:8px;height:8px;border-radius:50%;flex-shrink:0}
.focus-dot-p{background:#4ade80}
.focus-dot-d{background:#f87171}
.focus-dot-n{background:var(--text-3)}
.focus-lbl{flex:1;color:var(--text-2)}
.focus-time{color:var(--text);font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;min-width:56px;text-align:right}
.focus-pct{color:var(--text-3);font-size:12px;min-width:36px;text-align:right;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.focus-sep{border:none;border-top:1px solid var(--hairline);margin:8px 0}
.focus-grand{display:flex;align-items:center;gap:8px;font-size:13px;font-weight:700;color:var(--text)}
.focus-legend{margin-top:14px;font-size:12px;color:var(--text-3)}
.focus-legend summary{cursor:pointer;user-select:none;outline:none;list-style:none;display:flex;align-items:center;gap:6px}
.focus-legend summary::-webkit-details-marker{display:none}
.focus-legend summary::before{content:"▸ How are sessions classified?"}
details[open].focus-legend summary::before{content:"▾ How are sessions classified?"}
.focus-legend summary:hover::before{color:var(--text-2)}
.focus-legend-body{margin-top:10px;display:flex;flex-direction:column;gap:8px;padding:10px 12px;background:var(--raised);border-radius:6px;border:1px solid var(--hairline)}
.focus-legend-row{display:flex;align-items:baseline;gap:8px;line-height:1.55;color:var(--text-2)}
.focus-legend-row strong{color:var(--text);font-weight:600;white-space:nowrap}

/* Git commits */
.git-commits{background:var(--surface);border:1px solid var(--hairline);border-radius:8px;padding:16px 18px}
.git-repo{margin-bottom:16px}
.git-repo:last-child{margin-bottom:0}
.git-repo-name{font-size:13px;font-weight:600;color:var(--text);margin-bottom:8px;display:flex;align-items:center;gap:6px}
.git-repo-name::before{content:"⬡";color:var(--text-3);font-size:12px}
.git-commit-row{display:flex;align-items:baseline;gap:10px;padding:5px 0;border-bottom:1px solid var(--hairline);font-size:13px}
.git-commit-row:last-child{border-bottom:none}
.git-hash{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:11px;color:var(--accent);background:var(--accent-soft);padding:1px 6px;border-radius:4px;flex-shrink:0;letter-spacing:.02em}
.git-subject{flex:1;color:var(--text);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.git-meta{color:var(--text-3);font-size:11px;white-space:nowrap;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}

/* Live + empty + flash */
#empty{color:var(--text-2);font-size:14px;padding:80px 0;text-align:center;line-height:1.6}
#live-session{margin-bottom:24px}
.live-entry{display:flex;align-items:center;gap:10px;padding:10px 14px;background:var(--accent-soft);border:1px solid var(--accent-border);border-radius:8px;font-size:13px;color:var(--text)}
.live-dot{display:inline-block;width:8px;height:8px;border-radius:50%;background:var(--live);box-shadow:0 0 0 0 rgba(74,222,128,.7);animation:heartbeat 1.4s ease-out infinite}
@keyframes heartbeat{0%{transform:scale(1);box-shadow:0 0 0 0 rgba(74,222,128,.55)}50%{transform:scale(1.18);box-shadow:0 0 0 6px rgba(74,222,128,0)}100%{transform:scale(1);box-shadow:0 0 0 0 rgba(74,222,128,0)}}
.live-label{font-weight:600}
.live-dur{color:var(--text-2);font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;margin-left:auto}
.flash{box-shadow:0 0 0 2px var(--accent),0 0 16px rgba(20,184,166,.4) !important;transition:box-shadow .4s}
</style>
</head>
<body>
<div id="sidebar">
  <div class="sidebar-title">Days</div>
  {{range .Days}}<a href="/report?date={{.}}" class="{{if eq . $.Selected}}active {{end}}{{if eq . $.Today}}today{{end}}">{{. | fmtSidebarDate}}</a>
  {{end}}{{if not .Days}}<p style="padding:10px 18px;color:var(--text-3);font-size:12px">No data yet</p>{{end}}
</div>
<div id="main">
  <div id="toolbar">
    {{if .Selected}}
    <span class="toolbar-date">{{.Selected | fmtHeaderDate}}</span>
    <div class="toolbar-actions">
      <button onclick="copyForAI(this)" class="primary">Copy for AI</button>
      <div class="menu">
        <button class="menu-trigger" onclick="toggleMenu(this, event)">Export ▾</button>
        <div class="menu-content">
          <button class="menu-item" onclick="copyReport(this)">Copy markdown</button>
          <a class="menu-item" href="/export?date={{.Selected}}">Download .md</a>
        </div>
      </div>
      <div class="menu">
        <button class="menu-trigger kebab" onclick="toggleMenu(this, event)" title="More actions">⋯</button>
        <div class="menu-content">
          <button class="menu-item danger" onclick="deleteDay('{{.Selected}}')">Delete day</button>
        </div>
      </div>
    </div>
    {{else}}
    <span class="toolbar-date toolbar-empty">Select a day from the sidebar</span>
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
            <div class="strip-seg{{if .HasNote}} has-note{{end}}" style="left:{{.LeftPct}}%;width:{{.WidthPct}}%;background:{{.Color}}"
                 title="{{.TimeRange}} · {{.ContextLabel}}{{if .HasNote}} · 📝{{end}}"
                 onclick="jumpToSession({{.SessionID}})"></div>
            {{end}}
            {{if $strip.HasNoon}}<div class="strip-noon" style="left:{{printf "%.1f" $strip.NoonPct}}%" title="noon"></div>{{end}}
          </div>
          <div class="strip-ticks">
            {{range $strip.Ticks}}<span class="strip-tick" style="left:{{.LeftPct}}%">{{.Label}}</span>{{end}}
          </div>
        </div>
        {{end}}
        <div class="section-label">Timeline by Period</div>
        {{range timelineByPeriod .Report.Sessions}}
        <div class="period-section {{.CSSClass}}">
        <div class="period-header-row">
          <div class="period-icon">{{.Icon}}</div>
          <div>
            <div class="period-name">{{.Label}}</div>
            <div class="period-stats">{{.Count}} activities · {{fmtDur .TotalSecs}} total</div>
          </div>
        </div>
        <div class="period-items">
        {{range .Items}}
          {{if .Group}}
          <div class="tl-card tl-group{{if .NoteCount}} has-children-note{{end}}" id="g-{{.FirstID}}" data-children="{{.ChildrenIDs}}">
            <div class="tl-group-head" onclick="toggleGroup(this)">
              <span class="tl-group-arrow">▶</span>
              <span class="tl-group-title">{{.Count}} short entries{{if .NoteCount}} · {{.NoteCount}} 📝{{end}} · click to expand</span>
              <span class="tl-time">{{.FmtRange}}</span>
              <span class="tl-dur">{{fmtDur .TotalSecs}}</span>
            </div>
            <div class="tl-group-children">
              {{range .Children}}{{if and .EndUTC .DurationSecs}}
              {{$focus := sessionFocus .ContextType .ContextLabel}}
              <div class="tl-group-child" id="s-{{.ID}}">
                <span class="tl-time">{{fmtTimeRange .StartUTC .EndUTC}}</span>
                <span class="tl-dur">{{fmtDurP .DurationSecs}}</span>
                <span class="tl-chip" style="color:{{contextColor .ContextType}};background:{{contextColor .ContextType}}1f">{{.ContextType}}</span>
                <span class="fc-dot fc-{{$focus}}" title="{{$focus}}"></span>
                <span class="tl-child-label">{{.ContextLabel}}</span>{{if .Note}} <span class="tl-note-mark" title="has note" style="display:inline">📝</span>{{end}}
              </div>
              {{end}}{{end}}
            </div>
          </div>
          {{else}}{{with .Session}}{{if and .EndUTC .DurationSecs}}
          {{$files := changedFiles $.Report .ID}}
          {{$focus := sessionFocus .ContextType .ContextLabel}}
          <div class="tl-card{{if .Note}} has-note{{end}}" id="s-{{.ID}}" data-session-id="{{.ID}}">
            <div class="tl-label"><span class="tl-dot" style="background:{{contextColor .ContextType}}"></span><span>{{.ContextLabel}}</span></div>
            <div class="tl-meta">
              <span class="tl-time">{{fmtTimeRange .StartUTC .EndUTC}}</span>
              <span class="tl-dur">{{fmtDurP .DurationSecs}}</span>
              <span class="tl-chip" style="color:{{contextColor .ContextType}};background:{{contextColor .ContextType}}1f">{{.ContextType}}</span>
              <span class="fc-dot fc-{{$focus}}" title="{{$focus}}"></span>
              <span class="tl-note-mark" title="has note">📝</span>
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
        </div>
        </div>{{end}}
        {{if .Report.GitCommits}}
        <div class="section-label">Git Commits</div>
        <div class="git-commits">
          {{range .Report.GitCommits}}
          <div class="git-repo">
            <div class="git-repo-name">{{.RepoName}}</div>
            {{range .Commits}}
            <div class="git-commit-row">
              <span class="git-hash">{{.Hash}}</span>
              <span class="git-subject" title="{{.Subject}}">{{.Subject}}</span>
              <span class="git-meta">{{fmtCommitTime .Timestamp}} · {{.Author}}</span>
            </div>
            {{end}}
          </div>
          {{end}}
        </div>
        {{end}}
        {{with focusStats .Report.Sessions}}{{if .TotalSecs}}
        <div class="section-label">Day Focus</div>
        <div class="focus-card">
          <div class="focus-bar">
            <div class="focus-seg-p" style="width:{{.ProductivePct}}%"></div>
            <div class="focus-seg-d" style="width:{{.DistractionPct}}%"></div>
            <div class="focus-seg-n"></div>
          </div>
          <div class="focus-row">
            <span class="focus-dot focus-dot-p"></span>
            <span class="focus-lbl">Productive</span>
            <span class="focus-time">{{fmtDur .ProductiveSecs}}</span>
            <span class="focus-pct">{{.ProductivePct}}%</span>
          </div>
          <div class="focus-row">
            <span class="focus-dot focus-dot-d"></span>
            <span class="focus-lbl">Distraction</span>
            <span class="focus-time">{{fmtDur .DistractionSecs}}</span>
            <span class="focus-pct">{{.DistractionPct}}%</span>
          </div>
          <div class="focus-row">
            <span class="focus-dot focus-dot-n"></span>
            <span class="focus-lbl">Neutral</span>
            <span class="focus-time">{{fmtDur .NeutralSecs}}</span>
            <span class="focus-pct">{{.NeutralPct}}%</span>
          </div>
          <hr class="focus-sep">
          <div class="focus-grand">
            <span class="focus-dot" style="background:transparent"></span>
            <span class="focus-lbl">Total</span>
            <span class="focus-time">{{fmtDur .TotalSecs}}</span>
            <span class="focus-pct"></span>
          </div>
          <details class="focus-legend">
            <summary></summary>
            <div class="focus-legend-body">
              <div class="focus-legend-row">
                <span class="fc-dot fc-productive" style="margin-top:4px;flex-shrink:0"></span>
                <span><strong>Productive:</strong> VS Code &amp; compatible editors (always) · Meetings / Zoom / Teams (always) · Browser: GitHub, GitLab, Stack Overflow, MDN, Figma, Notion, Linear, Jira, Confluence, Vercel, Netlify, localhost, npm, Postman, Swagger, Google Docs</span>
              </div>
              <div class="focus-legend-row">
                <span class="fc-dot fc-distraction" style="margin-top:4px;flex-shrink:0"></span>
                <span><strong>Distraction:</strong> Browser: YouTube, Twitter, Instagram, Facebook, Netflix, TikTok, Twitch, Reddit, 9gag, Pinterest</span>
              </div>
              <div class="focus-legend-row">
                <span class="fc-dot fc-neutral" style="margin-top:4px;flex-shrink:0"></span>
                <span><strong>Neutral:</strong> Everything else — Slack, Gmail, unknown sites, terminal apps, file managers…</span>
              </div>
            </div>
          </details>
        </div>
        {{end}}{{end}}
        {{else}}
        <div id="empty">Nothing recorded for this day yet.</div>
        {{end}}
      {{else if .Selected}}
      <div id="empty">Nothing recorded for this day yet.</div>
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
    .then(function(){
      var orig = btn.textContent;
      btn.textContent='Copied!';
      setTimeout(function(){ btn.textContent = orig; closeAllMenus(); }, 1200);
    })
    .catch(function(e){ alert('Copy failed: ' + e); });
}
function toggleMenu(triggerEl, ev) {
  if (ev) ev.stopPropagation();
  var menu = triggerEl.parentElement;
  var wasOpen = menu.classList.contains('open');
  closeAllMenus();
  if (!wasOpen) menu.classList.add('open');
}
function closeAllMenus() {
  document.querySelectorAll('.menu.open').forEach(function(m){ m.classList.remove('open'); });
}
document.addEventListener('click', function(ev){
  if (!ev.target.closest('.menu')) closeAllMenus();
});
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
      var card = wrap.closest('.tl-card');
      var trimmed = text.trim();
      wrap.innerHTML = trimmed
        ? '<div class="tl-note" onclick="editNote(this)" data-raw="' + escAttr(trimmed) + '">' + escHtml(trimmed) + '</div>'
        : '<button class="tl-note-btn" onclick="editNote(this)">+ add note</button>';
      if (card) card.classList.toggle('has-note', !!trimmed);
      var seg = document.querySelector('.strip-seg[onclick*="jumpToSession(' + sessionId + ')"]');
      if (seg) seg.classList.toggle('has-note', !!trimmed);
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
