// parlay-feature: parlay-tool/domain-document-api
// parlay-component: cross-cutting/out-of-process-validate-endpoint
//
// The component name says out-of-process and the mechanism is not, which is
// deliberate rather than overlooked. The identifier is a spec anchor that the
// coverage and drift checks read through the marker-to-buildfile
// correspondence. Renaming it here alone would break that correspondence;
// renaming it properly is a spec migration across those artifacts, which is
// not this change. Recorded so the next reader knows the staleness was priced
// rather than missed.

package commands

import (
	"context"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/domainmodel"
)

// domainValidator is the ValidatorFunc reserved for the domain-model write
// path: Core's domain-model rules, called in-process, in authoring mode.
// TODAY its only callers are the one-engine parity tests — no production
// write is gated on it yet; the Phase-3 save gate will be its first.
//
// This function is the whole of what used to be a subprocess. The browser
// editor once reached these rules by locating a `parlay` binary and running
// `validate --type domain-model --json -` with the draft on stdin, because it
// shipped as its own Go module and had no other way in. It is one call now,
// and the editor it was written for is gone.
//
// It stays on this side of the seam rather than moving into domainmodel
// because domainmodel cannot import agent: agent imports domainmodel for Model
// and Diff, so the reverse edge would close a cycle. domainmodel declares what
// it needs (domainmodel.ValidatorFunc); this layer knows agent satisfies it.
//
// Authoring mode is load-bearing, not a default: it is the mode the future
// save gate is specified to validate in, so the gate and `parlay validate`
// will agree on severity. Note domain-operations-unsupported is an ERROR in
// both modes (the field was removed in v0.3); the contract the Phase-3 gate
// must implement is that an UNCHANGED legacy operations block is grandfathered
// by the monotonic non-worsening comparison, not by a severity downgrade —
// nothing enforces that yet, because no production writer calls this seam.
//
// domainmodel.StdinLabel is passed for the same reason the CLI's --json mode uses
// "<stdin>": a draft in memory has no path, and anchoring both entry points on
// the same token keeps their messages byte-identical. The parity suite compares
// exactly that.
func domainValidator(_ context.Context, draftYAML []byte) []domainmodel.CoreFinding {
	errs := agent.ValidateDomainModelStructuredMode(domainmodel.StdinLabel, draftYAML, agent.ModeAuthoring)
	out := make([]domainmodel.CoreFinding, 0, len(errs))
	for _, e := range errs {
		out = append(out, domainmodel.CoreFinding{
			Code:     e.Code,
			Message:  e.Message,
			Context:  e.Context,
			Fix:      e.Fix,
			Severity: e.Severity,
		})
	}
	return out
}
