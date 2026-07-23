// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/compare-and-swap-save

package domain

import (
	"crypto/sha256"
	"encoding/hex"
)

// Etag is the content identity token used by the compare-and-swap save path.
// It is derived from the on-disk file's raw bytes at load time; a save must
// present the token from its originating load.
type Etag string

// SentinelEmpty is the distinguished token a project with no domain-model.yaml
// returns from the load bootstrap. A first save presenting this sentinel
// creates the file (see Save). It is never a real content hash.
const SentinelEmpty Etag = "empty"

// computeEtag derives the content identity token from a file's raw bytes.
// Identical bytes always produce an identical token, so two byte-identical
// serializations of the same model carry the same Etag.
func computeEtag(raw []byte) Etag {
	sum := sha256.Sum256(raw)
	return Etag("sha256:" + hex.EncodeToString(sum[:]))
}
