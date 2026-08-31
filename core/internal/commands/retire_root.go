// parlay-feature: parlay-tool/root-retirement
// parlay-component: cross-cutting/retirement-target-and-destination-preconditions
// parlay-extends: parlay-tool/root-retirement/cross-cutting/retirement-authorization-preview-unattended
//
// `parlay retire-root <name>` — end a subproject without pretending it
// never happened: preserve the complete child root verifiably under
// <parent>/.parlay/retired/<name>/, record what became of every feature
// in it, and deregister it from the parent's roots index, in that order.
//
// An in-flight retirement is looked for first of all, by scanning the
// journal location rather than by resolving a registration that a
// part-finished run may already have removed (see FindRetirementJournal).
// Only when nothing is in flight does a fresh run begin.
//
// For a fresh run the preconditions come first, and both are settled
// before any enumeration, sweep, or read of the root's contents:
//
//   - Target resolution against the parent's roots index: exactly one
//     registered child proceeds; zero matches refuses enumerating the
//     registered roots; more than one candidate refuses without
//     selecting by ordering or proximity; a directory that carries
//     .parlay root configuration but is not in the index refuses with
//     that stated as the reason; and the parent root itself is never a
//     valid target. Resolution is not authorization: the registered path
//     must also resolve strictly inside the project, since the archive
//     reads that directory and the final step deletes it, and being named
//     in an editable list is not a licence to act on a location.
//   - Destination absence: an existing <parent>/.parlay/retired/<name>/
//     refuses the run naming both possible explanations (an earlier
//     retirement of the same root, or unrelated content under the same
//     name) as the operator's call.
//
// Authorization is the operation's own. --preview runs the entire
// preflight and writes nothing, reserves nothing, leaves no state
// behind. Execution requires explicit authorization from a person after
// that preview has been shown — a mandatory Y/N confirmation
// (invoke-destructive per the adapter, never skipped) naming what will
// be preserved, where, and what will be deregistered. A run that cannot
// ask — no TTY, or --non-interactive — refuses and writes nothing, and
// says the absence of a person to authorize it is the reason; preview
// remains available unattended because reporting commits to nothing.

package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/ddwht/parlay/core/internal/config"
)

var retireRootCmd = &cobra.Command{
	Use:   "retire-root <name>",
	Short: "Retire a child root: archive it verifiably, record dispositions, deregister it",
	Long: `Retire a registered child root. The complete child directory is
preserved byte for byte under <parent>/.parlay/retired/<name>/ with a
manifest of content hashes, the operator-authored disposition record is
preserved alongside it, a retirement record says what was decided and
when, and the root's registration is removed — last, so no failure
leaves the project half-retired.

The preflight refuses while anything outside the root still stands on
it: a project-wide, source-aware sweep reports every inbound reference
(ownership markers, path references, feature refs, guidance
instructions), and every authority-re-homed-to disposition target must
exist, be active, and already claim the surviving work.

Use --preview to see the entire preflight without changing anything.
Execution asks for explicit confirmation; a run with nobody to ask
(--non-interactive, or no terminal) refuses.`,
	Args: cobra.ExactArgs(1),
	RunE: runRetireRoot,
}

var (
	retireRootDispositions   string
	retireRootPreview        bool
	retireRootNonInteractive bool
	// retireRootTTYOverride lets tests pin the interactive probe without
	// standing up a PTY, mirroring ttyInteractive's override parameter.
	retireRootTTYOverride *bool
)

func init() {
	retireRootCmd.Flags().StringVar(&retireRootDispositions, "dispositions", "",
		"Path to the operator-authored disposition record naming what became of every feature in the retiring root")
	retireRootCmd.Flags().BoolVar(&retireRootPreview, "preview", false,
		"Run the entire preflight and report it without writing, reserving, or changing anything")
	retireRootCmd.Flags().BoolVar(&retireRootNonInteractive, "non-interactive", false,
		"Force headless mode even when a TTY is attached — execution then refuses (a person must authorize it); --preview still works")
}

