package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// adapterPaths mirrors file-conventions.paths from adapter.schema.md Section 4.
type adapterPaths struct {
	Component       string   `yaml:"component"`
	ComponentExtras []string `yaml:"component-extras"`
	Test            string   `yaml:"test"`
	Model           string   `yaml:"model"`
	Service         string   `yaml:"service"`
	Types           string   `yaml:"types"`
	FeatureRoutes   string   `yaml:"feature-routes"`
	Routes          string   `yaml:"routes"`
	// Controller and Module are application-adapter (backend) templates: a
	// feature-driven target emits one module/controller/service trio per
	// parlay feature, keyed off {feature}. Presentation adapters leave these
	// empty. See deriveApplicationPlan.
	Controller string `yaml:"controller"`
	Module     string `yaml:"module"`
	// Seed is where the composed runtime seed lands — the one dataset the
	// whole prototype boots from. Parlay computes the data (scaffold_seed.go)
	// and knows nothing about how it is rendered: the template says where the
	// file goes, the adapter's framework decides its shape. Absence is not an
	// error — an adapter that declares no seed path gets no seed row, exactly
	// as with paths.model.
	Seed string `yaml:"seed"`
	// Store is where the shared runtime state lands — whatever holds domain
	// state between two user actions. Parlay declares only THAT a project has
	// one and where its code goes, never its shape: for a stateful client
	// that is a root-provided object, for a CLI a file or a database, for a
	// static generator nothing at all. Absence is the default and is correct
	// for most frameworks.
	Store string `yaml:"store"`
}

type adapterFileConventions struct {
	SourceRoot string       `yaml:"source-root"`
	Naming     string       `yaml:"naming"`
	Paths      adapterPaths `yaml:"paths"`
}

type adapterForPlan struct {
	FileConventions adapterFileConventions `yaml:"file-conventions"`
	// Toolchain is the adapter's Section-10 external-skill/MCP block. Additive:
	// the plan derivers ignore it; toolchain-plan reads it so multi-target
	// resolution (adaptersForProject) surfaces it per kind for free.
	Toolchain *agent.Toolchain `yaml:"toolchain"`
}

// planEntry is one derived plan row.
type planEntry struct {
	Path    string   `yaml:"path" json:"path"`
	Sources []string `yaml:"sources" json:"sources"`
}

// derivedPlan is the deterministic part of a buildfile's plan: the rows that
// follow mechanically from the component set and the adapter's path
// templates. Cross-cutting rows are not here — routing those depends on
// what exists on disk and on the other features in the same pass, which is
// the validator's job, not a template's.
type derivedPlan struct {
	Creates     []planEntry `json:"creates"`
	Undecidable []string    `json:"undecidable,omitempty"`
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// applyNaming converts a component or feature name to the adapter's file
// naming convention.
func applyNaming(s, naming string) string {
	words := splitWords(s)
	switch naming {
	case "snake_case":
		return strings.Join(lowerAll(words), "_")
	case "PascalCase":
		return strings.Join(titleAll(words), "")
	case "camelCase":
		t := titleAll(words)
		if len(t) > 0 {
			t[0] = strings.ToLower(t[0])
		}
		return strings.Join(t, "")
	default: // kebab-case
		return strings.Join(lowerAll(words), "-")
	}
}

func splitWords(s string) []string {
	// Split on non-alphanumerics first, then on camelCase humps, so
	// "expenseWizard" and "expense-wizard" and "expense_wizard" all yield
	// the same words and therefore the same path.
	var words []string
	for _, chunk := range nonAlnum.Split(s, -1) {
		if chunk == "" {
			continue
		}
		start := 0
		for i := 1; i < len(chunk); i++ {
			if unicode.IsUpper(rune(chunk[i])) && !unicode.IsUpper(rune(chunk[i-1])) {
				words = append(words, chunk[start:i])
				start = i
			}
		}
		words = append(words, chunk[start:])
	}
	return words
}

func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, w := range in {
		out[i] = strings.ToLower(w)
	}
	return out
}

