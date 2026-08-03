// parlay-feature: parlay-tool/adapter-authoring
// parlay-component: complete-adapter-validator
//
// The single, complete adapter validator.
//
// Before this, adapter validation was split across two disjoint halves that no
// single invocation ran: `validate --type adapter` checked ONLY the toolchain
// block (its own doc comment said "the rest is validated at registration"),
// while `register-adapter` checked only name/componentVocabulary/tokens. Six
// schema sections had no validator at all. The practical result was a false
// green: an adapter with no name, no kind, no shows, and no file-conventions
// validated OK — the worst possible signal for an agent authoring one, because
// it iterates to green and ships something broken.
//
// Two deliberate properties:
//
//   - It COLLECTS every finding. The registration validators returned on the
//     first error, so one bad component hid every other problem — an authoring
//     loop then needs one round-trip per defect.
//   - It emits real CODES. The registration validators returned bare prose
//     errors, which cannot enter the ValidationOutcome/--json model the rest of
//     the tool reports through.

package agent

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var pathPlaceholderRe = regexp.MustCompile(`\{([^}]*)\}`)

// ValidateAdapter checks an adapter file against every section of
// adapter.schema.md. Rules are kind-conditional: a presentation adapter owes
// the framework vocabulary and must not declare `supports:`; a backend adapter
// owes `supports:` and is not asked for widgets.
func ValidateAdapter(mode ValidationMode, path string, content []byte) []ValidationOutcome {
	var out []ValidationOutcome
	add := func(code, msg string) {
		out = append(out, NewOutcome(mode, code, fmt.Sprintf("%s: %s", path, msg)))
	}

	var a deepAdapter
	if err := yaml.Unmarshal(content, &a); err != nil {
		add("adapter-invalid-yaml", err.Error())
		return out
	}

	kind := a.ResolvedKind()

	// --- Identity ---------------------------------------------------------
	if strings.TrimSpace(a.Name) == "" {
		add("adapter-name-missing", "adapter declares no name:")
	} else if base := filepath.Base(path); base != "" {
		// The filename is how every resolver finds an adapter
		// (.parlay/adapters/<slug>.adapter.yaml), while diagnostics report the
		// name: field. A mismatch desynchronises the two silently.
		if slug := strings.TrimSuffix(base, ".adapter.yaml"); slug != base && slug != a.Name {
			add("adapter-name-slug-mismatch",
				fmt.Sprintf("name: %q does not match the filename slug %q — resolvers look the adapter up by filename", a.Name, slug))
		}
	}

	// --- §0 kind ----------------------------------------------------------
	if a.Kind != "" && !ClosedSetAdapterKinds[a.Kind] {
		add("adapter-kind-unknown",
			fmt.Sprintf("kind: %q is outside the closed set {presentation, transport, application, persistence}", a.Kind))
	}

	// --- §0.5 supports ----------------------------------------------------
	validateAdapterSupportsSection(a, kind, add)

	// --- §1 framework vocabulary (presentation only) ----------------------
	if kind == "presentation" {
		validateAdapterVocabularySection(a, add)
	}

	// --- §2 compositions (optional; shape when present) -------------------
	for name, c := range a.Compositions {
		if strings.TrimSpace(c.Trigger) == "" {
			add("adapter-composition-invalid", fmt.Sprintf("composition %q declares no trigger:", name))
		}
		if strings.TrimSpace(c.Wiring) == "" {
			add("adapter-composition-invalid", fmt.Sprintf("composition %q declares no wiring:", name))
		}
		if strings.TrimSpace(c.Description) == "" {
			add("adapter-composition-invalid", fmt.Sprintf("composition %q declares no description:", name))
		}
	}

	// --- §3 conventions (optional; shape when present) --------------------
	for name, c := range a.Conventions {
		if strings.TrimSpace(c.Rule) == "" {
			add("adapter-convention-invalid", fmt.Sprintf("convention %q declares no rule:", name))
		}
		if strings.TrimSpace(c.AppliesTo) == "" {
			add("adapter-convention-invalid", fmt.Sprintf("convention %q declares no applies-to:", name))
		}
	}

	// --- §4 file-conventions ----------------------------------------------
	validateAdapterFileConventions(a, add)

	// --- §5 design-system --------------------------------------------------
	for cat, e := range a.DesignSystem {
		if !ClosedSetDesignSystemSources[e.Source] {
			add("adapter-design-system-source-unknown",
				fmt.Sprintf("design-system.%s source %q is outside {framework, figma, not-defined}", cat, e.Source))
		}
	}

	// --- §6 patterns -------------------------------------------------------
	// Deliberately unvalidated. Section 6 defines patterns as "framework-level
	// taste, expressed as preferences rather than rules", with "any further
	// keys" permitted — the value space is open by design. A closed set here
	// would reject correct framework-appropriate values: the bundled go-cli
	// adapter uses error-placement `console` and confirmation `prompt`, which
	// are exactly right for a CLI and appear in no browser-framework enum.
	// Validating taste would be parlay inventing a rule the schema declines to
	// state.

	// --- §7 mount-strategies ----------------------------------------------
	for name, m := range a.MountStrategies {
		if strings.TrimSpace(m.Detection) == "" {
			add("adapter-mount-strategy-invalid", fmt.Sprintf("mount-strategy %q declares no detection:", name))
		}
		if strings.TrimSpace(m.Description) == "" {
			add("adapter-mount-strategy-invalid", fmt.Sprintf("mount-strategy %q declares no description:", name))
		}
		if !strings.Contains(m.Template, "{{") {
			add("adapter-mount-strategy-invalid",
				fmt.Sprintf("mount-strategy %q template has no {{placeholder}} — a template with nothing to substitute cannot mount a component", name))
		}
	}

	// --- §8/§9 componentVocabulary + tokens --------------------------------
	validateAdapterComponentVocabulary(a.ComponentVocabulary, add)
	validateAdapterTokensSection(a.Tokens, add)

	// --- §10 toolchain -----------------------------------------------------
	sourceRoot := ""
	if a.FileConventions != nil {
		sourceRoot = a.FileConventions.SourceRoot
	}
	for _, e := range ValidateToolchain(a.Toolchain, sourceRoot) {
		out = append(out, NewOutcome(mode, e.Code, fmt.Sprintf("%s: %s", path, e.Message)))
	}
	validateToolchainRequiredEnums(a.Toolchain, add)

	return out
}

