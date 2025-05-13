package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/korbiniankuhn/gitops-compose/internal/config"
	"github.com/korbiniankuhn/gitops-compose/internal/docker"
	"github.com/korbiniankuhn/gitops-compose/internal/git"
	"github.com/korbiniankuhn/gitops-compose/internal/gitops"
	"github.com/korbiniankuhn/gitops-compose/internal/metrics"
)

func panicOnError(message string, err error) {
	if err != nil {
		slog.Error(message, "error", err)
		panic(err)
	}
}

func main() {
	slog.Info("starting gitops compose")

	// Load config
	config, err := config.Get()
	panicOnError("failed to load config", err)
	slog.Info("config loaded")

	if config.RepositoryUsername == "" {
		slog.Warn("no credentials set in repository origin")
	}

	// Verify git repository
	r, err := git.NewDeploymentRepo(config.RepositoryUsername, config.RepositoryPassword, config.RepositoryPath)
	panicOnError("failed to create deployment repo", err)

	// Verify git remote access
	panicOnError("failed to verify git remote access", r.VerifyRemoteAccess())
	slog.Info("git remote access verified")

	// TODO: skip when go-git is able to pull changes without losing untracked files
	// Verify git cli
	panicOnError("failed to verify git cli", r.VerifyGitCli())

	slog.Info("deployment repo configured", "path", config.RepositoryPath)

	// Verify docker socket connection
	d := docker.NewDocker(config.DockerRegistries)
	panicOnError("failed to verify docker socket connection", d.VerifySocketConnection())
	slog.Info("docker socket connection verified")

	// Verify docker credentials (if set)
	loggedIn, err := d.LoginIfCredentialsSet()
	panicOnError("failed to verify docker registry credentials", err)
	if loggedIn {
		slog.Info("docker registry credentials verified")
	}

	// Initialise metrics
	m := metrics.NewMetrics()
	if config.MetricsEnabled {
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
		// TODO: graceful shutdown wait group
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
	if config.WebhookEnabled {
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

	// TODO: Move to go routine
	// TODO: Add health check endpoint
	// Start http server
	panicOnError("failed to start http server", http.ListenAndServe(":2112", nil))
	slog.Info("starting http server", "port", "2112")

	// Wait for termination signal
	osSignal := make(chan os.Signal, 1)
	signal.Notify(osSignal, syscall.SIGINT, syscall.SIGTERM)
	<-osSignal
	slog.Info("received termination signal, shutting down")
	close(trigger)

	// TODO: graceful shutdown wait group
}
