package internal

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

// MARK: NewHealthChecker

// Creates a new health checker with alive status set to true by default.
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{alive: 1}
}

// MARK: SetReady

// Updates the readiness status of the service.
func (hc *HealthChecker) SetReady(ready bool) {
	value := int64(0)
	if ready {
		value = 1
	}
	atomic.StoreInt64(&hc.ready, value)
}

// MARK: SetAlive

// Updates the liveness status of the service.
func (hc *HealthChecker) SetAlive(alive bool) {
	value := int64(0)
	if alive {
		value = 1
	}
	atomic.StoreInt64(&hc.alive, value)
}

// MARK: LivenessHandler

// HTTP handler for Kubernetes-style liveness probes.
func (hc *HealthChecker) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	status := HealthStatus{
		Timestamp: time.Now(),
		Version:   "0.1.0",
	}

	if atomic.LoadInt64(&hc.alive) == 1 {
		status.Status = "alive"
		w.WriteHeader(http.StatusOK)
	} else {
		status.Status = "dead"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// MARK: ReadinessHandler

// HTTP handler for Kubernetes-style readiness probes.
func (hc *HealthChecker) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	status := HealthStatus{
		Timestamp: time.Now(),
		Version:   "0.1.0",
	}

	if atomic.LoadInt64(&hc.ready) == 1 {
		status.Status = "ready"
		w.WriteHeader(http.StatusOK)
	} else {
		status.Status = "not ready"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
