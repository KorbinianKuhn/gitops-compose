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
    if !config.DisableMetrics {
        http.Handle("/metrics", m.GetMetricsHandler())
        slog.Info("metrics enabled", "url", "/metrics")
    } else {
        slog.Info("skipping metrics (disabled in config)")
    }

    // Initialise gitops
    g := gitops.NewGitOps(r, d, m)
    
    if err := g.EnsureDeploymentsAreRunning(); err != nil {
        m.TrackCheckStatus("error")
        slog.Error("failed to ensure deployments are running on initial start", "error", err)
    } else {
        m.TrackCheckStatus("success")
    }

    trigger := make(chan struct{})

    // Run gitops check on trigger
    go func() {
        for range trigger {
            if err := g.CheckAndUpdateDeployments(); err != nil {
                m.TrackCheckStatus("error")
            } else {
                m.TrackCheckStatus("success")
            }
        }
    }()

    // Run gitops check on interval
    if config.CheckIntervalInSeconds > 0 {
        slog.Info(fmt.Sprintf("starting gitops repeated pull (every %s seconds)", fmt.Sprint(config.CheckIntervalInSeconds)))
        go func() {
            ticker := time.NewTicker(time.Duration(config.CheckIntervalInSeconds) * time.Second)
            defer ticker.Stop()
            for range ticker.C {
                trigger <- struct{}{}
            }
        }()
    } else {
        slog.Info("skipping gitops repeated pull (check interval is negative)")
    }

    // Webhook to trigger deployments
    if !config.DisableWebhook {
        http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
            select {
                case trigger <- struct{}{}:
                    slog.Info("triggered check via webhook")
                default:
                    slog.Info("ignored webhook as channel is already full")
            }
            w.WriteHeader(http.StatusAccepted)
        })
        slog.Info("webhook enabled", "url", "/webhook")
    } else {
        slog.Info("skipping webhook (disabled in config)")
    }
    
    // Start http server
    if !config.DisableMetrics || !config.DisableWebhook {
        if err := http.ListenAndServe(":2112", nil); err != nil {
            slog.Error("failed to start metrics server", "error", err)
            panic(err)
        }

        slog.Info("starting http server", "port", "2112")
    } else {
        slog.Info("skipping http server")
    }
}