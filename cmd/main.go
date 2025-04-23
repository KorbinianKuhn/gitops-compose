package main

import (
	"net/http"
	"time"

	"github.com/korbiniankuhn/gitops-compose/internal/config"
	"github.com/korbiniankuhn/gitops-compose/internal/git"
	"github.com/korbiniankuhn/gitops-compose/internal/gitops"
	"github.com/korbiniankuhn/gitops-compose/internal/metrics"

	"log/slog"
)

func main() {
    slog.Info("starting gitops compose")
    config, err := config.Get()
    if err != nil {
        slog.Error("failed to load config", "error", err)
        panic(err)
    }
    slog.Info("config loaded")

    // TODO: check if docker socket is available

    r, err := git.NewDeploymentRepo(config.RepositoryUsername, config.RepositoryPassword, config.RepositoryPath)
    if err != nil {
        slog.Error("failed to open deployment repo", "error", err)
        panic(err)
    }
    slog.Info("deployment repo configured", "path", config.RepositoryPath)

    m := metrics.NewMetrics()
    m.TrackActiveDeployments(0,0,0)

    slog.Info("starting gitops repeated pull")
    go func() {
        for {
            gitops.Check(r, m)
            time.Sleep(5 * time.Second)
        }
    }()

    slog.Info("starting metrics server", "port", "2112")
    handler := m.GetMetricsHandler()
    http.Handle("/metrics", handler)
    http.ListenAndServe(":2112", nil)
}