// resolveRetirementTarget resolves <name> against the parent's roots
// index: registered children only, exactly-one-or-refuse.
func resolveRetirementTarget(idx *config.RootsIndex, name string) (config.Root, error) {
	parentPath := idx.ParentPath

	// The parent root itself — the one holding the shared resources and
	// the root registration — is never a valid target.
	if name == filepath.Base(parentPath) ||
		filepath.Clean(filepath.Join(parentPath, name)) == filepath.Clean(parentPath) {
		return config.Root{}, fmt.Errorf("refusing to retire %q: it is the parent root holding the shared resources and the root registration — ending the parent is the project ending, not a subproject ending", name)
	}

	// A child matches by registered name OR by registered relative path
	// (decision: target-match-name-or-relative-path).
	cleanName := filepath.ToSlash(filepath.Clean(name))
	seen := map[string]bool{}
	var candidates []config.Root
	for _, c := range idx.Children {
		if c.Name == name || filepath.ToSlash(filepath.Clean(c.RelativePath)) == cleanName {
			if !seen[c.Name] {
				seen[c.Name] = true
				candidates = append(candidates, c)
			}
		}
	}

	switch len(candidates) {
	case 1:
		// Registration is not path authorization. Before the registered
		// name and path can become an archive source, a destination, a
		// journal filename or a deletion target, both must be shown to
		// address nothing but their own place inside the project root.
		if err := validateRootName(candidates[0].Name); err != nil {
			return config.Root{}, fmt.Errorf("refusing to retire %q: %w", name, err)
		}
		if _, err := resolveContainedChildDir(parentPath, candidates[0].RelativePath); err != nil {
			return config.Root{}, fmt.Errorf("refusing to retire %q: %w", candidates[0].Name, err)
		}
		return candidates[0], nil
	case 0:
		// A directory that carries root configuration but is not in the
		// index is refused with that stated as the reason — it may look
		// like a root, but retirement acts on the registration.
		dirPath := filepath.Join(parentPath, filepath.FromSlash(name))
		if _, err := os.Stat(filepath.Join(dirPath, config.ParlayDir, config.ConfigFile)); err == nil {
			return config.Root{}, fmt.Errorf("%s carries .parlay root configuration but is not registered in the parent's roots index — only registered child roots can be retired", name)
		}
		names := idx.Names()
		sort.Strings(names)
		if len(names) == 0 {
			return config.Root{}, fmt.Errorf("no registered child root named %q — the parent's roots index registers no children", name)
		}
		return config.Root{}, fmt.Errorf("no registered child root named %q — registered child roots: %s", name, strings.Join(names, ", "))
	default:
		var lines []string
		for _, c := range candidates {
			lines = append(lines, fmt.Sprintf("%s (%s/)", c.Name, c.RelativePath))
		}
		return config.Root{}, fmt.Errorf("%q matches more than one registered child root — %s — refusing to select one by ordering or proximity; name the one you mean", name, strings.Join(lines, ", "))
	}
}

