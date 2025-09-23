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

func (g *GitOps) applyDeploymentChange(d *deployment.Deployment) {
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
		g.metrics.State.Invalid++
		slog.Error("invalid compose file", "file", d.Filepath)
		return
	} else if err != nil {
		g.metrics.State.Failed++
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
			g.metrics.State.Started++
			slog.Info("started new deployment", "file", d.Filepath)
		} else {
			// Should never happen
			g.metrics.State.Unchanged++
			slog.Warn("new deployment was already running", "file", d.Filepath)
		}
	case deployment.Updated:
		if wasChanged {
			g.metrics.State.Updated++
			slog.Info("updated deployment", "file", d.Filepath)
		} else {
			// Should never happen
			g.metrics.State.Unchanged++
			slog.Warn("updated deployment was already running", "file", d.Filepath)
		}
	case deployment.Removed:
		if wasChanged {
			g.metrics.State.Stopped++
			slog.Info("stopped removed deployment", "file", d.Filepath)
		} else {
			g.metrics.State.Unchanged++
			slog.Warn("removed deployment was not running", "file", d.Filepath)
		}
	case deployment.Unchanged:
		if wasChanged {
			g.metrics.State.Started++
			slog.Warn("started unchanged but not running deployment", "file", d.Filepath)
		} else {
			g.metrics.State.Unchanged++
		}
	}
}

func (g *GitOps) checkAndUpdateDeployments() ([]*deployment.Deployment, error) {
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

	// Track deployment states
	g.metrics.State.Reset()

	// Stop removed deployments
	for _, d := range deployments {
		if d.IsIgnored() || d.IsController() {
			continue
		}
		if d.State == deployment.Removed {
			g.applyDeploymentChange(d)
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
		g.applyDeploymentChange(d)
	}

	// Post deployment operations
	for _, d := range deployments {
		if d.IsIgnored() {
			if d.State != deployment.Removed {
				g.metrics.State.Ignored++
				slog.Info("skipping deployment due to gitops ignore label", "file", d.Filepath)
			}
			continue
		}
		if d.IsController() {
			switch d.State {
			case deployment.Removed:
				{
					slog.Error("cannot remove controller deployment", "file", d.Filepath)
					g.metrics.State.Failed++
				}
			case deployment.Added:
				{
					slog.Error("cannot add controller deployment", "file", d.Filepath)
					g.metrics.State.Failed++
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

		// Update metrics
		g.metrics.UpdateMetrics()
	}()

	if hasChanges || g.isFirstCheck {
		deployments, err := g.checkAndUpdateDeployments()
		if err != nil {
			slog.Error("error checking and updating deployments", "err", err)
			g.metrics.TrackCheckStatus("error")
			return
		}

		for _, d := range deployments {
			if d.Error == deployment.ErrImagePullBackoff {
				newRetryDeployments = append(newRetryDeployments, d)
			}
		}
	} else {
		for _, d := range g.retryDeployments {
			g.metrics.State.Failed--
			g.applyDeploymentChange(d)
			if d.Error == deployment.ErrImagePullBackoff {
				newRetryDeployments = append(newRetryDeployments, d)
			}
		}
	}
}
