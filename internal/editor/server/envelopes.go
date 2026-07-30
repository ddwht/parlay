// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/http-server-harness

package server

import (
	"errors"
	"log"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// Stable error kinds the harness translates into JSON envelopes per the
// go-studio-app adapter's error-to-status mapping convention.
//
//	validation-failed -> 400 {code, fields: [...]}
//	not-found         -> 404 {code, target}
//	conflict          -> 409 {code, current_etag, attempted_etag}
//	server-error      -> 500 {code, request_id}
//
// Handlers cooperate by returning one of the typed errors below (or wrapping
// a package-local sentinel that satisfies the interface).

// ValidationError carries one or more field-level error descriptors.
type ValidationError struct {
	Fields []FieldError
}

// FieldError is one entry in ValidationError.Fields.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string { return "validation-failed" }

// NotFoundError names the missing target resource.
type NotFoundError struct {
	Target string
}

func (e *NotFoundError) Error() string { return "not-found: " + e.Target }

// ConflictError carries the etag pair from a failed compare-and-swap.
type ConflictError struct {
	CurrentETag   string
	AttemptedETag string
}

func (e *ConflictError) Error() string { return "conflict" }

// ServerError is the catchall fallthrough. Handlers wrap unmapped errors
// with a ServerError; the underlying err is logged but never returned to
// the client.
type ServerError struct {
	Cause error
}

func (e *ServerError) Error() string {
	if e == nil || e.Cause == nil {
		return "server-error"
	}
	return "server-error: " + e.Cause.Error()
}

// Unwrap exposes the underlying error so errors.Is/As keeps working.
func (e *ServerError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// WriteError is the canonical translator from handler-level errors to the
// harness JSON envelope. Handlers call this exactly once per response when
// they cannot succeed. Unrecognised errors fall through to server-error
// (and are logged with the request ID).
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		writeEnvelope(w, http.StatusOK, map[string]any{"status": "ok"})
		return
	}
	rid := chimw.GetReqID(r.Context())

	var verr *ValidationError
	if errors.As(err, &verr) {
		writeEnvelope(w, http.StatusBadRequest, map[string]any{
			"code":   "validation-failed",
			"fields": verr.Fields,
		})
		return
	}

	var nferr *NotFoundError
	if errors.As(err, &nferr) {
		writeEnvelope(w, http.StatusNotFound, map[string]any{
			"code":   "not-found",
			"target": nferr.Target,
		})
		return
	}

	var cerr *ConflictError
	if errors.As(err, &cerr) {
		writeEnvelope(w, http.StatusConflict, map[string]any{
			"code":           "conflict",
			"current_etag":   cerr.CurrentETag,
			"attempted_etag": cerr.AttemptedETag,
		})
		return
	}

	// Fallthrough — server-error. Log the underlying detail with the
	// request_id so an operator can correlate; never return the detail in
	// the body.
	log.Printf("ERROR handler: request_id=%s error=%v", rid, err)
	writeEnvelope(w, http.StatusInternalServerError, map[string]any{
		"code":       "server-error",
		"request_id": rid,
	})
}