// resolveContainedChildDir turns a registered child root's relative path
// into the absolute directory the retirement may act on, and refuses
// unless that directory resolves STRICTLY INSIDE the project root.
//
// Registration is not path authorization. The roots index is an ordinary
// YAML file a person or a bad merge can edit, and every destructive step
// of a retirement — the archive walk, the removal of the root's
// directory — is derived from the registered path. So the path is
// validated here, fail-closed, before it can become any of them:
//
//   - an absolute registered path is refused outright (a child root is
//     located relative to its parent, by construction);
//   - a path that leaves the parent lexically (".." segments, "." alone,
//     the empty string) is refused before it touches the filesystem;
//   - the resolved path — filepath.EvalSymlinks on both parent and child
//     — must be a strict descendant of the resolved parent, so a symlink
//     pointing out of the project is refused however ordinary its name
//     looks. The parent root itself is not "inside" itself: retiring it
//     would delete the project.
//
// A directory that does not exist resolves to the lexically contained
// join: nothing can be archived or removed there, and the caller's own
// existence checks report it in their own terms.
func resolveContainedChildDir(parentPath, relPath string) (string, error) {
	if strings.TrimSpace(relPath) == "" {
		return "", fmt.Errorf("registered path is empty — a registered child root must name a directory inside the project")
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("registered path %q is absolute — a child root is located relative to its parent, and an absolute registration is not path authorization for archiving or deleting it", relPath)
	}
	clean := filepath.Clean(filepath.FromSlash(relPath))
	if clean == "." || clean == string(filepath.Separator) {
		return "", fmt.Errorf("registered path %q resolves to the project root itself — ending the parent is the project ending, not a subproject ending", relPath)
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("registered path %q escapes the project root — a retirement archives and deletes the registered directory, and a path leading outside the project is refused rather than followed", relPath)
	}

	childDir := filepath.Join(parentPath, clean)

	resolvedParent, err := filepath.EvalSymlinks(parentPath)
	if err != nil {
		return "", fmt.Errorf("resolve project root %s: %w", parentPath, err)
	}
	resolvedChild, err := filepath.EvalSymlinks(childDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing is there to archive or delete; the lexical checks
			// above already established the path stays inside.
			return childDir, nil
		}
		// Cannot tell is not inside.
		return "", fmt.Errorf("resolve registered path %q: %w", relPath, err)
	}
	if resolvedChild == resolvedParent {
		return "", fmt.Errorf("registered path %q resolves to the project root itself — ending the parent is the project ending, not a subproject ending", relPath)
	}
	if !pathWithin(resolvedParent, resolvedChild) {
		return "", fmt.Errorf("registered path %q resolves to %s, which is outside the project root %s — a retirement archives and deletes the registered directory, so a registration escaping the project is refused rather than followed",
			relPath, resolvedChild, resolvedParent)
	}
	return childDir, nil
}

// rootNameRe is the shape a registered root NAME must have to be usable
// as a path component: lowercase alphanumerics in dash-separated words.
// No separators, no dots, no leading or trailing dash, nothing empty.
var rootNameRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// validateRootName refuses a registered root name that is not a plain
// slug.
//
// The name is not decoration: it is concatenated into the staging
// directory, the archive destination and the journal filename, all three
// of which are then created, renamed and removed. A name carrying a
// separator or a traversal segment would place those somewhere other
// than under the retired-roots directory — and the destination is
// rolled back with a recursive delete, so a name is as much a deletion
// target as a path is. Registration is not authorization here either:
// roots.yaml is an ordinary file, and a name read out of it reaches the
// same destructive calls a path does.
//
// Restricting the shape rather than escaping it is deliberate. Escaping
// asks every future caller to remember to escape; a closed shape is
// checked once, at the boundary, and everything downstream can treat
// the name as a single safe path component. Every name parlay itself
// generates already satisfies it.
func validateRootName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("registered root name is empty — a name is a path component of the archive destination and the journal, and an empty one names no location")
	}
	if !rootNameRe.MatchString(name) {
		return fmt.Errorf("registered root name %q is not a plain slug (lowercase letters, digits and dashes) — the name becomes a path component of the staging directory, the archive destination and the journal file, each of which this operation creates, renames and removes, so a name that can address a location other than its own is refused rather than escaped", name)
	}
	return nil
}

// The retirement's on-disk locations, expressed RELATIVE to the project
// root so they can be reached through a rooted handle (see
// mutateUnderParent). Every one of them is under the retired-roots
// directory, and every rootName reaching them has passed
// validateRootName, so none can be anything but a leaf under it.
func retiredRootsRel() string {
	return filepath.Join(config.ParlayDir, "retired")
}

func retirementDestinationRel(rootName string) string {
	return filepath.Join(retiredRootsRel(), rootName)
}

func retirementStagingRel(rootName string) string {
	return filepath.Join(retiredRootsRel(), ".staging-"+rootName)
}

// mutateUnderParent runs fn against a handle rooted at the project root.
//
// This is the answer to the gap a lexical containment check leaves open.
// resolveContainedChildDir resolves and compares a path, and then the
// caller acts on that path some microseconds later; in between, an
// intermediate directory can be replaced with a symlink pointing out of
// the project, and the ordinary os.RemoveAll that follows would follow
// it. Checking again would only narrow the window, not close it.
//
// os.Root closes it. The handle references the project root directory
// itself (a file descriptor on every platform this tool targets), and
// every operation through it refuses a name whose components resolve
// outside that root — the resolution and the operation are the same
// syscall sequence, so there is no interval between them to exploit.
// Every destructive step of a retirement goes through here.
func mutateUnderParent(parentPath string, fn func(root *os.Root) error) error {
	root, err := os.OpenRoot(parentPath)
	if err != nil {
		return fmt.Errorf("open project root %s: %w", parentPath, err)
	}
	defer root.Close()
	return fn(root)
}

