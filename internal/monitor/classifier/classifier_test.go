package classifier_test

import (
	"testing"

	"github.com/user/activitytracker/internal/monitor/classifier"
)

func TestClassify_VSCode(t *testing.T) {
	tests := []struct {
		process     string
		title       string
		wantType    string
		wantLabel   string
	}{
		{"Code.exe", "main.go — myproject — Visual Studio Code", "vscode", "myproject"},
		{"code", "README.md — backend — Visual Studio Code", "vscode", "backend"},
		{"Code.exe", "myproject — Visual Studio Code", "vscode", "myproject"},
		{"Code.exe", "Visual Studio Code", "vscode", "Visual Studio Code"},
		{"Code.exe", "main.go - myproject - Visual Studio Code", "vscode", "myproject"},
		{"code", "README.md - backend - Visual Studio Code", "vscode", "backend"},
		{"Code.exe", "myproject - Visual Studio Code", "vscode", "myproject"},
		{"Code.exe", "Add frequent auto-save f... - trackingSystem - Visual Studio Code", "vscode", "trackingSystem"},
	}
	for _, tt := range tests {
		ct, cl := classifier.Classify(tt.process, tt.title)
		if ct != tt.wantType {
			t.Errorf("Classify(%q, %q) type = %q, want %q", tt.process, tt.title, ct, tt.wantType)
		}
		if cl != tt.wantLabel {
			t.Errorf("Classify(%q, %q) label = %q, want %q", tt.process, tt.title, cl, tt.wantLabel)
		}
	}
}

func TestClassify_Meeting(t *testing.T) {
	tests := []struct {
		process   string
		title     string
		wantType  string
		wantLabel string
	}{
		{"ms-teams.exe", "Sprint Planning | Microsoft Teams", "meeting", "Sprint Planning"},
		{"Teams.exe", "Microsoft Teams", "meeting", "Microsoft Teams"},
		{"zoom.exe", "Zoom Meeting", "meeting", "Zoom Meeting"},
	}
	for _, tt := range tests {
		ct, cl := classifier.Classify(tt.process, tt.title)
		if ct != tt.wantType {
			t.Errorf("Classify(%q, %q) type = %q, want %q", tt.process, tt.title, ct, tt.wantType)
		}
		if cl != tt.wantLabel {
			t.Errorf("Classify(%q, %q) label = %q, want %q", tt.process, tt.title, cl, tt.wantLabel)
		}
	}
}

func TestClassify_Browser(t *testing.T) {
	browsers := []string{"chrome.exe", "msedge.exe", "firefox.exe", "brave.exe", "google-chrome", "firefox"}
	for _, proc := range browsers {
		ct, cl := classifier.Classify(proc, "Some Page Title")
		if ct != "browser" {
			t.Errorf("Classify(%q) type = %q, want browser", proc, ct)
		}
		if cl != "browser/research" {
			t.Errorf("Classify(%q) label = %q, want browser/research", proc, cl)
		}
	}
}

func TestClassify_Other(t *testing.T) {
	ct, cl := classifier.Classify("Slack.exe", "Slack")
	if ct != "other" {
		t.Errorf("type = %q, want other", ct)
	}
	if cl != "Slack" {
		t.Errorf("label = %q, want Slack", cl)
	}
}

func TestClassify_Other_TruncatesLongTitle(t *testing.T) {
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}
	_, cl := classifier.Classify("notepad.exe", string(long))
	if len(cl) > 100 {
		t.Errorf("label length = %d, want <= 100", len(cl))
	}
}
