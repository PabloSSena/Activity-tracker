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
}

// HasContent reports whether the strip has any segments to render.
func (d DayStrip) HasContent() bool { return len(d.Segments) > 0 }

// BuildDayStrip computes proportional positions for every session over the
// day's active span (earliest start → latest end, padded out to whole hours).
// Hour ticks step by 1h or 2h depending on the span.
func BuildDayStrip(sessions []storage.Session) DayStrip {
	if len(sessions) == 0 {
		return DayStrip{}
	}
	loc := time.Local
	var earliest, latest time.Time
	for _, s := range sessions {
		if s.EndUTC == nil {
			continue
		}
		st := s.StartUTC.In(loc)
		en := s.EndUTC.In(loc)
		if earliest.IsZero() || st.Before(earliest) {
			earliest = st
		}
		if en.After(latest) {
			latest = en
		}
	}
	if earliest.IsZero() || latest.IsZero() || !latest.After(earliest) {
		return DayStrip{}
	}

	start := time.Date(earliest.Year(), earliest.Month(), earliest.Day(),
		earliest.Hour(), 0, 0, 0, loc)
	end := latest
	if latest.Minute() != 0 || latest.Second() != 0 || latest.Nanosecond() != 0 {
		end = time.Date(latest.Year(), latest.Month(), latest.Day(),
			latest.Hour()+1, 0, 0, 0, loc)
	}
	span := end.Sub(start).Seconds()
	if span <= 0 {
		return DayStrip{}
	}

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
		})
	}

	totalHours := int(end.Sub(start).Hours())
	step := 1
	if totalHours > 12 {
		step = 2
	}
	ticks := make([]StripTick, 0, totalHours/step+1)
	for h := 0; h <= totalHours; h += step {
		t := start.Add(time.Duration(h) * time.Hour)
		left := t.Sub(start).Seconds() / span * 100.0
		ticks = append(ticks, StripTick{
			LeftPct: left,
			Label:   t.Format("15h"),
		})
	}

	return DayStrip{Segments: segs, Ticks: ticks}
}