// removeUnderParent deletes relPath through a rooted handle, refusing
// any path that leaves the project however the filesystem changes
// underneath it.
func removeUnderParent(parentPath, relPath string) error {
	return mutateUnderParent(parentPath, func(root *os.Root) error {
		return root.RemoveAll(relPath)
	})
}

// checkRetirementDestination enforces the destination-absence
// precondition: the archive destination is stat'd before anything else
// is read, and an existing destination refuses the run naming both
// possible explanations as the operator's call.
func checkRetirementDestination(parentPath, rootName string) error {
	dest := retirementDestination(parentPath, rootName)
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("destination %s already exists — either an earlier retirement of the same root left it there, or unrelated content sits under the same name; deciding which, and moving it aside, is the operator's call", dest)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat retirement destination %s: %w", dest, err)
	}
	return nil
}

// retirementRecord is what was decided, when, and where the evidence
// went — written to <destination>/retirement-record.yaml by the
// write-record journal step. History, never the owner of surviving
// work: the re-home check recognizes only live features as owners.
type retirementRecord struct {
	Root             string        `yaml:"root"`
	RelativePath     string        `yaml:"relative-path"`
	RetiredAt        string        `yaml:"retired-at"`
	Archive          string        `yaml:"archive"`
	Manifest         string        `yaml:"manifest"`
	Dispositions     string        `yaml:"dispositions"`
	PreservedMembers int           `yaml:"preserved-members"`
	Features         []Disposition `yaml:"feature-dispositions"`
}

func writeRetirementRecord(destination string, record *retirementRecord) error {
	data, err := yaml.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal retirement record: %w", err)
	}
	if err := os.WriteFile(filepath.Join(destination, "retirement-record.yaml"), data, 0o644); err != nil {
		return fmt.Errorf("write retirement record: %w", err)
	}
	return nil
}

// retirementPreflight is everything the run established before any
// mutation — what the preview shows and what execution is authorized
// against.
type retirementPreflight struct {
	Target       config.Root
	Features     []string
	Record       *DispositionRecord
	RecordErrs   []error
	Sweep        RootSweepResult
	RehomeErrs   []error
	Members      []memberEntry
	WalkErr      error
	MissingBlock []string // human summary of everything that blocks execution
}

// runRetirementPreflight performs target resolution and destination
// checking first, then — only after both succeed — enumerates the
// root's features, validates the disposition record, runs the
// project-wide sweep, checks re-home readiness, and validates the
// archive walk. No mutation occurs here.
func runRetirementPreflight(parentPath string, idx *config.RootsIndex, target config.Root) (*retirementPreflight, error) {
	pf := &retirementPreflight{Target: target}

	features, err := enumerateRetiringFeatures(target.Path)
	if err != nil {
		return nil, fmt.Errorf("enumerate features of retiring root %q: %w", target.Name, err)
	}
	sort.Strings(features)
	pf.Features = features

	if retireRootDispositions != "" {
		rec, err := LoadDispositionRecord(retireRootDispositions)
		if err != nil {
			return nil, err
		}
		pf.Record = rec
		pf.RecordErrs = checkDispositionCompleteness(features, rec)
	}

	sweep, err := sweepRootRetirement(parentPath, target, pf.Record)
	if err != nil {
		return nil, err
	}
	pf.Sweep = sweep

	if pf.Record != nil {
		pf.RehomeErrs = checkRehomeTargets(parentPath, idx, target, pf.Record, sweep)
	}

	members, _, walkErr := validateArchiveWalk(target.Path)
	pf.Members = members
	pf.WalkErr = walkErr

	// Everything that blocks execution, gathered for the report.
	if pf.Record == nil {
		pf.MissingBlock = append(pf.MissingBlock, "no disposition record given (--dispositions <path>)")
	}
	for _, e := range pf.RecordErrs {
		pf.MissingBlock = append(pf.MissingBlock, e.Error())
	}
	for _, f := range sweep.BlockingFindings() {
		pf.MissingBlock = append(pf.MissingBlock, "inbound reference: "+f.String())
	}
	for _, f := range sweep.Failures {
		pf.MissingBlock = append(pf.MissingBlock, "scan failure (cannot tell is not none): "+f.String())
	}
	for _, e := range pf.RehomeErrs {
		pf.MissingBlock = append(pf.MissingBlock, e.Error())
	}
	if walkErr != nil {
		pf.MissingBlock = append(pf.MissingBlock, "archive walk: "+walkErr.Error())
	}
	return pf, nil
}

