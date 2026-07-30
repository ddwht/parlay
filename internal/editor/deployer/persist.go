// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency

// persist.go holds the deployer's persisted-manifest helpers. The
// deployer writes the list of paths it deployed at the end of each
// successful run; on the next run, that persisted list is the
// "previously owned" set used to compute orphans by set difference
// (orphans = previously-owned − currently-owned).
//
// This is the corrective for the v0.1.0 orphan-scoping bug: the
// directory-walk approach treated every parlay-* file under the agent
// skill surface as a Studio orphan candidate, but parlay-core ships
// parlay-* skills (parlay-add-feature, parlay-loop, ...). The walk
// claimed ownership over files Studio never wrote. The fix grounds
// ownership in what Studio actually deployed, not what the directory
// happens to contain.

package deployer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// persistedManifestPath returns the canonical location for the
// deployer's persisted manifest, namespaced inside .parlay/ so the
// file cannot collide with parlay-core's state files.
func persistedManifestPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".parlay", "studio-deployer-manifest.json")
}

// loadPreviousManifest reads the persisted manifest from a previous
// deployer run. Returns an empty set on first deploy (file missing);
// read errors other than NotExist bubble up.
func loadPreviousManifest(projectRoot string) (map[string]struct{}, error) {
	p := persistedManifestPath(projectRoot)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]struct{}{}, nil
		}
		return nil, fmt.Errorf("loadPreviousManifest: %w", err)
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, fmt.Errorf("loadPreviousManifest: parse %s: %w", p, err)
	}
	set := make(map[string]struct{}, len(paths))
	for _, q := range paths {
		set[q] = struct{}{}
	}
	return set, nil
}

// savePersistedManifest writes the current run's manifest paths to
// disk for the next run's orphan-detection diff. Sorted for stable
// output in case the file gets diffed by version control or a human.
// Uses the same write-temp + rename atomic helper the rest of the
// deployer uses.
func savePersistedManifest(projectRoot string, currentPaths map[string]struct{}) error {
	p := persistedManifestPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("savePersistedManifest: mkdir %s: %w", filepath.Dir(p), err)
	}
	paths := make([]string, 0, len(currentPaths))
	for k := range currentPaths {
		paths = append(paths, k)
	}
	sort.Strings(paths)
	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return fmt.Errorf("savePersistedManifest: marshal: %w", err)
	}
	if err := writeAtomicWith(p, data, defaultRenamer); err != nil {
		return fmt.Errorf("savePersistedManifest: write %s: %w", p, err)
	}
	return nil
}

// diffOrphans returns the paths in prev that are not in current,
// sorted lexicographically for stable WARN output.
func diffOrphans(prev, current map[string]struct{}) []string {
	var out []string
	for k := range prev {
		if _, ok := current[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
