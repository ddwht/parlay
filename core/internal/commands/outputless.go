package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// The output-less blessing path (WP4).
//
// A spec-only feature — one whose amendments change contract artifacts but
// which emits no generated code — could be created and baselined by a full
// save but never re-blessed by a partial one, because a partial save's ONLY
// proof of feature membership is generated-file provenance. It resolved to no
// files, so it entered no blessing set, so its amendment stayed pending
// forever and every advancing boundary blocked on it.
//
// The fix is a non-code proof admitted only where no code can exist. It is
// deliberately narrow:
//
//   - It never touches emittedFeatures. That set is provenance and reporting,
//     and synthesising an emission for a feature that emitted nothing would be
//     a lie told to every downstream consumer of it. Only the blessing set
//     gains the slug.
//   - It cannot downgrade a governance or combined record. `--confirm-outputless`
//     confirms exactly one thing — "this feature owes no generated code" — and
//     that is not confirmation of a promise list. A combined record stays
//     refused however perfect the empty manifest and the journal are.
//   - Its confirmation is user authority. The zero value refuses; nothing
//     defaults it true.

// outputlessClaim is a caller's assertion that one named feature owes no
// generated code on this run.
type outputlessClaim struct {
	// Feature is the slug the caller named. Empty means no claim.
	Feature string
	// Confirmed is the human's assertion. The zero value is a refusal: this
	// is the one judgement no check can make, so it is never inferred.
	Confirmed bool
}

func (c outputlessClaim) made() bool { return c.Feature != "" }

// validateOutputlessClaim runs every mechanical precondition.
//
// All of it happens before any write, and all of it is necessary but not
// sufficient — the human assertion is the part no check can supply.
func validateOutputlessClaim(cfg *config.Context, c outputlessClaim, features []string, partial bool, emitted *emissionDeclaration) error {
	if !c.made() {
		if c.Confirmed {
			return fmt.Errorf("--confirm-outputless was given with no --outputless-feature: a " +
				"confirmation with no named subject confirms nothing, and accepting it as a " +
				"silent no-op would train the habit of passing it")
		}
		return nil
	}
	if !c.Confirmed {
		return fmt.Errorf("--outputless-feature %s requires --confirm-outputless: whether a "+
			"feature owes generated code is a judgement no check can make, and blessing one that "+
			"silently emitted nothing would record output as reviewed that nobody wrote", c.Feature)
	}
	if !partial {
		return fmt.Errorf("--outputless-feature is a --partial concept: a full save already " +
			"blesses every feature, so naming one here would claim a scope that does not exist")
	}
	known := false
	for _, slug := range features {
		if slug == c.Feature {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("--outputless-feature %s names no feature in this project", c.Feature)
	}
	// An explicitly present but EMPTY manifest. A missing manifest is a
	// different claim — "I do not know what this run wrote" — and must not be
	// spelled the same way as "this run wrote nothing".
	if emitted == nil {
		return fmt.Errorf("--outputless-feature %s requires an explicitly present emission "+
			"manifest that is empty; a missing manifest says nothing was recorded, not that "+
			"nothing was written", c.Feature)
	}
	if n := len(emitted.Paths); n != 0 {
		return fmt.Errorf("--outputless-feature %s was named, but the emission manifest lists "+
			"%d file(s) — a run that wrote files is not output-less", c.Feature, n)
	}
	// Necessary but NOT sufficient. This reads the PRIOR snapshot, so it
	// returns empty for a feature whose output is introduced by this very
	// amendment and for one whose codegen silently failed. It proves previous
	// output vanished, never that no output was owed — which is why the plan
	// inventory below and the human assertion above both exist.
	owned, failures := generatedFilesOwnedBy(cfg, c.Feature)
	if len(failures) > 0 {
		return fmt.Errorf("--outputless-feature %s: its generated output cannot be established "+
			"(%s) — an output-less claim is not safe on an unreadable answer", c.Feature, failures[0])
	}
	if len(owned) > 0 {
		return fmt.Errorf("--outputless-feature %s owns %d tracked generated file(s) (%s) — it "+
			"is not output-less, so bless it through the ordinary manifest",
			c.Feature, len(owned), joinNames(firstN(owned, 3)))
	}
	// The obligation check. If the feature's plan predicts files, an empty
	// emission means codegen did not run or did not write — the exact case a
	// blessing must refuse rather than record.
	predicted, known, err := plannedOutputCount(cfg, c.Feature)
	if err != nil {
		return fmt.Errorf("--outputless-feature %s: its buildfile cannot be read (%v), so no "+
			"output obligation can be established", c.Feature, err)
	}
	if known && predicted > 0 {
		return fmt.Errorf("--outputless-feature %s: its buildfile plans %d file(s), so this "+
			"feature owes generated code. An empty emission here means codegen did not write "+
			"them, not that none were owed", c.Feature, predicted)
	}
	return nil
}

// plannedOutputCount reports how many files a feature's buildfile predicts.
//
// The second return says whether an answer exists at all: a feature with no
// buildfile has no mechanical inventory, which is not the same as an inventory
// saying zero. That distinction is why the confirmation is recorded durably
// beside the baseline it justified rather than living only in a flag.
func plannedOutputCount(cfg *config.Context, slug string) (int, bool, error) {
	path := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var bf planBuildfile
	if err := yaml.Unmarshal(data, &bf); err != nil {
		return 0, false, err
	}
	return len(bf.Plan.Creates) + len(bf.Plan.Modifies), true, nil
}

// outputlessTailProven reports whether the journal proves this output-less
// feature's tail, reusing the ordinary partial-path proof.
//
// Deliberately the SAME evidence a code-emitting refine must produce. The
// output-less path relaxes what counts as membership, never what counts as a
// completed refinement.
func outputlessTailProven(cfg *config.Context, slug string, pending []parser.Amendment) []string {
	return proveTailJournal(cfg, slug, pending)
}

// outputlessRecord renders the claim for the project baseline's durable
// record. Empty when no claim was made, so the key drops from the file.
func outputlessRecord(c outputlessClaim) []string {
	if !c.made() {
		return nil
	}
	return []string{c.Feature}
}
