package gitops

import (
	"log/slog"

	"slices"

	"github.com/korbiniankuhn/gitops-compose/internal/deployment"
	"github.com/korbiniankuhn/gitops-compose/internal/docker"
	"github.com/korbiniankuhn/gitops-compose/internal/git"
	"github.com/korbiniankuhn/gitops-compose/internal/metrics"
)

func Check(r *git.DeploymentRepo, d *docker.Docker, m *metrics.Metrics) {
    m.TrackLastCheckTime()

    hasChanges, err := r.HasChanges()
    if err != nil {
        m.TrackErrorMetrics()
        slog.Error("error checking for changes", "err", err.Error())
        return
    }

    if !hasChanges {
        m.TrackSuccessMetrics()
        slog.Info("no changes to pull")
        return
    }

    slog.Info("changes detected")

    localComposeFiles, err := r.GetLocalComposeFiles()
    if err != nil {
        m.TrackErrorMetrics()
        slog.Error("error getting local compose files", "err", err.Error())
        return
    }

    remoteComposeFiles, err := r.GetRemoteComposeFiles()
    if err != nil {
        m.TrackErrorMetrics()
        slog.Error("error getting remote compose files", "err", err.Error())
        return
    }

    activeDeployments := []deployment.Deployment{}
    removedDeployments := []deployment.Deployment{}
    for _, localFile := range localComposeFiles {
        d := deployment.NewDeployment(localFile)
        if slices.Contains(remoteComposeFiles, localFile) {
            activeDeployments = append(activeDeployments, *d)
        } else {
            removedDeployments = append(removedDeployments, *d)
        }
    }
    addedDeployments := []deployment.Deployment{}
    for _, remoteFile := range remoteComposeFiles {
        if !slices.Contains(localComposeFiles, remoteFile) {
            d := deployment.NewDeployment(remoteFile)
            addedDeployments = append(addedDeployments, *d)
        }
    }

    // Metrics counter variables
    activeOk := 0
    activeFailed := 0
    removalFailed := 0

    // Eventually login to docker registry
    if err := d.LoginIfCredentialsSet(); err != nil {
        slog.Error("error logging in to docker registry", "err", err.Error())
        m.TrackErrorMetrics()
        return
    }
    slog.Info("logged in to docker registry")

    // Stop removed deployments
    for _, d := range removedDeployments {
        if err := d.Stop(); err != nil {
            removalFailed++
            m.TrackDeploymentErrorMetrics()
            slog.Error("error stopping removed deployment", "err", err.Error())
            continue
        }
        m.TrackDeploymentSuccessMetrics()
        slog.Info("stopped removed deployment", "file", d.Filepath)
    }

    // Pull Git changes
    if err := r.Pull(); err != nil {
        slog.Error("error pulling changes", "err", err.Error())
        m.TrackErrorMetrics()
        return
    }

    // Start added deployments
    for _, d := range addedDeployments {
        if err := d.PullImages(); err != nil {
            activeFailed++
            m.TrackDeploymentErrorMetrics()
            slog.Error("error preparing deployment", "err", err.Error())
            continue
        }
        if err := d.StopAndStart(); err != nil {
            activeFailed++
            m.TrackDeploymentErrorMetrics()
            slog.Error("error starting deployment", "err", err.Error())
            continue
        }
        activeOk++
        m.TrackDeploymentSuccessMetrics()
        slog.Info("started new deployment", "file", d.Filepath)
    }

    // Update changed deployments
    for _, d := range activeDeployments {
        hasChanged, err := d.HasChanged()
        if err != nil {
            activeFailed++
            m.TrackDeploymentErrorMetrics()
            slog.Error("error checking if deployment has changed", "err", err.Error())
            continue
        }

        isRunning, err := d.IsRunning()
        if err != nil {
            activeFailed++
            m.TrackDeploymentErrorMetrics()
            slog.Error("error checking if deployment is running", "err", err.Error())
            continue
        }

        if hasChanged || !isRunning {
            if err := d.PullImages(); err != nil {
                activeFailed++
                m.TrackDeploymentErrorMetrics()
                slog.Error("error preparing deployment", "err", err.Error())
                continue
            }
            if err := d.StopAndStart(); err != nil {
                activeFailed++
                m.TrackDeploymentErrorMetrics()
                if hasChanged {
                    slog.Error("error updating deployment", "err", err.Error())
                } else {
                    slog.Error("error starting deployment", "err", err.Error())
                }
                continue
            }

            if hasChanged {
                m.TrackDeploymentSuccessMetrics()
                slog.Info("updated deployment", "file", d.Filepath)
            } else {
                m.TrackDeploymentSuccessMetrics()
                slog.Info("started deployment", "file", d.Filepath)
            }
        }

        activeOk++
    }

    // Track active deployments
    m.TrackActiveDeployments(activeOk, activeFailed, removalFailed)

    // Logout from docker registry
    if err := d.LogoutIfCredentialsSet(); err != nil {
        slog.Error("error logging out from docker registry", "err", err.Error())
        m.TrackErrorMetrics()
        return
    }
    slog.Info("logged out from docker registry")

    // Track success metrics
    m.TrackSuccessMetrics()
}