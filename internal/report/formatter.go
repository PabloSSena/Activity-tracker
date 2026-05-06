package report

import (
	"fmt"
	"strings"
	"time"
)

// Formatter renders DailyReports as Markdown.
type Formatter struct{}

// NewFormatter creates a Formatter.
func NewFormatter() *Formatter { return &Formatter{} }

// Format renders a DailyReport as a plain Markdown string.
func (f *Formatter) Format(dr DailyReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Activity Report — %s\n", dr.Date)

	if len(dr.Sessions) == 0 {
		b.WriteString("\nNo activity recorded for this day.\n")
		return b.String()
	}

	// Timeline table
	b.WriteString("\n## Timeline\n\n")
	b.WriteString("| Time | Duration | Context | Detail | |\n")
	b.WriteString("|------|----------|---------|--------|---|\n")
	for _, s := range dr.Sessions {
		if s.EndUTC == nil || s.DurationSecs == nil {
			continue
		}
		startLocal := s.StartUTC.In(time.Local)
		endLocal := s.EndUTC.In(time.Local)
		timeRange := fmt.Sprintf("%s–%s",
			startLocal.Format("15:04"),
			endLocal.Format("15:04"))
		noteMark := ""
		if strings.TrimSpace(s.Note) != "" {
			noteMark = "📝"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			timeRange,
			f.FormatDuration(*s.DurationSecs),
			s.ContextType,
			s.ContextLabel,
			noteMark)
	}

	// User notes — full-text annotations the user attached to specific sessions.
	hasNotes := false
	for _, s := range dr.Sessions {
		if strings.TrimSpace(s.Note) != "" {
			hasNotes = true
			break
		}
	}
	if hasNotes {
		b.WriteString("\n## My notes\n")
		for _, s := range dr.Sessions {
			note := strings.TrimSpace(s.Note)
			if note == "" || s.EndUTC == nil {
				continue
			}
			startLocal := s.StartUTC.In(time.Local)
			endLocal := s.EndUTC.In(time.Local)
			fmt.Fprintf(&b, "\n### %s–%s · %s — %s\n%s\n",
				startLocal.Format("15:04"),
				endLocal.Format("15:04"),
				s.ContextType,
				s.ContextLabel,
				note)
		}
	}

	// Code changes per VS Code session
	if len(dr.ChangedFiles) > 0 {
		b.WriteString("\n## Code changes\n")
		for _, s := range dr.Sessions {
			files := dr.ChangedFiles[s.ID]
			if len(files) == 0 || s.EndUTC == nil {
				continue
			}
			startLocal := s.StartUTC.In(time.Local)
			endLocal := s.EndUTC.In(time.Local)
			fmt.Fprintf(&b, "\n### %s — %s (%s–%s)\n",
				s.ContextLabel,
				f.FormatDuration(*s.DurationSecs),
				startLocal.Format("15:04"),
				endLocal.Format("15:04"))
			for _, p := range files {
				fmt.Fprintf(&b, "- %s\n", p)
			}
		}
	}

	// Summary
	if len(dr.Groups) > 0 {
		b.WriteString("\n## Summary\n")
		for _, g := range dr.Groups {
			fmt.Fprintf(&b, "\n### %s\n", g.Label)
			for _, e := range g.Entries {
				fmt.Fprintf(&b, "- %s: %s\n", e.Label, f.FormatDuration(e.DurationSecs))
			}
			fmt.Fprintf(&b, "- **Total: %s**\n", f.FormatDuration(g.TotalSecs))
		}
	}

	// Totals table
	b.WriteString("\n## Totals\n\n")
	b.WriteString("| Context | Total |\n")
	b.WriteString("|---------|-------|\n")
	grand := 0
	for _, g := range dr.Groups {
		fmt.Fprintf(&b, "| %s | %s |\n", g.Label, f.FormatDuration(g.TotalSecs))
		grand += g.TotalSecs
	}
	fmt.Fprintf(&b, "| **Total** | **%s** |\n", f.FormatDuration(grand))
	b.WriteString("\n")
	return b.String()
}

// FormatDuration converts seconds to a human-readable duration string.
func (f *Formatter) FormatDuration(secs int) string {
	if secs < 60 {
		return "< 1m"
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%dm", mins)
	}
	h := mins / 60
	m := mins % 60
	return fmt.Sprintf("%dh %dm", h, m)
}