// validateAdapterSupportsSection enforces the kind/supports contract in both
// directions. The "presentation must not declare supports" half was previously
// unreachable: both call sites of ValidateSupports skip presentation slots, so
// the branch existed and could never fire.
func validateAdapterSupportsSection(a deepAdapter, kind string, add func(code, msg string)) {
	if kind == "presentation" {
		if a.Supports != nil {
			add("adapter-supports-shape-mismatch",
				"kind: presentation must not declare supports: — the backend capability contract belongs to transport/application/persistence adapters")
		}
		return
	}
	if a.Supports == nil {
		add("adapter-supports-shape-mismatch",
			fmt.Sprintf("kind: %s requires a supports: block declaring the operation kinds, steps, policies and errors this layer implements", kind))
		return
	}
	for _, t := range outsideClosedSet(a.Supports.OperationKinds, ClosedSetOperationKinds) {
		add("adapter-supports-unknown-term", fmt.Sprintf("supports.operation_kinds entry %q is outside the closed vocabulary", t))
	}
	for _, t := range outsideClosedSet(a.Supports.Steps, ClosedSetSteps) {
		add("adapter-supports-unknown-term", fmt.Sprintf("supports.steps entry %q is outside the closed vocabulary", t))
	}
	for _, t := range outsideClosedSet(a.Supports.Policies, ClosedSetPolicies) {
		add("adapter-supports-unknown-term", fmt.Sprintf("supports.policies entry %q is outside the closed vocabulary", t))
	}
	for _, t := range outsideClosedSet(a.Supports.Errors, ClosedSetErrors) {
		add("adapter-supports-unknown-term", fmt.Sprintf("supports.errors entry %q is outside the closed vocabulary", t))
	}
}

