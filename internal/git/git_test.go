package git

import (
	"os"
	"os/exec"
	"path/filepath"
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
