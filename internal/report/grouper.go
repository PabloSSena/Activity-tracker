package report

import (
	"github.com/user/activitytracker/internal/storage"
)

// Grouper applies the adjacent browser grouping rule (FR-013).
type Grouper struct {
	browserAdjacencyMins int
}

// NewGrouper creates a Grouper with the given adjacency window.
func NewGrouper(browserAdjacencyMins int) *Grouper {
	return &Grouper{browserAdjacencyMins: browserAdjacencyMins}
}

// Group processes sessions and returns context groups.
// Browser sessions < adjacency window sandwiched between same-workspace coding
// sessions are merged as "research/ai-assist" under that workspace.
func (g *Grouper) Group(sessions []storage.Session) []Group {
	if len(sessions) == 0 {
		return nil
	}

	maxBrowserSecs := g.browserAdjacencyMins * 60
	merged := make([]bool, len(sessions))

	// Mark browser sessions eligible for merging
	for i, s := range sessions {
		if s.ContextType != "browser" {
			continue
		}
		if s.DurationSecs != nil && *s.DurationSecs > maxBrowserSecs {
			continue
		}
		// Look for same-workspace vscode session before and after
		prevIdx := i - 1
		nextIdx := i + 1
		if prevIdx < 0 || nextIdx >= len(sessions) {
			continue
		}
		prev := sessions[prevIdx]
		next := sessions[nextIdx]
		if prev.ContextType == "vscode" &&
			next.ContextType == "vscode" &&
			prev.ContextLabel == next.ContextLabel {
			merged[i] = true
		}
	}

	// Build groups
	order := []string{}
	seen := map[string]bool{}
	groupMap := map[string]*Group{}

	for i, s := range sessions {
		if merged[i] {
			// Attach to the surrounding workspace group
			prevLabel := sessions[i-1].ContextLabel
			if _, ok := groupMap[prevLabel]; !ok {
				order = append(order, prevLabel)
				seen[prevLabel] = true
				groupMap[prevLabel] = &Group{Label: prevLabel}
			}
			dur := 0
			if s.DurationSecs != nil {
				dur = *s.DurationSecs
			}
			groupMap[prevLabel].TotalSecs += dur
			groupMap[prevLabel].Entries = append(groupMap[prevLabel].Entries,
				GroupEntry{Label: "research/ai-assist", DurationSecs: dur})
			continue
		}

		label := s.ContextLabel
		if !seen[label] {
			order = append(order, label)
			seen[label] = true
			groupMap[label] = &Group{Label: label}
		}
		dur := 0
		if s.DurationSecs != nil {
			dur = *s.DurationSecs
		}
		entryLabel := labelForContext(s.ContextType)
		groupMap[label].TotalSecs += dur
		groupMap[label].Entries = append(groupMap[label].Entries,
			GroupEntry{Label: entryLabel, DurationSecs: dur})
	}

	groups := make([]Group, 0, len(order))
	for _, lbl := range order {
		groups = append(groups, *groupMap[lbl])
	}
	return groups
}

func labelForContext(ct string) string {
	switch ct {
	case "vscode":
		return "Coding"
	case "meeting":
		return "Meeting"
	case "browser":
		return "Browser"
	default:
		return "Other"
	}
}
