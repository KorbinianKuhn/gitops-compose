package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
    lastCheckTime       prometheus.Gauge
    lastSuccessTime     prometheus.Gauge
    lastErrorTime       prometheus.Gauge
    checkSuccessCounter prometheus.Counter
    checkErrorCounter   prometheus.Counter
    deploymentSuccessCounter prometheus.Counter
    deploymentErrorCounter   prometheus.Counter
    activeDeployments   *prometheus.GaugeVec
}

func NewMetrics() *Metrics {
    return &Metrics{
        lastCheckTime: promauto.NewGauge(
            prometheus.GaugeOpts{
                Namespace: "gitops",
                Subsystem: "check",
                Name:      "last_timestamp_seconds",
                Help:      "Unix timestamp of the last check.",
            },
        ),
        lastSuccessTime: promauto.NewGauge(
            prometheus.GaugeOpts{
                Namespace: "gitops",
                Subsystem: "check",
                Name:      "last_success_timestamp_seconds",
                Help:      "Unix timestamp of the last successful check.",
            },
        ),
        lastErrorTime: promauto.NewGauge(
            prometheus.GaugeOpts{
                Namespace: "gitops",
                Subsystem: "check",
                Name:      "last_error_timestamp_seconds",
                Help:      "Unix timestamp of the last failed check.",
            },
        ),
        checkSuccessCounter: promauto.NewCounter(
            prometheus.CounterOpts{
                Namespace: "gitops",
                Subsystem: "check",
                Name:      "success_total",
                Help:      "Total number of successful checks.",
            },
        ),
        checkErrorCounter: promauto.NewCounter(
            prometheus.CounterOpts{
                Namespace: "gitops",
                Subsystem: "check",
                Name:      "error_total",
                Help:      "Total number of failed checks.",
            },
        ),
        deploymentSuccessCounter: promauto.NewCounter(
            prometheus.CounterOpts{
                Namespace: "gitops",
                Subsystem: "deployment",
                Name:      "success_total",
                Help:      "Total number of successful deployments.",
            },
        ),
        deploymentErrorCounter: promauto.NewCounter(
            prometheus.CounterOpts{
                Namespace: "gitops",
                Subsystem: "deployment",
                Name:      "error_total",
                Help:      "Total number of failed deployments.",
            },
        ),
        // TODO: This is empty after a restart (should we really track a status?)
        activeDeployments: prometheus.NewGaugeVec(
            prometheus.GaugeOpts{
                Namespace: "gitops",
                Subsystem: "deployment",
                Name: "active_total",
                Help: "Number of active deployments by status",
            },
            []string{"status"},
        ),
    }
}

func (c *Metrics) TrackLastCheckTime() {
    c.lastCheckTime.Set(float64(time.Now().Unix()))
}

func (c *Metrics) TrackErrorMetrics() {
    c.lastErrorTime.Set(float64(time.Now().Unix()))
    c.checkErrorCounter.Inc()
}

func (c *Metrics) TrackSuccessMetrics() {
    c.lastSuccessTime.Set(float64(time.Now().Unix()))
    c.checkSuccessCounter.Inc()
}

func (c *Metrics) TrackDeploymentSuccessMetrics() {
    c.deploymentSuccessCounter.Inc()
}

func (c *Metrics) TrackDeploymentErrorMetrics() {
    c.deploymentErrorCounter.Inc()
}

func (c *Metrics) TrackActiveDeployments(ok, errors, removalFailed int) {
    c.activeDeployments.WithLabelValues("ok").Set(float64(ok))
    c.activeDeployments.WithLabelValues("error").Set(float64(errors))
    c.activeDeployments.WithLabelValues("removal_failed").Set(float64(removalFailed))
}

func (c *Metrics) GetMetricsHandler() http.Handler {
	
	var r = prometheus.NewRegistry()
	r.MustRegister(
		c.lastCheckTime, 
		c.lastSuccessTime, 
		c.lastErrorTime, 
		c.checkSuccessCounter, 
		c.checkErrorCounter,
        c.deploymentSuccessCounter,
        c.deploymentErrorCounter,
        c.activeDeployments,
	)

	handler := promhttp.HandlerFor(r, promhttp.HandlerOpts{})

	return handler
}