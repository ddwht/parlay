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
	Adapter      string      `json:"adapter"`
	Creates      []planEntry `json:"creates"`
	Undecidable  []string    `json:"undecidable,omitempty"`
	OnlyDerived  []string    `json:"only_in_derived,omitempty"`
	OnlyAuthored []string    `json:"only_in_authored,omitempty"`
	Agrees       *bool       `json:"agrees,omitempty"`
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
		Components map[string]struct{} `yaml:"components"`
		Plan       struct {
			Creates []planEntry `yaml:"creates"`
		} `yaml:"plan"`
	}
	if err := yaml.Unmarshal(data, &bf); err != nil {
		return fmt.Errorf("parse %s: %w", buildfilePath, err)
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

	entities := domainEntityNamesAt(filepath.Join(cfg.RepoRoot(), "domain-model.yaml"))
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

// domainEntityNamesAt reads the top-level entity keys from a
// domain-model.yaml. A missing or unparseable file yields no entities rather
// than an error: a project can legitimately have none, and plan derivation
// for components does not depend on the model layer.
func domainEntityNamesAt(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var dm struct {
		Entities map[string]interface{} `yaml:"entities"`
	}
	if err := yaml.Unmarshal(data, &dm); err != nil {
		return nil
	}
	var names []string
	for k := range dm.Entities {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
