// parlay-feature: parlay-tool/intent-supersession
// parlay-component: active-specification-resolver
//
// The I/O half of intent supersession: read the three facts the resolver needs
// and hand them to agent.ResolveIntentAuthority, which owns the rule.
//
// The split is deliberate. The semantic question — which promises stand — is
// validation semantics and belongs beside the other validators, where it can be
// tested without a project on disk. Only the reading of intents, ledger and
// baseline needs config and the filesystem, and that is all that lives here.

package commands

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// lastAppliedAmendment reads only how far the ledger has been applied.
//
// A missing or pre-v3 baseline reads as 0, so every amendment counts as
// unapplied. That is the conservative reading and the one check-amendments
// already takes: it keeps a promise in force rather than retiring it on the
// strength of a build that may never have happened.
func lastAppliedAmendment(cfg *config.Context, slug string) int {
	data, err := os.ReadFile(baselinePath(cfg, slug))
	if err != nil {
		return 0
	}
	var baseline Baseline
	if yaml.Unmarshal(data, &baseline) != nil {
		return 0
	}
	return baseline.LastAppliedAmendment
}

// resolveIntents answers what a feature currently promises.
//
// Ordinary callers use resolveActiveIntents below. This form takes the
// authority mode explicitly and exists for the apply workflow, which is the
// only caller entitled to see the unapplied tail as though it were in force.
func resolveIntents(cfg *config.Context, slug string, mode agent.IntentAuthority) (agent.IntentResolution, error) {
	featDir := cfg.FeaturePath(slug)

	intents, err := parser.ParseIntentsFile(filepath.Join(featDir, "intents.md"))
	if err != nil {
		return agent.IntentResolution{}, err
	}

	// An unreadable ledger is check-amendments' finding to report, not this
	// resolver's to fail on. Resolving to "everything active" keeps every
	// promise in force rather than silently retiring one on the strength of a
	// file we could not read.
	amendments, err := parser.LoadFeatureAmendments(featDir)
	if err != nil {
		return agent.IntentResolution{Active: intents}, nil
	}

	return agent.ResolveIntentAuthority(intents, amendments, lastAppliedAmendment(cfg, slug), mode), nil
}

// resolveActiveIntents is what every ordinary consumer calls: the promises in
// force right now, with the unapplied tail deliberately not counted.
func resolveActiveIntents(cfg *config.Context, slug string) (agent.IntentResolution, error) {
	return resolveIntents(cfg, slug, agent.AppliedAuthority)
}
