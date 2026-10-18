package deployment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/korbiniankuhn/gitops-compose/internal/docker"
)

func newTestDeployment(t *testing.T, composeYaml string) *Deployment {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(file, []byte(composeYaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewDeployment(docker.NewDocker(nil), file)
}

func TestLoadConfigAcceptsImageServices(t *testing.T) {
	d := newTestDeployment(t, `
services:
  web:
    image: nginx:1.27
`)
	if err := d.LoadConfig(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigRejectsBuildServices(t *testing.T) {
	d := newTestDeployment(t, `
services:
  web:
    image: nginx:1.27
  worker:
    build: .
  api:
    image: my-api:local
    build:
      context: .
`)
	err := d.LoadConfig()
	if !errors.Is(err, ErrBuildNotSupported) {
		t.Fatalf("expected ErrBuildNotSupported, got %v", err)
	}
	if want := "(services: api, worker)"; err.Error() != ErrBuildNotSupported.Error()+" "+want {
		t.Errorf("unexpected error message: %v", err)
	}

	// Rejected deployments are reported as invalid instead of being applied
	if _, err := d.Apply(); !errors.Is(err, ErrInvalidComposeFile) {
		t.Fatalf("expected ErrInvalidComposeFile from Apply, got %v", err)
	}
}

func TestLoadConfigAllowsBuildInIgnoredStacks(t *testing.T) {
	d := newTestDeployment(t, `
services:
  worker:
    build: .
    labels:
      gitops.ignore: "true"
`)
	if err := d.LoadConfig(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.IsIgnored() {
		t.Fatal("expected deployment to be ignored")
	}
}
