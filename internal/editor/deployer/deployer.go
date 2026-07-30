// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency

// deployer.go ties the embedded source surface, the agent-surface
// detection, the owned-files manifest, the atomic-write helper, and the
// four-status enum into one Run method. The Deployer is constructed by
// the init/upgrade subcommand entry points (see subcommands.go); Run is
// what actually fans the embedded source out to the project's agent
// surfaces.
package deployer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
)

// Deployer carries the dependencies one Run needs. The two function
// fields (SkillReader and SkillLister) are injected by the constructor
// so the test suite can drive Run against a controlled overlay without
// touching the production embedded package.
type Deployer struct {
	ProjectRoot string
	Agents      []AgentTarget
	SkillReader func(slug string) ([]byte, error)
	SkillLister func() ([]string, error)

	// Logger is the optional WARN sink for orphan-detected lines. When
	// nil, the deployer logs to the standard logger.
	Logger *log.Logger

	// renameFn is the test seam for atomic-write rename injection. When
	// nil, writeAtomic uses os.Rename.
	renameFn renamer
}

// RunResult is the outcome of one Run invocation: one entry per manifest
// path (written/unchanged/failed) plus the orphan entries discovered
// during the post-write scan.
type RunResult struct {
	Entries  []FileStatusEntry
	ExitCode int
}

// Run executes the deployer's fixed-order pipeline:
//
//  1. Derive the owned-files manifest from SkillLister × Agents.
//  2. For each detected agent in DetectAgentSurfaces order:
//     a. mkdir -p the agent's skill parent directory if missing.
//     b. Clean up any orphan .tmp files that correspond to manifest paths.
//     c. For each manifest entry on this agent: compare the embedded
//     source hash against the on-disk content hash. Skip on match
//     (StatusUnchanged); writeAtomic on mismatch (StatusWritten or
//     StatusFailed).
//  3. Compute orphans as the set difference between the previously-
//     persisted manifest (from the last successful Run, if any) and the
//     current manifest. Files that Studio never deployed do not appear
//     in any persisted manifest and are therefore never claimed as
//     orphans — this is the load-bearing property that prevents Studio
//     from reporting parlay-core's parlay-* skills as Studio orphans.
//     Each orphan is recorded as StatusOrphan and logged with
//     studio-deployer-orphan-detected at WARN. Orphans are never deleted.
//  4. Aggregate ExitCode: non-zero when any entry is StatusFailed; zero
//     otherwise (orphans do not affect ExitCode).
//  5. When ExitCode is zero, persist the current manifest's paths so the
//     next run's orphan-diff sees them. A partial deploy does not
//     overwrite the prior persisted manifest.
//
// Per-agent atomicity: a failure on one agent's writes does not block
// writes to other detected agents. The agents loop catches per-write
// errors and proceeds.
func (d *Deployer) Run(ctx context.Context) (RunResult, error) {
	if d.SkillLister == nil || d.SkillReader == nil {
		return RunResult{ExitCode: 1}, fmt.Errorf("deployer.Run: SkillLister and SkillReader are required")
	}
	if d.ProjectRoot == "" {
		return RunResult{ExitCode: 1}, fmt.Errorf("deployer.Run: ProjectRoot is empty")
	}
	if len(d.Agents) == 0 {
		return RunResult{ExitCode: 1}, fmt.Errorf("deployer.Run: Agents is empty (DetectAgentSurfaces must run before Run)")
	}

	skills, err := d.SkillLister()
	if err != nil {
		return RunResult{ExitCode: 1}, fmt.Errorf("deployer.Run: list skills: %w", err)
	}
	manifest, err := DeriveManifest(skills, d.Agents, d.ProjectRoot, d.SkillReader)
	if err != nil {
		return RunResult{ExitCode: 1}, fmt.Errorf("deployer.Run: derive manifest: %w", err)
	}
	manifestPaths := manifest.PathSet()

	var entries []FileStatusEntry

	// Step 2: per-agent atomic write step, in detection order.
	for _, agent := range d.Agents {
		skillsParent := skillsDirFor(d.ProjectRoot, agent.Surface)
		// Step 2a: ensure the per-skill parent directory exists. Failure
		// here propagates as a per-entry failed status on every entry for
		// this agent; subsequent agents still get a chance.
		if err := os.MkdirAll(skillsParent, 0o755); err != nil {
			for _, e := range manifest {
				if e.Agent != agent.Surface {
					continue
				}
				entries = append(entries, FileStatusEntry{
					Path:   e.TargetPath,
					Status: StatusFailed,
					Source: e.SkillSlug,
					Err:    fmt.Errorf("mkdir %s: %w", skillsParent, err),
				})
			}
			continue
		}
		// Step 2b: clean up orphan .tmp files corresponding to manifest paths.
		if err := cleanupOrphanTmpFiles(skillsParent, manifestPaths); err != nil {
			d.warn("studio-deployer-tmp-cleanup-failed: %v", err)
		}
		// Step 2c: per-entry hash check + write.
		for _, e := range manifest {
			if e.Agent != agent.Surface {
				continue
			}
			entries = append(entries, d.processEntry(e))
		}
	}

	// Step 3: compute orphans by diffing the previously-persisted
	// manifest against the current manifest. Files Studio never
	// deployed have no presence in any persisted manifest and are
	// therefore never claimed — parlay-core's parlay-* skills under
	// .claude/skills/ are correctly ignored.
	prevPaths, err := loadPreviousManifest(d.ProjectRoot)
	if err != nil {
		d.warn("studio-deployer-prev-manifest-load-failed: %v", err)
		prevPaths = map[string]struct{}{}
	}
	for _, p := range diffOrphans(prevPaths, manifestPaths) {
		// Defensive: only report orphans that still exist on disk. A
		// previously-deployed file the operator manually deleted is
		// not an orphan; it's just gone.
		if _, statErr := os.Stat(p); statErr != nil {
			continue
		}
		d.warn("studio-deployer-orphan-detected: %s (not on current manifest; leaving on disk)", p)
		entries = append(entries, FileStatusEntry{Path: p, Status: StatusOrphan})
	}

	// Step 4: aggregate exit code.
	exit := 0
	for _, e := range entries {
		if e.Status == StatusFailed {
			exit = 1
			break
		}
	}

	// Step 5: persist the current manifest only on full success. A
	// partial deploy (any StatusFailed) leaves the prior persisted
	// manifest intact so the next run still sees the same orphan
	// candidates rather than dropping them silently.
	if exit == 0 {
		if err := savePersistedManifest(d.ProjectRoot, manifestPaths); err != nil {
			d.warn("studio-deployer-persist-manifest-failed: %v", err)
		}
	}
	return RunResult{Entries: entries, ExitCode: exit}, nil
}

