package classifier

import "strings"

var vsCodeProcesses = map[string]bool{
	"code.exe":          true,
	"code":              true,
	"code-insiders":     true,
	"code - insiders":   true,
	"codium":            true,
	"vscodium":          true,
	"cursor":            true,
	"cursor.exe":        true,
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

// vscodeSuffixes lists every editor brand we treat as "VS Code-family".
// Each pair covers em dash and hyphen variants; the order matters because
// "Visual Studio Code - Insiders" must be tried before "Visual Studio Code".
var vscodeSuffixes = []string{
	" — Visual Studio Code - Insiders",
	" - Visual Studio Code - Insiders",
	" — Visual Studio Code",
	" - Visual Studio Code",
	" — VSCodium",
	" - VSCodium",
	" — Codium",
	" - Codium",
	" — Cursor",
	" - Cursor",
}

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
		return "browser", extractBrowserTitle(windowTitle)
	}
	label := windowTitle
	if len([]rune(label)) > 100 {
		label = string([]rune(label)[:100])
	}
	return "other", label
}

func extractVSCodeWorkspace(title string) string {
	t := title

	// Strip the editor-brand suffix. Try the longer/more specific suffixes
	// first so "Visual Studio Code - Insiders" wins over "Visual Studio Code".
	for _, suffix := range vscodeSuffixes {
		if idx := strings.LastIndex(t, suffix); idx >= 0 {
			t = t[:idx]
			break
		}
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

var browserSuffixes = []string{
	" - Google Chrome",
	" — Google Chrome",
	" - Microsoft Edge",
	" — Microsoft Edge",
	" - Mozilla Firefox",
	" — Mozilla Firefox",
	" - Firefox",
	" — Firefox",
	" - Brave",
	" — Brave",
}

func extractBrowserTitle(title string) string {
	for _, suffix := range browserSuffixes {
		if idx := strings.LastIndex(title, suffix); idx >= 0 {
			if page := strings.TrimSpace(title[:idx]); page != "" {
				return page
			}
		}
	}
	if title == "" {
		return "browser/research"
	}
	return title
}

func extractTeamsMeeting(title string) string {
	// "Meeting Name | Microsoft Teams" → "Meeting Name"
	if before, _, ok := strings.Cut(title, " | Microsoft Teams"); ok {
		return before
	}
	return "Microsoft Teams"
}
