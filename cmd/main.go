package main

import (
	"context"
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
	// Default logger (will be overwritten during config load)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Load config
	c, err := config.Get()
	panicOnError("failed to load config", err)

	// Set logger
	switch c.LogFormat {
	case "text":
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.Level(c.LogLevel),
		})))
	case "json":
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.Level(c.LogLevel),
		})))
	default:
		slog.SetLogLoggerLevel(slog.Level(c.LogLevel))
	}

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

	// Cancelled on termination signal
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Buffered so one webhook can be queued while a check is running.
	// Never closed (the check loop exits via ctx), so late webhooks can't panic.
	trigger := make(chan struct{}, 1)

	// A nil channel blocks forever in select, which disables the interval
	var tick <-chan time.Time
	if c.CheckIntervalInSeconds > 0 {
		slog.Info(fmt.Sprintf("starting gitops repeated pull (every %s seconds)", fmt.Sprint(c.CheckIntervalInSeconds)))
		ticker := time.NewTicker(time.Duration(c.CheckIntervalInSeconds) * time.Second)
		defer ticker.Stop()
		tick = ticker.C
	} else {
		slog.Info("skipping gitops repeated pull (check interval is negative)")
	}

	// Webhook to trigger deployments
	if c.WebhookEnabled {
		http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
			select {
			case trigger <- struct{}{}:
				slog.Info("triggered check via webhook")
			default:
				slog.Info("ignored webhook as a check is already queued")
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
	go func() {
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panicOnError("failed to start http server", err)
		}
	}()
	slog.Info("http server started", "port", "2112")

	// Run checks on start, interval and webhook until a termination signal is received.
	// A check in progress always completes before shutdown.
	for ctx.Err() == nil {
		g.CheckAndUpdate()

		select {
		case <-ctx.Done():
		case <-tick:
		case <-trigger:
		}
	}
	slog.Info("received termination signal, shutting down")

	// Stop http server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(shutdownCtx); err != nil {
		panicOnError("failed to shutdown http server", err)
	}

	slog.Info("gitops compose gracefully stopped")
}
