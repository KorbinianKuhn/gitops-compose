package compose

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
)

var (
	ErrInvalidComposeFile = fmt.Errorf("invalid compose file")
	ErrPullDryRunFailed = fmt.Errorf("pull dry run failed")
	ErrPullFailed = fmt.Errorf("pull failed")
	ErrCheckRunningFailed = fmt.Errorf("error checking if compose stack is running")
	ErrStopFailed = fmt.Errorf("error stopping compose stack")
	ErrStartFailed = fmt.Errorf("error starting compose stack")
)

type ComposeFile struct {
	Filepath string
}

func NewComposeFile(filepath string) *ComposeFile {
	return &ComposeFile{
		Filepath: filepath,
	}
}

func (c ComposeFile) GetConfig() ([]byte, error) {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "config")
	output, err := cmd.Output()
	if err != nil {
		return nil, ErrInvalidComposeFile
	}
	return output, nil
}

func (c ComposeFile) GetConfigHash() (string, error) {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "config")
	output, err := cmd.Output()
	if err != nil {
		return "", ErrInvalidComposeFile
	}
	hash := sha256.Sum256(output)
	return hex.EncodeToString(hash[:]), nil
}

func (c ComposeFile) IsPullRequired() (bool, error) {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "pull", "--dry-run", "--quiet")
	output, err := cmd.Output()
	if err != nil {
		return false, ErrPullDryRunFailed
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
		return ErrPullFailed
	}
	return nil
}

func (c ComposeFile) IsRunning() (bool, error) {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "ps", "--quiet")
	output, err := cmd.Output()
	if err != nil {
		return false, ErrCheckRunningFailed
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
		return ErrStopFailed
	}
	return nil
}

func (c ComposeFile) Start() error {
	cmd := exec.Command("docker", "compose", "-f", c.Filepath, "up", "-d", "--remove-orphans")
	_, err := cmd.Output()
	if err != nil {
		return ErrStartFailed
	}
	return nil
}