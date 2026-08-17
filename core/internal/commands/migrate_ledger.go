// parlay-feature: parlay-tool/ledger-and-contract
// parlay-component: ledger-migration
//
// Carries a pre-v0.4 project into the single ledger regime. Freezing is
// implicit — a founding doc is frozen at the moment its feature's baseline
// is written — so an old project needs repair in exactly one state: a
// feature whose intents.md/dialogs.md were edited after its last green
// build. Under the old regime that was pending drift ("rebuild me"); under
// the new regime it reads as a ledger_integrity violation for an edit that
// was legal when it was made. This migrator dissolves that state by
// accepting the current text as the founding state.
//
// Design point — freeze-stamp must not bless the build. save-build-state
// stamps the whole feature, spec AND build hashes; using it here would mark
// an un-rebuilt build state green (the WP6 false-stable bug as a migration
// step). The re-stamp below rewrites ONLY the spec-side founding hashes —
// baseline Intents, Sources.Intents, Sources.Dialogs — and leaves every
// build-side hash untouched, so real spec→build staleness keeps reporting.

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var migrateLedgerCmd = &cobra.Command{
	Use:   "migrate-ledger",
	Short: "Freeze drifted founding docs at their current text so the ledger starts clean",
	Args:  cobra.NoArgs,
	RunE:  runMigrateLedger,
}

var migrateLedgerDryRun bool

func init() {
	migrateLedgerCmd.Flags().BoolVar(&migrateLedgerDryRun, "dry-run", false,
		"Print the per-feature verdicts without writing anything")
}

// ledgerMigrationState classifies one feature's migration verdict.
type ledgerMigrationState string

const (
	// ledgerClean — founding docs match the baseline; nothing to do.
	ledgerClean ledgerMigrationState = "clean"
	// ledgerNoBaseline — never built; freezes normally at first green build.
	ledgerNoBaseline ledgerMigrationState = "no-baseline"
	// ledgerNeedsFreeze — founding docs drifted with no amendments; the
	// migrator re-stamps the spec-side founding hashes.
	ledgerNeedsFreeze ledgerMigrationState = "needs-freeze"
	// ledgerRefuseAmendments — founding docs drifted AND amendments exist.
	// That project was already ledger-mode; a drifted frozen doc there is a
	// real integrity violation the migrator must not paper over.
	ledgerRefuseAmendments ledgerMigrationState = "refuse-amendments"
	// ledgerRefuseSurfaceMD — the feature still carries a surface.md;
	// `parlay migrate-spec --retire-md` first.
	ledgerRefuseSurfaceMD ledgerMigrationState = "refuse-surface-md"
	// ledgerError — the feature's state could not be read.
	ledgerError ledgerMigrationState = "error"
)

// ledgerMigrationVerdict is one feature's scan result.
type ledgerMigrationVerdict struct {
	Feature string
	State   ledgerMigrationState
	// Detail lists, per founding file, exactly which slugs drifted — what
	// the operator is grandfathering into the founding state.
	Detail []string
	Err    string
}

// scanLedgerMigration computes the per-feature verdicts for one root.
// Path-based rather than Context-based so `parlay upgrade` can scan the
// repo root and each registered child root with the same code.
func scanLedgerMigration(rootPath string) ([]ledgerMigrationVerdict, error) {
	intentsRoot := filepath.Join(rootPath, config.SpecDir, config.IntentsDir)
	if _, err := os.Stat(intentsRoot); os.IsNotExist(err) {
		// A root with no spec/intents/ — a bare multi-root parent whose
		// features all live in child roots — has nothing to migrate.
		return nil, nil
	}
	features, err := config.ScanFeatureTree(intentsRoot)
	if err != nil {
		return nil, fmt.Errorf("scan feature tree: %w", err)
	}
	var verdicts []ledgerMigrationVerdict
	for _, feature := range features {
		verdicts = append(verdicts, scanFeatureLedgerMigration(rootPath, feature))
	}
	return verdicts, nil
}

