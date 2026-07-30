// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/http-server-harness

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// defaultReadHeaderTimeout is applied to http.Server.ReadHeaderTimeout to
// short-circuit slow header reads on the loopback interface.
const defaultReadHeaderTimeout = 10 * time.Second

// PanicRecovery recovers from panics in downstream handlers, logs the panic
// detail with the request ID, and emits a server-error envelope. The panic
// detail NEVER leaks into the response body; only the request ID does. The
// log line is the only place the operator can correlate the response back
// to the recovered panic.
func PanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				rid := chimw.GetReqID(r.Context())
				log.Printf("ERROR panic recovered: request_id=%s panic=%v\nstack=%s",
					rid, rec, debug.Stack())
				writeEnvelope(w, http.StatusInternalServerError, map[string]any{
					"code":       "server-error",
					"request_id": rid,
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// IdleTimeoutReset returns a middleware that calls tracker.Touch() on every
// inbound request. The middleware is applied via a chi.Group that excludes
// /api/health, so health checks do not advance the idle timer.
func IdleTimeoutReset(tracker *IdleTracker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tracker != nil {
				tracker.Touch()
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Logger emits one structured log line per HTTP request. The shape matches
// the documented harness contract:
//
//	INFO http: request_id=<id> method=<m> path=<p> status=<s> duration=<d>
//
// The X-Request-ID response header is set to the chi-assigned request ID so
// the response, the log line, and any error envelope all reference the same
// identifier.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := chimw.GetReqID(r.Context())
		if rid != "" {
			w.Header().Set("X-Request-ID", rid)
		}
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		log.Printf("INFO http: request_id=%s method=%s path=%s status=%d duration=%s",
			rid, r.Method, r.URL.Path, ww.Status(), time.Since(start))
	})
}

// ErrorEnvelopeTranslate intercepts the response body when downstream
// handlers signal a structured error via the harness helpers (WriteError /
// FailRequest). Handlers that signal an unmapped error fall through to
// server-error.
//
// Handlers cooperate with this middleware by calling WriteError(w, err) —
// the middleware itself does not introspect arbitrary error values pulled
// from a context; it is the response-side complement to FailRequest.
func ErrorEnvelopeTranslate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The translation is performed at the call site (WriteError) — this
		// middleware exists in the chain so the documented order matches the
		// harness contract and so future cross-cutting hooks (e.g. response
		// validation) can be added without touching handler code.
		next.ServeHTTP(w, r)
	})
}

// writeEnvelope marshals body as JSON, sets Content-Type, and writes status.
// On marshal failure it emits a minimal text/plain server-error rather than
// looping back through the JSON path.
func writeEnvelope(w http.ResponseWriter, status int, body any) {
	buf := bytes.NewBuffer(nil)
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "server-error\n")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}
