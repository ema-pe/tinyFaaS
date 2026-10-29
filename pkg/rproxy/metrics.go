package rproxy

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	activationSync  = "sync"
	activationAsync = "async"
)

var (
	functionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tinyfaas_function_duration_seconds",
		Help:    "Duration of function invocations in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"function_name"})

	functionInvocations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tinyfaas_function_invocation_total",
		Help: "Total number of function invocations.",
	}, []string{"function_name", "status", "activation_type"})
)

// observeInvocation records a completed function invocation. status is the
// HTTP status code returned by the function handler.
func observeInvocation(name string, status int, async bool, duration time.Duration) {
	activation := activationSync
	if async {
		activation = activationAsync
	}

	functionDuration.WithLabelValues(name).Observe(duration.Seconds())
	functionInvocations.WithLabelValues(name, strconv.Itoa(status), activation).Inc()
}
