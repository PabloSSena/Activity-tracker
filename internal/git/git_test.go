package git_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/user/activitytracker/internal/git"
)

// initRepo creates a real git repo in a temp dir, configures user identity,
// and returns the repo path.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@example.com")
	run(t, dir, "git", "config", "user.name", "Test User")
	return dir
}

// addCommit creates a file and commits it with the given message,
// optionally forcing the author date via env vars.
func addCommit(t *testing.T, dir, msg string, env []string) {
	t.Helper()
	f := filepath.Join(dir, fmt.Sprintf("f%d.txt", time.Now().UnixNano()))
	if err := os.WriteFile(f, []byte(msg), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	run(t, dir, "git", "add", ".")
	runEnv(t, dir, env, "git", "commit", "-m", msg)
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	runEnv(t, dir, nil, name, args...)
}

func runEnv(t *testing.T, dir string, extra []string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extra...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cmd %s %v: %v\n%s", name, args, err, out)
	}
}

// today returns today's local date in YYYY-MM-DD format.
func today() string { return time.Now().Local().Format("2006-01-02") }

// yesterday returns yesterday's local date in YYYY-MM-DD format.
func yesterday() string { return time.Now().Local().AddDate(0, 0, -1).Format("2006-01-02") }

// ── GitRoot tests ─────────────────────────────────────────────────────────────

func TestGitRoot_InsideRepo(t *testing.T) {
	dir := initRepo(t)
	got := git.GitRoot(dir)
	if got == "" {
		t.Fatal("GitRoot returned empty for a valid repo")
	}
	// Should resolve to the canonical root (may differ in case on Windows)
	if filepath.Base(got) != filepath.Base(dir) {
		t.Errorf("GitRoot(%q) = %q, base mismatch", dir, got)
	}
}

func TestGitRoot_Subdirectory(t *testing.T) {
	dir := initRepo(t)
	sub := filepath.Join(dir, "subdir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	root := git.GitRoot(sub)
	if root == "" {
		t.Fatal("GitRoot returned empty for subdirectory of a repo")
	}
}

func TestGitRoot_NotARepo(t *testing.T) {
	dir := t.TempDir()
	if got := git.GitRoot(dir); got != "" {
		t.Errorf("GitRoot(%q) = %q, want empty", dir, got)
	}
}

func TestGitRoot_NonExistentPath(t *testing.T) {
	if got := git.GitRoot("/nonexistent/path/xyz"); got != "" {
		t.Errorf("GitRoot(nonexistent) = %q, want empty", got)
	}
}

// ── DayCommits tests ──────────────────────────────────────────────────────────

func TestDayCommits_TodayCommit(t *testing.T) {
	dir := initRepo(t)
	addCommit(t, dir, "initial commit", nil)

	commits, err := git.DayCommits(dir, today())
	if err != nil {
		t.Fatalf("DayCommits error: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("expected at least one commit for today, got none")
	}
	if commits[0].Subject != "initial commit" {
		t.Errorf("subject = %q, want %q", commits[0].Subject, "initial commit")
	}
	if commits[0].Hash == "" {
		t.Error("commit hash should not be empty")
	}
	if commits[0].Author == "" {
		t.Error("commit author should not be empty")
	}
	if commits[0].Timestamp.IsZero() {
		t.Error("commit timestamp should not be zero")
	}
}

func TestDayCommits_MultipleCommits(t *testing.T) {
	dir := initRepo(t)
	addCommit(t, dir, "first", nil)
	addCommit(t, dir, "second", nil)
	addCommit(t, dir, "third", nil)

	commits, err := git.DayCommits(dir, today())
	if err != nil {
		t.Fatalf("DayCommits error: %v", err)
	}
	if len(commits) != 3 {
		t.Errorf("got %d commits, want 3", len(commits))
	}
}

func TestDayCommits_NoCommitsYesterday(t *testing.T) {
	dir := initRepo(t)
	addCommit(t, dir, "commit made today", nil)

	commits, err := git.DayCommits(dir, yesterday())
	if err != nil {
		t.Fatalf("DayCommits error: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("expected 0 commits for yesterday, got %d", len(commits))
	}
}

func TestDayCommits_NotARepo(t *testing.T) {
	dir := t.TempDir()
	commits, err := git.DayCommits(dir, today())
	if err != nil {
		t.Fatalf("DayCommits should not return error for non-repo, got: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("expected nil commits for non-repo, got %d", len(commits))
	}
}

func TestDayCommits_MergeCommitsExcluded(t *testing.T) {
	dir := initRepo(t)

	// Create a branch and merge commit
	addCommit(t, dir, "base commit", nil)
	run(t, dir, "git", "checkout", "-b", "feature")
	addCommit(t, dir, "feature work", nil)
	run(t, dir, "git", "checkout", "-")
	// Merge with a commit (not fast-forward)
	runEnv(t, dir, nil, "git", "merge", "--no-ff", "-m", "Merge feature branch", "feature")

	commits, err := git.DayCommits(dir, today())
	if err != nil {
		t.Fatalf("DayCommits error: %v", err)
	}
	for _, c := range commits {
		if c.Subject == "Merge feature branch" {
			t.Error("merge commit should be excluded from results")
		}
	}
}

func TestDayCommits_EmptyRepo(t *testing.T) {
	dir := initRepo(t)
	// No commits yet
	commits, err := git.DayCommits(dir, today())
	if err != nil {
		t.Fatalf("DayCommits error: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("expected 0 commits for empty repo, got %d", len(commits))
	}
}
