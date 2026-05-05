package vscode

import (
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

// skipDirs lists directory names that are always skipped during walks
// because they contain build output, dependencies, or VCS metadata.
var skipDirs = map[string]bool{
	".git":          true,
	".svn":          true,
	".hg":           true,
	"node_modules":  true,
	".next":         true,
	".nuxt":         true,
	".turbo":        true,
	"dist":          true,
	"build":         true,
	"target":        true,
	"vendor":        true,
	"bin":           true,
	"obj":           true,
	"out":           true,
	"__pycache__":   true,
	".pytest_cache": true,
	".gradle":       true,
	".idea":         true,
	".cache":        true,
	"coverage":      true,
}

// FileMod records a single modified file, relative to a workspace root.
type FileMod struct {
	Path    string // forward-slash, relative to root
	ModTime time.Time
}

// WalkMods walks root once and returns every file with mtime in [since, until].
// Hidden directories (starting with ".") and well-known build/dep dirs are
// skipped. The cap bounds work on huge trees; callers should pick a value
// generous enough to cover an entire day.
func WalkMods(root string, since, until time.Time, cap int) []FileMod {
	if root == "" || cap <= 0 {
		return nil
	}
	var out []FileMod
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path == root {
				return nil
			}
			if skipDirs[name] || (len(name) > 1 && strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		m := info.ModTime()
		if m.Before(since) || m.After(until) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		out = append(out, FileMod{Path: filepath.ToSlash(rel), ModTime: m})
		if len(out) >= cap {
			return filepath.SkipAll
		}
		return nil
	})
	return out
}

// FilterWindow returns the subset of mods with ModTime in [start, end].
func FilterWindow(mods []FileMod, start, end time.Time) []string {
	if len(mods) == 0 {
		return nil
	}
	var out []string
	for _, m := range mods {
		if m.ModTime.Before(start) || m.ModTime.After(end) {
			continue
		}
		out = append(out, m.Path)
	}
	return out
}
