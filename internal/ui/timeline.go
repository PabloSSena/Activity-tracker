package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/user/activitytracker/internal/storage"
)

// ShortSessionThresholdSecs is the upper bound (exclusive) for "short" sessions
// that get collapsed into a single timeline row when adjacent.
const ShortSessionThresholdSecs = 60

// TimelineItem is a single timeline row: either a single session or a
// collapsed run of short adjacent sessions.
type TimelineItem struct {
	Group    bool
	Session  *storage.Session  // when !Group
	Children []storage.Session // when Group
}

// FirstID returns the first session ID in the item (used as DOM id anchor).
func (t TimelineItem) FirstID() int64 {
	if t.Group {
		if len(t.Children) == 0 {
			return 0
		}
		return t.Children[0].ID
	}
	if t.Session == nil {
		return 0
	}
	return t.Session.ID
}

// ChildrenIDs returns space-separated session IDs of the group's children.
func (t TimelineItem) ChildrenIDs() string {
	if !t.Group {
		return ""
	}
	parts := make([]string, len(t.Children))
	for i, c := range t.Children {
		parts[i] = strconv.FormatInt(c.ID, 10)
	}
	return strings.Join(parts, " ")
}

// Count returns 1 for single-session items, len(Children) for groups.
func (t TimelineItem) Count() int {
	if t.Group {
		return len(t.Children)
	}
	return 1
}

// NoteCount returns how many of the item's sessions have a non-empty note.
// For single-session items, this is 0 or 1. For groups, it's the count of
// annotated children, used to badge collapsed groups in the timeline.
func (t TimelineItem) NoteCount() int {
	if t.Group {
		var n int
		for _, c := range t.Children {
			if strings.TrimSpace(c.Note) != "" {
				n++
			}
		}
		return n
	}
	if t.Session == nil {
		return 0
	}
	if strings.TrimSpace(t.Session.Note) != "" {
		return 1
	}
	return 0
}

// TotalSecs returns the summed duration of the item.
func (t TimelineItem) TotalSecs() int {
	if t.Group {
		var total int
		for _, c := range t.Children {
			if c.DurationSecs != nil {
				total += *c.DurationSecs
			}
		}
		return total
	}
	if t.Session == nil || t.Session.DurationSecs == nil {
		return 0
	}
	return *t.Session.DurationSecs
}

// StartUTC returns the start time of the item (first child for groups).
func (t TimelineItem) StartUTC() time.Time {
	if t.Group {
		if len(t.Children) == 0 {
			return time.Time{}
		}
		return t.Children[0].StartUTC
	}
	if t.Session == nil {
		return time.Time{}
	}
	return t.Session.StartUTC
}

// EndUTC returns the end time of the item (last child's end for groups).
func (t TimelineItem) EndUTC() *time.Time {
	if t.Group {
		if len(t.Children) == 0 {
			return nil
		}
		return t.Children[len(t.Children)-1].EndUTC
	}
	if t.Session == nil {
		return nil
	}
	return t.Session.EndUTC
}

// FmtRange formats the item's time span as "HH:MM – HH:MM".
func (t TimelineItem) FmtRange() string {
	return FmtTimeRange(t.StartUTC(), t.EndUTC())
}

// BuildTimelineItems collapses runs of two or more consecutive short sessions
// (duration < ShortSessionThresholdSecs) into a single TimelineItem group.
// The remaining sessions become single-item entries. Order is preserved
// (chronological, ascending start time).
func BuildTimelineItems(sessions []storage.Session) []TimelineItem {
	items := make([]TimelineItem, 0, len(sessions))
	i := 0
	for i < len(sessions) {
		if isShort(sessions[i]) {
			j := i
			for j < len(sessions) && isShort(sessions[j]) {
				j++
			}
			if j-i >= 2 {
				children := make([]storage.Session, j-i)
				copy(children, sessions[i:j])
				items = append(items, TimelineItem{Group: true, Children: children})
				i = j
				continue
			}
		}
		s := sessions[i]
		items = append(items, TimelineItem{Session: &s})
		i++
	}
	return items
}

// ReverseTimelineItems returns a new slice in reverse order without mutating
// the input.
func ReverseTimelineItems(items []TimelineItem) []TimelineItem {
	out := make([]TimelineItem, len(items))
	for i, it := range items {
		out[len(items)-1-i] = it
	}
	return out
}

