// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/domain-model-subsystem-registration
// parlay-extends: domain-model-editor/domain-model-editor-validation/cross-cutting/out-of-process-validate-endpoint

package domain

import (
	"context"

	"github.com/go-chi/chi/v5"

	"github.com/ddwht/parlay/internal/editor/server"
)

// toolName is the unique subsystem name the harness registers this tool
// under. A second registration under the same name is rejected at
// router-construction time via the harness's tool-name-collision path.
const toolName = "domain-model"

// Subsystem is the domain-model editor tool subsystem. It satisfies
// server.ToolRegistration and mounts exactly one route group at the
// /api/domain-model prefix: two PERSISTENCE endpoints (GET load, PUT save)
// plus one QUERY endpoint (POST validate). The query does not widen the
// persistence surface.
//
// Root is the resolved project root whose domain-model.yaml the editor reads
// and writes (see resolveModelPath). In a multi-root project, v1 targets the
// resolved root's file with no root selector.
//
// parlayBin is the `parlay` executable path resolved ONCE at construction and
// reused by both the validate endpoint and the save gate. validate is the
// out-of-process validation function those two paths call; it defaults to the
// real subprocess wrapper over parlayBin, and is a field so tests can inject a
// fake validator without a live `parlay` on PATH.
type Subsystem struct {
	Root      string
	parlayBin string
	validate  func(ctx context.Context, model Model) ([]Finding, error)
}

// compile-time assertion that Subsystem satisfies the harness plug-in surface.
var _ server.ToolRegistration = (*Subsystem)(nil)

// New constructs the domain-model subsystem bound to a resolved project root.
// It resolves the `parlay` executable once (locateParlayBinary); a resolution
// failure is non-fatal — parlayBin stays empty and the real validate path
// surfaces a server-error if invoked. The signature is unchanged.
func New(root string) *Subsystem {
	bin, _ := locateParlayBinary()
	s := &Subsystem{Root: root, parlayBin: bin}
	// Default validator: the real out-of-process wrapper over the resolved
	// binary. Tests override s.validate to run the suite without a `parlay`.
	s.validate = func(ctx context.Context, model Model) ([]Finding, error) {
		return Validate(ctx, s.parlayBin, model)
	}
	return s
}

// Name returns the unique tool subsystem name.
func (s *Subsystem) Name() string { return toolName }

// Mount attaches the domain-model route group under the /api/domain-model
// prefix and nothing outside it:
//
//	GET  /api/domain-model/model    -> load handler        (persistence)
//	PUT  /api/domain-model/model    -> compare-and-swap    (persistence)
//	POST /api/domain-model/validate -> validate handler    (query)
//
// The persistence surface stays at exactly two endpoints; the validate route
// is a QUERY that reads and writes nothing, mounted in the same group.
func (s *Subsystem) Mount(r chi.Router) {
	r.Get("/api/domain-model/model", s.loadHandler)
	r.Put("/api/domain-model/model", s.saveHandler)
	r.Post("/api/domain-model/validate", s.validateHandler)
}