// printRetirementPreview reports the entire preflight: target,
// destination, the features requiring dispositions, the inbound sweep
// findings, re-home readiness, and the extent of what would be
// preserved. Reporting commits to nothing.
func printRetirementPreview(cmd *cobra.Command, parentPath string, pf *retirementPreflight) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Retirement preview: %s (%s/)\n", pf.Target.Name, pf.Target.RelativePath)
	fmt.Fprintf(out, "  Destination: %s\n", retirementDestination(parentPath, pf.Target.Name))

	byFeat := pf.Record.byFeature()
	fmt.Fprintf(out, "  Features requiring dispositions: %d\n", len(pf.Features))
	for _, f := range pf.Features {
		if d, ok := byFeat[f]; ok {
			extra := ""
			if d.Target != "" {
				extra = " -> " + d.Target
			}
			fmt.Fprintf(out, "    - %s: %s%s\n", f, d.Term, extra)
		} else {
			fmt.Fprintf(out, "    - %s: (no disposition)\n", f)
		}
	}

	blocking := pf.Sweep.BlockingFindings()
	fmt.Fprintf(out, "  Inbound sweep: %d blocking finding(s), %d scan failure(s)\n", len(blocking), len(pf.Sweep.Failures))
	for _, f := range blocking {
		fmt.Fprintf(out, "    - %s\n", f)
	}
	for _, f := range pf.Sweep.Failures {
		fmt.Fprintf(out, "    - [ERR] %s\n", f)
	}
	for _, e := range pf.RehomeErrs {
		fmt.Fprintf(out, "  Re-home readiness: [ERR] %v\n", e)
	}
	if pf.WalkErr != nil {
		fmt.Fprintf(out, "  Archive walk: [ERR] %v\n", pf.WalkErr)
	} else {
		fmt.Fprintf(out, "  Would preserve: %d file(s) — the complete directory, configuration, adapters and build state included\n", len(pf.Members))
	}
}

func runRetireRoot(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	parentPath := cfg.RepoRoot()
	name := args[0]

	idx, err := config.LoadRootsIndex(parentPath)
	if err != nil {
		return fmt.Errorf("load roots index: %w", err)
	}

	// An in-flight retirement is looked for FIRST, by scanning the
	// journal location — before the registration is consulted at all.
	// The last steps of a retirement remove the root's directory,
	// deregister the root and then remove the journal, so the state a
	// resume most needs to reach is one where the registration no longer
	// names the root. Resolving the target through the registration
	// first would make that state unreachable, which is the difference
	// between a journal that documents an interruption and one that can
	// actually finish it. A part-finished run owns the root until it
	// completes: resume it or refuse, never start over.
	journal, err := FindRetirementJournal(parentPath, name)
	if err != nil {
		return err
	}
	if journal != nil {
		return resumeRetirement(cmd, parentPath, idx, journal)
	}

	// Preconditions: resolution and destination checking happen once,
	// before any enumeration, sweep, or read of the root's contents.
	target, err := resolveRetirementTarget(idx, name)
	if err != nil {
		return err
	}

	if err := checkRetirementDestination(parentPath, target.Name); err != nil {
		return err
	}

	pf, err := runRetirementPreflight(parentPath, idx, target)
	if err != nil {
		return err
	}

	printRetirementPreview(cmd, parentPath, pf)

	// A preview asked for on its own leaves the project byte-identical:
	// nothing written, nothing reserved, no state behind.
	if retireRootPreview {
		return nil
	}

	if len(pf.MissingBlock) > 0 {
		return fmt.Errorf("refusing to retire root %q while anything still stands on it:\n  - %s",
			target.Name, strings.Join(pf.MissingBlock, "\n  - "))
	}

	// Execution requires explicit authorization from a person after the
	// preview has been shown. A run that cannot ask refuses and writes
	// nothing.
	if retireRootNonInteractive || !ttyInteractive(retireRootTTYOverride) {
		return fmt.Errorf("refusing to retire root %q: execution requires a person to authorize it after the preview, and this run has no person to ask (--non-interactive or no terminal) — use --preview to inspect without changing anything, or re-run interactively", target.Name)
	}

	dest := retirementDestination(parentPath, target.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "\nThis will preserve %d file(s) from %s/ at %s and deregister %q from the project.\n",
		len(pf.Members), target.RelativePath, dest, target.Name)
	fmt.Fprint(cmd.OutOrStdout(), "Retire this root? [y/N] ")
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		fmt.Fprintln(cmd.OutOrStdout(), "[SKIP] Nothing changed.")
		return nil
	}

	return executeRetirement(cmd, parentPath, idx, pf)
}

