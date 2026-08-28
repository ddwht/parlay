// parlay-feature: parlay-tool/active-page-composition
// parlay-component: composition-signature
//
// The cross-feature input that feature-local source signatures cannot carry.
//
// featureSignatureSources hashes intents, dialogs, surface, capabilities,
// infrastructure, layouts, authored units and the shared domain model — every
// one of them a file inside THIS feature's directory, plus one project-wide
// artifact. So when feature B adds `supersedes: @A/fragment`, nothing feature
// A owns changes: A's buildfile stays fresh, A's generated component keeps
// being emitted and routed, and the only place the retirement shows up is a
// view-page run nobody's build consults. The same hole swallows a page
// manifest edit, which can reorder or re-scope a page without touching any
// feature's own files.
//
// This adds the missing term: a hash over the RESOLVED composition of every
// page this feature contributes to. Any change to a sibling's contribution on
// a shared page, to a supersedes: edge pointing at this feature, or to a
// manifest ordering one of its pages, moves the value and marks the buildfile
// stale — which is the only signal a rebuild has to remove output that left
// the page.
package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
)

// compositionSignature hashes the composed view of every page `feature`
// contributes to, including the pages it USED to contribute to via a fragment
// another feature has since retired.
//
// Scoped to touched pages rather than the whole project on purpose: a global
// hash would mark every feature stale whenever any unrelated page moved, which
// trains people to ignore staleness. Scoped to this feature's own fragments
// only would miss the sibling edit that is the entire point.
func compositionSignature(specDir, feature string) (string, error) {
	fragments, err := parser.ScanAllSurfaces(specDir)
	if err != nil {
		// No surfaces at all is a legitimate shape (a backend-only feature).
		// An empty composition hashes to a stable constant rather than an
		// error, so a feature with no pages does not fail the gate forever.
		if os.IsNotExist(err) {
			return emptyCompositionHash(), nil
		}
		return "", err
	}
	view := agent.ResolveActiveView(fragments)

	// Pages this feature touches, whether its contribution survived or not.
	// A retired fragment still names the page whose composition changed.
	pages := map[string]bool{}
	for _, f := range fragments {
		if f.Feature == feature && f.Page != "" {
			pages[f.Page] = true
		}
	}
	if len(pages) == 0 {
		return emptyCompositionHash(), nil
	}
	pageNames := make([]string, 0, len(pages))
	for p := range pages {
		pageNames = append(pageNames, p)
	}
	sort.Strings(pageNames)

	h := sha256.New()
	for _, page := range pageNames {
		fmt.Fprintf(h, "page:%s\n", page)

		// Every ACTIVE fragment on the page, from any feature. A sibling's
		// fragment appearing, moving region or changing order changes what
		// this feature's own code is assembled beside, so it belongs in the
		// hash even though this feature does not own it.
		active := view.ActiveOnPage(page)
		lines := make([]string, 0, len(active))
		for _, f := range active {
			lines = append(lines, fmt.Sprintf("active:%s:%s:%d", agent.FragmentRef(f), normalizeRegionName(f.Region), f.Order))
		}
		sort.Strings(lines)
		for _, l := range lines {
			fmt.Fprintln(h, l)
		}

		// Retirements ON this page, with the ref that caused each. This is
		// the term that moves when a sibling adds a supersedes: edge and
		// nothing else in the tree changes.
		var retired []string
		for _, f := range fragments {
			if f.Page != page {
				continue
			}
			ref := agent.FragmentRef(f)
			if by, ok := view.Retired[ref]; ok {
				retired = append(retired, fmt.Sprintf("retired:%s:by:%s", ref, by))
			}
		}
		sort.Strings(retired)
		for _, r := range retired {
			fmt.Fprintln(h, r)
		}

		// The manifest, when one exists. It can reorder or re-scope a page
		// without any surface changing, so its bytes are part of the composed
		// answer. Absent is hashed as an explicit marker, so locking a page
		// and unlocking it again do not collide.
		manifest := filepath.Join(specDir, "pages", page+".page.md")
		if data, err := os.ReadFile(manifest); err == nil {
			fmt.Fprintf(h, "manifest:present:%d\n", len(data))
			h.Write(data)
		} else {
			fmt.Fprintln(h, "manifest:absent")
		}

		// A composition the resolver REFUSED is not the same state as one it
		// resolved cleanly, and a rebuild must not treat the two alike: while
		// a fork or cycle stands, the page is deliberately left uncomposed.
		for _, e := range view.Errors {
			fmt.Fprintf(h, "refusal:%s:%s\n", e.Code, e.Message)
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// normalizeRegionName mirrors agent.normalizeRegion, which is unexported.
// Kept identical deliberately: if these two disagreed, the signature would
// treat "" and "main" as different slots while the resolver treats them as
// the same one, and a feature would flap stale on a cosmetic edit.
func normalizeRegionName(region string) string {
	r := strings.ToLower(strings.TrimSpace(region))
	if r == "" {
		return "main"
	}
	return r
}

func emptyCompositionHash() string {
	sum := sha256.Sum256([]byte("parlay:empty-composition:v1"))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// compositionSignatureForFeatureDir resolves the spec dir and feature slug
// from a feature directory, so callers holding only the path used by
// computeSourceSignatures do not have to re-derive either.
func compositionSignatureForFeatureDir(featureDir, projectRoot string) (string, error) {
	specDir := filepath.Join(projectRoot, config.SpecDir)
	// A feature dir is <spec>/intents/<slug> or <spec>/intents/<group>/<slug>;
	// the slug is what Fragment.Feature carries, which ScanAllSurfaces builds
	// from the path below intents/.
	intents := filepath.Join(specDir, "intents")
	rel, err := filepath.Rel(intents, featureDir)
	if err != nil {
		return "", err
	}
	return compositionSignature(specDir, filepath.ToSlash(rel))
}
