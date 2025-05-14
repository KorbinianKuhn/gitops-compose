package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
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
	c, err := config.Get()
	panicOnError("failed to load config", err)
	slog.Info("config loaded")

	if c.RepositoryUsername == "" {
		slog.Warn("no credentials set in repository origin")
	}

	// Verify git repository
	deploymentRepoOptions := []git.DeploymentRepoOption{}
	if c.RepositoryUsername != "" {
		deploymentRepoOptions = append(deploymentRepoOptions, git.WithAuth(c.RepositoryUsername, c.RepositoryPassword))
	}
	r, err := git.NewDeploymentRepo(c.RepositoryPath, deploymentRepoOptions...)
	panicOnError("failed to create deployment repo", err)
	slog.Info("deployment repo initialised", "path", c.RepositoryPath)

	// Verify git remote access
	panicOnError("failed to verify git remote access", r.VerifyRemoteAccess())
	panicOnError("failed to verify git cli", r.VerifyGitCli())
	slog.Info("git remote access verified")

	// Verify docker socket connection
	d := docker.NewDocker(c.DockerRegistries)
	panicOnError("failed to verify docker socket connection", d.VerifySocketConnection())
	slog.Info("docker socket connection verified")

	// Warn if dockerised gitops-compose is running on docker desktop
	if c.IsRunningInDocker {
		isDockerDesktop, err := d.IsDockerDesktop()
		panicOnError("failed to verify if docker is running in docker desktop", err)
		if isDockerDesktop {
			slog.Warn("docker is running in docker desktop (volume mounts might cause issues)")
		}
	}

	// Verify docker credentials (if set)
	loggedIn, err := d.LoginIfCredentialsSet()
	panicOnError("failed to verify docker registry credentials", err)
	if loggedIn {
		slog.Info("docker registry credentials verified")
	}

	// Initialise metrics
	m := metrics.NewMetrics()
	if c.MetricsEnabled {
		http.Handle("/metrics", m.GetMetricsHandler())
		slog.Info("metrics enabled", "url", "/metrics")
	}

	// Initialise gitops
	g := gitops.NewGitOps(r, d, m)

	if err := g.EnsureDeploymentsAreRunning(); err != nil {
		m.TrackCheckStatus("error")
		slog.Error("failed to ensure deployments are running on initial start", "error", err)
	} else {
		m.TrackCheckStatus("success")
	}

	wg := sync.WaitGroup{}
	check := make(chan struct{})

	// Run gitops check on trigger
	wg.Add(1)
	go func() {
		for range check {
			if err := g.CheckAndUpdateDeployments(); err != nil {
				m.TrackCheckStatus("error")
			} else {
				m.TrackCheckStatus("success")
			}
		}
		wg.Done()
	}()

	// Run gitops check on interval
	if c.CheckIntervalInSeconds > 0 {
		slog.Info(fmt.Sprintf("starting gitops repeated pull (every %s seconds)", fmt.Sprint(c.CheckIntervalInSeconds)))
		go func() {
			ticker := time.NewTicker(time.Duration(c.CheckIntervalInSeconds) * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				check <- struct{}{}
			}
		}()
	} else {
		slog.Info("skipping gitops repeated pull (check interval is negative)")
	}

	// Webhook to trigger deployments
	if c.WebhookEnabled {
		http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
			select {
			case check <- struct{}{}:
				slog.Info("triggered check via webhook")
			default:
				slog.Info("ignored webhook as channel is already full")
			}
			w.WriteHeader(http.StatusAccepted)
		})
		slog.Info("webhook enabled", "url", "/webhook")
	}

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	slog.Info("health check endpoint", "url", "/health")

	// Start http server
	s := http.Server{
		Addr: ":2112",
	}
	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panicOnError("failed to start http server", err)
		}
	}()
	slog.Info("http server started", "port", "2112")

	// Wait for termination signal
	osSignal := make(chan os.Signal, 1)
	signal.Notify(osSignal, syscall.SIGINT, syscall.SIGTERM)

	<-osSignal
	slog.Info("received termination signal, shutting down")

	close(check)

	// Stop http server
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		panicOnError("failed to shutdown http server", err)
	}

	// Run until shutdown is complete
	wg.Wait()
	slog.Info("gitops compose gracefully stopped")
}
