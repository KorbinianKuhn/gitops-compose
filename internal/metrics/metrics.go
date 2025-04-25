package metrics

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
    checkTimestamp *prometheus.GaugeVec
    checkCounter *prometheus.CounterVec
    activeDeploymentsGauge *prometheus.GaugeVec
    deploymentOperationsCounter *prometheus.CounterVec
}

func NewMetrics() *Metrics {
    metrics := &Metrics{
        checkTimestamp: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Namespace: "gitops",
                Subsystem: "check",
                Name: "timestamp_seconds",
                Help: "Unix timestamp of the last GitOps check by status",
            },
            []string{"status"},
        ),
        checkCounter: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: "gitops",
                Subsystem: "check",
                Name: "total",
                Help: "Total number of GitOps checks by status",
            },
            []string{"status"},
        ),
        activeDeploymentsGauge: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Namespace: "gitops",
                Subsystem: "deployments",
                Name: "active_total",
                Help: "Number of active deployments by status",
            },
            []string{"status"},
        ),
        deploymentOperationsCounter: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: "gitops",
                Subsystem: "deployments",
                Name:      "operations_total",
                Help:      "Total number of deployment operations.",
            },
            []string{"operation", "result"},
        ),
    }

    metrics.checkCounter.WithLabelValues("success").Add(0)
    metrics.checkTimestamp.WithLabelValues("success").Set(0)
    metrics.checkCounter.WithLabelValues("error").Add(0)
    metrics.checkTimestamp.WithLabelValues("error").Set(0)

    metrics.activeDeploymentsGauge.WithLabelValues("ok").Add(0)
    metrics.activeDeploymentsGauge.WithLabelValues("removed").Add(0)
    metrics.activeDeploymentsGauge.WithLabelValues("failed").Add(0)
    metrics.activeDeploymentsGauge.WithLabelValues("invalid").Add(0)

    metrics.deploymentOperationsCounter.WithLabelValues("start", "success").Add(0)
    metrics.deploymentOperationsCounter.WithLabelValues("start", "error").Add(0)
    metrics.deploymentOperationsCounter.WithLabelValues("config", "success").Add(0)
    metrics.deploymentOperationsCounter.WithLabelValues("config", "error").Add(0)
    metrics.deploymentOperationsCounter.WithLabelValues("stop", "success").Add(0)
    metrics.deploymentOperationsCounter.WithLabelValues("stop", "error").Add(0)

    return metrics
}

func (c *Metrics) TrackCheckStatus(status string) {
    c.checkCounter.WithLabelValues(status).Inc()
    c.checkTimestamp.WithLabelValues(status).SetToCurrentTime()
}

func (c *Metrics) TrackActiveDeployments(ok, removed, failed, invalid int) {
    slog.Info("Tracking active deployments", "ok", ok, "removed", removed, "failed", failed, "invalid", invalid)
    c.activeDeploymentsGauge.WithLabelValues("ok").Set(float64(ok))
    c.activeDeploymentsGauge.WithLabelValues("removed").Set(float64(removed))
    c.activeDeploymentsGauge.WithLabelValues("failed").Set(float64(failed))
    c.activeDeploymentsGauge.WithLabelValues("invalid").Set(float64(invalid))
}

func (c *Metrics) TrackDeploymentOperation(operation, result string) {
    slog.Info("Tracking deployment operation", "operation", operation, "result", result)
    c.deploymentOperationsCounter.WithLabelValues(operation, result).Inc()
}

func (c *Metrics) GetMetricsHandler() http.Handler {
	
	var r = prometheus.NewRegistry()
	r.MustRegister(
        c.checkTimestamp,
        c.checkCounter,
        c.activeDeploymentsGauge,
        c.deploymentOperationsCounter,
	)

	handler := promhttp.HandlerFor(r, promhttp.HandlerOpts{})

	return handler
}