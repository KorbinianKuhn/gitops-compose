package deployment

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/korbiniankuhn/gitops-compose/internal/compose"
	"github.com/korbiniankuhn/gitops-compose/internal/docker"
)

var (
	ErrInvalidComposeFile     = fmt.Errorf("invalid compose file")
	ErrUnknownDeploymentState = fmt.Errorf("unknown deployment state")
	ErrImagePullBackoff       = fmt.Errorf("image pull backoff")
)

type DeploymentState int

const (
	Added DeploymentState = iota
	Removed
	Updated
	Unchanged
)

type Deployment struct {
	docker   docker.Docker
	Filepath string
	compose  compose.ComposeFile
	State    DeploymentState
	config   DeploymentConfig
	Error    error
}

type DeploymentConfig struct {
	hash             string
	isValid          bool
	gitopsIgnore     bool
	gitopsController bool
}

func NewDeployment(docker *docker.Docker, filepath string) *Deployment {
	c := compose.NewComposeFile(filepath)

	return &Deployment{
		docker:   *docker,
		Filepath: filepath,
		compose:  *c,
		State:    Unchanged,
		config:   DeploymentConfig{},
		Error:    nil,
	}
}

func (d *Deployment) LoadConfig() error {
	oldConfig := d.config

	d.config = DeploymentConfig{
		hash:             "",
		isValid:          false,
		gitopsIgnore:     false,
		gitopsController: false,
	}

	project, err := d.compose.LoadProject()
	if err != nil {
		return fmt.Errorf("failed to load project from compose file %s: %w", d.Filepath, err)
	}

	projectYaml, err := project.MarshalYAML()
	if err != nil {
		return fmt.Errorf("failed to marshal compose project to YAML: %w", err)
	}

	for _, service := range project.Services {
		for label, value := range service.Labels {
			if label == "gitops.ignore" && value == "true" {
				d.config.gitopsIgnore = true
			}
			if label == "gitops.controller" && value == "true" {
				d.config.gitopsController = true
			}
		}
	}

	hash := sha256.New()
	hash.Write(projectYaml)

	watchFiles := d.compose.GetWatchFiles(project)
	for _, filepath := range watchFiles {
		f, err := os.Open(filepath)
		if err != nil {
			continue
		}
		io.Copy(hash, f)
		f.Close()
	}

	d.config.hash = hex.EncodeToString(hash.Sum(nil)[:])
	d.config.isValid = true

	if oldConfig != (DeploymentConfig{}) {
		if oldConfig.hash != d.config.hash {
			d.State = Updated
		}
	}

	return nil
}

func (d *Deployment) IsIgnored() bool {
	return d.config.gitopsIgnore
}

func (d *Deployment) IsController() bool {
	return d.config.gitopsController
}

func (d *Deployment) Apply() (bool, error) {
	// Reset error state before applying changes
	d.Error = nil

	if !d.config.isValid {
		d.Error = ErrInvalidComposeFile
		return false, ErrInvalidComposeFile
	}
	if d.config.gitopsIgnore {
		return false, nil
	}
	if d.config.gitopsController {
		// TODO: start temporary container that restarts the controller
		return false, nil
	}
	switch d.State {
	case Added:
		{
			if err := d.prepareImages(); err != nil {
				slog.Error("failed to prepare images for updated deployment", "file", d.Filepath, "err", err)
				d.Error = ErrImagePullBackoff
				return false, ErrImagePullBackoff
			}
			if err := d.compose.Start(); err != nil {
				d.Error = err
				return false, err
			}
			return true, nil
		}
	case Removed:
		{
			wasStopped, err := d.ensureIsStopped()
			if err != nil {
				d.Error = err
				return false, err
			}
			return wasStopped, nil
		}
	case Updated:
		{
			if err := d.prepareImages(); err != nil {
				d.Error = err
				return false, err
			}
			_, err := d.ensureIsStopped()
			if err != nil {
				d.Error = err
				return false, err
			}
			wasStarted, err := d.ensureIsRunning()
			if err != nil {
				d.Error = err
				return false, err
			}
			return wasStarted, nil
		}
	case Unchanged:
		{
			if err := d.prepareImages(); err != nil {
				d.Error = err
				return false, err
			}
			wasStarted, err := d.ensureIsRunning()
			if err != nil {
				d.Error = err
				return false, err
			}
			return wasStarted, nil
		}
	}
	d.Error = ErrUnknownDeploymentState
	return false, ErrUnknownDeploymentState
}

func (d *Deployment) prepareImages() error {
	images, err := d.compose.ListImages()
	if err != nil {
		return err
	}

	for _, image := range images {
		err := d.docker.Pull(image)
		if err != nil {
			slog.Error("failed to pull image", "image", image, "err", err)
			return ErrImagePullBackoff
		}
	}

	return nil
}

func (d *Deployment) ensureIsStopped() (bool, error) {
	isRunning, err := d.compose.IsRunning()
	if err != nil {
		return false, err
	}
	if isRunning {
		if err := d.compose.Stop(); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (d *Deployment) ensureIsRunning() (bool, error) {
	isRunning, err := d.compose.IsRunning()
	if err != nil {
		return false, err
	}
	if isRunning {
		return false, nil
	}
	if err := d.compose.Start(); err != nil {
		return false, err
	}
	return true, nil
}