// executeRetirement performs the authorized mutations, in the order that
// keeps the project whole: archive staged and promoted first (complete
// or absent), then the journal, then the record, then — last — the
// registration change.
func executeRetirement(cmd *cobra.Command, parentPath string, idx *config.RootsIndex, pf *retirementPreflight) error {
	target := pf.Target
	// The name has passed validateRootName at resolution, so it is a
	// single path component and these three locations are leaves under
	// the retired-roots directory by construction.
	dest := retirementDestination(parentPath, target.Name)
	staging := filepath.Join(retiredRootsDir(parentPath), ".staging-"+target.Name)
	stagingRel := retirementStagingRel(target.Name)
	destRel := retirementDestinationRel(target.Name)

	_, retiredDirErr := os.Lstat(retiredRootsDir(parentPath))
	createdRetiredDir := os.IsNotExist(retiredDirErr)
	if err := mutateUnderParent(parentPath, func(root *os.Root) error {
		return root.MkdirAll(retiredRootsRel(), 0o755)
	}); err != nil {
		return fmt.Errorf("create retired-roots directory: %w", err)
	}
	// A pre-archive failure restores exactly the prior state: the staged
	// directory is removed — and so is the retired/ directory when this
	// run created it and nothing else lives there (Remove refuses a
	// non-empty directory, which is exactly the guard needed). Both
	// deletions go through the rooted handle, like every other
	// destructive step.
	cleanupStaging := func() {
		_ = removeUnderParent(parentPath, stagingRel)
		if createdRetiredDir {
			_ = mutateUnderParent(parentPath, func(root *os.Root) error {
				return root.Remove(retiredRootsRel())
			})
		}
	}

	if err := retirementEvent("stage-archive"); err != nil {
		cleanupStaging()
		return err
	}
	manifest, err := archiveRoot(target.Path, staging)
	if err != nil {
		cleanupStaging()
		return err
	}

	// The operator-authored disposition record is preserved verbatim
	// alongside the archive — staged before promotion so the promoted
	// destination is complete from its first instant.
	recData, err := os.ReadFile(retireRootDispositions)
	if err != nil {
		cleanupStaging()
		return fmt.Errorf("read disposition record for preservation: %w", err)
	}
	if err := mutateUnderParent(parentPath, func(root *os.Root) error {
		return root.WriteFile(filepath.Join(stagingRel, "dispositions.yaml"), recData, 0o644)
	}); err != nil {
		cleanupStaging()
		return fmt.Errorf("preserve disposition record: %w", err)
	}

	if err := retirementEvent("promote"); err != nil {
		cleanupStaging()
		return err
	}
	if err := mutateUnderParent(parentPath, func(root *os.Root) error {
		return root.Rename(stagingRel, destRel)
	}); err != nil {
		cleanupStaging()
		return fmt.Errorf("promote archive to %s: %w", dest, err)
	}

	journal := &RetirementJournal{
		Root:         target.Name,
		RelativePath: filepath.ToSlash(target.RelativePath),
		Outstanding:  []string{journalStepWriteRecord, journalStepDeregisterRoot},
	}
	if err := retirementEvent("write-journal"); err != nil {
		// The journal is what makes the destination resumable; without
		// it the archive must not stand, so restore the prior state.
		_ = removeUnderParent(parentPath, destRel)
		return err
	}
	if err := WriteRetirementJournal(parentPath, journal); err != nil {
		_ = removeUnderParent(parentPath, destRel)
		return err
	}

	record := &retirementRecord{
		Root:             target.Name,
		RelativePath:     filepath.ToSlash(target.RelativePath),
		RetiredAt:        time.Now().UTC().Format(time.RFC3339),
		Archive:          "contents/",
		Manifest:         "manifest.yaml",
		Dispositions:     "dispositions.yaml",
		PreservedMembers: len(manifest.Members),
		Features:         pf.Record.Dispositions,
	}
	if err := executeJournal(parentPath, idx, journal, record); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "[OK] Retired root %q: %d file(s) preserved at %s, registration removed.\n",
		target.Name, len(manifest.Members), dest)
	fmt.Fprintf(cmd.OutOrStdout(), "Verify any time with: parlay retired-roots --check\n")
	return nil
}

