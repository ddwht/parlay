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

// domainValidator is the ValidatorFunc the domain-model write path runs:
// Core's domain-model rules, called in-process, in authoring mode.
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
// Authoring mode is load-bearing, not a default. It is what keeps
// domain-operations-deprecated a warning instead of an error, and therefore what
// lets a model carrying a deprecated operations block still be saved through
// the write path. Build mode would promote it and the save gate would refuse
// drafts that `parlay validate` accepts.
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
