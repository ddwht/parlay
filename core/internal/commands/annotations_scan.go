// parlay-feature: annotations
// parlay-component: annotation-collector
//
// The collector: which files a feature exposes to review, and what the scanner
// finds in them. Everything user-facing in this feature — the probe, the
// listing, the reply and clear writers, the readiness codes — reads from here,
// so there is one answer to "what is under review" rather than four.

package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"gopkg.in/yaml.v3"
)

// featureAnnotationFiles are the human-facing files of a feature, in the order
// a reviewer reads them. Deliberately not everything under the feature
// directory: `.parlay/build/` holds tool internals — buildfile.yaml,
// testcases.yaml, the baseline — which are never user-facing and whose
// comments are the tool's own prose.
var featureAnnotationFiles = []string{
	"intents.md",
	"dialogs.md",
	"surface.yaml",
	"capabilities.yaml",
	"infrastructure.md",
	"domain-model.md",
	"domain-model.yaml",
	// authored.yaml is a real artifact — backlog-and-activity has one — and a
	// reviewer reads it like any other. It is project-owned rather than
	// ledgered, so it routes as a direct edit.
	"authored.yaml",
}

// annotationFileScan is one file's threads and findings, with the two facts
// routing needs: whether the file is under the ledger freeze, and whether it is
// an amendment record the baseline has already applied.
type annotationFileScan struct {
	Path     string
	Rel      string
	Feature  string
	Frozen   bool
	Applied  bool
	Threads  []parser.AnnotationThread
	Findings []parser.AnnotationFinding
}

// annotationCounts is the summary every consumer prints.
type annotationCounts struct {
	Open     int `json:"open"`
	Answered int `json:"answered"`
	Closed   int `json:"closed"`
}

func (c annotationCounts) total() int { return c.Open + c.Answered + c.Closed }

// collectFeatureAnnotations scans one feature's human-facing files.
func collectFeatureAnnotations(cfg *config.Context, slug string) ([]annotationFileScan, error) {
	featureDir := cfg.FeaturePath(slug)
	if _, err := os.Stat(featureDir); err != nil {
		return nil, fmt.Errorf("feature %s: %w", slug, err)
	}

	frozen, err := hasBaseline(cfg, slug)
	if err != nil {
		return nil, err
	}
	appliedThrough, appliedHashes, err := appliedAmendmentState(cfg, slug)
	if err != nil {
		return nil, err
	}

	var out []annotationFileScan
	for _, name := range featureAnnotationFiles {
		path := filepath.Join(featureDir, name)
		scan, ok, err := scanAnnotationFile(cfg, slug, path)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		// Only the founding documents are under the freeze. A contract
		// artifact is generated and human-reviewed; editing it is legal and
		// unremarkable, which is exactly why an annotation on one routes to
		// refine rather than to an amendment ceremony.
		scan.Frozen = frozen && (name == "intents.md" || name == "dialogs.md")
		out = append(out, scan)
	}

	// The handoff is derived and stays derived, but a reviewer reads it and
	// will comment on it, so it is collected. §6.2 routes every thread here to
	// the artifact the passage came from, and never to an edit — the next
	// regeneration would erase one.
	handoff, ok, err := scanAnnotationFile(cfg, slug, filepath.Join(cfg.HandoffPath(slug), "specification.md"))
	if err != nil {
		return nil, err
	}
	if ok {
		out = append(out, handoff)
	}

	// A page manifest that genuinely lives in the feature directory. The
	// project's pages live in spec/pages/ and are scanned ONCE, at project
	// level: a page is multi-feature by construction, and scanning it per
	// feature would report the same thread as many times as it has fragments.
	pages, err := featurePageFiles(featureDir)
	if err != nil {
		return nil, err
	}
	for _, path := range pages {
		scan, ok, err := scanAnnotationFile(cfg, slug, path)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, scan)
		}
	}

	records, err := featureAmendmentFiles(featureDir)
	if err != nil {
		return nil, err
	}
	for _, path := range records {
		scan, ok, err := scanAnnotationFile(cfg, slug, path)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		scan.Applied = amendmentIsApplied(path, appliedThrough, appliedHashes)
		out = append(out, scan)
	}

	return out, nil
}