// processEntry implements the per-entry hash check and write. The
// content-hash-skip means a re-run over identical on-disk state performs
// zero writes — the idempotency guarantee.
func (d *Deployer) processEntry(e ManifestEntry) FileStatusEntry {
	existing, err := os.ReadFile(e.TargetPath)
	if err == nil {
		// File exists; compare hashes.
		onDisk := sha256.Sum256(existing)
		if bytes.Equal(onDisk[:], e.SourceHash[:]) {
			return FileStatusEntry{Path: e.TargetPath, Status: StatusUnchanged, Source: e.SkillSlug}
		}
	} else if !os.IsNotExist(err) {
		return FileStatusEntry{Path: e.TargetPath, Status: StatusFailed, Source: e.SkillSlug, Err: err}
	}
	// File missing or hash mismatch → write.
	rename := d.renameFn
	if rename == nil {
		rename = defaultRenamer
	}
	if err := writeAtomicWith(e.TargetPath, e.SourceBytes, rename); err != nil {
		return FileStatusEntry{Path: e.TargetPath, Status: StatusFailed, Source: e.SkillSlug, Err: err}
	}
	return FileStatusEntry{Path: e.TargetPath, Status: StatusWritten, Source: e.SkillSlug}
}

// warn writes the formatted message to the configured Logger (or the
// standard logger if none was set) as a WARN line. The standardized
// "studio-deployer-orphan-detected: ..." prefix lets external tools grep
// for the stable code.
func (d *Deployer) warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if d.Logger != nil {
		d.Logger.Printf("WARN %s", msg)
		return
	}
	log.Printf("WARN %s", msg)
}
