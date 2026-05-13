package report

import (
	"log"
	"path/filepath"
	"time"

	igit "github.com/user/activitytracker/internal/git"
	"github.com/user/activitytracker/internal/storage"
	"github.com/user/activitytracker/internal/vscode"
)

// WorkspaceResolver returns the absolute path for a VS Code workspace name,
// or empty string if unknown.
type WorkspaceResolver func(name string) string

// DailyReport is the in-memory report for one day.
type DailyReport struct {
	Date         string
	Sessions     []storage.Session
	Groups       []Group
	Totals       map[string]int     // context_label → total seconds
	ChangedFiles map[int64][]string // session.ID → relative file paths modified during session
	GitCommits   []igit.RepoCommits // commits per repo for this day, derived on demand
}

// Group is a context cluster in the grouped summary.
type Group struct {
	Label     string
	TotalSecs int
	Entries   []GroupEntry
}

// GroupEntry is a line item within a Group.
type GroupEntry struct {
	Label        string
	DurationSecs int
}

// Generator builds DailyReports from stored sessions.
type Generator struct {
	grouper  *Grouper
	resolver WorkspaceResolver
}

// NewGenerator creates a Generator with an optional Grouper (nil = no grouping).
func NewGenerator(grouper *Grouper) *Generator {
	return &Generator{grouper: grouper}
}

// WithWorkspaceResolver attaches a resolver used to look up filesystem paths
// for VS Code workspaces. When set, BuildReport will list files modified
// during each VS Code session.
func (g *Generator) WithWorkspaceResolver(r WorkspaceResolver) *Generator {
	g.resolver = r
	return g
}

// BuildReport constructs a DailyReport for the given date and sessions.
func (g *Generator) BuildReport(dateLocal string, sessions []storage.Session) DailyReport {
	now := time.Now()

	// Fill in estimated duration for in-progress sessions so the report
	// shows current activity instead of 0s.
	filled := make([]storage.Session, len(sessions))
	for i, s := range sessions {
		if s.DurationSecs == nil {
			secs := int(now.Sub(s.StartUTC).Seconds())
			if secs < 0 {
				secs = 0
			}
			end := now
			s.EndUTC = &end
			s.DurationSecs = &secs
		}
		filled[i] = s
	}

	dr := DailyReport{
		Date:         dateLocal,
		Sessions:     filled,
		Totals:       make(map[string]int),
		ChangedFiles: make(map[int64][]string),
	}

	for _, s := range filled {
		if s.DurationSecs != nil {
			dr.Totals[s.ContextLabel] += *s.DurationSecs
		}
	}

	if g.grouper != nil {
		dr.Groups = g.grouper.Group(filled)
	} else {
		dr.Groups = defaultGroups(filled)
	}

	g.attachChangedFiles(&dr, filled)
	g.attachGitCommits(&dr, filled)

	return dr
}

// attachGitCommits scans VS Code workspace paths used in this day's sessions
// for git repositories and populates dr.GitCommits with commits on dr.Date.
func (g *Generator) attachGitCommits(dr *DailyReport, sessions []storage.Session) {
	if g.resolver == nil {
		return
	}

	rootsSeen := map[string]bool{}

	for _, s := range sessions {
		if s.ContextType != "vscode" {
			continue
		}
		path := g.resolver(s.ContextLabel)
		if path == "" {
			continue
		}
		var candidateRoots []string
		if root := igit.GitRoot(path); root != "" {
			candidateRoots = []string{root}
		} else {
			candidateRoots = igit.SubRepoRoots(path)
		}

		for _, root := range candidateRoots {
			if rootsSeen[root] {
				continue
			}
			rootsSeen[root] = true
			commits, err := igit.DayCommits(root, dr.Date)
			if err != nil {
				log.Printf("report: git commits for %s: %v", root, err)
				continue
			}
			if len(commits) == 0 {
				continue
			}
			dr.GitCommits = append(dr.GitCommits, igit.RepoCommits{
				RepoName: filepath.Base(root),
				RepoPath: root,
				Commits:  commits,
			})
		}
	}
}

// attachChangedFiles walks each VS Code workspace once and buckets modified
// files by the sessions that overlap their mtime.
func (g *Generator) attachChangedFiles(dr *DailyReport, sessions []storage.Session) {
	if g.resolver == nil {
		return
	}

	type sessionWindow struct {
		id    int64
		start time.Time
		end   time.Time
	}
	byWorkspace := map[string][]sessionWindow{}
	for _, s := range sessions {
		if s.ContextType != "vscode" || s.EndUTC == nil {
			continue
		}
		byWorkspace[s.ContextLabel] = append(byWorkspace[s.ContextLabel], sessionWindow{
			id: s.ID, start: s.StartUTC, end: *s.EndUTC,
		})
	}

	const perWorkspaceCap = 5000

	for name, windows := range byWorkspace {
		path := g.resolver(name)
		if path == "" {
			continue
		}
		var earliest, latest time.Time
		for _, w := range windows {
			if earliest.IsZero() || w.start.Before(earliest) {
				earliest = w.start
			}
			if w.end.After(latest) {
				latest = w.end
			}
		}
		mods := vscode.WalkMods(path, earliest, latest, perWorkspaceCap)
		if len(mods) == 0 {
			continue
		}
		for _, w := range windows {
			files := vscode.FilterWindow(mods, w.start, w.end)
			if len(files) > 0 {
				dr.ChangedFiles[w.id] = files
			}
		}
	}
}

// defaultGroups builds a simple ungrouped summary (one group per unique label).
func defaultGroups(sessions []storage.Session) []Group {
	order := []string{}
	seen := map[string]bool{}
	secs := map[string]int{}

	for _, s := range sessions {
		if !seen[s.ContextLabel] {
			order = append(order, s.ContextLabel)
			seen[s.ContextLabel] = true
		}
		if s.DurationSecs != nil {
			secs[s.ContextLabel] += *s.DurationSecs
		}
	}

	groups := make([]Group, 0, len(order))
	for _, label := range order {
		groups = append(groups, Group{
			Label:     label,
			TotalSecs: secs[label],
			Entries:   []GroupEntry{{Label: label, DurationSecs: secs[label]}},
		})
	}
	return groups
}