func titleAll(in []string) []string {
	out := make([]string, len(in))
	for i, w := range in {
		if w == "" {
			continue
		}
		out[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return out
}

// expandTemplate substitutes the placeholders adapter.schema.md defines.
// It is a plain string substitution with no conditionals — a template that
// leaves an unrecognized {placeholder} behind is reported rather than
// emitted, because a path with a literal brace in it is not a path.
func expandTemplate(tmpl, feature, name, entity, naming string) (string, error) {
	repl := map[string]string{
		"{feature}": applyNaming(feature, naming),
		"{name}":    applyNaming(name, naming),
		"{entity}":  applyNaming(entity, naming),
		"{Feature}": applyNaming(feature, "PascalCase"),
		"{Name}":    applyNaming(name, "PascalCase"),
		"{Entity}":  applyNaming(entity, "PascalCase"),
	}
	out := tmpl
	for k, v := range repl {
		out = strings.ReplaceAll(out, k, v)
	}
	if i := strings.Index(out, "{"); i >= 0 {
		return "", fmt.Errorf("template %q left an unresolved placeholder at %q", tmpl, out[i:])
	}
	return out, nil
}

// derivePlanCreates computes the plan.creates rows implied by a feature's
// components, its entities, and the adapter's path templates.
//
// Every row cites the component that produced it, so `parlay validate
// --deep`'s "every component has a plan row referencing it" check passes by
// construction rather than by the agent having remembered.
//
// The two categories that caused P2-16 — test files and section-derived
// artifacts (models, the route tables) — are emitted here. Codegen is
// required to write them and build-feature emitted no rows for them, so a
// literal reading of the plan allowlist forbade exactly the files codegen
// mandates; 19 files were written outside it in one run. With templates they
// become ordinary derived rows and the exemption stops being load-bearing.
func derivePlanCreates(feature string, components []string, entities []string, ad adapterForPlan) derivedPlan {
	fc := ad.FileConventions
	naming := fc.Naming
	root := fc.SourceRoot

	var out derivedPlan
	seen := map[string]int{} // path -> index in out.Creates, for merging sources

	add := func(p string, source string) {
		full := path.Join(root, p)
		if i, ok := seen[full]; ok {
			for _, s := range out.Creates[i].Sources {
				if s == source {
					return
				}
			}
			out.Creates[i].Sources = append(out.Creates[i].Sources, source)
			return
		}
		seen[full] = len(out.Creates)
		out.Creates = append(out.Creates, planEntry{Path: full, Sources: []string{source}})
	}

	// Deduplicated: a missing template is one fact about the adapter, not
	// one fact per component. Repeating it 34 times buries every other
	// reason derivation came up short.
	reported := map[string]bool{}
	undecidable := func(what string) {
		if reported[what] {
			return
		}
		reported[what] = true
		out.Undecidable = append(out.Undecidable, what)
	}

	// Components, in sorted order so the same inputs always produce the
	// same plan — the plan is compared against on later runs, and a
	// reordered plan reads as a changed one.
	sorted := append([]string{}, components...)
	sort.Strings(sorted)

	for _, name := range sorted {
		source := "component/" + name
		// A slice, not a map: Go randomizes map iteration order, and these
		// rows are appended in iteration order. The plan is compared
		// against on later runs, so a reshuffled plan reads as a changed
		// one.
		for _, t := range []struct{ label, tmpl string }{
			{"component", fc.Paths.Component},
			{"test", fc.Paths.Test},
		} {
			label, tmpl := t.label, t.tmpl
			if tmpl == "" {
				undecidable(fmt.Sprintf("no file-conventions.paths.%s template; %s rows must be authored by hand", label, label))
				continue
			}
			p, err := expandTemplate(tmpl, feature, name, "", naming)
			if err != nil {
				undecidable(err.Error())
				continue
			}
			add(p, source)
		}
		for _, tmpl := range fc.Paths.ComponentExtras {
			p, err := expandTemplate(tmpl, feature, name, "", naming)
			if err != nil {
				undecidable(err.Error())
				continue
			}
			add(p, source)
		}
	}

	// Domain entities. Sourced as section/models rather than as a component:
	// the merged model layer belongs to no single feature, which is why
	// nothing owned these rows before.
	if len(entities) > 0 {
		if fc.Paths.Model == "" {
			undecidable("no file-conventions.paths.model template; model rows must be authored by hand")
		} else {
			ents := append([]string{}, entities...)
			sort.Strings(ents)
			for _, e := range ents {
				p, err := expandTemplate(fc.Paths.Model, feature, "", e, naming)
				if err != nil {
					undecidable(err.Error())
					continue
				}
				add(p, "section/models")
			}
		}
	}

	// The composed runtime seed. One file for the whole project, sourced
	// section/seed and derived from the same entity set as the models —
	// which is why it is gated the same way, and why it sits here rather
	// than among the per-feature support files below.
	//
	// It is claimed as a create by every feature that has entities, exactly
	// as the model rows are. The alternative — claiming it only for whichever
	// feature happens to build first — makes the plan depend on build order,
	// and the plan is compared against on later runs.
	if len(entities) > 0 && fc.Paths.Seed != "" {
		p, err := expandTemplate(fc.Paths.Seed, feature, "", "", naming)
		if err != nil {
			undecidable(err.Error())
		} else {
			add(p, "section/seed")
		}
	}

	// The shared store. Derived from the same entity set as the models and the
	// seed — the store's shape IS the entity set — so it regenerates on the
	// same `sections.models` trigger and is gated the same way.
	//
	// Every feature that participates in the runtime claims this row, which is
	// what makes composition-flow-unsatisfiable decidable: a feature whose plan
	// omits the store is a feature whose writes stay local, and that is exactly
	// the condition a cross-feature flow assertion cannot survive.
	if len(entities) > 0 && fc.Paths.Store != "" {
		p, err := expandTemplate(fc.Paths.Store, feature, "", "", naming)
		if err != nil {
			undecidable(err.Error())
		} else {
			add(p, "section/store")
		}
	}

	// Feature-scoped support files. These exist on disk in every feature of
	// the regression project and appeared in no derived plan until now —
	// they are per-feature, not per-component, so no component row implies
	// them.
	if len(sorted) > 0 {
		for _, t := range []struct{ label, tmpl string }{
			{"service", fc.Paths.Service},
			{"types", fc.Paths.Types},
		} {
			if t.tmpl == "" {
				continue
			}
			if p, err := expandTemplate(t.tmpl, feature, "", "", naming); err == nil {
				add(p, "section/"+t.label)
			} else {
				undecidable(err.Error())
			}
		}
	}

	// Route tables. The feature's own table is this feature's to create; the
	// project table is shared, so it is a modify for every feature after the
	// first and is left to the validator's project pass rather than claimed
	// here as a create.
	if fc.Paths.FeatureRoutes != "" && len(sorted) > 0 {
		if p, err := expandTemplate(fc.Paths.FeatureRoutes, feature, "", "", naming); err == nil {
			add(p, "section/routes")
		} else {
			undecidable(err.Error())
		}
	}

	if len(out.Creates) == 0 && len(out.Undecidable) == 0 && len(components) > 0 {
		undecidable("adapter declares no file-conventions.paths block; plan derivation unavailable")
	}
	return out
}

// adaptersForProject reads .parlay/adapter-set.yaml and loads the adapter
// file occupying each slot, keyed by kind. It is the multi-target counterpart
// of firstAdapterFile: instead of grabbing the lexically-first adapter, it
// resolves the right adapter per kind from the pinned topology. Returns an
// error if the adapter-set is unreadable or any pinned adapter file is
// missing.
func adaptersForProject(cfg *config.Context) (map[string]adapterForPlan, *parser.AdapterSet, error) {
	as, err := parser.ParseAdapterSet(cfg.AdapterSetPath())
	if err != nil {
		return nil, nil, err
	}
	out := map[string]adapterForPlan{}
	for kind, tgt := range as.Targets {
		p := filepath.Join(cfg.AdaptersPath(), tgt.Adapter+".adapter.yaml")
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, fmt.Errorf("adapter for target %s (%q): %w", kind, tgt.Adapter, err)
		}
		var ad adapterForPlan
		if err := yaml.Unmarshal(data, &ad); err != nil {
			return nil, nil, fmt.Errorf("parse adapter %s: %w", p, err)
		}
		out[kind] = ad
	}
	return out, as, nil
}

// derivePlanTargets derives one plan per filled adapter-set slot. Each kind is
// driven by its natural inputs:
//   - presentation is component-driven (reuses derivePlanCreates);
//   - application is feature-driven (one module/controller/service trio);
//   - persistence is entity-driven (the shared schema, one row for all
//     entities).
//
// Every slot's rows are pathed under that slot's root: from the adapter-set,
// which overrides the adapter's own source-root — the topology, not the
// adapter, decides where each target lands. In a project with a persistence
// slot the domain entities become that slot's schema, so the presentation slot
// derives no model rows; entities belong to exactly one target.
func derivePlanTargets(feature string, presComponents []string, hasOperations bool, entities []string, as *parser.AdapterSet, adapters map[string]adapterForPlan) map[string]derivedPlan {
	out := map[string]derivedPlan{}
	_, hasPersistence := as.Targets["persistence"]
	for kind, tgt := range as.Targets {
		ad, ok := adapters[kind]
		if !ok {
			continue
		}
		ad.FileConventions.SourceRoot = tgt.Root
		switch kind {
		case "presentation":
			ents := entities
			if hasPersistence {
				ents = nil // models live in the persistence target
			}
			out[kind] = derivePlanCreates(feature, presComponents, ents, ad)
		case "application":
			out[kind] = deriveApplicationPlan(feature, hasOperations, ad)
		case "persistence":
			// The persistence schema is entity-driven and component-less;
			// derivePlanCreates with no components emits exactly the model
			// rows (which collapse to the single shared schema file).
			out[kind] = derivePlanCreates(feature, nil, entities, ad)
		default:
			// transport and any future kinds are out of the vertical slice.
		}
	}
	return out
}

// deriveApplicationPlan emits the per-feature backend files an application
// adapter produces — module, controller, service — for a feature that
// declares operations. Unlike a presentation target there is no per-component
// fan-out: one trio per feature, keyed off {feature}. A feature with no
// operations gets no backend files.
func deriveApplicationPlan(feature string, hasOperations bool, ad adapterForPlan) derivedPlan {
	var out derivedPlan
	if !hasOperations {
		return out
	}
	fc := ad.FileConventions
	for _, t := range []struct{ label, tmpl string }{
		{"service", fc.Paths.Service},
		{"controller", fc.Paths.Controller},
		{"module", fc.Paths.Module},
	} {
		if t.tmpl == "" {
			out.Undecidable = append(out.Undecidable,
				fmt.Sprintf("no file-conventions.paths.%s template; %s rows must be authored by hand", t.label, t.label))
			continue
		}
		p, err := expandTemplate(t.tmpl, feature, "", "", fc.Naming)
		if err != nil {
			out.Undecidable = append(out.Undecidable, err.Error())
			continue
		}
		out.Creates = append(out.Creates, planEntry{Path: path.Join(fc.SourceRoot, p), Sources: []string{"section/operations"}})
	}
	return out
}

// scaffoldPlanCmd derives the mechanical plan.creates rows for a feature and
// prints them. It does not write the buildfile: the point of the first
// release is to be able to compare derivation against what agents actually
// authored, and a command that overwrites the thing you are comparing
// against cannot be used for that.
var scaffoldPlanCmd = &cobra.Command{
	Use:   "scaffold-plan @<feature>",
	Short: "Derive a feature's mechanical plan.creates rows from adapter path templates (JSON output)",
	Args:  cobra.ExactArgs(1),
	RunE:  runScaffoldPlan,
}

var scaffoldPlanCompare bool

func init() {
	scaffoldPlanCmd.Flags().BoolVar(&scaffoldPlanCompare, "compare", false,
		"Diff the derived rows against the buildfile's existing plan.creates")
}

type scaffoldPlanOutput struct {
	Feature      string      `json:"feature"`
	Adapter      string      `json:"adapter,omitempty"`
	Creates      []planEntry `json:"creates,omitempty"`
	Undecidable  []string    `json:"undecidable,omitempty"`
	OnlyDerived  []string    `json:"only_in_derived,omitempty"`
	OnlyAuthored []string    `json:"only_in_authored,omitempty"`
	Agrees       *bool       `json:"agrees,omitempty"`
	// Targets carries the per-kind derived plans for a multi-target
	// (adapter-set) buildfile, mirroring plan.targets.<kind> in the schema.
	// Empty for single-target projects, which populate Creates instead.
	Targets map[string]derivedPlan `json:"targets,omitempty"`
}

func runScaffoldPlan(cmd *cobra.Command, args []string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	slug := parser.FeatureSlug(args[0])
	buildfilePath := filepath.Join(cfg.BuildPath(slug), "buildfile.yaml")
	data, err := os.ReadFile(buildfilePath)
	if err != nil {
		return fmt.Errorf("feature %s: %w", slug, err)
	}

	var bf struct {
		Components map[string]struct{}  `yaml:"components"`
		AdapterSet string               `yaml:"adapter-set"`
		Operations map[string]yaml.Node `yaml:"operations"`
		Targets    struct {
			Presentation struct {
				Components map[string]struct{} `yaml:"components"`
			} `yaml:"presentation"`
		} `yaml:"targets"`
		Plan struct {
			Creates []planEntry `yaml:"creates"`
			Targets map[string]struct {
				Creates []planEntry `yaml:"creates"`
			} `yaml:"targets"`
		} `yaml:"plan"`
	}
	if err := yaml.Unmarshal(data, &bf); err != nil {
		return fmt.Errorf("parse %s: %w", buildfilePath, err)
	}

	// Multi-target (adapter-set) buildfiles derive one plan per slot, each
	// pathed under its target's root. Single-target buildfiles keep the
	// original single-adapter path below.
	if bf.AdapterSet != "" {
		var presComps []string
		for name := range bf.Targets.Presentation.Components {
			presComps = append(presComps, name)
		}
		authored := map[string][]planEntry{}
		for kind, t := range bf.Plan.Targets {
			authored[kind] = t.Creates
		}
		return runScaffoldPlanMultiTarget(cmd, cfg, slug, presComps, len(bf.Operations) > 0, authored)
	}

	var comps []string
	for name := range bf.Components {
		comps = append(comps, name)
	}

	adapterPath := firstAdapterFile(cfg.AdaptersPath())
	if adapterPath == "" {
		return fmt.Errorf("no adapter found under %s", cfg.AdaptersPath())
	}
	adData, err := os.ReadFile(adapterPath)
	if err != nil {
		return err
	}
	var ad adapterForPlan
	if err := yaml.Unmarshal(adData, &ad); err != nil {
		return fmt.Errorf("parse adapter %s: %w", adapterPath, err)
	}

	// The ACTIVE root's model, not the repo-level one. RepoRoot()
	// resolves to the parent for a child root, so joining it here made
	// plan derivation read a different file from the one
	// declaredCapabilityEntities validates against — and each child
	// root has an independent domain-model.yaml by contract.
	entities := domainEntityNamesAt(cfg.DomainModelPath())
	derived := derivePlanCreates(slug, comps, entities, ad)

	out := scaffoldPlanOutput{
		Feature:     slug,
		Adapter:     filepath.Base(adapterPath),
		Creates:     derived.Creates,
		Undecidable: derived.Undecidable,
	}

	if scaffoldPlanCompare {
		derivedSet, authoredSet := map[string]bool{}, map[string]bool{}
		for _, e := range derived.Creates {
			derivedSet[e.Path] = true
		}
		for _, e := range bf.Plan.Creates {
			authoredSet[e.Path] = true
		}
		for p := range derivedSet {
			if !authoredSet[p] {
				out.OnlyDerived = append(out.OnlyDerived, p)
			}
		}
		for p := range authoredSet {
			if !derivedSet[p] {
				out.OnlyAuthored = append(out.OnlyAuthored, p)
			}
		}
		sort.Strings(out.OnlyDerived)
		sort.Strings(out.OnlyAuthored)
		agrees := len(out.OnlyDerived) == 0 && len(out.OnlyAuthored) == 0
		out.Agrees = &agrees
	}

	buf, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(buf))
	return nil
}

