package gitops

import (
	"log/slog"
	"slices"

	"github.com/korbiniankuhn/gitops-compose/internal/deployment"
	"github.com/korbiniankuhn/gitops-compose/internal/docker"
	"github.com/korbiniankuhn/gitops-compose/internal/git"
	"github.com/korbiniankuhn/gitops-compose/internal/metrics"
)

type GitOps struct {
	repo             *git.DeploymentRepo
	docker           *docker.Docker
	metrics          *metrics.Metrics
	retryDeployments []*deployment.Deployment
	isFirstCheck     bool
}

func NewGitOps(repo *git.DeploymentRepo, docker *docker.Docker, metrics *metrics.Metrics) *GitOps {
	return &GitOps{
		repo:             repo,
		docker:           docker,
		metrics:          metrics,
		retryDeployments: []*deployment.Deployment{},
		isFirstCheck:     true,
	}
}

func (g *GitOps) applyDeploymentChange(d *deployment.Deployment, state *metrics.DeploymentState) {
	wasChanged, err := d.Apply()

	var operation string
	switch d.State {
	case deployment.Added:
		operation = "start"
	case deployment.Updated:
		operation = "update"
	case deployment.Removed:
		operation = "remove"
	case deployment.Unchanged:
		operation = "unchanged"
	default:
		operation = "unknown"
	}

	if err == deployment.ErrInvalidComposeFile {
		state.Invalid++
		slog.Error("invalid compose file", "file", d.Filepath)
		return
	} else if err != nil {
		state.Failed++
		if d.State == deployment.Unchanged {
			slog.Error("error checking unchanged deployment", "file", d.Filepath, "err", err)
		} else {
			slog.Error("error applying deployment change", "file", d.Filepath, "operation", operation, "err", err)
		}
		return
	}

	switch d.State {
	case deployment.Added:
		if wasChanged {
			state.Started++
			slog.Info("started new deployment", "file", d.Filepath)
		} else {
			// Should never happen
			state.Unchanged++
			slog.Warn("new deployment was already running", "file", d.Filepath)
		}
	case deployment.Updated:
		if wasChanged {
			state.Updated++
			slog.Info("updated deployment", "file", d.Filepath)
		} else {
			// Should never happen
			state.Unchanged++
			slog.Warn("updated deployment was already running", "file", d.Filepath)
		}
	case deployment.Removed:
		if wasChanged {
			state.Stopped++
			slog.Info("stopped removed deployment", "file", d.Filepath)
		} else {
			state.Unchanged++
			slog.Warn("removed deployment was not running", "file", d.Filepath)
		}
	case deployment.Unchanged:
		if wasChanged {
			state.Started++
			slog.Warn("started unchanged but not running deployment", "file", d.Filepath)
		} else {
			state.Unchanged++
		}
	}
}

