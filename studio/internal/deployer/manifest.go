// parlay-feature: studio-foundation/studio-deployer
// parlay-section: cross-cutting
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/manifest-based-ownership-atomic-writes-idempotency

// manifest.go derives the owned-files manifest from the embedded source
// surface and the detected agent targets. The manifest is reconstructed
// from scratch on every Run — never persisted between runs — so a user
// file matching the parlay-* naming convention but absent from the
// current manifest is never owned and never touched.
package deployer

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
)

// ManifestEntry pairs one embedded skill source with one detected agent
// target. The SourceBytes are captured at derivation time so the deployer
// does not re-read the embedded source mid-run; the SourceHash is the
// sha256 of SourceBytes (used for the content-hash-skip idempotency
// check).
type ManifestEntry struct {
	SkillSlug   string
	Agent       AgentSurface
	TargetPath  string // absolute path under the project root
	SourceBytes []byte
	SourceHash  [sha256.Size]byte
}

// Manifest is the ordered set of files the deployer claims ownership of
// for a single Run. Iteration order is deterministic: lexicographic skill
// slug × DetectAgentSurfaces precedence order.
type Manifest []ManifestEntry

// DeriveManifest constructs the manifest from the inputs. The read
// function (typically embedded.ReadSkill) is taken as a parameter so the
// test suite can inject a controlled overlay without depending on the
// production embedded package.
//
// The traversal order is deterministic: outer loop walks skills in the
// lexicographic order they were passed (callers — typically embedded.ListSkills —
// already sort), inner loop walks agents in the order DetectAgentSurfaces
// returned them. Same inputs → byte-equivalent Manifest slice.
func DeriveManifest(skills []string, agents []AgentTarget, projectRoot string, read func(string) ([]byte, error)) (Manifest, error) {
	if read == nil {
		return nil, fmt.Errorf("DeriveManifest: read function is nil")
	}
	out := make(Manifest, 0, len(skills)*len(agents))
	for _, slug := range skills {
		content, err := read(slug)
		if err != nil {
			return nil, fmt.Errorf("DeriveManifest: read %q: %w", slug, err)
		}
		hash := sha256.Sum256(content)
		for _, a := range agents {
			out = append(out, ManifestEntry{
				SkillSlug:   slug,
				Agent:       a.Surface,
				TargetPath:  filepath.Join(projectRoot, a.SkillTargetPath(slug)),
				SourceBytes: content,
				SourceHash:  hash,
			})
		}
	}
	return out, nil
}

// PathSet returns the set of target paths the manifest claims. Used by
// cleanupOrphanTmpFiles to decide which .tmp siblings the deployer may
// remove, and by the orphan scan to decide which on-disk parlay-* files
// are NOT on the current manifest.
func (m Manifest) PathSet() map[string]struct{} {
	out := make(map[string]struct{}, len(m))
	for _, e := range m {
		out[e.TargetPath] = struct{}{}
	}
	return out
}
