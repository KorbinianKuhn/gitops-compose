package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	checkTimestamp              *prometheus.GaugeVec
	checkCounter                *prometheus.CounterVec
	deploymentTimestamp         *prometheus.GaugeVec
	activeDeploymentsGauge      *prometheus.GaugeVec
	deploymentOperationsCounter *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	metrics := &Metrics{
		checkTimestamp: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "gitops",
				Subsystem: "check",
				Name:      "timestamp_seconds",
				Help:      "Unix timestamp of the last GitOps check by status",
			},
			[]string{"status"},
		),
		checkCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "gitops",
				Subsystem: "check",
				Name:      "total",
				Help:      "Total number of GitOps checks by status",
			},
			[]string{"status"},
		),
		deploymentTimestamp: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "gitops",
				Subsystem: "deployments",
				Name:      "change_timestamp_seconds",
				Help:      "Unix timestamp of the last deployment change by status",
			},
			[]string{"status"},
		),
		activeDeploymentsGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "gitops",
				Subsystem: "deployments",
				Name:      "active_total",
				Help:      "Number of active deployments by status",
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
	metrics.activeDeploymentsGauge.WithLabelValues("ignored").Add(0)

	metrics.deploymentOperationsCounter.WithLabelValues("started").Add(0)
	metrics.deploymentOperationsCounter.WithLabelValues("stopped").Add(0)
	metrics.deploymentOperationsCounter.WithLabelValues("updated").Add(0)
	metrics.deploymentOperationsCounter.WithLabelValues("failed").Add(0)
	metrics.deploymentOperationsCounter.WithLabelValues("invalid").Add(0)
	metrics.deploymentOperationsCounter.WithLabelValues("ignored").Add(0)

	return metrics
}

func (c *Metrics) TrackCheckStatus(status string) {
	c.checkCounter.WithLabelValues(status).Inc()
	c.checkTimestamp.WithLabelValues(status).SetToCurrentTime()
}

type DeploymentState struct {
	Unchanged int
	Started   int
	Stopped   int
	Updated   int
	Failed    int
	Invalid   int
	Ignored   int
}

func NewState() *DeploymentState {
	return &DeploymentState{
		Unchanged: 0,
		Started:   0,
		Stopped:   0,
		Updated:   0,
		Failed:    0,
		Invalid:   0,
		Ignored:   0,
	}
}

func (s *DeploymentState) HasErrors() bool {
	return s.Failed > 0 || s.Invalid > 0
}

func (s *DeploymentState) HasChanges() bool {
	return s.Started > 0 || s.Stopped > 0 || s.Updated > 0
}

func (s *DeploymentState) CountRunning() int {
	return s.Unchanged + s.Started + s.Updated
}

func (c *Metrics) TrackDeploymentState(state *DeploymentState) {
	// Timestamps
	if state.HasErrors() {
		c.deploymentTimestamp.WithLabelValues("error").SetToCurrentTime()
	} else if state.HasChanges() {
		c.deploymentTimestamp.WithLabelValues("success").SetToCurrentTime()
	}

	// Active states
	c.activeDeploymentsGauge.WithLabelValues("running").Set(float64(state.CountRunning()))
	c.activeDeploymentsGauge.WithLabelValues("failed").Set(float64(state.Failed))
	c.activeDeploymentsGauge.WithLabelValues("invalid").Set(float64(state.Invalid))
	c.activeDeploymentsGauge.WithLabelValues("ignored").Set(float64(state.Ignored))

	// Operations
	c.deploymentOperationsCounter.WithLabelValues("started").Add(float64(state.Started))
	c.deploymentOperationsCounter.WithLabelValues("stopped").Add(float64(state.Stopped))
	c.deploymentOperationsCounter.WithLabelValues("updated").Add(float64(state.Updated))
	c.deploymentOperationsCounter.WithLabelValues("failed").Add(float64(state.Failed))
	c.deploymentOperationsCounter.WithLabelValues("invalid").Add(float64(state.Invalid))
	c.deploymentOperationsCounter.WithLabelValues("ignored").Add(float64(state.Ignored))
}

func (m *Metrics) GetMetricsHandler() http.Handler {

	var r = prometheus.NewRegistry()
	r.MustRegister(
		m.checkTimestamp,
		m.checkCounter,
		m.deploymentTimestamp,
		m.activeDeploymentsGauge,
		m.deploymentOperationsCounter,
	)

	handler := promhttp.HandlerFor(r, promhttp.HandlerOpts{})

	return handler
}
