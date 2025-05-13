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
	repo    *git.DeploymentRepo
	docker  *docker.Docker
	metrics *metrics.Metrics
}

func NewGitOps(repo *git.DeploymentRepo, docker *docker.Docker, metrics *metrics.Metrics) *GitOps {
	return &GitOps{
		repo:    repo,
		docker:  docker,
		metrics: metrics,
	}
}

func applyDeploymentChange(d *deployment.Deployment, state *metrics.DeploymentState) {
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
			slog.Error("error checking unchanged deployment", "file", d.Filepath, "err", err.Error())
		} else {
			slog.Error("error applying deployment change", "file", d.Filepath, "operation", operation, "err", err.Error())
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

func (g *GitOps) EnsureDeploymentsAreRunning() error {
	// Check if there are any changes in the git repository
	hasChanges, err := g.repo.HasChanges()
	if err != nil {
		slog.Error("error checking for changes", "err", err.Error())
		return err
	}

	// If there are changes, skip and let repeated check handle it
	if hasChanges {
		return err
	}

	// Get local compose files
	composeFiles, err := g.repo.GetLocalComposeFiles()
	if err != nil {
		slog.Error("error getting local compose files", "err", err.Error())
		return err
	}

	slog.Info("ensuring deployments are running")

	// Track deployment states
	state := metrics.NewState()
	defer func() {
		g.metrics.TrackDeploymentState(state)
	}()

	// Ensure all deployments are running
	for _, composeFile := range composeFiles {
		d := deployment.NewDeployment(g.docker, composeFile)
		d.LoadConfig()

		if d.IsController() {
			slog.Info("skipping controller deployment", "file", d.Filepath)
			continue
		}

		if d.IsIgnored() {
			state.Ignored++
			slog.Info("skipping deployment due to gitops ignore label", "file", d.Filepath)
			continue
		}

		applyDeploymentChange(d, state)
	}

	return nil
}

func (g *GitOps) CheckAndUpdateDeployments() error {
	// Check if there are any changes in the git repository
	hasChanges, err := g.repo.HasChanges()
	if err != nil {
		slog.Error("error checking for git changes", "err", err.Error())
		return err
	}

	// If there are no changes, return
	if !hasChanges {
		slog.Info("no git changes detected")
		return nil
	}

	slog.Info("changes detected")

	// Get local and remote compose files
	localComposeFiles, err := g.repo.GetLocalComposeFiles()
	if err != nil {
		slog.Error("error getting local compose files", "err", err.Error())
		return err
	}

	remoteComposeFiles, err := g.repo.GetRemoteComposeFiles()
	if err != nil {
		slog.Error("error getting remote compose files", "err", err.Error())
		return err
	}

	// Determine which deployments to add, remove, or update
	deployments := []*deployment.Deployment{}
	for _, localFile := range localComposeFiles {
		d := deployment.NewDeployment(g.docker, localFile)
		d.LoadConfig()
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

	// Track deployment states
	state := metrics.NewState()
	defer func() {
		g.metrics.TrackDeploymentState(state)
	}()

	// Ensure docker login if credentials are set
	_, err = g.docker.LoginIfCredentialsSet()
	if err != nil {
		slog.Error("error logging in to docker registry", "err", err.Error())
		return err
	}

	// Stop removed deployments
	for _, d := range deployments {
		if d.IsIgnored() || d.IsController() {
			continue
		}
		if d.State == deployment.Removed {
			applyDeploymentChange(d, state)
		}
	}

	// Pull Git changes
	if err := g.repo.Pull(); err != nil {
		slog.Error("error pulling changes", "err", err.Error())
		return err
	}

	// Update deployment states (check if compose files are valid and if they changed)
	for _, d := range deployments {
		if d.State != deployment.Removed {
			d.LoadConfig()
		}
	}

	// Update deployments (add, changed, unchanged)
	for _, d := range deployments {
		if d.IsIgnored() || d.IsController() || d.State == deployment.Removed {
			continue
		}
		applyDeploymentChange(d, state)
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
					// slog.Warn("scheduling controller deployment restart", "file", d.Filepath)
					// _, err := d.Apply()
					// if err != nil {
					//     slog.Error("error updating controller deployment", "file", d.Filepath, "err", err.Error())
					//     state.Failed++
					// }
					// slog.Info("controller deployment scheduled, main process will exit soon")
				}
			}
		}
	}

	return nil
}
