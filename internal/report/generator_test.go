package report_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	igit "github.com/user/activitytracker/internal/git"
	"github.com/user/activitytracker/internal/report"
	"github.com/user/activitytracker/internal/storage"
)

// ── git integration helpers ───────────────────────────────────────────────────

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "git", "init")
	runGit(t, dir, "git", "config", "user.email", "test@example.com")
	runGit(t, dir, "git", "config", "user.name", "Test User")
	return dir
}

func commitInRepo(t *testing.T, dir, msg string) {
	t.Helper()
	f := filepath.Join(dir, fmt.Sprintf("f%d.txt", time.Now().UnixNano()))
	if err := os.WriteFile(f, []byte(msg), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGit(t, dir, "git", "add", ".")
	runGit(t, dir, "git", "commit", "-m", msg)
}

func runGit(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cmd %s %v: %v\n%s", name, args, err, out)
	}
}

// ── attachGitCommits tests ────────────────────────────────────────────────────

func TestGenerator_GitCommits_PopulatedWhenCommitsExist(t *testing.T) {
	repoDir := initTestRepo(t)
	commitInRepo(t, repoDir, "feat: add widget")
	commitInRepo(t, repoDir, "fix: correct typo")

	resolver := func(name string) string {
		if name == "myrepo" {
			return repoDir
		}
		return ""
	}

	sessions := []storage.Session{
		makeSession("vscode", "myrepo", 9, 0, 60),
	}
	today := time.Now().Local().Format("2006-01-02")

	g := report.NewGenerator(nil).WithWorkspaceResolver(resolver)
	dr := g.BuildReport(today, sessions)

	if len(dr.GitCommits) == 0 {
		t.Fatal("expected GitCommits to be populated, got empty")
	}
	if dr.GitCommits[0].RepoName != filepath.Base(repoDir) {
		t.Errorf("RepoName = %q, want %q", dr.GitCommits[0].RepoName, filepath.Base(repoDir))
	}
	if len(dr.GitCommits[0].Commits) != 2 {
		t.Errorf("got %d commits, want 2", len(dr.GitCommits[0].Commits))
	}
}

func TestGenerator_GitCommits_EmptyWhenNoCommits(t *testing.T) {
	repoDir := initTestRepo(t)
	// No commits

	resolver := func(name string) string {
		if name == "myrepo" {
			return repoDir
		}
		return ""
	}

	sessions := []storage.Session{makeSession("vscode", "myrepo", 9, 0, 60)}
	today := time.Now().Local().Format("2006-01-02")

	g := report.NewGenerator(nil).WithWorkspaceResolver(resolver)
	dr := g.BuildReport(today, sessions)

	if len(dr.GitCommits) != 0 {
		t.Errorf("expected empty GitCommits for repo with no commits, got %d entries", len(dr.GitCommits))
	}
}

func TestGenerator_GitCommits_DeduplicatesByGitRoot(t *testing.T) {
	repoDir := initTestRepo(t)
	commitInRepo(t, repoDir, "initial")

	subDir := filepath.Join(repoDir, "subpkg")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Two sessions both resolve to paths within the same git root
	resolver := func(name string) string {
		switch name {
		case "root":
			return repoDir
		case "subpkg":
			return subDir
		}
		return ""
	}

	sessions := []storage.Session{
		makeSession("vscode", "root", 9, 0, 30),
		makeSession("vscode", "subpkg", 9, 30, 30),
	}
	today := time.Now().Local().Format("2006-01-02")

	g := report.NewGenerator(nil).WithWorkspaceResolver(resolver)
	dr := g.BuildReport(today, sessions)

	if len(dr.GitCommits) != 1 {
		t.Errorf("expected 1 repo entry (deduplicated), got %d", len(dr.GitCommits))
	}
}

func TestGenerator_GitCommits_NilWhenNoResolver(t *testing.T) {
	sessions := []storage.Session{makeSession("vscode", "proj", 9, 0, 60)}
	today := time.Now().Local().Format("2006-01-02")

	g := report.NewGenerator(nil) // no resolver
	dr := g.BuildReport(today, sessions)

	if len(dr.GitCommits) != 0 {
		t.Errorf("expected no GitCommits when resolver is nil, got %d", len(dr.GitCommits))
	}
}

// Ensure GitCommits field type is igit.RepoCommits (compile-time check)
var _ []igit.RepoCommits = (report.DailyReport{}).GitCommits

func makeSession(ct, cl string, startH, startM, durationMins int) storage.Session {
	base := time.Date(2026, 5, 1, startH, startM, 0, 0, time.UTC)
	end := base.Add(time.Duration(durationMins) * time.Minute)
	dur := durationMins * 60
	return storage.Session{
		ID:           1,
		DateLocal:    "2026-05-01",
		ContextType:  ct,
		ContextLabel: cl,
		StartUTC:     base,
		EndUTC:       &end,
		DurationSecs: &dur,
	}
}

func TestGenerator_EmptyDay(t *testing.T) {
	g := report.NewGenerator(nil)
	dr := g.BuildReport("2026-05-01", nil)
	if dr.Date != "2026-05-01" {
		t.Errorf("Date = %q, want 2026-05-01", dr.Date)
	}
	if len(dr.Sessions) != 0 {
		t.Errorf("Sessions len = %d, want 0", len(dr.Sessions))
	}
}

func TestGenerator_SingleSession(t *testing.T) {
	sessions := []storage.Session{makeSession("vscode", "myproject", 9, 0, 60)}
	g := report.NewGenerator(nil)
	dr := g.BuildReport("2026-05-01", sessions)
	if len(dr.Sessions) != 1 {
		t.Fatalf("Sessions len = %d, want 1", len(dr.Sessions))
	}
	if dr.Sessions[0].ContextLabel != "myproject" {
		t.Errorf("label = %q, want myproject", dr.Sessions[0].ContextLabel)
	}
}

func TestGenerator_MultiSession_OrderPreserved(t *testing.T) {
	sessions := []storage.Session{
		makeSession("vscode", "proj", 9, 0, 30),
		makeSession("browser", "browser/research", 9, 30, 10),
		makeSession("vscode", "proj", 9, 40, 20),
	}
	g := report.NewGenerator(nil)
	dr := g.BuildReport("2026-05-01", sessions)
	if len(dr.Sessions) != 3 {
		t.Fatalf("Sessions len = %d, want 3", len(dr.Sessions))
	}
	// Chronological order preserved
	for i := 1; i < len(dr.Sessions); i++ {
		if dr.Sessions[i].StartUTC.Before(dr.Sessions[i-1].StartUTC) {
			t.Errorf("sessions not in chronological order at index %d", i)
		}
	}
}

func TestGenerator_TotalsCorrect(t *testing.T) {
	sessions := []storage.Session{
		makeSession("vscode", "proj", 9, 0, 60),
		makeSession("vscode", "proj", 10, 0, 30),
	}
	g := report.NewGenerator(nil)
	dr := g.BuildReport("2026-05-01", sessions)
	total := dr.Totals["proj"]
	if total != 90*60 {
		t.Errorf("total secs = %d, want %d", total, 90*60)
	}
}
