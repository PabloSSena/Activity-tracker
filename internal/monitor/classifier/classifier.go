package classifier

import "strings"

var vsCodeProcesses = map[string]bool{
	"code.exe": true,
	"code":     true,
}

var teamsProcesses = map[string]bool{
	"ms-teams.exe": true,
	"teams.exe":    true,
}

var zoomProcesses = map[string]bool{
	"zoom.exe":    true,
	"us.zoom.xip": true,
}

var browserProcesses = map[string]bool{
	"chrome.exe":    true,
	"msedge.exe":    true,
	"firefox.exe":   true,
	"brave.exe":     true,
	"google-chrome": true,
	"chrome":        true,
	"firefox":       true,
	"firefox-esr":   true,
	"brave":         true,
	"msedge":        true,
}

const vscSuffixEm = " — Visual Studio Code"  // em dash —
const vscSuffixHyp = " - Visual Studio Code" // regular hyphen

// Classify maps a process name and window title to a context type and label.
// Priority: vscode > meeting (Teams) > meeting (Zoom) > browser > other.
func Classify(processName, windowTitle string) (contextType, contextLabel string) {
	proc := strings.ToLower(processName)

	if vsCodeProcesses[proc] {
		return "vscode", extractVSCodeWorkspace(windowTitle)
	}
	if teamsProcesses[proc] {
		return "meeting", extractTeamsMeeting(windowTitle)
	}
	if zoomProcesses[proc] {
		return "meeting", "Zoom Meeting"
	}
	if browserProcesses[proc] {
		return "browser", "browser/research"
	}
	label := windowTitle
	if len(label) > 100 {
		label = label[:100]
	}
	return "other", label
}

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

func extractTeamsMeeting(title string) string {
	// "Meeting Name | Microsoft Teams" → "Meeting Name"
	if before, _, ok := strings.Cut(title, " | Microsoft Teams"); ok {
		return before
	}
	return "Microsoft Teams"
}
