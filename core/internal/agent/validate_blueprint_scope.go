// parlay-feature: parlay-tool/multi-adapter
// parlay-component: blueprint-scope-and-precedence
//
// Pins blueprint's owned scope to the set blueprint.schema.md's "Owned scope"
// section documents, rejects topology declarations (which belong in
// adapter-set), and validates strategy choices against the relevant adapter's
// declared support.

package agent

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// blueprintAllowedScope is the closed set of top-level blueprint keys.
//
// This MUST equal the list in blueprint.schema.md's "Owned scope" section;
// TestConformance_BlueprintScopeMatchesSchema asserts the equality. It did not
// once: the schema was corrected to {app, shells, navigation, authorization,
// data, errors, state, platform} and this map was not, so `validate --project`
// rejected `shells:` and `authorization:` — both documented in the schema body,
// both accepted by `validate --type blueprint`. Whichever validator you
// happened to run decided whether your blueprint was correct.
//
// The `auth` spelling in the old set was not merely cosmetic. The strategy
// gate below decoded `auth.strategy` while every real blueprint writes
// `authorization:`, so the key was simply absent and the closed-vocab check
// passed vacuously — it had never once fired on a real file.
var blueprintAllowedScope = map[string]bool{
	"app":           true, // app: header is informational; allowed but unused
	"shells":        true,
	"navigation":    true,
	"authorization": true,
	"data":          true,
	"errors":        true,
	"state":         true,
	"platform":      true,
}

