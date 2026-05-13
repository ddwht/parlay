// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/http-server-harness

package server

import (
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// HealthHandler is the universal liveness handler mounted at /api/health.
// It returns 200 with {status:"ok", request_id:<id>} and intentionally
// avoids advancing the idle timer (it is mounted at the router root rather
// than inside the /api/* group that carries the IdleTimeoutReset
// middleware).
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	rid := chimw.GetReqID(r.Context())
	if rid != "" {
		w.Header().Set("X-Request-ID", rid)
	}
	writeEnvelope(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"request_id": rid,
	})
}

// ShutdownHandler returns the explicit-close trigger handler. Callers
// supply the unified shutdown channel; the handler pushes the explicit
// reason string onto it and returns 202 Accepted. The handler does NOT
// wait for the actual shutdown to complete — the boot goroutine consumes
// the channel and orchestrates the drain.
func ShutdownHandler(shutdownChan chan<- string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const reason = "explicit: /api/shutdown"
		if shutdownChan != nil {
			// Non-blocking send: if a shutdown is already in flight, drop
			// the duplicate trigger rather than blocking the request.
			select {
			case shutdownChan <- reason:
			default:
			}
		}
		writeEnvelope(w, http.StatusAccepted, map[string]any{
			"status": "shutting-down",
			"reason": reason,
		})
	}
}
