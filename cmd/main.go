package main

import (
	"net/http"
	"time"

	"github.com/korbiniankuhn/gitops-compose/internal/compose"
	"github.com/korbiniankuhn/gitops-compose/internal/config"
	"github.com/korbiniankuhn/gitops-compose/internal/docker"
	"github.com/korbiniankuhn/gitops-compose/internal/git"
	"github.com/korbiniankuhn/gitops-compose/internal/gitops"
	"github.com/korbiniankuhn/gitops-compose/internal/metrics"

	"log/slog"
)

func main() {
    slog.Info("starting gitops compose")

    // Load config
    config, err := config.Get()
    if err != nil {
        slog.Error("failed to load config", "error", err)
        panic(err)
    }
    slog.Info("config loaded")

    // Verify git repository
    r, err := git.NewDeploymentRepo(config.RepositoryUsername, config.RepositoryPassword, config.RepositoryPath)
    if err != nil {
        slog.Error("failed to open deployment repo", "error", err)
        panic(err)
    }
    slog.Info("deployment repo configured", "path", config.RepositoryPath)

    // Verify docker socket connection
    d := docker.NewDocker(config.DockerRegistryUrl, config.DockerRegistryUsername, config.DockerRegistryPassword)
    if err := d.VerifySocketConnection(); err != nil {
        slog.Error("failed to verify docker socket connection", "error", err)
        panic(err)
    }
    slog.Info("docker socket connection verified")

    // Verify docker credentials (if set)
    if err := d.VerifyCredentialsIfSet(); err != nil {
        slog.Error("failed to verify docker credentials", "error", err)
        panic(err)
    }
    if d.AreCredentialsSet() {
        slog.Info("docker credentials verified", "url", config.DockerRegistryUrl)
    }

    // Verify docker compose cli
    if err:= compose.VerifyComposeCli(); err != nil {
        slog.Error("failed to verify docker compose cli", "error", err)
        panic(err)
    }
    slog.Info("docker compose cli verified")

    // Initialise metrics
    m := metrics.NewMetrics()
    m.TrackActiveDeployments(0,0,0)

    slog.Info("starting gitops repeated pull")
    go func() {
        for {
            gitops.Check(r, d, m)
            time.Sleep(time.Duration(config.CheckIntervalInSeconds) * time.Second)
        }
    }()

    slog.Info("starting metrics server", "port", "2112")
    handler := m.GetMetricsHandler()
    http.Handle("/metrics", handler)
    http.ListenAndServe(":2112", nil)
}