// validateAdapterVocabularySection enforces surface.schema.md:177 — the adapter
// is responsible for mapping EVERY Show, Action and Flow to a framework
// implementation. An adapter missing an entry leaves codegen with no widget for
// a surface term a designer may legitimately write.
func validateAdapterVocabularySection(a deepAdapter, add func(code, msg string)) {
	for _, sec := range []struct {
		name  string
		got   map[string]interface{}
		vocab map[string]bool
	}{
		{"shows", a.Shows, ClosedSetShows},
		{"actions", a.Actions, ClosedSetActions},
		{"flows", a.Flows, ClosedSetFlows},
	} {
		if len(sec.got) == 0 {
			add("adapter-vocabulary-incomplete",
				fmt.Sprintf("kind: presentation requires a %s: section mapping every %s term to a framework widget", sec.name, strings.TrimSuffix(sec.name, "s")))
			continue
		}
		var missing []string
		for term := range sec.vocab {
			if _, ok := sec.got[term]; !ok {
				missing = append(missing, term)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			add("adapter-vocabulary-incomplete",
				fmt.Sprintf("%s: is missing %d term(s): %s", sec.name, len(missing), strings.Join(missing, ", ")))
		}
		var unknown []string
		for term := range sec.got {
			if !sec.vocab[term] {
				unknown = append(unknown, term)
			}
		}
		sort.Strings(unknown)
		for _, t := range unknown {
			add("adapter-vocabulary-unknown-term",
				fmt.Sprintf("%s.%s is not a term in the surface vocabulary", sec.name, t))
		}
	}
}

func validateAdapterFileConventions(a deepAdapter, add func(code, msg string)) {
	fc := a.FileConventions
	if fc == nil {
		add("adapter-file-conventions-missing",
			"adapter declares no file-conventions: — nothing can decide where generated code goes")
		return
	}
	if strings.TrimSpace(fc.SourceRoot) == "" {
		// Not cosmetic: an empty source-root silently disables the
		// toolchain write-set containment rule.
		add("adapter-source-root-missing",
			"file-conventions.source-root is required — every path template and every toolchain write-set is bounded by it")
	}
	if strings.TrimSpace(fc.Naming) == "" {
		add("adapter-naming-unknown",
			"file-conventions.naming is required — without it path templates silently fall back to kebab-case")
	} else if !ClosedSetNaming[fc.Naming] {
		add("adapter-naming-unknown",
			fmt.Sprintf("file-conventions.naming %q is outside {kebab-case, snake_case, PascalCase, camelCase}", fc.Naming))
	}
	if strings.TrimSpace(fc.ComponentPattern) == "" {
		add("adapter-file-conventions-incomplete", "file-conventions.component-pattern is required")
	}
	if strings.TrimSpace(fc.EntryPoint) == "" {
		add("adapter-file-conventions-incomplete", "file-conventions.entry-point is required")
	}

	// paths: templates must only use placeholders expandTemplate substitutes —
	// an unknown one expands to a literal brace, and a path with a brace in it
	// is not a path.
	for field, tmpl := range fc.Paths.All() {
		for _, m := range pathPlaceholderRe.FindAllStringSubmatch(tmpl, -1) {
			if !KnownPathPlaceholders[m[1]] {
				add("adapter-path-template-invalid",
					fmt.Sprintf("file-conventions.paths.%s uses unknown placeholder {%s} — known: {feature} {name} {entity} {Feature} {Name} {Entity}", field, m[1]))
			}
		}
	}

	// packages: is the shared-code directory map. It never derives a plan row,
	// but `parlay simplify` resolves an extracted helper's destination from it,
	// so an empty value is a silent wrong answer.
	for name, dir := range fc.Packages {
		if strings.TrimSpace(dir) == "" {
			add("adapter-packages-invalid",
				fmt.Sprintf("file-conventions.packages.%s is empty — it must name a directory", name))
		}
	}
}

func validateAdapterComponentVocabulary(v *deepComponentVocabulary, add func(code, msg string)) {
	if v == nil {
		return
	}
	if strings.TrimSpace(v.Name) == "" || !strings.Contains(v.Name, "@") {
		add("adapter-component-vocabulary-invalid",
			fmt.Sprintf("componentVocabulary.name %q must include @<version> (e.g. clarity@17) — layouts pin the vocabulary revision they were authored against", v.Name))
	}
	seen := map[string]bool{}
	for _, c := range v.Components {
		if strings.TrimSpace(c.Type) == "" {
			add("adapter-component-vocabulary-invalid", "componentVocabulary component is missing type:")
			continue
		}
		if seen[c.Type] {
			add("adapter-component-vocabulary-invalid", fmt.Sprintf("componentVocabulary declares component %q twice", c.Type))
		}
		seen[c.Type] = true

		if c.Category == "" {
			add("adapter-component-vocabulary-invalid", fmt.Sprintf("component %q declares no category:", c.Type))
		} else if !ClosedSetComponentCategories[c.Category] {
			add("adapter-component-vocabulary-invalid",
				fmt.Sprintf("component %q category %q is outside {container, leaf, data-shape}", c.Type, c.Category))
		}
		if c.Category == "container" && len(c.AllowedChildren) == 0 {
			add("adapter-component-vocabulary-invalid",
				fmt.Sprintf("container component %q declares no allowed-children — nothing constrains what a layout may nest inside it", c.Type))
		}
		for _, p := range c.Properties {
			if UniversalContainerFields[p.Name] {
				add("adapter-component-vocabulary-invalid",
					fmt.Sprintf("component %q re-declares universal container field %q — those live in the layout schema", c.Type, p.Name))
			}
			if strings.TrimSpace(p.Type) == "" {
				add("adapter-component-vocabulary-invalid", fmt.Sprintf("component %q property %q is missing type:", c.Type, p.Name))
				continue
			}
			if !ClosedSetPropertyTypes[p.Type] {
				add("adapter-component-vocabulary-invalid",
					fmt.Sprintf("component %q property %q type %q is outside {string, token-reference, enum, boolean, int, child-list}", c.Type, p.Name, p.Type))
				continue
			}
			if p.Type == "enum" && len(p.EnumValues) == 0 {
				add("adapter-component-vocabulary-invalid",
					fmt.Sprintf("component %q property %q is type enum but declares no enum-values", c.Type, p.Name))
			}
			if p.Type == "child-list" && len(p.ChildTypes) == 0 {
				add("adapter-component-vocabulary-invalid",
					fmt.Sprintf("component %q property %q is type child-list but declares no child-types", c.Type, p.Name))
			}
		}
	}
}

func validateAdapterTokensSection(t *deepAdapterTokens, add func(code, msg string)) {
	if t == nil {
		return
	}
	if len(t.Modes) == 0 {
		add("adapter-tokens-invalid", "tokens: declares no modes (typically `modes: [light]`)")
	}
	declared := map[string]bool{}
	for _, m := range t.Modes {
		if strings.TrimSpace(m) == "" {
			add("adapter-tokens-invalid", "tokens.modes contains an empty entry")
			continue
		}
		declared[m] = true
	}

	seenSpacing := map[string]bool{}
	seenOrder := map[int]string{}
	for _, s := range t.Spacing {
		if strings.TrimSpace(s.Name) == "" {
			add("adapter-tokens-invalid", "tokens.spacing entry is missing name:")
			continue
		}
		if seenSpacing[s.Name] {
			add("adapter-tokens-invalid", fmt.Sprintf("tokens.spacing declares %q twice", s.Name))
		}
		seenSpacing[s.Name] = true
		if strings.TrimSpace(s.EmitForm) == "" {
			add("adapter-tokens-invalid", fmt.Sprintf("tokens.spacing.%s is missing emit-form:", s.Name))
		}
		if prev, ok := seenOrder[s.Order]; ok {
			add("adapter-tokens-invalid",
				fmt.Sprintf("tokens.spacing.%s reuses order %d (already used by %q) — order is the scale, so it must be unique", s.Name, s.Order, prev))
		}
		seenOrder[s.Order] = s.Name
	}

	seenColor := map[string]bool{}
	for _, c := range t.Color {
		if strings.TrimSpace(c.Name) == "" {
			add("adapter-tokens-invalid", "tokens.color entry is missing name:")
			continue
		}
		if seenColor[c.Name] {
			add("adapter-tokens-invalid", fmt.Sprintf("tokens.color declares %q twice", c.Name))
		}
		seenColor[c.Name] = true
		if !ClosedSetColorTones[c.Tone] {
			add("adapter-tokens-invalid",
				fmt.Sprintf("tokens.color.%s tone %q is outside {neutral, info, warning, danger, success}", c.Name, c.Tone))
		}
		covered := map[string]bool{}
		for _, ef := range c.EmitForms {
			if parts := strings.SplitN(ef, ":", 2); len(parts) > 0 && parts[0] != "" {
				covered[parts[0]] = true
			}
		}
		var missing []string
		for m := range declared {
			if !covered[m] {
				missing = append(missing, m)
			}
		}
		sort.Strings(missing)
		for _, m := range missing {
			add("adapter-tokens-invalid",
				fmt.Sprintf("tokens.color.%s has no emit-form for declared mode %q", c.Name, m))
		}
	}

	seenType := map[string]bool{}
	for _, ty := range t.Typography {
		if strings.TrimSpace(ty.Name) == "" {
			add("adapter-tokens-invalid", "tokens.typography entry is missing name:")
			continue
		}
		if seenType[ty.Name] {
			add("adapter-tokens-invalid", fmt.Sprintf("tokens.typography declares %q twice", ty.Name))
		}
		seenType[ty.Name] = true
		if strings.TrimSpace(ty.EmitForm) == "" {
			add("adapter-tokens-invalid", fmt.Sprintf("tokens.typography.%s is missing emit-form:", ty.Name))
		}
		if !ClosedSetTypographyUseSites[ty.UseSite] {
			add("adapter-tokens-invalid",
				fmt.Sprintf("tokens.typography.%s use-site %q is outside {heading-page, heading-section, body, caption}", ty.Name, ty.UseSite))
		}
	}
}

// validateToolchainRequiredEnums covers the two Section 10 fields the schema
// marks Required and ValidateToolchain never checked: source and stage.
func validateToolchainRequiredEnums(tc *Toolchain, add func(code, msg string)) {
	if tc == nil {
		return
	}
	check := func(e ToolchainEntry, kindLabel string) {
		at := fmt.Sprintf("toolchain.%s[%s]", kindLabel, e.Name())
		switch e.Source {
		case "community", "first-party", "project":
		case "":
			add("toolchain-source-missing", fmt.Sprintf("%s: source: is required — a reviewer weighs a tool by its provenance", at))
		default:
			add("toolchain-source-missing", fmt.Sprintf("%s: source %q is outside {community, first-party, project}", at, e.Source))
		}
		switch e.Stage {
		case "pre-emit", "post-emit":
		case "":
			add("toolchain-stage-unknown", fmt.Sprintf("%s: stage: is required — codegen needs to know whether to run it before or after emission", at))
		default:
			add("toolchain-stage-unknown", fmt.Sprintf("%s: stage %q is outside {pre-emit, post-emit}", at, e.Stage))
		}
	}
	for _, e := range tc.Skills {
		check(e, "skill")
	}
	for _, e := range tc.MCP {
		check(e, "mcp")
	}
}