// scanAnnotationFile reads one file. A file that is not there contributes
// nothing — every one of these is optional. Any OTHER read error is returned:
// "I could not read this" is not "there is nothing here", and answering a
// boundary with the second when the first is true advances a build over a file
// nobody could see.
func scanAnnotationFile(cfg *config.Context, slug, path string) (annotationFileScan, bool, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return annotationFileScan{}, false, nil
	}
	if err != nil {
		return annotationFileScan{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	scan := parser.ScanAnnotations(path, content)
	if len(scan.Threads) == 0 && len(scan.Findings) == 0 {
		return annotationFileScan{}, false, nil
	}
	resolveAnnotationRefs(slug, path, content, scan.Threads)
	rel := relativeToRoot(cfg, path)
	for i := range scan.Threads {
		scan.Threads[i].File = rel
	}
	for i := range scan.Findings {
		scan.Findings[i].File = rel
	}
	return annotationFileScan{
		Path:     path,
		Rel:      rel,
		Feature:  slug,
		Threads:  scan.Threads,
		Findings: scan.Findings,
	}, true, nil
}

func featurePageFiles(featureDir string) ([]string, error) {
	entries, err := readDirIfPresent(featureDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".page.md") {
			out = append(out, filepath.Join(featureDir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

// readDirIfPresent treats a missing directory as empty and every other error
// as an error. An ignored ReadDir failure reads as "this feature has no
// amendments", which is the most dangerous sentence in this file.
func readDirIfPresent(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	return entries, nil
}

// featureAmendmentFiles lists the ledger, live records and archived ones both.
// An archived record is still applied history; the scanner refuses annotations
// there for the same reason.
func featureAmendmentFiles(featureDir string) ([]string, error) {
	var out []string
	for _, dir := range []string{
		parser.AmendmentsDir(featureDir),
		filepath.Join(parser.AmendmentsDir(featureDir), "archive"),
	} {
		entries, err := readDirIfPresent(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() && parser.AmendmentFileNameValid(e.Name()) {
				out = append(out, filepath.Join(dir, e.Name()))
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// hasBaseline reports whether a feature has been built. It distinguishes
// ABSENCE from any other stat error: "no baseline" means never built and
// unfreezes the founding documents, so a permissions error answering the same
// way would quietly hand a frozen feature back to direct editing.
func hasBaseline(cfg *config.Context, slug string) (bool, error) {
	_, err := os.Stat(baselinePath(cfg, slug))
	switch {
	case err == nil:
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, fmt.Errorf("read baseline for %s: %w", slug, err)
	}
}

// appliedAmendmentState reads the two facts that say whether a record is
// history: how far applied authority reached, and which record bytes the
// baseline hashed when it did.
//
// It fails CLOSED. Returning a zero marker for an unreadable or malformed
// baseline would classify every applied record as unapplied — mutable in
// place, by §6.2's routing — at exactly the moment authority state is corrupt
// and least trustworthy. A missing baseline is different and is not an error:
// it means the feature was never built, so no record can be applied.
func appliedAmendmentState(cfg *config.Context, slug string) (int, map[string]string, error) {
	data, err := os.ReadFile(baselinePath(cfg, slug))
	if os.IsNotExist(err) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, fmt.Errorf("read baseline for %s: %w", slug, err)
	}
	var baseline Baseline
	if err := yaml.Unmarshal(data, &baseline); err != nil {
		return 0, nil, fmt.Errorf("parse baseline for %s: %w — cannot tell which amendments are applied", slug, err)
	}
	var hashes map[string]string
	if baseline.Sources != nil {
		hashes = baseline.Sources.Amendments
	}
	return baseline.LastAppliedAmendment, hashes, nil
}

// amendmentIsApplied reports whether a record's bytes are under the integrity
// hash. Either fact is enough: the recorded hash names the file directly, and
// the sequence marker covers a record applied before hashing existed.
func amendmentIsApplied(path string, through int, hashes map[string]string) bool {
	name := filepath.Base(path)
	if _, ok := hashes[name]; ok {
		return true
	}
	seq := 0
	if _, err := fmt.Sscanf(name, "%03d-", &seq); err != nil {
		return false
	}
	return seq > 0 && seq <= through
}

// refuseAnnotationsInAppliedRecords converts every thread found in an applied
// amendment into the finding §7 requires.
//
// An applied record's bytes are hashed into HashedSources.Amendments and
// re-checked by check-drift, apply-amendment, compaction and the applied-history
// reader; a moved hash there means "recorded history was edited", which is a
// far more serious claim than "someone left a comment". Canonicalising would
// weaken the one hash whose whole purpose is to notice any byte moving, in five
// readers at once. So the comment is refused instead.
func refuseAnnotationsInAppliedRecords(scans []annotationFileScan) {
	for i := range scans {
		if !scans[i].Applied || len(scans[i].Threads) == 0 {
			continue
		}
		for _, thread := range scans[i].Threads {
			scans[i].Findings = append(scans[i].Findings, parser.AnnotationFinding{
				Code:    parser.AnnotationInAppliedRecord,
				File:    scans[i].Rel,
				Line:    thread.Line,
				Message: "this amendment has been applied; its bytes are under the ledger's integrity hash, where a comment reads as recorded history being edited",
				Fix:     "comment on the contract entry this amendment changed, or open a superseding amendment through /parlay-refine",
			})
		}
		scans[i].Threads = nil
	}
}

func countAnnotations(scans []annotationFileScan) annotationCounts {
	var counts annotationCounts
	for _, scan := range scans {
		for _, thread := range scan.Threads {
			switch thread.State {
			case parser.AnnotationOpen:
				counts.Open++
			case parser.AnnotationAnswered:
				counts.Answered++
			case parser.AnnotationClosed:
				counts.Closed++
			}
		}
	}
	return counts
}

// projectAnnotationFiles are the files a --all scan adds: the ones that belong
// to the project rather than to any feature. They carry no ref — nothing in
// the `affects:` vocabulary names them — so a thread here keeps its generic
// identity, which is the whole reason the generic identity exists.
func collectProjectAnnotations(cfg *config.Context) ([]annotationFileScan, error) {
	paths := []string{
		cfg.DomainModelPath(),
		cfg.BlueprintPath(),
		cfg.AdapterSetPath(),
	}
	// Pages are project-owned and multi-feature, so they belong here and are
	// scanned exactly once.
	pageEntries, err := readDirIfPresent(cfg.PagesPath())
	if err != nil {
		return nil, err
	}
	for _, e := range pageEntries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".page.md") {
			paths = append(paths, filepath.Join(cfg.PagesPath(), e.Name()))
		}
	}
	adapterEntries, err := readDirIfPresent(cfg.AdaptersPath())
	if err != nil {
		return nil, err
	}
	for _, e := range adapterEntries {
		if !e.IsDir() && parser.AnnotationHostFor(e.Name()) != "" {
			paths = append(paths, filepath.Join(cfg.AdaptersPath(), e.Name()))
		}
	}

	var out []annotationFileScan
	for _, path := range paths {
		scan, ok, err := scanAnnotationFile(cfg, "", path)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, scan)
		}
	}
	return out, nil
}

// relativeToRoot renders a path the way a reviewer would type it. A path
// outside the root keeps its absolute form: "../../../../../../tmp/x.yaml" is
// a worse address than the absolute one it was derived from.
func relativeToRoot(cfg *config.Context, path string) string {
	rel, err := filepath.Rel(cfg.RepoRoot(), path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}

// annotationPathIsAppliedRecord resolves one file's governance from its path
// alone, for the commands that take a path rather than a feature.
//
// `annotations reply <file>:<line>` and `annotations clear --file` both bypass
// collection, so neither ever consulted the routing that collection performs.
// Without this, the sweep could delete a closed thread out of an APPLIED
// amendment — whose bytes are hashed into HashedSources.Amendments and
// re-checked by check-drift, apply-amendment, compaction and the applied-history
// reader — and the next check would report recorded history as edited, against
// a record nobody had touched. That is the sweep forging a ledger-integrity
// violation, which is worse than any thread it was removing.
func annotationPathIsAppliedRecord(cfg *config.Context, path string) (bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	if !isAmendmentPath(abs) {
		return false, nil
	}

	// amendments/NNN-slug.md, or amendments/archive/NNN-slug.md.
	featureDir := filepath.Dir(filepath.Dir(abs))
	if filepath.Base(featureDir) == "amendments" {
		featureDir = filepath.Dir(featureDir)
	}
	slug, err := filepath.Rel(cfg.IntentsRoot(), featureDir)
	if err != nil || strings.HasPrefix(slug, "..") {
		// A record outside this root's intents tree. Refuse rather than guess:
		// an unresolvable governance answer is not "ungoverned".
		return false, fmt.Errorf("%s is an amendment record outside %s; its applied state cannot be resolved", path, cfg.IntentsRoot())
	}

	through, hashes, err := appliedAmendmentState(cfg, filepath.ToSlash(slug))
	if err != nil {
		return false, err
	}
	return amendmentIsApplied(abs, through, hashes), nil
}

// refuseWriteToAppliedRecord is the guard both path-taking commands share.
func refuseWriteToAppliedRecord(cfg *config.Context, path string) error {
	applied, err := annotationPathIsAppliedRecord(cfg, path)
	if err != nil {
		return err
	}
	if applied {
		return fmt.Errorf("%s: %s — this amendment has been applied and its bytes are under the ledger's integrity hash; comment on the contract entry it changed, or open a superseding amendment through /parlay-refine",
			path, parser.AnnotationInAppliedRecord)
	}
	return nil
}
