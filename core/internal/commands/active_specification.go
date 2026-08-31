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
	"fmt"
	"os"
	"path/filepath"
	"sort"

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

	// CURRENT-STATE RESOLUTION FAILS CLOSED. Every record that determines what
	// is in force must be trusted applied evidence, and if that cannot be
	// established the answer is an error rather than an older answer.
	//
	// This used to be fail-soft, and the comment justifying it said resolving
	// to "everything active" keeps every promise in force rather than retiring
	// one on the strength of a file we could not read. That was conservative
	// when the only semantic operation was ENDING a lineage: the worst outcome
	// was keeping a promise that should have gone. The revision vocabulary
	// invalidated it. Falling back to the founding intents now silently rolls
	// applied promise TEXT backward, which is not a cautious answer — it is a
	// confident wrong one, and the caller cannot tell.
	//
	// Location does not imply trust either. An earlier version of this fed
	// every well-formed file in amendments/archive/ straight to the resolver,
	// so a hand-written record could become current truth by choosing a
	// sequence at or below the marker. The rule here is the one the rest of the
	// authority layer already uses: at or below the marker, plus an exact
	// filename hash in the capsule, plus retained bytes that match it.
	snap, err := acquireAppliedLedger(cfg, slug, featDir)
	if err != nil {
		return agent.IntentResolution{}, err
	}
	return resolveIntentsFrom(snap, intents, mode), nil
}

// resolveActiveIntents is what every ordinary consumer calls: the promises in
// force right now, with the unapplied tail deliberately not counted.
func resolveActiveIntents(cfg *config.Context, slug string) (agent.IntentResolution, error) {
	return resolveIntents(cfg, slug, agent.AppliedAuthority)
}

// appliedLedgerSnapshot is ONE coherent view of a feature's applied history.
//
// One acquisition, consumed by everything that derives from it. Before this,
// resolution observed the capsule strictly, then re-read the marker through the
// fail-soft reader, and provenance read it a third time — so a concurrent
// writer could produce a view whose marker, promise text, provenance and
// evidence never coexisted, and a transient failure on the second read could
// roll an already-authenticated ledger back to marker 0. A snapshot cannot be
// half of two states.
type appliedLedgerSnapshot struct {
	// Through is the marker this whole snapshot was authenticated against.
	Through int
	Capsule appliedAuthority
	// Records are parsed from the EXACT bytes that were authenticated.
	Records []parser.Amendment
}

// appliedLedgerReadHook fires immediately after a record's bytes are read, so a
// test can mutate the file at the one instant that used to matter. Nil outside
// tests.
var appliedLedgerReadHook func(name string)

// appliedLedgerCapsuleHook fires between the opening and closing observations of
// the authority capsule, so a test can advance authority mid-acquisition.
var appliedLedgerCapsuleHook func(slug string)

