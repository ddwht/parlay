// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: cross-cutting/out-of-process-validate-endpoint

package commands

import (
	"context"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/internal/editor/domain"
)

// domainValidator is the editor's validator: Core's domain-model rules, called
// in-process, in authoring mode.
//
// This function is the whole of what used to be a subprocess. The editor once
// reached these rules by locating a `parlay` binary and running `validate --type
// domain-model --json -` with the draft on stdin, because it shipped as its own
// Go module and had no other way in. It is one call now.
//
// It lives on Core's side of the boundary rather than in the editor package
// because the editor cannot import agent — core/internal/agent is visible only
// from under core/ — and because this is the layer that legitimately knows both
// halves. The editor declares what it needs (domain.ValidatorFunc); Core knows
// agent satisfies it.
//
// Authoring mode is load-bearing, not a default. It is what keeps
// domain-operations-deprecated a warning instead of an error, and therefore what
// lets a model carrying a deprecated operations block still be saved from the
// editor. Build mode would promote it and the save gate would start refusing
// drafts that `parlay validate` accepts.
//
// domain.StdinLabel is passed for the same reason the CLI's --json mode uses
// "<stdin>": a draft in memory has no path, and anchoring both entry points on
// the same token keeps their messages byte-identical. The parity suite compares
// exactly that.
func domainValidator(_ context.Context, draftYAML []byte) []domain.CoreFinding {
	errs := agent.ValidateDomainModelStructuredMode(domain.StdinLabel, draftYAML, agent.ModeAuthoring)
	out := make([]domain.CoreFinding, 0, len(errs))
	for _, e := range errs {
		out = append(out, domain.CoreFinding{
			Code:     e.Code,
			Message:  e.Message,
			Context:  e.Context,
			Fix:      e.Fix,
			Severity: e.Severity,
		})
	}
	return out
}