func scanFeatureLedgerMigration(rootPath, feature string) ledgerMigrationVerdict {
	v := ledgerMigrationVerdict{Feature: feature}
	featureDir := filepath.Join(rootPath, config.SpecDir, config.IntentsDir, filepath.FromSlash(feature))
	blPath := filepath.Join(rootPath, config.ParlayDir, config.BuildDir, filepath.FromSlash(feature), ".baseline.yaml")

	// A leftover surface.md refuses regardless of drift: the single regime
	// has no runtime surface.md, and freezing around a stale prose artifact
	// would grandfather exactly the misleading document the ledger model
	// exists to prevent.
	if _, err := os.Stat(filepath.Join(featureDir, "surface.md")); err == nil {
		v.State = ledgerRefuseSurfaceMD
		return v
	}

	blData, err := os.ReadFile(blPath)
	if err != nil {
		if os.IsNotExist(err) {
			v.State = ledgerNoBaseline
			return v
		}
		v.State, v.Err = ledgerError, fmt.Sprintf("read baseline: %v", err)
		return v
	}
	var baseline Baseline
	if err := yaml.Unmarshal(blData, &baseline); err != nil {
		v.State, v.Err = ledgerError, fmt.Sprintf("invalid baseline: %v", err)
		return v
	}

	detail, err := foundingDocDelta(&baseline, featureDir)
	if err != nil {
		v.State, v.Err = ledgerError, err.Error()
		return v
	}
	if len(detail) == 0 {
		v.State = ledgerClean
		return v
	}
	v.Detail = detail

	amendments, err := parser.LoadFeatureAmendments(featureDir)
	if err != nil {
		v.State, v.Err = ledgerError, fmt.Sprintf("read amendments: %v", err)
		return v
	}
	if len(amendments) > 0 {
		v.State = ledgerRefuseAmendments
		return v
	}
	v.State = ledgerNeedsFreeze
	return v
}

// foundingDocDelta lists the founding-doc changes since the baseline, per
// file and slug. It mirrors detectDrift's comparison exactly — same per-slug
// hashes, same stored-dialogs-only rule — so the migrator and check-drift
// always agree on what "clean" means.
func foundingDocDelta(baseline *Baseline, featureDir string) ([]string, error) {
	var detail []string

	intents, err := parser.ParseIntentsFile(filepath.Join(featureDir, "intents.md"))
	if err != nil {
		return nil, fmt.Errorf("read intents: %w", err)
	}
	currentSlugs := make(map[string]bool)
	for _, intent := range intents {
		currentSlugs[intent.Slug] = true
		oldHash, exists := baseline.Intents[intent.Slug]
		if !exists {
			detail = append(detail, "intents.md: \""+intent.Slug+"\" added")
			continue
		}
		if changed := diffHashes(oldHash, hashIntent(intent)); len(changed) > 0 {
			detail = append(detail, "intents.md: \""+intent.Slug+"\" changed")
		}
	}
	for slug := range baseline.Intents {
		if !currentSlugs[slug] {
			detail = append(detail, "intents.md: \""+slug+"\" removed")
		}
	}

	if baseline.Sources != nil && len(baseline.Sources.Dialogs) > 0 {
		current := map[string]string{}
		if dialogs, err := parser.ParseDialogsFile(filepath.Join(featureDir, "dialogs.md")); err == nil {
			for _, d := range dialogs {
				current[d.Slug] = hashDialogContent(d)
			}
		}
		for slug, stored := range baseline.Sources.Dialogs {
			cur, present := current[slug]
			switch {
			case !present:
				detail = append(detail, "dialogs.md: \""+slug+"\" removed")
			case cur != stored:
				detail = append(detail, "dialogs.md: \""+slug+"\" changed")
			}
		}
		for slug := range current {
			if _, was := baseline.Sources.Dialogs[slug]; !was {
				detail = append(detail, "dialogs.md: \""+slug+"\" added")
			}
		}
	}

	sort.Strings(detail)
	return detail, nil
}