// runScaffoldPlanMultiTarget derives and prints the per-target plan for a
// multi-target (adapter-set) buildfile. Each slot's rows are pathed under its
// own root; --compare flattens all targets into one path set (the roots make
// paths globally unique) and reports aggregate agreement against the authored
// plan.targets.<kind>.creates rows.
func runScaffoldPlanMultiTarget(cmd *cobra.Command, cfg *config.Context, slug string, presComps []string, hasOperations bool, authored map[string][]planEntry) error {
	adapters, as, err := adaptersForProject(cfg)
	if err != nil {
		return err
	}
	entities := domainEntityNamesAt(cfg.DomainModelPath())
	derived := derivePlanTargets(slug, presComps, hasOperations, entities, as, adapters)

	out := scaffoldPlanOutput{
		Feature: slug,
		Targets: derived,
	}

	if scaffoldPlanCompare {
		derivedSet, authoredSet := map[string]bool{}, map[string]bool{}
		for _, dp := range derived {
			for _, e := range dp.Creates {
				derivedSet[e.Path] = true
			}
		}
		for _, rows := range authored {
			for _, e := range rows {
				authoredSet[e.Path] = true
			}
		}
		for p := range derivedSet {
			if !authoredSet[p] {
				out.OnlyDerived = append(out.OnlyDerived, p)
			}
		}
		for p := range authoredSet {
			if !derivedSet[p] {
				out.OnlyAuthored = append(out.OnlyAuthored, p)
			}
		}
		sort.Strings(out.OnlyDerived)
		sort.Strings(out.OnlyAuthored)
		agrees := len(out.OnlyDerived) == 0 && len(out.OnlyAuthored) == 0
		out.Agrees = &agrees
	}

	buf, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(buf))
	return nil
}