// acquireAppliedLedger reads, authenticates and parses a feature's applied
// history as one coherent snapshot.
//
// Three populations, and only one of them needs authority:
//
//   - AT OR BELOW the capsule marker, a record is applied history. It must be
//     trusted: its exact filename recorded in the capsule, and the bytes still
//     retained — in amendments/ or, after compaction, amendments/archive/ —
//     hashing to what was recorded. Anything else is refused outright rather
//     than skipped, because skipping it resolves to the text that preceded it,
//     which is a silent rollback wearing the costume of caution.
//   - ABOVE the marker, a record is pending. It has no applied evidence yet by
//     definition and needs none.
//   - RECORDED in the capsule but present in neither location. Its effect
//     cannot be reconstructed, so its absence is refused too — an erased applied
//     decision must not read as one that never happened.
//
// EACH RECORD IS READ EXACTLY ONCE, and the bytes that are hashed are the bytes
// that are parsed. Reading a path to parse it and reading it again to hash it
// authenticates one thing and interprets another; between those two reads a
// file can be swapped, so forged content gets parsed while genuine content gets
// hashed and the forgery comes out authenticated. Append-only policy does not
// close that: this boundary exists to detect policy violations, and recovery
// and concurrent filesystem activity are part of the model.
//
// Active/archive precedence is resolved BEFORE the read, for the same reason —
// choosing the location after hashing would reintroduce the gap. Active bytes
// win, so altering the active copy destroys trust rather than letting a pristine
// archived copy vouch for it.
//
// The capsule is observed at both ends. If authority moved during the
// acquisition, the result would be a mixture of two states, so it is refused
// rather than returned.
func acquireAppliedLedger(cfg *config.Context, slug, featDir string) (appliedLedgerSnapshot, error) {
	capsule, cerr := observeAppliedAuthority(cfg, slug)
	if cerr != nil {
		return appliedLedgerSnapshot{}, fmt.Errorf("the applied authority for %s cannot be read "+
			"(%w), so which decisions are in force cannot be established", slug, cerr)
	}

	activeDir := filepath.Join(featDir, "amendments")
	archiveDir := filepath.Join(activeDir, "archive")
	chosen, lerr := chooseRecordPaths(activeDir, archiveDir, slug)
	if lerr != nil {
		return appliedLedgerSnapshot{}, lerr
	}

	if appliedLedgerCapsuleHook != nil {
		appliedLedgerCapsuleHook(slug)
	}

	present := map[string]bool{}
	out := make([]parser.Amendment, 0, len(chosen))
	for _, name := range sortedRecordNames(chosen) {
		path := chosen[name]
		content, rerr := os.ReadFile(path)
		if rerr != nil {
			return appliedLedgerSnapshot{}, fmt.Errorf("%s could not be read for %s (%w), so what "+
				"it decided cannot be established", name, slug, rerr)
		}
		if appliedLedgerReadHook != nil {
			appliedLedgerReadHook(name)
		}
		rec, perr := parser.ParseAmendmentRecord(path, content)
		if perr != nil {
			return appliedLedgerSnapshot{}, fmt.Errorf("the amendment ledger for %s cannot be "+
				"read (%w), so what it currently promises cannot be established", slug, perr)
		}
		present[name] = true

		if rec.Seq > capsule.Through {
			out = append(out, *rec) // pending: no applied evidence is owed
			continue
		}
		stored, recorded := capsule.Hashes[name]
		if !recorded {
			return appliedLedgerSnapshot{}, fmt.Errorf("%s sits at or below %s's applied marker "+
				"but the baseline records no evidence that it was ever applied. Resolving without "+
				"it would answer with the text that preceded it, so nothing is answered",
				name, slug)
		}
		// Over the bytes just parsed, never over a second read of the path.
		if sha256Hex(string(content)) != stored {
			return appliedLedgerSnapshot{}, fmt.Errorf("%s is recorded applied for %s but its "+
				"bytes are not the ones that were applied. What is in force cannot be established "+
				"from a record that has changed since", name, slug)
		}
		out = append(out, *rec)
	}
	for name := range capsule.Hashes {
		if present[name] {
			continue
		}
		return appliedLedgerSnapshot{}, fmt.Errorf("%s's baseline records %s as applied, but no "+
			"such record exists in amendments/ or amendments/archive/. An erased decision must "+
			"not read as one that never happened", slug, name)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })

	// Authority must not have moved while all of that was read.
	after, aerr := observeAppliedAuthority(cfg, slug)
	if aerr != nil {
		return appliedLedgerSnapshot{}, fmt.Errorf("the applied authority for %s could not be "+
			"re-read after the ledger (%w), so it cannot be confirmed that one state was "+
			"observed", slug, aerr)
	}
	if !sameAuthority(capsule, after) {
		return appliedLedgerSnapshot{}, fmt.Errorf("%s's applied authority changed while its "+
			"ledger was being read, so what came back would be part of one state and part of "+
			"another. Nothing is answered — run it again", slug)
	}
	return appliedLedgerSnapshot{Through: capsule.Through, Capsule: capsule, Records: out}, nil
}

// chooseRecordPaths resolves active/archive precedence BEFORE anything is read.
func chooseRecordPaths(activeDir, archiveDir, slug string) (map[string]string, error) {
	chosen := map[string]string{}
	collect := func(dir string, required bool) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) && !required {
				return nil
			}
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("%s cannot be read for %s (%w), so what this feature currently "+
				"promises cannot be established", dir, slug, err)
		}
		for _, e := range entries {
			if e.IsDir() || !parser.AmendmentFileNameValid(e.Name()) {
				continue
			}
			if _, taken := chosen[e.Name()]; taken {
				continue // active already claimed this name
			}
			chosen[e.Name()] = filepath.Join(dir, e.Name())
		}
		return nil
	}
	if err := collect(activeDir, true); err != nil {
		return nil, err
	}
	if err := collect(archiveDir, false); err != nil {
		return nil, err
	}
	return chosen, nil
}

func sortedRecordNames(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// resolveIntentsFrom answers from ONE snapshot.
//
// The marker comes from the snapshot, never from a second read. Re-reading it
// through the fail-soft reader meant an already-authenticated ledger could be
// interpreted against a different marker — or against 0 on a transient failure,
// which reads every applied decision as pending and rolls the promise text back
// to its founding version.
func resolveIntentsFrom(snap appliedLedgerSnapshot, intents []parser.Intent, mode agent.IntentAuthority) agent.IntentResolution {
	return agent.ResolveIntentAuthority(intents, snap.Records, snap.Through, mode)
}

// intentProvenance maps each lineage to the decision that last changed it and
// the mode it was changed under, from the trusted applied ledger.
//
// Shared, because two callers need exactly this and a second copy would drift:
// the spec view reports it to a person, and the authority projection protects
// it across compaction. A projection that guarded the promise TEXT but not the
// record behind it would let compaction change what `parlay spec` says decided
// a promise while passing its own equivalence check.
func intentProvenanceFrom(snap appliedLedgerSnapshot) (map[string]string, map[string]string) {
	version, mode := map[string]string{}, map[string]string{}
	for _, a := range snap.Records {
		if a.Seq > snap.Through {
			continue
		}
		for _, tr := range a.IntentTransitions() {
			if tr.Mode.EndsLineage() {
				continue
			}
			version[tr.Intent] = fmt.Sprintf("%03d-%s", a.Seq, a.FileSlug)
			mode[tr.Intent] = string(tr.Mode)
		}
	}
	return version, mode
}
