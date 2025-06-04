package deployment

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/korbiniankuhn/gitops-compose/internal/compose"
	"github.com/korbiniankuhn/gitops-compose/internal/docker"
)

var (
	ErrInvalidComposeFile     = fmt.Errorf("invalid compose file")
	ErrUnknownDeploymentState = fmt.Errorf("unknown deployment state")
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
	}
}

func (d *Deployment) LoadConfig() {
	oldConfig := d.config

	d.config = DeploymentConfig{
		hash:             "",
		isValid:          false,
		gitopsIgnore:     false,
		gitopsController: false,
	}

	project, err := d.compose.LoadProject()
	if err != nil {
		return
	}

	projectYaml, err := project.MarshalYAML()
	if err != nil {
		return
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

	hash := sha256.Sum256(projectYaml)
	d.config.hash = hex.EncodeToString(hash[:])
	d.config.isValid = true

	if oldConfig != (DeploymentConfig{}) {
		if oldConfig.hash != d.config.hash {
			d.State = Updated
		}
	}

	return
}

func (d *Deployment) IsIgnored() bool {
	return d.config.gitopsIgnore
}

func (d *Deployment) IsController() bool {
	return d.config.gitopsController
}

func (d *Deployment) Apply() (bool, error) {
	if !d.config.isValid {
		return false, ErrInvalidComposeFile
	}
	if d.config.gitopsIgnore {
		return false, nil
	}
	if d.config.gitopsController {
		switch d.State {
		case Updated:
			// TODO: start temporary container that restarts the controller
			// if err := d.prepareImages(); err != nil {
			// 	return false, err
			// }
			// if err := d.compose.StartWithDelay(); err != nil {
			// 	return false, err
			// }
			return true, nil
		default:
			return false, nil
		}
	}
	switch d.State {
	case Added:
		{
			if err := d.prepareImages(); err != nil {
				return false, err
			}
			if err := d.compose.Start(); err != nil {
				return false, err
			}
			return true, nil
		}
	case Removed:
		{
			wasStopped, err := d.ensureIsStopped()
			if err != nil {
				return false, err
			}
			return wasStopped, nil
		}
	case Updated:
		{
			if err := d.prepareImages(); err != nil {
				return false, err
			}
			_, err := d.ensureIsStopped()
			if err != nil {
				return false, err
			}
			wasStarted, err := d.ensureIsRunning()
			if err != nil {
				return false, err
			}
			return wasStarted, nil
		}
	case Unchanged:
		{
			if err := d.prepareImages(); err != nil {
				return false, err
			}
			wasStarted, err := d.ensureIsRunning()
			if err != nil {
				return false, err
			}
			return wasStarted, nil
		}
	}
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
			return err
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