// resumeRetirement handles a retire-root of a root whose earlier run
// left an outstanding journal: it reports what is already done and what
// remains, and continues from the journal rather than starting over —
// completed steps are not repeated, because their preconditions were
// consumed (the destination exists, the contents have moved). A fresh
// retirement of the same root refuses while the journal is outstanding,
// naming the part-finished run.
func resumeRetirement(cmd *cobra.Command, parentPath string, idx *config.RootsIndex, journal *RetirementJournal) error {
	out := cmd.OutOrStdout()
	done := []string{"archive (complete: the destination exists)"}
	all := []string{journalStepWriteRecord, journalStepDeregisterRoot}
	outstanding := map[string]bool{}
	for _, s := range journal.Outstanding {
		outstanding[s] = true
	}
	for _, s := range all {
		if !outstanding[s] {
			done = append(done, s)
		}
	}
	fmt.Fprintf(out, "A retirement of %q is part-finished (journal at %s).\n",
		journal.Root, retirementJournalPath(parentPath, journal.Root))
	fmt.Fprintf(out, "  Done: %s\n", strings.Join(done, ", "))
	fmt.Fprintf(out, "  Outstanding, in order: %s\n", strings.Join(journal.Outstanding, ", "))

	if retireRootPreview {
		return nil
	}
	if retireRootNonInteractive || !ttyInteractive(retireRootTTYOverride) {
		return fmt.Errorf("a retirement of %q is part-finished — refusing to act on it without a person to authorize the resume (--non-interactive or no terminal); re-run interactively to resume, or inspect with --preview", journal.Root)
	}
	fmt.Fprint(out, "Resume and complete the outstanding steps? [y/N] ")
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("a retirement of %q is part-finished and was not resumed — the journal still names the outstanding steps", journal.Root)
	}

	// The record for a resumed write-record step is rebuilt from the
	// evidence the archive already preserves.
	dest := retirementDestination(parentPath, journal.Root)
	var features []Disposition
	if rec, err := LoadDispositionRecord(filepath.Join(dest, "dispositions.yaml")); err == nil {
		features = rec.Dispositions
	}
	memberCount := 0
	if m, err := ReadManifest(filepath.Join(dest, "manifest.yaml")); err == nil {
		memberCount = len(m.Members)
	}
	record := &retirementRecord{
		Root:             journal.Root,
		RelativePath:     journal.RelativePath,
		RetiredAt:        time.Now().UTC().Format(time.RFC3339),
		Archive:          "contents/",
		Manifest:         "manifest.yaml",
		Dispositions:     "dispositions.yaml",
		PreservedMembers: memberCount,
		Features:         features,
	}
	if err := executeJournal(parentPath, idx, journal, record); err != nil {
		return err
	}
	fmt.Fprintf(out, "[OK] Resumed retirement of %q: outstanding steps completed, registration removed.\n", journal.Root)
	return nil
}
