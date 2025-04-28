package compose

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/cli"
	"github.com/compose-spec/compose-go/v2/types"
)

type ComposeFile struct {
	Filepath string
}

func NewComposeFile(filepath string) *ComposeFile {
	return &ComposeFile{
		Filepath: filepath,
	}
}

func VerifyComposeCli() (error) {
	cmd := exec.Command("docker", "compose", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose cli is not working: %w %s", err, output)
	}
	if bytes.Contains(output, []byte("Docker Compose")) {
		return nil
	}
	return fmt.Errorf("docker compose cli is not working: %w %s", err, output)
}

func (c ComposeFile) LoadProject() (types.Project, error) {
	ctx := context.Background()

	workingDirectory := path.Dir(c.Filepath)

	options, err := cli.NewProjectOptions(
		[]string{c.Filepath},
		cli.WithWorkingDirectory(workingDirectory),
		cli.WithEnvFiles(filepath.Join(workingDirectory, ".env")),
		cli.WithDotEnv,
		cli.WithInterpolation(true),
		cli.WithResolvedPaths(true),
	)
	if err != nil {
		return types.Project{}, fmt.Errorf("failed to create project options: %w", err)
	}

	project, err := options.LoadProject(ctx)
	if err != nil {
		return types.Project{}, fmt.Errorf("invalid compose file: %w", err)
	}

	return *project, nil
}

func (c ComposeFile) IsPullRequired() (bool, error) {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "pull", "--dry-run", "--quiet")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("docker compose pull dry-run failed: %w %s", err, output)
	}
	if bytes.Contains(output, []byte("Would pull")) {
		return true, nil
	} else {
		return false, nil
	}
}

func (c ComposeFile) PullImages() (error) {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "pull", "--quiet")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose pull failed: %w %s", err, output)
	}
	return nil
}

func (c ComposeFile) IsRunning() (bool, error) {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "ps", "--quiet")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("docker ps failed: %w %s", err, output)
	}
	if len(output) > 0 {
		return true, nil
	} else {
		return false, nil
	}
}

func (c ComposeFile) Stop() error {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "down")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down failed: %w %s", err, output)
	}
	return nil
}

func (c ComposeFile) Start() error {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "up", "-d", "--remove-orphans")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up failed: %w %s", err, output)
	}
	return nil
}

func (c ComposeFile) StartWithDelay() (error) {
	// Compose down + up with sleep
	cmd := exec.Command("sh", "-c", `(sleep 5 && docker compose -f %s down && docker compose -f %s up -d) &`, c.Filepath, c.Filepath)

	// Start the command but don't wait for it
    err := cmd.Start()
    if err != nil {
        return fmt.Errorf("failed to schedule compose restart: %v", err)
    }

	return nil
}