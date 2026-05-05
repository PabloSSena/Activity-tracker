package vscode

import (
	"runtime"
	"testing"
)

func TestDiscover_ProbeLocal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("VS Code workspaceStorage layout is OS-specific; probe is Windows-only")
	}
	m := Discover()
	if len(m) == 0 {
		t.Skip("no VS Code workspaces found on this machine — nothing to verify")
	}
	t.Logf("discovered %d workspaces", len(m))
	for k, v := range m {
		t.Logf("  %s -> %s", k, v)
	}
}

func TestDecodeFileURI(t *testing.T) {
	tests := []struct{ in, want string }{
		{"file:///c%3A/Users/pablo/Documents/freedom", `c:\Users\pablo\Documents\freedom`},
		{"file:///home/user/proj", `\home\user\proj`},
		{"", ""},
		{"http://example.com/foo", ""},
	}
	for _, tt := range tests {
		got := decodeFileURI(tt.in)
		if runtime.GOOS != "windows" {
			// filepath.Clean uses OS separator; just check it's non-empty for valid input
			if tt.want != "" && got == "" {
				t.Errorf("decodeFileURI(%q) = empty, want non-empty", tt.in)
			}
			continue
		}
		if got != tt.want {
			t.Errorf("decodeFileURI(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
