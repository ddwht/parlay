// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/domain-model-subsystem-registration
// parlay-extends: domain-model-editor/domain-model-editor-validation/cross-cutting/out-of-process-validate-endpoint
// parlay-extends: domain-model-editor/domain-model-editor-validation/cross-cutting/save-validation-gate-before-cas

package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/ddwht/parlay/internal/editor/server"
)

// modelResponse is the JSON body of a successful load or save: the model plus
// its current content-identity token.
type modelResponse struct {
	Model Model  `json:"model"`
	Etag  string `json:"etag"`
}

// saveRequest is the JSON body of a save: the edited model plus the token
// from its originating load.
type saveRequest struct {
	Model Model  `json:"model"`
	Etag  string `json:"etag"`
}

// validateRequest is the JSON body of a validate query: the draft to check.
// It carries no etag — validation is side-effect-free and reads nothing from
// disk, so there is no compare-and-swap to reconcile.
type validateRequest struct {
	Model Model `json:"model"`
}

// validateResponse is the 200 body of a validate query: the complete finding
// list for the submitted draft. It is a query result — an EMPTY list for a
// clean draft, never a 4xx. A malformed request (unparseable bytes) is the one
// HTTP-error case and travels through the validation-failed envelope instead.
type validateResponse struct {
	Fields []Finding `json:"fields"`
}

// loadHandler serves GET /api/domain-model/model. It returns the parsed model
// plus the content identity token. A load against a project with no model
// file returns the empty-model bootstrap (etag "empty"), never not-found. Its
// error surface is closed to validation-failed and server-error.
func (s *Subsystem) loadHandler(w http.ResponseWriter, r *http.Request) {
	model, etag, err := Load(r.Context(), s.Root)
	if err != nil {
		server.WriteError(w, r, mapLoadError(err))
		return
	}
	writeJSON(w, r, http.StatusOK, modelResponse{Model: model, Etag: string(etag)})
}

// validateHandler serves POST /api/domain-model/validate. It is a QUERY: it
// decodes the submitted draft and returns the complete finding list at HTTP
// 200 (an EMPTY list when clean — a finding list is a query result, never a
// 4xx). Only a malformed request (unparseable bytes) is an HTTP error: it
// returns the validation-failed envelope at 400 WITHOUT running the
// subprocess. The endpoint is side-effect-free — it reads nothing from disk,
// mutates nothing, and reflects the submitted bytes alone.
func (s *Subsystem) validateHandler(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Malformed request: reject before the subprocess is ever run.
		server.WriteError(w, r, &server.ValidationError{
			Fields: []server.FieldError{{Field: "body", Message: "request body is not valid JSON"}},
		})
		return
	}
	findings, err := s.validate(r.Context(), req.Model)
	if err != nil {
		server.WriteError(w, r, fmt.Errorf("domain: validate draft: %w", err))
		return
	}
	if findings == nil {
		findings = []Finding{}
	}
	writeJSON(w, r, http.StatusOK, validateResponse{Fields: findings})
}

// saveHandler serves PUT /api/domain-model/model. It accepts an edited model
// plus the token from its originating load. Before the compare-and-swap it
// runs the server-side validation gate: a draft carrying ANY error-severity
// finding is rejected with the validation-failed envelope and nothing touches
// disk. Warnings (domain-operations-deprecated) never block. Because the gate
// runs BEFORE Save, validation-failed precedes conflict for a draft that is
// both invalid and stale. The gate is authoritative — a save submitted
// directly to this endpoint (bypassing the UI's blocked-save affordance) is
// gated identically; there is no force-save path. Its error surface is closed
// to validation-failed, conflict, and server-error.
func (s *Subsystem) saveHandler(w http.ResponseWriter, r *http.Request) {
	var req saveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.WriteError(w, r, &server.ValidationError{
			Fields: []server.FieldError{{Field: "body", Message: "request body is not valid JSON"}},
		})
		return
	}

	// Validation gate — ordered BEFORE Save (and thus before the etag
	// compare-and-swap). Reuses the same out-of-process validator the validate
	// endpoint uses; no second validation code path, no Studio-side rule.
	findings, err := s.validate(r.Context(), req.Model)
	if err != nil {
		server.WriteError(w, r, fmt.Errorf("domain: validate on save: %w", err))
		return
	}
	if HasErrorFinding(findings) {
		server.WriteError(w, r, &server.ValidationError{Fields: errorFindingFields(findings)})
		return
	}

	newEtag, err := Save(r.Context(), s.Root, req.Model, Etag(req.Etag))
	if err != nil {
		// A *server.ConflictError translates to the 409 conflict envelope
		// carrying current+attempted etags; anything else falls through to
		// server-error.
		server.WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, modelResponse{Model: req.Model, Etag: string(newEtag)})
}

// errorFindingFields projects the error-severity findings into the
// validation-failed envelope's Fields, so a blocked save names exactly what
// must be repaired (the element path and the actionable message). Warnings are
// omitted — they never gate.
func errorFindingFields(findings []Finding) []server.FieldError {
	var fields []server.FieldError
	for _, f := range findings {
		if !f.IsError() {
			continue
		}
		msg := f.Message
		if f.Fix != "" {
			msg = f.Fix
		}
		fields = append(fields, server.FieldError{Field: f.Field, Message: msg})
	}
	return fields
}

// mapLoadError narrows loader failures to the closed error surface. Failures a
// person can act on map to validation-failed and carry the reason; everything
// else falls through to server-error.
//
// Unparseable YAML belongs in the first group and used to be in the second.
// A hand-broken domain-model.yaml produced a 500 whose body was a request id,
// while `parlay validate` on the same file named the offending line — so the
// user best placed to fix it was the one told least about it, and "clean in
// the editor" and "passes the build" stopped being the same statement for the
// most ordinary kind of breakage there is.
func mapLoadError(err error) error {
	if errors.Is(err, ErrSchemaVersionNewer) || errors.Is(err, ErrMissingSchemaVersion) {
		return &server.ValidationError{
			Fields: []server.FieldError{{Field: "schema_version", Message: err.Error()}},
		}
	}
	if errors.Is(err, ErrInvalidYAML) {
		return &server.ValidationError{
			Fields: []server.FieldError{{Field: "domain-model.yaml", Message: err.Error()}},
		}
	}
	return err
}

// writeJSON writes a success body as JSON. Handlers are in a different package
// from the harness's unexported envelope writer, so success responses are
// serialized here; error responses go through server.WriteError.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// The header/status are already written; log-and-drop is the only
		// remaining option. Fall through to the harness error path is not
		// possible once WriteHeader has been called.
		_ = err
	}
}
