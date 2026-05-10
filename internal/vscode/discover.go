// Package vscode reads VS Code's local state to map workspace names to paths
// and lists files modified inside a given time window.
package vscode

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// DebugDiscover, when true, makes Discover log every workspaceStorage root it
// scans plus the count of workspace.json files it found in each. Useful to
// diagnose Linux installs (snap/flatpak/.deb) where layouts vary.
var DebugDiscover = false

type workspaceFile struct {
	Folder string `json:"folder"`
}

// workspaceStorageDirs returns candidate VS Code workspaceStorage directories
// for the current OS. Covers VS Code, VS Code Insiders, VSCodium, and Cursor.
func workspaceStorageDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var roots []string
	switch runtime.GOOS {
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			roots = []string{appData}
		}
	case "darwin":
		roots = []string{filepath.Join(home, "Library", "Application Support")}
	default: // linux, *bsd
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			roots = append(roots, xdg)
		} else {
			roots = append(roots, filepath.Join(home, ".config"))
		}
		// Snap installs sandbox config under ~/snap/<app>/current/.config.
		// Enumerate all snaps so we don't need to maintain a list of bundle IDs.
		if entries, err := os.ReadDir(filepath.Join(home, "snap")); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					roots = append(roots, filepath.Join(home, "snap", e.Name(), "current", ".config"))
				}
			}
		}
		// Flatpak installs sandbox config under ~/.var/app/<app>/config.
		if entries, err := os.ReadDir(filepath.Join(home, ".var", "app")); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					roots = append(roots, filepath.Join(home, ".var", "app", e.Name(), "config"))
				}
			}
		}
	}
	flavors := []string{"Code", "Code - Insiders", "VSCodium", "Cursor"}
	var out []string
	for _, root := range roots {
		for _, flavor := range flavors {
			out = append(out, filepath.Join(root, flavor, "User", "workspaceStorage"))
		}
	}
	return out
}

// Discover returns a map of workspace basename → absolute path, built from
// VS Code's workspaceStorage directory. When multiple workspaces share a
// basename, the most recently used one wins. Cross-platform: scans all
// known editor flavors (VS Code, Insiders, VSCodium, Cursor).
func Discover() map[string]string {
	type candidate struct {
		path  string
		mtime int64
	}
	byName := map[string]candidate{}

	for _, base := range workspaceStorageDirs() {
		entries, err := os.ReadDir(base)
		if err != nil {
			if DebugDiscover {
				log.Printf("vscode.discover: skip %s: %v", base, err)
			}
			continue
		}
		var found, missingPath int
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			jsonPath := filepath.Join(base, e.Name(), "workspace.json")
			data, err := os.ReadFile(jsonPath)
			if err != nil {
				continue
			}
			var wf workspaceFile
			if err := json.Unmarshal(data, &wf); err != nil {
				continue
			}
			path := decodeFileURI(wf.Folder)
			if path == "" {
				continue
			}
			if _, err := os.Stat(path); err != nil {
				missingPath++
				if DebugDiscover {
					log.Printf("vscode.discover: workspace.json points to missing path %s (uri=%s)", path, wf.Folder)
				}
				continue
			}
			info, err := os.Stat(jsonPath)
			if err != nil {
				continue
			}
			name := filepath.Base(path)
			c := candidate{path: path, mtime: info.ModTime().UnixNano()}
			if existing, ok := byName[name]; !ok || c.mtime > existing.mtime {
				byName[name] = c
			}
			found++
		}
		if DebugDiscover {
			log.Printf("vscode.discover: scanned %s found=%d missing=%d", base, found, missingPath)
		}
	}

	out := make(map[string]string, len(byName))
	keys := make([]string, 0, len(byName))
	for k := range byName {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out[k] = byName[k].path
	}
	return out
}

// decodeFileURI converts a "file:///c%3A/foo" URI to a filesystem path.
// Returns empty string on any failure.
func decodeFileURI(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return ""
	}
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	p := u.Path
	// On Windows file:///c:/foo decodes to /c:/foo — strip the leading slash.
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.Clean(p)
}