func (g *GitOps) checkAndUpdateDeployments(state *metrics.DeploymentState) ([]*deployment.Deployment, error) {
	// Get local and remote compose files
	localComposeFiles, err := g.repo.GetLocalComposeFiles()
	if err != nil {
		slog.Error("error getting local compose files", "err", err)
		return []*deployment.Deployment{}, err
	}

	remoteComposeFiles, err := g.repo.GetRemoteComposeFiles()
	if err != nil {
		slog.Error("error getting remote compose files", "err", err)
		return []*deployment.Deployment{}, err
	}

	// Determine which deployments to add, remove, or update
	deployments := []*deployment.Deployment{}
	for _, localFile := range localComposeFiles {
		d := deployment.NewDeployment(g.docker, localFile)

		err := d.LoadConfig()
		if err != nil {
			slog.Error("error loading deployment config", "file", d.Filepath, "err", err)
		}

		if !slices.Contains(remoteComposeFiles, localFile) {
			d.State = deployment.Removed
		}
		deployments = append(deployments, d)
	}
	for _, remoteFile := range remoteComposeFiles {
		if !slices.Contains(localComposeFiles, remoteFile) {
			d := deployment.NewDeployment(g.docker, remoteFile)
			d.State = deployment.Added
			deployments = append(deployments, d)
		}
	}

	// Ensure docker login if credentials are set
	_, err = g.docker.LoginIfCredentialsSet()
	if err != nil {
		slog.Error("error logging in to docker registry", "err", err)
		return []*deployment.Deployment{}, err
	}

	// Stop removed deployments
	for _, d := range deployments {
		if d.IsIgnored() || d.IsController() {
			continue
		}
		if d.State == deployment.Removed {
			g.applyDeploymentChange(d, state)
		}
	}

	// Pull Git changes
	if err := g.repo.Pull(); err != nil {
		slog.Error("error pulling changes", "err", err)
		return deployments, err
	}

	// Update deployment states (check if compose files are valid and if they changed)
	for _, d := range deployments {
		if d.State != deployment.Removed {
			err := d.LoadConfig()
			if err != nil {
				slog.Error("error loading deployment config", "file", d.Filepath, "err", err)
			}
		}
	}

	// Update deployments (add, changed, unchanged)
	for _, d := range deployments {
		if d.IsIgnored() || d.IsController() || d.State == deployment.Removed {
			continue
		}
		g.applyDeploymentChange(d, state)
	}

	// Post deployment operations
	for _, d := range deployments {
		if d.IsIgnored() {
			if d.State != deployment.Removed {
				state.Ignored++
				slog.Info("skipping deployment due to gitops ignore label", "file", d.Filepath)
			}
			continue
		}
		if d.IsController() {
			switch d.State {
			case deployment.Removed:
				{
					slog.Error("cannot remove controller deployment", "file", d.Filepath)
					state.Failed++
				}
			case deployment.Added:
				{
					slog.Error("cannot add controller deployment", "file", d.Filepath)
					state.Failed++
				}
			case deployment.Updated:
				{
					slog.Error("update controller deployment is not implemented yet", "file", d.Filepath)
					// TODO: skip for docker desktop or non-docker use
				}
			}
		}
	}

	return deployments, nil
}

func (g *GitOps) CheckAndUpdate() {
	if g.isFirstCheck {
		defer func() {
			g.isFirstCheck = false
		}()
	}

	hasChanges, err := g.repo.HasChanges()

	if err != nil {
		g.metrics.TrackCheckStatus("error")
		slog.Error("error checking for git changes", "err", err)
		return
	} else {
		g.metrics.TrackCheckStatus("success")
		if hasChanges {
			slog.Info("git changes detected")
		} else if g.isFirstCheck {
			slog.Info("first run, ensure all deployments are running")
		} else {
			slog.Info("no git changes detected")
		}
	}

	newRetryDeployments := []*deployment.Deployment{}
	defer func() {
		// Save deployments that need to be retried
		g.retryDeployments = newRetryDeployments
		for _, d := range g.retryDeployments {
			slog.Info("scheduling deployment for retry due to image pull backoff", "file", d.Filepath)
		}
	}()

	// Track deployment operations
	state := metrics.NewState()

	if hasChanges || g.isFirstCheck {
		deployments, err := g.checkAndUpdateDeployments(state)
		if err != nil {
			slog.Error("error checking and updating deployments", "err", err)
			g.metrics.TrackCheckStatus("error")
			return
		}
		g.metrics.TrackState(state, true)

		for _, d := range deployments {
			if d.Error == deployment.ErrImagePullBackoff {
				newRetryDeployments = append(newRetryDeployments, d)
			}
		}
	} else {
		for _, d := range g.retryDeployments {
			g.applyDeploymentChange(d, state)
			if d.Error == deployment.ErrImagePullBackoff {
				newRetryDeployments = append(newRetryDeployments, d)
			}
		}
		g.metrics.TrackState(state, false)
	}
}
