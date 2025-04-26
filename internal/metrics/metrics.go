package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
    checkTimestamp *prometheus.GaugeVec
    checkCounter *prometheus.CounterVec
    deploymentTimestamp *prometheus.GaugeVec
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
        deploymentTimestamp: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Namespace: "gitops",
                Subsystem: "deployments",
                Name: "change_timestamp_seconds",
                Help: "Unix timestamp of the last deployment change by status",
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
            []string{"operation"},
        ),
    }

    metrics.checkCounter.WithLabelValues("success").Add(0)
    metrics.checkTimestamp.WithLabelValues("success").Set(0)
    metrics.checkCounter.WithLabelValues("error").Add(0)
    metrics.checkTimestamp.WithLabelValues("error").Set(0)

    metrics.deploymentTimestamp.WithLabelValues("success").Set(0)
    metrics.deploymentTimestamp.WithLabelValues("error").Set(0)

    metrics.activeDeploymentsGauge.WithLabelValues("running").Add(0)
    metrics.activeDeploymentsGauge.WithLabelValues("failed").Add(0)
    metrics.activeDeploymentsGauge.WithLabelValues("invalid").Add(0)

    metrics.deploymentOperationsCounter.WithLabelValues("started").Add(0)
    metrics.deploymentOperationsCounter.WithLabelValues("stopped").Add(0)
    metrics.deploymentOperationsCounter.WithLabelValues("updated").Add(0)
    metrics.deploymentOperationsCounter.WithLabelValues("failed").Add(0)
    metrics.deploymentOperationsCounter.WithLabelValues("invalid").Add(0)

    return metrics
}

func (c *Metrics) TrackCheckStatus(status string) {
    c.checkCounter.WithLabelValues(status).Inc()
    c.checkTimestamp.WithLabelValues(status).SetToCurrentTime()
}

func (c *Metrics) TrackDeploymentState(unchanged, started, stopped, updated, failed, invalid int) {
    // Timestamps
    if failed + invalid > 0 {
        c.deploymentTimestamp.WithLabelValues("error").SetToCurrentTime()
    } else if (started + stopped + updated) > 0 {
        c.deploymentTimestamp.WithLabelValues("success").SetToCurrentTime()
    }

    // Active states
    running := unchanged + started + stopped + updated
    c.activeDeploymentsGauge.WithLabelValues("running").Set(float64(running))
    c.activeDeploymentsGauge.WithLabelValues("failed").Set(float64(failed))
    c.activeDeploymentsGauge.WithLabelValues("invalid").Set(float64(invalid))

    // Operations
    c.deploymentOperationsCounter.WithLabelValues("started").Add(float64(started))
    c.deploymentOperationsCounter.WithLabelValues("stopped").Add(float64(stopped))
    c.deploymentOperationsCounter.WithLabelValues("updated").Add(float64(updated))
    c.deploymentOperationsCounter.WithLabelValues("failed").Add(float64(failed))
    c.deploymentOperationsCounter.WithLabelValues("invalid").Add(float64(invalid))
}

func (c *Metrics) GetMetricsHandler() http.Handler {
	
	var r = prometheus.NewRegistry()
	r.MustRegister(
        c.checkTimestamp,
        c.checkCounter,
        c.deploymentTimestamp,
        c.activeDeploymentsGauge,
        c.deploymentOperationsCounter,
	)

	handler := promhttp.HandlerFor(r, promhttp.HandlerOpts{})

	return handler
}