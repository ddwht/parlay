// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/domain-model-subsystem-registration

package domain

import (
	"github.com/go-chi/chi/v5"

	"github.com/parlay-tool/parlay/studio/internal/server"
)

// toolName is the unique subsystem name the harness registers this tool
// under. A second registration under the same name is rejected at
// router-construction time via the harness's tool-name-collision path.
const toolName = "domain-model"

// Subsystem is the domain-model editor tool subsystem. It satisfies
// server.ToolRegistration and mounts exactly one route group at the
// /api/domain-model prefix, exposing exactly two persistence endpoints.
//
// Root is the resolved project root whose domain-model.yaml the editor reads
// and writes (see resolveModelPath). In a multi-root project, v1 targets the
// resolved root's file with no root selector.
type Subsystem struct {
	Root string
}

// compile-time assertion that Subsystem satisfies the harness plug-in surface.
var _ server.ToolRegistration = (*Subsystem)(nil)

// New constructs the domain-model subsystem bound to a resolved project root.
func New(root string) *Subsystem {
	return &Subsystem{Root: root}
}

// Name returns the unique tool subsystem name.
func (s *Subsystem) Name() string { return toolName }

// Mount attaches the two persistence endpoints under the /api/domain-model
// prefix and nothing outside it:
//
//	GET  /api/domain-model/model -> load handler
//	PUT  /api/domain-model/model -> compare-and-swap save handler
func (s *Subsystem) Mount(r chi.Router) {
	r.Get("/api/domain-model/model", s.loadHandler)
	r.Put("/api/domain-model/model", s.saveHandler)
}
