// Package health serves the liveness and readiness endpoints for a service.
//
// The two probes answer different questions and must not be merged. Liveness asks
// whether the process should be restarted. Readiness asks whether the process should
// receive traffic. A liveness probe that touches a dependency restarts every replica
// when that dependency blips, which turns a partial outage into a full one.
package health

import (
	"context"
	"encoding/json"
	"net/http"
)

// Status is the response body. Probes are the only consumer, so one field is enough.
type Status struct {
	Status string `json:"status"`
}

const (
	statusOK           = "ok"
	statusUnavailable  = "unavailable"
	contentTypeJSON    = "application/json"
	pathLiveness       = "/healthz"
	pathReadiness      = "/readyz"
	contentTypeHeader  = "Content-Type"
	statusServiceUnavl = http.StatusServiceUnavailable
)

// Check reports whether a dependency is usable. A non-nil error means not ready.
//
// A Check must honour its context. The probe deadline is the only thing standing
// between a hung dependency and a hung kubelet, so a Check that ignores ctx makes
// readiness meaningless.
type Check func(context.Context) error

// New returns a mux serving the liveness and readiness endpoints.
//
// ready may be nil, which reports ready. That suits a service with no dependencies yet
// and keeps the package free of any service-specific import.
func New(ready Check) *http.ServeMux {
	mux := http.NewServeMux()
	// Registered by path alone, so any method is answered. Probes use GET, and a
	// 405 here would read as a broken service.
	mux.HandleFunc(pathLiveness, liveness)
	mux.HandleFunc(pathReadiness, readiness(ready))
	return mux
}

// LivenessHandler answers the liveness probe. Exported for a service that mounts its
// own routes rather than using New.
func LivenessHandler(w http.ResponseWriter, _ *http.Request) {
	writeStatus(w, http.StatusOK, statusOK)
}

// ReadinessHandler returns a readiness handler backed by ready. Exported for the same
// reason as LivenessHandler.
func ReadinessHandler(ready Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// Checked before the dependency, so a cancelled request reports 503 without
		// waiting on a dependency that may be the reason it was cancelled.
		if err := ctx.Err(); err != nil {
			writeStatus(w, statusServiceUnavl, statusUnavailable)
			return
		}
		if ready != nil {
			if err := ready(ctx); err != nil {
				writeStatus(w, statusServiceUnavl, statusUnavailable)
				return
			}
		}
		writeStatus(w, http.StatusOK, statusOK)
	}
}

func liveness(w http.ResponseWriter, _ *http.Request) {
	writeStatus(w, http.StatusOK, statusOK)
}

func readiness(ready Check) http.HandlerFunc {
	return ReadinessHandler(ready)
}

// writeStatus writes a single-field JSON body. The status text differs between ok and
// unavailable so an operator reading a probe response is never told "ok" beside a 503.
func writeStatus(w http.ResponseWriter, code int, status string) {
	w.Header().Set(contentTypeHeader, contentTypeJSON)
	w.WriteHeader(code)
	// Encode on a Status value, so a write failure cannot leave a partial body.
	_ = json.NewEncoder(w).Encode(Status{Status: status})
}