func isShort(s storage.Session) bool {
	if s.DurationSecs == nil {
		return false
	}
	return *s.DurationSecs < ShortSessionThresholdSecs
}

// StripSegment is a colored bar in the day overview strip.
type StripSegment struct {
	SessionID    int64
	LeftPct      float64
	WidthPct     float64
	Color        string
	ContextType  string
	ContextLabel string
	TimeRange    string
	HasNote      bool
}

// StripTick is an hour marker on the strip's time axis.
type StripTick struct {
	LeftPct float64
	Label   string
}

// DayStrip holds everything needed to render the day overview bar.
type DayStrip struct {
	Segments []StripSegment
	Ticks    []StripTick
	HasNoon  bool
	NoonPct  float64
}

// PeriodGroup is a named slice of TimelineItems for one part of the day.
type PeriodGroup struct {
	Label     string
	CSSClass  string
	Color     string
	Icon      string
	Count     int
	TotalSecs int
	Items     []TimelineItem
}

// TimelineByPeriod groups reversed-within-period timeline items in chronological
// period order: Morning (<12h), Afternoon (12h–17h), Evening (≥17h). Empty periods omitted.
func TimelineByPeriod(sessions []storage.Session) []PeriodGroup {
	items := ReverseTimelineItems(BuildTimelineItems(sessions))
	type meta struct{ label, cssClass, color, icon string }
	order := []meta{
		{"Morning", "period-morning", "#f59e0b", "☀️"},
		{"Afternoon", "period-afternoon", "#f97316", "🌅"},
		{"Evening", "period-evening", "#8b5cf6", "🌙"},
	}
	buckets := make(map[string][]TimelineItem, 3)
	for _, it := range items {
		h := it.StartUTC().Local().Hour()
		var label string
		switch {
		case h < 12:
			label = "Morning"
		case h < 17:
			label = "Afternoon"
		default:
			label = "Evening"
		}
		buckets[label] = append(buckets[label], it)
	}
	var groups []PeriodGroup
	for _, m := range order {
		its, ok := buckets[m.label]
		if !ok {
			continue
		}
		var count, total int
		for _, it := range its {
			count += it.Count()
			total += it.TotalSecs()
		}
		groups = append(groups, PeriodGroup{
			Label:     m.label,
			CSSClass:  m.cssClass,
			Color:     m.color,
			Icon:      m.icon,
			Count:     count,
			TotalSecs: total,
			Items:     its,
		})
	}
	return groups
}

// HasContent reports whether the strip has any segments to render.
func (d DayStrip) HasContent() bool { return len(d.Segments) > 0 }

// BuildDayStrip positions every session on a fixed 24-hour axis (midnight→midnight),
// giving a consistent scale across days and placing noon always at 50%.
func BuildDayStrip(sessions []storage.Session) DayStrip {
	if len(sessions) == 0 {
		return DayStrip{}
	}
	loc := time.Local
	var ref time.Time
	for _, s := range sessions {
		if s.EndUTC != nil {
			ref = s.StartUTC.In(loc)
			break
		}
	}
	if ref.IsZero() {
		return DayStrip{}
	}

	start := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, loc)
	const span = 86400.0 // seconds in a day

	segs := make([]StripSegment, 0, len(sessions))
	for _, s := range sessions {
		if s.EndUTC == nil {
			continue
		}
		st := s.StartUTC.In(loc)
		en := s.EndUTC.In(loc)
		left := st.Sub(start).Seconds() / span * 100.0
		width := en.Sub(st).Seconds() / span * 100.0
		if width < 0.05 {
			width = 0.05
		}
		segs = append(segs, StripSegment{
			SessionID:    s.ID,
			LeftPct:      left,
			WidthPct:     width,
			Color:        ContextColor(s.ContextType),
			ContextType:  s.ContextType,
			ContextLabel: s.ContextLabel,
			TimeRange:    FmtTimeRange(s.StartUTC, s.EndUTC),
			HasNote:      strings.TrimSpace(s.Note) != "",
		})
	}

	ticks := []StripTick{
		{LeftPct: 0, Label: "0h"},
		{LeftPct: 25, Label: "6h"},
		{LeftPct: 50, Label: "12h"},
		{LeftPct: 75, Label: "18h"},
		{LeftPct: 100, Label: "24h"},
	}

	return DayStrip{Segments: segs, Ticks: ticks, HasNoon: true, NoonPct: 50.0}
}
