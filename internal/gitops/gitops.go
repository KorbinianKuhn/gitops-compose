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

    // Track deployment states
    ok := 0
    removed := 0
    failed := 0
    invalid := 0
    defer func() {
        g.metrics.TrackActiveDeployments(ok, removed, failed, invalid)
    }()

    // Ensure docker login if credentials are set
    if err := g.docker.LoginIfCredentialsSet(); err != nil {
        slog.Error("error logging in to docker registry", "err", err.Error())
        return err
    }
    defer func() {
        g.docker.LogoutIfCredentialsSet()
    }()

    // Ensure all deployments are running
    for _, composeFile := range composeFiles {
        d := deployment.NewDeployment(composeFile, deployment.Unchanged)

        wasStarted, err := d.Apply()
        if err == deployment.ErrInvalidComposeFile {
            invalid++
            g.metrics.TrackDeploymentOperation("config", "error")
            slog.Error("invalid compose file", "file", d.Filepath)
        } else if err != nil {
            failed++
            g.metrics.TrackDeploymentOperation("start", "error")
            slog.Error("error starting deployment", "file", d.Filepath, "err", err.Error())
        } else if wasStarted {
            ok++
            g.metrics.TrackDeploymentOperation("start", "success")
            slog.Info("started deployment", "file", d.Filepath)
        } else {
            ok++
            slog.Info("deployment is already running", "file", d.Filepath)
        }
    }

    return nil
}

func (g *GitOps) CheckAndUpdateDeployments() error {
    // Check if there are any changes in the git repository
    hasChanges, err := g.repo.HasChanges()
    if err != nil {
        slog.Error("error checking for changes", "err", err.Error())
        return err
    }

    // If there are no changes, return
    if !hasChanges {
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
    deployments := []deployment.Deployment{}
    for _, localFile := range localComposeFiles {
        if slices.Contains(remoteComposeFiles, localFile) {
            deployments = append(deployments, *deployment.NewDeployment(localFile, deployment.Unchanged))
        } else {
            deployments = append(deployments, *deployment.NewDeployment(localFile, deployment.Removed))
        }
    }
    for _, remoteFile := range remoteComposeFiles {
        if !slices.Contains(localComposeFiles, remoteFile) {
            deployments = append(deployments, *deployment.NewDeployment(remoteFile, deployment.Added))
        }
    }

    // Track deployment states
    ok := 0
    failed := 0
    invalid := 0
    removed := 0
    defer func() {
        g.metrics.TrackActiveDeployments(ok, removed, failed, invalid)
    }()

    // Ensure docker login if credentials are set
    if err := g.docker.LoginIfCredentialsSet(); err != nil {
        slog.Error("error logging in to docker registry", "err", err.Error())
        return err
    }
    defer func() {
        g.docker.LogoutIfCredentialsSet()
    }()

    // Stop removed deployments
    for _, d := range deployments {
        if d.State == deployment.Removed {
            wasStopped, err := d.Apply()
            if err == deployment.ErrInvalidComposeFile {
                invalid++
                g.metrics.TrackDeploymentOperation("config", "error")
                slog.Error("cannot stop removed deployment due to invalid compose file", "file", d.Filepath)
            } else if err != nil {
                failed++
                g.metrics.TrackDeploymentOperation("stop", "error")
                slog.Error("error stopping removed deployment", "file", d.Filepath, "err", err.Error())
            } else {
                removed++
                if wasStopped {
                    g.metrics.TrackDeploymentOperation("stop", "success")
                }
                slog.Info("stopped removed deployment", "file", d.Filepath)
            }
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
            d.UpdateState()
        }
    }

    // Update deployments
    for _, d := range deployments {
        switch d.State {
            case deployment.Added: {
                wasStarted, err := d.Apply()
                if err == deployment.ErrInvalidComposeFile {
                    invalid++
                    g.metrics.TrackDeploymentOperation("config", "error")
                    slog.Error("cannot start new deployment due to invalid compose file", "file", d.Filepath)
                } else if err != nil {
                    failed++
                    g.metrics.TrackDeploymentOperation("start", "error")
                    slog.Error("error starting new deployment", "file", d.Filepath, "err", err.Error())
                } else {
                    ok++
                    if (wasStarted) {
                        g.metrics.TrackDeploymentOperation("start", "success")
                    }
                    slog.Info("started new deployment", "file", d.Filepath)
                }
            }
            case deployment.Updated: {
                wasStarted, err := d.Apply()
                if err == deployment.ErrInvalidComposeFile {
                    invalid++
                    g.metrics.TrackDeploymentOperation("config", "error")
                    slog.Error("cannot update deployment due to invalid compose file", "file", d.Filepath)
                } else if err != nil {
                    failed++
                    g.metrics.TrackDeploymentOperation("start", "error")
                    slog.Error("error updating deployment", "file", d.Filepath, "err", err.Error())
                } else {
                    ok++
                    if (wasStarted) {
                        g.metrics.TrackDeploymentOperation("start", "success")
                    }
                    slog.Info("updated deployment", "file", d.Filepath)
                }
            }
            case deployment.Unchanged: {
                wasStarted, err := d.Apply()
                if err == deployment.ErrInvalidComposeFile {
                    invalid++
                    g.metrics.TrackDeploymentOperation("config", "error")
                    slog.Error("cannot check deployment due to invalid compose file", "file", d.Filepath)
                } else if err != nil {
                    failed++
                    g.metrics.TrackDeploymentOperation("start", "error")
                    slog.Error("error checking unchanged deployment", "file", d.Filepath, "err", err.Error())
                } else {
                    if (wasStarted) {
                        g.metrics.TrackDeploymentOperation("start", "success")
                        slog.Info("started unchanged but not running deployment", "file", d.Filepath)
                    }
                    ok++
                }
            }
        }
    }

    return nil
}