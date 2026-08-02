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
// validate is the validation function the validate endpoint and the save gate
// both call. New builds it from the ValidatorFunc the caller supplies; tests
// override the field directly to inject a chosen finding set.
//
// The parlayBin field is gone. It held a `parlay` executable located once at
// construction, back when validating meant running one — there is no binary to
// locate now, and nothing about the editor's behaviour depends on what happens
// to be installed on the machine running it.
type Subsystem struct {
	Root string
	// Contribution, when set, puts the editor in contribution mode: it serves
	// the root model as usual AND the named feature's proposal, so the UI can
	// show what the feature adds and where the two disagree before the normal
	// save path commits anything.
	//
	// Zero is the ordinary mode, and the ordinary mode is unchanged — the
	// contribution endpoint reports that there is nothing to review and the
	// editor behaves exactly as it did before contributions existed.
	Contribution ContributionSource
	validate     func(ctx context.Context, model Model) ([]Finding, error)
}

// ContributionSource names the feature whose contribution the editor is
// reviewing and the file it lives in. The path is supplied rather than derived
// because mapping a feature identifier to a directory is the caller's job —
// this package would otherwise need a second copy of that resolution.
type ContributionSource struct {
	Feature string
	Path    string
}

// compile-time assertion that Subsystem satisfies the harness plug-in surface.
var _ server.ToolRegistration = (*Subsystem)(nil)

// New constructs the domain-model subsystem bound to a resolved project root,
// validating through the supplied ValidatorFunc.
//
// The validator is a parameter rather than a package-level default because this
// package cannot import the one that owns Core's rules (Go's internal rule —
// core/internal/agent is visible only under core/). Core wires it at the two
// call sites that open the editor.
//
// Construction cannot fail. It used to resolve an executable and swallow the
// error, which left a subsystem that looked constructed and failed on its first
// validate instead — a bad state to be able to hold.
// The contribution source is a parameter for the same reason the validator is:
// it is resolved from a feature identifier, and that resolution lives on the
// caller's side of the boundary. Pass the zero value for an ordinary editing
// session — every call site that does gets exactly the behaviour it had before
// contributions existed.
func New(root string, validate ValidatorFunc, contribution ContributionSource) *Subsystem {
	s := &Subsystem{Root: root, Contribution: contribution}
	s.validate = func(ctx context.Context, model Model) ([]Finding, error) {
		return Validate(ctx, validate, model)
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
	// One more QUERY: what a feature proposes against the model being edited.
	// It reads two files and writes nothing, so it does not widen the
	// persistence surface — accepting a contribution goes out through the
	// same PUT as any other edit.
	r.Get("/api/domain-model/contribution", s.contributionHandler)
}
