package gitops

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/korbiniankuhn/gitops-compose/internal/docker"
	"github.com/korbiniankuhn/gitops-compose/internal/git"
	"github.com/korbiniankuhn/gitops-compose/internal/metrics"
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
}

// Returns a deploy repo cloned from a bare origin, plus an upstream clone to push changes from
func setupRepos(t *testing.T) (deploy, upstream string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	upstream = filepath.Join(root, "upstream")
	deploy = filepath.Join(root, "deploy")

	runGit(t, root, "init", "--bare", origin)
	runGit(t, root, "clone", origin, upstream)
	commitFile(t, upstream, "README.md", "init\n")
	runGit(t, upstream, "push", "origin", "main")
	runGit(t, root, "clone", origin, deploy)
	return deploy, upstream
}

func scrapeCheckTotals(t *testing.T, m *metrics.Metrics) map[string]string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.GetMetricsHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))

	totals := map[string]string{}
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		for _, status := range []string{"success", "error"} {
			prefix := `gitops_check_total{status="` + status + `"} `
			if strings.HasPrefix(line, prefix) {
				totals[status] = strings.TrimPrefix(line, prefix)
			}
		}
	}
	return totals
}

func newTestGitOps(t *testing.T, deploy string, registries ...docker.DockerRegistryCredentials) (*GitOps, *metrics.Metrics) {
	t.Helper()
	r, err := git.NewDeploymentRepo(deploy)
	if err != nil {
		t.Fatal(err)
	}
	m := metrics.NewMetrics()
	return NewGitOps(r, docker.NewDocker(registries), m), m
}

func TestCheckAndUpdateTracksSuccessOnce(t *testing.T) {
	deploy, _ := setupRepos(t)
	g, m := newTestGitOps(t, deploy)

	g.CheckAndUpdate()

	totals := scrapeCheckTotals(t, m)
	if totals["success"] != "1" || totals["error"] != "0" {
		t.Fatalf("expected success=1 error=0, got %v", totals)
	}
}

func TestCheckAndUpdateTracksFailedUpdateOnlyAsError(t *testing.T) {
	deploy, upstream := setupRepos(t)

	// Diverge: local commit on the deploy host and a new remote commit, so git pull --ff-only fails
	commitFile(t, deploy, "local.txt", "local\n")
	commitFile(t, upstream, "remote.txt", "remote\n")
	runGit(t, upstream, "push", "origin", "main")

	g, m := newTestGitOps(t, deploy)

	g.CheckAndUpdate()

	totals := scrapeCheckTotals(t, m)
	if totals["success"] != "0" || totals["error"] != "1" {
		t.Fatalf("expected success=0 error=1, got %v", totals)
	}
}

func TestCheckAndUpdateIgnoresUnreachableRegistry(t *testing.T) {
	deploy, upstream := setupRepos(t)

	// A new remote commit, so the check has to pull
	commitFile(t, upstream, "remote.txt", "remote\n")
	runGit(t, upstream, "push", "origin", "main")

	// Credentials for a registry that can't be reached must not block the check
	g, m := newTestGitOps(t, deploy, docker.DockerRegistryCredentials{
		Url:      "registry.invalid",
		Username: "user",
		Password: "secret",
	})

	g.CheckAndUpdate()

	totals := scrapeCheckTotals(t, m)
	if totals["success"] != "1" || totals["error"] != "0" {
		t.Fatalf("expected success=1 error=0, got %v", totals)
	}
	if _, err := os.Stat(filepath.Join(deploy, "remote.txt")); err != nil {
		t.Fatalf("expected remote commit to be pulled: %v", err)
	}
}