// blueprintScopeList renders the closed set for an error message.
//
// Derived from the map rather than typed out beside it. The literal that used
// to sit in the message omitted `app`, which the map allowed — so the sentence
// naming the closed scope could disagree with the check that produced it.
func blueprintScopeList() string {
	keys := make([]string, 0, len(blueprintAllowedScope))
	for k := range blueprintAllowedScope {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return "{" + strings.Join(keys, ", ") + "}"
}

// Closed strategy vocabularies per architecture §5.4. These are the values
// blueprint may select for each cross-cutting setting. Adapters declare
// which subset they support via their `supports.<setting>` blocks (or via
// per-adapter pattern descriptions for non-supports settings); blueprint
// values outside the closed set fail with `blueprint-strategy-unknown`.

// ClosedSetDataFetching — data.fetching strategy values.
var ClosedSetDataFetching = map[string]bool{
	"on-mount":               true,
	"prefetch":               true,
	"stale-while-revalidate": true,
	"graphql":                true,
	"none":                   true,
}

// The other three closed sets that used to live here — ClosedSetDataCaching,
// ClosedSetAuthStrategy, ClosedSetErrorsRetry — are gone, along with the
// strategy gate in ValidateBlueprintScope that was their only caller. All
// three named a different concept from the one the schema documents:
//
//	setting                 this file said            blueprint.schema.md says
//	data.caching            {none, per-route,         .strategy: {none, in-memory,
//	                         shared} — cache scope     local-storage, service-worker}
//	                                                   — cache location
//	auth.strategy           {none, session, jwt,      authorization.strategy:
//	                         oauth2} — authn           {role-based, permission-based,
//	                         mechanism                 attribute-based, none} — authz model
//	errors.retry            {none, reads, writes,     .retry.strategy: {none,
//	                         all}                      exponential-backoff,
//	                                                   immediate-once}
//
// Renaming the keys without correcting the vocabularies would have been worse
// than leaving them: the gate would finally have fired, and rejected every
// blueprint written from the schema table. `role-based` is not in
// {none, session, jwt, oauth2}.
//
// ValidateBlueprint (validate.go) already decodes these at the shapes the
// schema documents and gates them against the documented vocabularies. It is
// the single owner now. This file keeps the closed-KEY check; that one has no
// second implementation.
//
// The replacements below carry the schema's vocabularies, and their names say
// which node they gate — the old names said `DataCaching` for a value living
// at data.caching.strategy, which is part of how the shape drifted unnoticed.

// ClosedSetDataCachingStrategy — data.caching.strategy values
// (blueprint.schema.md § Section 4). Cache location, not cache scope.
var ClosedSetDataCachingStrategy = map[string]bool{
	"none":           true,
	"in-memory":      true,
	"local-storage":  true,
	"service-worker": true,
}

// ClosedSetDataOfflineStrategy — data.offline.strategy values.
var ClosedSetDataOfflineStrategy = map[string]bool{
	"none":              true,
	"read-only-cache":   true,
	"optimistic-writes": true,
}

// ClosedSetErrorsRetryStrategy — errors.retry.strategy values. The old
// ClosedSetErrorsRetry held {none, reads, writes, all}, which is the
// vocabulary of the sibling key errors.retry.applies-to — so it would have
// rejected every legal strategy and accepted none of them.
var ClosedSetErrorsRetryStrategy = map[string]bool{
	"none":                true,
	"exponential-backoff": true,
	"immediate-once":      true,
}

// ClosedSetErrorsRetryAppliesTo — errors.retry.applies-to values.
var ClosedSetErrorsRetryAppliesTo = map[string]bool{
	"reads":  true,
	"writes": true,
	"all":    true,
}

// ValidateBlueprintScope ensures the blueprint declares only top-level keys
// in the closed scope set, and that no targets: block is present.
func ValidateBlueprintScope(mode ValidationMode, path string, content []byte) []ValidationOutcome {
	var outcomes []ValidationOutcome

	var raw map[string]yaml.Node
	if err := yaml.Unmarshal(content, &raw); err != nil {
		outcomes = append(outcomes, NewOutcome(mode, "blueprint-invalid-yaml",
			fmt.Sprintf("parse %s: %v", path, err)))
		return outcomes
	}

	if _, hasTargets := raw["targets"]; hasTargets {
		outcomes = append(outcomes, NewOutcome(mode, "blueprint-topology-not-allowed",
			fmt.Sprintf("%s: targets: block is not in blueprint scope (topology lives in .parlay/adapter-set.yaml)", path)))
	}

	for key := range raw {
		if !blueprintAllowedScope[key] {
			outcomes = append(outcomes, NewOutcome(mode, "blueprint-scope-violation",
				fmt.Sprintf("%s: top-level key %q is outside the closed scope %s", path, key, blueprintScopeList())))
		}
	}

	// The closed-vocab strategy gate that used to sit here is gone; see the
	// table above the closed sets. Two independent bugs made it inert, which
	// is why the wrong vocabularies survived: it decoded data.caching as a
	// string and errors.retry as a string, but the schema documents both as
	// maps (data.caching.strategy, errors.retry.strategy). Any blueprint that
	// actually used either section failed the unmarshal, and the `err == nil`
	// guard then skipped the ENTIRE block in silence — including the
	// data.fetching check, which was correct. A gate that switches itself off
	// when the file gets more complete is worse than no gate; nothing reported
	// that validation had been skipped.
	//
	// ValidateBlueprint owns strategy validation now, at the documented shapes.

	return outcomes
}

// ValidateBlueprintStrategy walks every chosen strategy in the blueprint
// against the adapter's declared support. Out-of-vocabulary values fail
// with blueprint-strategy-unknown; in-vocabulary values not declared by
// the adapter fail with blueprint-strategy-unsupported.
//
// The strategyVocab map is the closed vocabulary per setting (e.g.
// "data.fetching" -> {"on-mount", "stale-while-revalidate", "manual"}). The
// adapterSupport map is what the relevant adapter declares it supports
// (subset of the vocabulary).
func ValidateBlueprintStrategy(mode ValidationMode, setting, value string, strategyVocab, adapterSupport map[string]bool) []ValidationOutcome {
	var outcomes []ValidationOutcome
	if value == "" {
		return outcomes
	}
	if !strategyVocab[value] {
		outcomes = append(outcomes, NewOutcome(mode, "blueprint-strategy-unknown",
			fmt.Sprintf("%s = %q is outside the closed vocabulary", setting, value)))
		return outcomes
	}
	// A nil adapterSupport means "no adapter in scope", not "the adapter
	// supports nothing". Without this, calling the vocab half from a context
	// that has no adapter — which is every context today — would report every
	// legal strategy as unsupported. Same missing-means-unknown rule the
	// baseline drift check uses for absent hashed sources.
	if adapterSupport == nil {
		return outcomes
	}
	if !adapterSupport[value] {
		outcomes = append(outcomes, NewOutcome(mode, "blueprint-strategy-unsupported",
			fmt.Sprintf("%s = %q is in vocabulary but the relevant adapter does not declare support", setting, value)))
	}
	return outcomes
}