// domainEntityNamesAt reads the top-level entity keys from a
// domain-model.yaml. A missing or unparseable file yields no entities rather
// than an error: a project can legitimately have none, and plan derivation
// for components does not depend on the model layer.
// domainEntityNamesAt reads the entity names out of domain-model.yaml.
//
// `entities:` is a LIST of objects with a `name:` field, which is what
// domain-model.schema.md documents and what every real project on disk
// carries. This decoded it as a map keyed by entity name — a shape nothing
// writes — so yaml.v3 failed the unmarshal, the function returned nil, and
// derivation silently produced no section/models rows for any project that
// has ever existed. Silently, because a nil entity list is indistinguishable
// here from a project that genuinely has no entities, and the `if
// len(entities) > 0` guard downstream then skips the block without reporting
// anything undecidable.
//
// The map form is still accepted on the second attempt. It costs one extra
// decode and covers a hand-written shorthand a person might reasonably try.
func domainEntityNamesAt(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var asList struct {
		Entities []struct {
			Name string `yaml:"name"`
		} `yaml:"entities"`
	}
	if err := yaml.Unmarshal(data, &asList); err == nil && len(asList.Entities) > 0 {
		var names []string
		for _, e := range asList.Entities {
			if e.Name != "" {
				names = append(names, e.Name)
			}
		}
		sort.Strings(names)
		return names
	}

	var asMap struct {
		Entities map[string]interface{} `yaml:"entities"`
	}
	if err := yaml.Unmarshal(data, &asMap); err != nil {
		return nil
	}
	var names []string
	for k := range asMap.Entities {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
