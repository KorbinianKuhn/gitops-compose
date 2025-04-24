package deployment

import (
	"fmt"

	"github.com/korbiniankuhn/gitops-compose/internal/compose"
)

var (
	ErrInvalidComposeFile = fmt.Errorf("invalid compose file")
)

type Deployment struct {
	Filepath string
    compose compose.ComposeFile
    hash string
    isValid bool
}

func NewDeployment(filepath string) *Deployment {
    c := compose.NewComposeFile(filepath)
    hash, err := c.GetConfigHash()

    return &Deployment{
		Filepath: c.Filepath,
        compose: *c,
        hash: hash,
        isValid: err == nil,
    }
}

func (d *Deployment) HasChanged() (bool, error) {
	if !d.isValid {
		return false, ErrInvalidComposeFile
	}
	hash, err := d.compose.GetConfigHash()
	if err != nil {
		return false, err
	}
	return d.hash != hash, nil
}

func (d *Deployment) IsRunning() (bool, error) {
	if !d.isValid {
		return false, ErrInvalidComposeFile
	}
	return d.compose.IsRunning()
}

func (d *Deployment) Stop() error {
    if !d.isValid {
        return ErrInvalidComposeFile
    }
    isRunning, err := d.compose.IsRunning()
    if err != nil {
        return err
    }
    if isRunning {
        if err := d.compose.Stop(); err != nil {
            return err
        }
    }
	return nil
}

func (d *Deployment) PullImages() error {
	if !d.isValid {
		return ErrInvalidComposeFile
	}
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

func (d *Deployment) StopAndStart() error {
	if !d.isValid {
		return ErrInvalidComposeFile
	}
	
	isRunning, err := d.compose.IsRunning()
	if err != nil {
		return err
	}

	if isRunning {
		if err := d.compose.Stop(); err != nil {
			return err
		}
	}

	if err := d.compose.Start(); err != nil {
		return err
	}

	return nil
}