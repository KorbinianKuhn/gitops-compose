package deployment

import (
	"fmt"

	"github.com/korbiniankuhn/gitops-compose/internal/compose"
)

var (
	ErrInvalidComposeFile = fmt.Errorf("invalid compose file")
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
	Filepath string
	compose compose.ComposeFile
	State DeploymentState
	hash string
	isValid bool
}

func NewDeployment(filepath string, state DeploymentState) *Deployment {
	c := compose.NewComposeFile(filepath)
	hash, err := c.GetConfigHash()

	return &Deployment{
		Filepath: filepath,
		compose: *c,
		State: state,
		hash: hash,
		isValid: err == nil,
	}
}

func (d *Deployment) UpdateState() {
	oldHash := d.hash

	newHash, err := d.compose.GetConfigHash()
	d.hash = newHash
	if err != nil {
		d.isValid = false
		return
	}

	d.isValid = true

	if d.State == Added || d.State == Removed {
		return
	}

	if oldHash != newHash {
		d.State = Updated
	} else {
		d.State = Unchanged
	}
}

func (d *Deployment) Apply() (bool, error) {
	if !d.isValid {
		return false, ErrInvalidComposeFile
	}
	switch d.State {
		case Added: {
			if err := d.prepareImages(); err != nil {
				return false, err
			}
			if err := d.compose.Start(); err != nil {
				return false, err
			}
			return true, nil
		}
		case Removed: {
			wasStopped, err := d.ensureIsStopped()
			if err != nil {
				return false, err
			}
			return wasStopped, nil
		}
		case Updated: {
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
		case Unchanged: {
			wasStarted, err := d.ensureIsRunning()
			if err != nil {
				return false, err
			}
			return wasStarted, nil
		}
	}
	return false, ErrUnknownDeploymentState
}

func (d *Deployment) prepareImages() (error) {
	isPullRequired, err := d.compose.IsPullRequired()
	if err != nil {
		return err
	}
	if isPullRequired {
		if err := d.compose.PullImages(); err != nil {
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