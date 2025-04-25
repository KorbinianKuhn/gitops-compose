package compose

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
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
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("docker compose cli is not working: %w", err)
	}
	if bytes.Contains(output, []byte("Docker Compose")) {
		return nil
	}
	return fmt.Errorf("docker compose cli is not working: %w", err)
}

func (c ComposeFile) GetConfig() ([]byte, error) {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "config")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("invalid compose file: %w", err)
	}
	return output, nil
}

func (c ComposeFile) GetConfigHash() (string, error) {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "config")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("invalid compose file: %w", err)
	}
	hash := sha256.Sum256(output)
	return hex.EncodeToString(hash[:]), nil
}

func (c ComposeFile) IsPullRequired() (bool, error) {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "pull", "--dry-run", "--quiet")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("docker compose pull dry-run failed: %w", err)
	}
	if bytes.Contains(output, []byte("Would pull")) {
		return true, nil
	} else {
		return false, nil
	}
}

func (c ComposeFile) PullImages() (error) {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "pull", "--quiet")
	_, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("docker compose pull failed: %w", err)
	}
	return nil
}

func (c ComposeFile) IsRunning() (bool, error) {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "ps", "--quiet")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("docker ps failed: %w", err)
	}
	if len(output) > 0 {
		return true, nil
	} else {
		return false, nil
	}
}

func (c ComposeFile) Stop() error {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "down")
	_, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("docker compose down failed: %w", err)
	}
	return nil
}

func (c ComposeFile) Start() error {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "up", "-d", "--remove-orphans")
	_, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}
	return nil
}