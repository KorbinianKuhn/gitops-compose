package main

import (
	"fmt"
	"net/http"
	"time"

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

    // TODO: skip when go-git is able to pull changes without losing untracked files
    // Verify git cli
    if err := r.VerifyGitCli(); err != nil {
        slog.Error("failed to verify git cli", "error", err)
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
    loggedIn, err := d.LoginIfCredentialsSet()
    if err != nil {
        slog.Error("failed to verify docker credentials", "error", err)
        panic(err)
    }
    if loggedIn {
        slog.Info("docker credentials verified", "url", config.DockerRegistryUrl)
    }

    // Initialise metrics
    m := metrics.NewMetrics()

    // Initialise gitops
    g := gitops.NewGitOps(r, d, m)
    
    if err := g.EnsureDeploymentsAreRunning(); err != nil {
        m.TrackCheckStatus("error")
        slog.Error("failed to ensure deployments are running on initial start", "error", err)
    } else {
        m.TrackCheckStatus("success")
    }

    slog.Info(fmt.Sprintf("starting gitops repeated pull (every %s seconds)", fmt.Sprint(config.CheckIntervalInSeconds)))
    go func() {
        for {
            if err := g.CheckAndUpdateDeployments(); err != nil {
                m.TrackCheckStatus("error")
            } else {
                m.TrackCheckStatus("success")
            }
            time.Sleep(time.Duration(config.CheckIntervalInSeconds) * time.Second)
        }
    }()

    http.Handle("/metrics", m.GetMetricsHandler())

    // http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
    //     // TODO: check access token
    //     w.WriteHeader(http.StatusAccepted)
    //     w.Write([]byte("Check triggered"))
    // })
    
    if err := http.ListenAndServe(":2112", nil); err != nil {
        slog.Error("failed to start metrics server", "error", err)
        panic(err)
    }

    slog.Info("starting metrics server", "port", "2112")
}