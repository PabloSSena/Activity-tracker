package git

import (
	"os"
	"os/exec"
	"strings"
	"time"
)

// Commit represents a single git commit.
type Commit struct {
	Hash      string    // 7-char short hash
	Subject   string    // first line of commit message
	Author    string    // author name
	Timestamp time.Time // author date in local time
}

// RepoCommits groups commits from a single git repository.
type RepoCommits struct {
	RepoName string   // display name (basename of RepoPath)
	RepoPath string   // absolute path to git root
	Commits  []Commit // ordered newest-first
}

// GitRoot returns the root directory of the git repository containing path.
// Returns empty string if path is not inside a git repository or if git is unavailable.
func GitRoot(path string) string {
	out, err := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// SubRepoRoots scans immediate subdirectories of dir for git repositories and
// returns their unique root paths. Used for multi-root VS Code workspaces where
// the workspace label resolves to a parent directory, not a git root itself.
func SubRepoRoots(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var roots []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		root := GitRoot(dir + "/" + e.Name())
		if root != "" && !seen[root] {
			seen[root] = true
			roots = append(roots, root)
		}
	}
	return roots
}

// DayCommits returns all non-merge commits made in repoPath on the given local date (YYYY-MM-DD).
// Returns nil (no error) if git is unavailable, the path is not a git repo, or no commits exist.
func DayCommits(repoPath, dateLocal string) ([]Commit, error) {
	t, err := time.ParseInLocation("2006-01-02", dateLocal, time.Local)
	if err != nil {
		return nil, err
	}
	nextDay := t.AddDate(0, 0, 1).Format("2006-01-02")

	const fieldSep = "\x1f"
	out, err := exec.Command("git", "-C", repoPath,
		"log",
		"--after="+dateLocal+" 00:00:00",
		"--before="+nextDay+" 00:00:00",
		"--pretty=format:%h"+fieldSep+"%s"+fieldSep+"%ai"+fieldSep+"%an",
		"--no-merges",
	).Output()
	if err != nil || len(out) == 0 {
		return nil, nil
	}

	var commits []Commit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, fieldSep, 4)
		if len(parts) != 4 {
			continue
		}
		ts, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[2])
		commits = append(commits, Commit{
			Hash:      parts[0],
			Subject:   parts[1],
			Author:    parts[3],
			Timestamp: ts.In(time.Local),
		})
	}
	return commits, nil
}

