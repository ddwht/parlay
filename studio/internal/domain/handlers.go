// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/domain-model-subsystem-registration

package domain

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/parlay-tool/parlay/studio/internal/server"
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

// saveHandler serves PUT /api/domain-model/model. It accepts an edited model
// plus the token from its originating load and performs the compare-and-swap
// write. Its error surface is closed to validation-failed, conflict, and
// server-error.
func (s *Subsystem) saveHandler(w http.ResponseWriter, r *http.Request) {
	var req saveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.WriteError(w, r, &server.ValidationError{
			Fields: []server.FieldError{{Field: "body", Message: "request body is not valid JSON"}},
		})
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

// mapLoadError narrows loader failures to the closed error surface. The
// schema-version failures are actionable and map to validation-failed (NOT a
// generic server-error); everything else falls through to server-error.
func mapLoadError(err error) error {
	if errors.Is(err, ErrSchemaVersionNewer) || errors.Is(err, ErrMissingSchemaVersion) {
		return &server.ValidationError{
			Fields: []server.FieldError{{Field: "schema_version", Message: err.Error()}},
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