// restampFoundingHashes rewrites ONLY the spec-side founding hashes of a
// feature's baseline to match the current founding docs: Intents,
// Sources.Intents, Sources.Dialogs. Every other field — build-side hashes,
// buildfile sections, amendment records, last-applied-amendment, the
// generated-at stamp — passes through untouched, so any real spec→build
// staleness keeps reporting. This is deliberately NOT saveBuildState.
func restampFoundingHashes(rootPath, feature string) error {
	featureDir := filepath.Join(rootPath, config.SpecDir, config.IntentsDir, filepath.FromSlash(feature))
	blPath := filepath.Join(rootPath, config.ParlayDir, config.BuildDir, filepath.FromSlash(feature), ".baseline.yaml")

	blData, err := os.ReadFile(blPath)
	if err != nil {
		return fmt.Errorf("read baseline: %w", err)
	}
	var baseline Baseline
	if err := yaml.Unmarshal(blData, &baseline); err != nil {
		return fmt.Errorf("invalid baseline: %w", err)
	}

	intents, err := parser.ParseIntentsFile(filepath.Join(featureDir, "intents.md"))
	if err != nil {
		return fmt.Errorf("read intents: %w", err)
	}
	if len(intents) == 0 {
		return fmt.Errorf("intents.md exists but has no intent blocks — nothing to freeze")
	}
	baseline.Intents = make(map[string]IntentHash)
	if baseline.Sources == nil {
		baseline.Sources = &HashedSources{}
	}
	baseline.Sources.Intents = make(map[string]string)
	for _, intent := range intents {
		baseline.Intents[intent.Slug] = hashIntent(intent)
		baseline.Sources.Intents[intent.Slug] = hashIntentContent(intent)
	}

	// Same rule as buildBaseline: zero parsed dialogs contributes no dialog
	// hashes — which here means a dialogs.md deleted or emptied since the
	// old freeze is accepted into the founding state.
	baseline.Sources.Dialogs = nil
	if dialogs, err := parser.ParseDialogsFile(filepath.Join(featureDir, "dialogs.md")); err == nil && len(dialogs) > 0 {
		baseline.Sources.Dialogs = make(map[string]string)
		for _, d := range dialogs {
			baseline.Sources.Dialogs[d.Slug] = hashDialogContent(d)
		}
	}

	out, err := marshalBaseline(&baseline)
	if err != nil {
		return err
	}
	return os.WriteFile(blPath, out, 0o644)
}

func runMigrateLedger(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	rootPath := cfg.Root.Path

	verdicts, err := scanLedgerMigration(rootPath)
	if err != nil {
		return err
	}

	var toFreeze []ledgerMigrationVerdict
	var refusals, errored []ledgerMigrationVerdict
	clean, unbuilt := 0, 0
	for _, v := range verdicts {
		switch v.State {
		case ledgerClean:
			clean++
		case ledgerNoBaseline:
			unbuilt++
		case ledgerNeedsFreeze:
			toFreeze = append(toFreeze, v)
		case ledgerRefuseAmendments, ledgerRefuseSurfaceMD:
			refusals = append(refusals, v)
		case ledgerError:
			errored = append(errored, v)
		}
	}

	out := cmd.OutOrStdout()
	for _, v := range toFreeze {
		fmt.Fprintf(out, "  %s — founding docs drifted since the last green build; will freeze current text:\n", v.Feature)
		for _, d := range v.Detail {
			fmt.Fprintf(out, "      %s\n", d)
		}
	}
	for _, v := range refusals {
		switch v.State {
		case ledgerRefuseSurfaceMD:
			fmt.Fprintf(out, "  %s — REFUSED: surface.md is still present; run `parlay migrate-spec --retire-md` first\n", v.Feature)
		case ledgerRefuseAmendments:
			fmt.Fprintf(out, "  %s — REFUSED: founding docs drifted but the feature already has amendments — this project was already ledger-mode, so the drift is a real integrity violation; restore the founding text or record the change as an amendment (/parlay-refine)\n", v.Feature)
		}
	}
	for _, v := range errored {
		fmt.Fprintf(out, "  %s — ERROR: %s\n", v.Feature, v.Err)
	}

	if migrateLedgerDryRun {
		fmt.Fprintf(out, "Dry run: %d to freeze, %d refused, %d error(s), %d clean, %d not yet built — nothing written\n",
			len(toFreeze), len(refusals), len(errored), clean, unbuilt)
		return nil
	}

	// Refusals and read errors block the whole run before any write: a
	// half-migrated project reporting success is how a real integrity
	// violation gets papered over.
	if len(refusals) > 0 || len(errored) > 0 {
		return fmt.Errorf("migration refused: %d feature(s) need attention first (see above); nothing was written", len(refusals)+len(errored))
	}

	if len(toFreeze) == 0 {
		fmt.Fprintf(out, "Nothing to migrate: %d clean, %d not yet built\n", clean, unbuilt)
		return nil
	}

	for _, v := range toFreeze {
		if err := restampFoundingHashes(rootPath, v.Feature); err != nil {
			return fmt.Errorf("freeze %s: %w", v.Feature, err)
		}
		fmt.Fprintf(out, "  %s — froze current founding text (%d change(s) grandfathered)\n", v.Feature, len(v.Detail))
	}
	fmt.Fprintf(out, "Migrated: %d feature(s) frozen at current text, %d already clean, %d not yet built\n",
		len(toFreeze), clean, unbuilt)
	fmt.Fprintln(out, "The ledger starts clean from today — from here, change goes through amendments (/parlay-refine).")
	return nil
}
