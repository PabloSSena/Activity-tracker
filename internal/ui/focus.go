package ui

import (
	"strings"

	"github.com/user/activitytracker/internal/storage"
)

// FocusStats holds time aggregated by productivity category for one day.
type FocusStats struct {
	ProductiveSecs  int
	DistractionSecs int
	NeutralSecs     int
}

func (f FocusStats) TotalSecs() int { return f.ProductiveSecs + f.DistractionSecs + f.NeutralSecs }

func (f FocusStats) ProductivePct() int {
	if t := f.TotalSecs(); t > 0 {
		return f.ProductiveSecs * 100 / t
	}
	return 0
}

func (f FocusStats) DistractionPct() int {
	if t := f.TotalSecs(); t > 0 {
		return f.DistractionSecs * 100 / t
	}
	return 0
}

func (f FocusStats) NeutralPct() int {
	n := 100 - f.ProductivePct() - f.DistractionPct()
	if n < 0 {
		return 0
	}
	return n
}

// BuildFocusStats classifies every session and returns aggregated totals.
func BuildFocusStats(sessions []storage.Session) FocusStats {
	var f FocusStats
	for _, s := range sessions {
		dur := 0
		if s.DurationSecs != nil {
			dur = *s.DurationSecs
		}
		switch classifySession(s.ContextType, s.ContextLabel) {
		case "productive":
			f.ProductiveSecs += dur
		case "distraction":
			f.DistractionSecs += dur
		default:
			f.NeutralSecs += dur
		}
	}
	return f
}

// classifySession returns "productive", "distraction", or "neutral".
func classifySession(contextType, contextLabel string) string {
	switch contextType {
	case "vscode":
		return "productive"
	case "meeting":
		return "productive"
	case "browser":
		lc := strings.ToLower(contextLabel)
		for _, kw := range productiveKeywords {
			if strings.Contains(lc, kw) {
				return "productive"
			}
		}
		for _, kw := range distractionKeywords {
			if strings.Contains(lc, kw) {
				return "distraction"
			}
		}
		return "neutral"
	default:
		return "neutral"
	}
}

// productiveKeywords matches browser labels that indicate focused work.
var productiveKeywords = []string{
	"github", "gitlab",
	"stack overflow", "stackoverflow",
	"mdn",
	"figma", "notion", "linear", "jira", "confluence", "asana", "trello",
	"vercel", "netlify", "supabase", "firebase",
	"localhost", "127.0.0.1",
	"npm", "postman", "swagger", "insomnia",
	"google docs", "google sheets", "google slides",
	"documentation", "devdocs",
}

// distractionKeywords matches browser labels that indicate off-task browsing.
var distractionKeywords = []string{
	"youtube", "twitter", "instagram", "facebook",
	"netflix", "tiktok", "twitch",
	"reddit", "9gag", "pinterest", "buzzfeed",
	"globo.com", "uol.com", "terra.com", "g1.globo",
}
