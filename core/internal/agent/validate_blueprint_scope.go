// parlay-feature: parlay-tool/multi-adapter
// parlay-component: blueprint-scope-and-precedence
//
// Pins blueprint's owned scope to {data, auth, errors, state, navigation,
// platform}, rejects topology declarations (which belong in adapter-set),
// and validates strategy choices against the relevant adapter's declared
// support.

package agent

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

var blueprintAllowedScope = map[string]bool{
	"app":        true, // app: header is informational; allowed but unused
	"data":       true,
	"auth":       true,
	"errors":     true,
	"state":      true,
	"navigation": true,
	"platform":   true,
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

// ClosedSetDataCaching — data.caching strategy values.
var ClosedSetDataCaching = map[string]bool{
	"none":      true,
	"per-route": true,
	"shared":    true,
}

// ClosedSetAuthStrategy — auth.strategy values.
var ClosedSetAuthStrategy = map[string]bool{
	"none":    true,
	"session": true,
	"jwt":     true,
	"oauth2":  true,
}

// ClosedSetErrorsRetry — errors.retry policy values.
var ClosedSetErrorsRetry = map[string]bool{
	"none":   true,
	"reads":  true,
	"writes": true,
	"all":    true,
}

// blueprintStrategySettings names every (path, vocabulary) pair the
// scope-and-precedence validator walks. The closed-vocab gate fires for
// every entry; the supports gate is the caller's responsibility (it has
// to know which adapter is in scope for the setting).
var blueprintStrategySettings = []struct {
	path  string
	vocab map[string]bool
}{
	{"data.fetching", ClosedSetDataFetching},
	{"data.caching", ClosedSetDataCaching},
	{"auth.strategy", ClosedSetAuthStrategy},
	{"errors.retry", ClosedSetErrorsRetry},
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
				fmt.Sprintf("%s: top-level key %q is outside the closed scope {data, auth, errors, state, navigation, platform}", path, key)))
		}
	}

	// Closed-vocab gate per architecture §5.4. We re-decode the raw blueprint
	// into a typed shape so we can read the strategy values without losing
	// type fidelity.
	var typed struct {
		Data struct {
			Fetching string `yaml:"fetching"`
			Caching  string `yaml:"caching"`
		} `yaml:"data"`
		Auth struct {
			Strategy string `yaml:"strategy"`
		} `yaml:"auth"`
		Errors struct {
			Retry string `yaml:"retry"`
		} `yaml:"errors"`
	}
	if err := yaml.Unmarshal(content, &typed); err == nil {
		strategyValues := map[string]string{
			"data.fetching": typed.Data.Fetching,
			"data.caching":  typed.Data.Caching,
			"auth.strategy": typed.Auth.Strategy,
			"errors.retry":  typed.Errors.Retry,
		}
		// Delegate rather than re-implement. This loop used to inline the
		// closed-vocab comparison, which meant blueprint-strategy-unknown had
		// two implementations and ValidateBlueprintStrategy — the documented
		// one — was called from nowhere. Its supports half was therefore dead
		// code, and blueprint-strategy-unsupported could never fire even
		// though blueprint.schema.md documents it.
		//
		// adapterSupport is nil because the scope check has no adapter in
		// scope; the supports half stays inert until a caller that knows the
		// adapter passes one. That is a narrower gap than a whole unreachable
		// validator, and an honest one.
		for _, s := range blueprintStrategySettings {
			value := strategyValues[s.path]
			if value == "" {
				continue
			}
			for _, o := range ValidateBlueprintStrategy(mode, s.path, value, s.vocab, nil) {
				o.Message = fmt.Sprintf("%s: %s", path, o.Message)
				outcomes = append(outcomes, o)
			}
		}
	}

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
