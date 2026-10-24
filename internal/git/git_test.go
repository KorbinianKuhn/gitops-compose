package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	args = append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "init.defaultBranch=main"}, args...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func commitFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-m", "update "+name)
	runGit(t, dir, "push", "origin", "main")
}

func TestNewDeploymentRepoWithoutAuthHasNilAuth(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	deploy := filepath.Join(root, "deploy")

	runGit(t, root, "init", "--bare", origin)
	runGit(t, root, "clone", origin, deploy)

	r, err := NewDeploymentRepo(deploy)
	if err != nil {
		t.Fatal(err)
	}

	// A typed nil (e.g. (*http.BasicAuth)(nil)) is rejected by non-http transports with "invalid auth method"
	if r.auth != nil {
		t.Fatalf("expected nil auth, got %#v", r.auth)
	}
}

func TestHasChangesKeepsReportingUnpulledChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	upstream := filepath.Join(root, "upstream")
	deploy := filepath.Join(root, "deploy")

	runGit(t, root, "init", "--bare", origin)
	runGit(t, root, "clone", origin, upstream)
	commitFile(t, upstream, "docker-compose.yml", "services: {}\n")
	runGit(t, root, "clone", origin, deploy)

	r, err := NewDeploymentRepo(deploy)
	if err != nil {
		t.Fatal(err)
	}

	hasChanges, err := r.HasChanges()
	if err != nil {
		t.Fatal(err)
	}
	if hasChanges {
		t.Fatal("expected no changes after clone")
	}

	commitFile(t, upstream, "docker-compose.yml", "services:\n  app:\n    image: nginx\n")

	// First check fetches the new commit
	hasChanges, err = r.HasChanges()
	if err != nil {
		t.Fatal(err)
	}
	if !hasChanges {
		t.Fatal("expected changes after upstream commit")
	}

	// Second check without pull: nothing new to fetch, but local is still behind
	hasChanges, err = r.HasChanges()
	if err != nil {
		t.Fatal(err)
	}
	if !hasChanges {
		t.Fatal("expected changes to be reported until they are pulled")
	}

	if err := r.Pull(); err != nil {
		t.Fatal(err)
	}

	hasChanges, err = r.HasChanges()
	if err != nil {
		t.Fatal(err)
	}
	if hasChanges {
		t.Fatal("expected no changes after pull")
	}
}

func TestPullRefusesToMergeDivergedBranches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	upstream := filepath.Join(root, "upstream")
	deploy := filepath.Join(root, "deploy")

	runGit(t, root, "init", "--bare", origin)
	runGit(t, root, "clone", origin, upstream)
	commitFile(t, upstream, "docker-compose.yml", "services: {}\n")
	runGit(t, root, "clone", origin, deploy)

	// Host configured to merge on pull (so a plain git pull would create a merge commit)
	runGit(t, deploy, "config", "pull.rebase", "false")
	runGit(t, deploy, "config", "user.name", "test")
	runGit(t, deploy, "config", "user.email", "test@example.com")

	// Diverge: local commit on the deploy host and a new commit on the remote
	if err := os.WriteFile(filepath.Join(deploy, "local.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, deploy, "add", "local.txt")
	runGit(t, deploy, "commit", "-m", "local commit")
	commitFile(t, upstream, "docker-compose.yml", "services:\n  app:\n    image: nginx\n")

	headBefore := gitOutput(t, deploy, "rev-parse", "HEAD")

	r, err := NewDeploymentRepo(deploy)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Pull(); err == nil {
		t.Fatal("expected pull of diverged branches to fail")
	}

	if headAfter := gitOutput(t, deploy, "rev-parse", "HEAD"); headAfter != headBefore {
		t.Fatalf("expected HEAD to be unchanged, was %s, now %s", headBefore, headAfter)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func TestPullKeepsUntrackedAndIgnoredFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	upstream := filepath.Join(root, "upstream")
	deploy := filepath.Join(root, "deploy")

	runGit(t, root, "init", "--bare", origin)
	runGit(t, root, "clone", origin, upstream)
	commitFile(t, upstream, ".gitignore", "*.env\ndata/\n")
	commitFile(t, upstream, "docker-compose.yml", "services: {}\n")
	commitFile(t, upstream, "removed.yml", "removed\n")
	runGit(t, root, "clone", origin, deploy)

	// Files on the deploy host that are not part of the repository
	files := map[string]string{
		"app.env":            "SECRET=1\n",
		"data/db/state":      "state\n",
		"untracked/notes.md": "notes\n",
	}
	for name, content := range files {
		p := filepath.Join(deploy, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	commitFile(t, upstream, "docker-compose.yml", "services:\n  app:\n    image: nginx\n")
	runGit(t, upstream, "rm", "removed.yml")
	runGit(t, upstream, "commit", "-m", "remove removed.yml")
	runGit(t, upstream, "push", "origin", "main")

	r, err := NewDeploymentRepo(deploy)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Pull(); err != nil {
		t.Fatal(err)
	}

	if head, want := gitOutput(t, deploy, "rev-parse", "HEAD"), gitOutput(t, upstream, "rev-parse", "HEAD"); head != want {
		t.Fatalf("expected HEAD %s, got %s", want, head)
	}
	content, err := os.ReadFile(filepath.Join(deploy, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "nginx") {
		t.Fatalf("expected updated compose file, got %q", content)
	}
	if _, err := os.Stat(filepath.Join(deploy, "removed.yml")); !os.IsNotExist(err) {
		t.Fatalf("expected removed.yml to be deleted, got %v", err)
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(deploy, name))
		if err != nil {
			t.Fatalf("expected %s to be kept: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("expected %s to be unchanged, got %q", name, got)
		}
	}
	if status := gitOutput(t, deploy, "status", "--porcelain"); status != "?? untracked/" {
		t.Fatalf("expected only untracked/ in status, got %q", status)
	}
}

func TestPullIsNoopWhenUpToDate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	upstream := filepath.Join(root, "upstream")
	deploy := filepath.Join(root, "deploy")

	runGit(t, root, "init", "--bare", origin)
	runGit(t, root, "clone", origin, upstream)
	commitFile(t, upstream, "docker-compose.yml", "services: {}\n")
	runGit(t, root, "clone", origin, deploy)

	r, err := NewDeploymentRepo(deploy)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Pull(); err != nil {
		t.Fatalf("expected pull without changes to succeed, got %v", err)
	}
}
