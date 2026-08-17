package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/feedback"
	"github.com/ddwht/parlay/core/internal/parser"
	"gopkg.in/yaml.v3"
)

// Validator checks a file's content against its schema.
type Validator func(path string, content []byte) error

// ValidateYAML checks that a file is valid YAML.
func ValidateYAML(path string, content []byte) error {
	var v interface{}
	if err := yaml.Unmarshal(content, &v); err != nil {
		return fmt.Errorf("%s is not valid YAML: %w", path, err)
	}
	return nil
}

// ValidateBuildfile checks buildfile.yaml has required fields.
func ValidateBuildfile(path string, content []byte) error {
	if err := ValidateYAML(path, content); err != nil {
		return err
	}
	var bf struct {
		Feature    string `yaml:"feature"`
		Adapter    string `yaml:"adapter"`
		AdapterSet string `yaml:"adapter-set"`
		Targets    map[string]struct {
			Adapter string `yaml:"adapter"`
		} `yaml:"targets"`
		Components interface{} `yaml:"components"`
	}
	if err := yaml.Unmarshal(content, &bf); err != nil {
		return fmt.Errorf("buildfile structure invalid: %w", err)
	}
	if bf.Feature == "" {
		return fmt.Errorf("buildfile missing 'feature' field")
	}
	// v1 (frozen shape): a top-level adapter: is present. This branch is
	// byte-identical in behavior to before multi-target — a project that
	// sets adapter: never reaches the v2 code below, so single-target
	// buildfiles are provably untouched.
	if bf.Adapter != "" {
		return nil
	}
	// v2 (multi-target): accepted only when adapter-set: names the topology
	// AND a presentation target resolves an adapter. Anything short of that
	// keeps the exact legacy error string below, which the schema doc and
	// conformance match literally.
	if bf.AdapterSet != "" {
		if pres, ok := bf.Targets["presentation"]; ok && pres.Adapter != "" {
			return nil
		}
	}
	return fmt.Errorf("buildfile missing 'adapter' field")
}

// ValidateSurface checks surface.md has fragment headings with Shows fields.
func ValidateSurface(path string, content []byte) error {
	// surface.yaml is the target format for the surface artifact and is
	// what create-artifacts emits; surface.md is the legacy form kept
	// during the migration window. Dispatching on the file extension
	// matters because the two checks share no syntax: running the
	// markdown heading probe over YAML reports "surface.md has no fragment
	// headings (## )" for a perfectly valid surface.yaml, which made the
	// format the pipeline actually produces impossible to validate.
	if strings.HasSuffix(strings.ToLower(path), ".yaml") || strings.HasSuffix(strings.ToLower(path), ".yml") {
		return validateSurfaceYAML(path, content)
	}

	text := string(content)
	if !strings.Contains(text, "## ") {
		return fmt.Errorf("surface.md has no fragment headings (## )")
	}
	if !strings.Contains(text, "**Shows**:") {
		return fmt.Errorf("surface.md has no **Shows**: fields")
	}
	return nil
}

// validateSurfaceYAML checks the structural shape of a surface.yaml: valid
// YAML, a fragments list, and the per-fragment fields the downstream
// pipeline relies on.
func validateSurfaceYAML(path string, content []byte) error {
	if err := ValidateYAML(path, content); err != nil {
		return err
	}

	// Decode through the REAL parser before anything else.
	//
	// This used to decode only into the private struct below, which carries
	// feature, name, shows, source and page. yaml.v3 ignores keys a struct
	// does not declare, so every field the validator omitted was invisible to
	// it — including notes:, which is free prose and therefore the most
	// colon-prone field in the artifact set. An ordinary note containing ": "
	// makes YAML resolve that list item to a map rather than a string; the
	// validator saw nothing and returned OK, both plain and --deep, while
	// parser.ParseSurfaceFile — the reader the build stage actually uses —
	// failed with "cannot unmarshal !!map into string". The author found out
	// one phase later, as surface-not-readable.
	//
	// A validator that accepts input the parser rejects is worse than no
	// validator: it converts an authoring error into a phase-boundary error
	// and spends all the work in between. Routing through the same function
	// means the two cannot disagree again by construction, rather than by
	// keeping two field lists in step by hand.
	if _, err := parser.LoadSurfaceYAMLBytes(path, content); err != nil {
		return err
	}

	var doc struct {
		Feature   string `yaml:"feature"`
		Fragments []struct {
			Name   string `yaml:"name"`
			Shows  string `yaml:"shows"`
			Source string `yaml:"source"`
			Page   string `yaml:"page"`
		} `yaml:"fragments"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return fmt.Errorf("surface.yaml is not valid YAML: %w", err)
	}
	if len(doc.Fragments) == 0 {
		return fmt.Errorf("surface.yaml has no fragments: entries")
	}
	for i, f := range doc.Fragments {
		where := fmt.Sprintf("fragments[%d]", i)
		if f.Name != "" {
			where = fmt.Sprintf("fragment %q", f.Name)
		}
		if f.Name == "" {
			return fmt.Errorf("surface.yaml %s has no name:", where)
		}
		if f.Shows == "" {
			return fmt.Errorf("surface.yaml %s has no shows:", where)
		}
		if f.Source == "" {
			return fmt.Errorf("surface.yaml %s has no source: (traceability back to an intent)", where)
		}
		if f.Page == "" {
			return fmt.Errorf("surface.yaml %s has no page: target", where)
		}
	}
	return nil
}

// ValidateBlueprint checks blueprint.yaml has valid structure and cross-references.
func ValidateBlueprint(path string, content []byte) error {
	if err := ValidateYAML(path, content); err != nil {
		return err
	}

	var bp struct {
		App        string `yaml:"app"`
		Navigation *struct {
			Strategy     string `yaml:"strategy"`
			DefaultRoute string `yaml:"default-route"`
			Routes       []struct {
				Path  string `yaml:"path"`
				Shell string `yaml:"shell"`
				Guard string `yaml:"guard"`
			} `yaml:"routes"`
		} `yaml:"navigation"`
		Shells        map[string]interface{} `yaml:"shells"`
		Authorization *struct {
			Strategy string                 `yaml:"strategy"`
			Guards   map[string]interface{} `yaml:"guards"`
		} `yaml:"authorization"`
		Data *struct {
			Fetching string `yaml:"fetching"`
			Caching  *struct {
				Strategy string `yaml:"strategy"`
			} `yaml:"caching"`
			Offline *struct {
				Strategy string `yaml:"strategy"`
			} `yaml:"offline"`
		} `yaml:"data"`
		Errors *struct {
			Retry *struct {
				Strategy string `yaml:"strategy"`
			} `yaml:"retry"`
		} `yaml:"errors"`
	}
	if err := yaml.Unmarshal(content, &bp); err != nil {
		return fmt.Errorf("blueprint structure invalid: %w", err)
	}

	// Validate navigation strategy
	if bp.Navigation != nil && bp.Navigation.Strategy != "" {
		validStrategies := map[string]bool{
			"hash": true, "browser": true, "native-stack": true,
			"native-tab": true, "cli-subcommands": true,
		}
		if !validStrategies[bp.Navigation.Strategy] {
			return fmt.Errorf("invalid navigation.strategy %q — must be one of: hash, browser, native-stack, native-tab, cli-subcommands", bp.Navigation.Strategy)
		}
	}

	// Validate authorization strategy
	if bp.Authorization != nil && bp.Authorization.Strategy != "" {
		validAuthStrategies := map[string]bool{
			"role-based": true, "permission-based": true,
			"attribute-based": true, "none": true,
		}
		if !validAuthStrategies[bp.Authorization.Strategy] {
			return fmt.Errorf("invalid authorization.strategy %q — must be one of: role-based, permission-based, attribute-based, none", bp.Authorization.Strategy)
		}
	}

	// Closed-vocabulary gate for strategy settings. blueprint.schema.md
	// documents these as closed sets and defines blueprint-strategy-unknown
	// for out-of-vocabulary values, but nothing enforced it on the path the
	// CLI uses: a typo'd or invented strategy validated clean and only
	// surfaced (if at all) during codegen.
	//
	// The shapes matter. An earlier gate elsewhere in this package decoded
	// data.caching as a string and read auth.strategy, while real
	// blueprints carry data.caching.strategy (a map) and
	// authorization.strategy — so its unmarshal failed and the whole check
	// was skipped in silence. These read the shapes the schema body
	// actually documents.
	closedSets := []struct {
		path  string
		value string
		vocab map[string]bool
	}{}
	if bp.Data != nil {
		closedSets = append(closedSets, struct {
			path  string
			value string
			vocab map[string]bool
		}{"data.fetching", bp.Data.Fetching, ClosedSetDataFetching})
		// data.caching.strategy is deliberately NOT gated here. The Go
		// closed set (ClosedSetDataCaching) is {none, per-route, shared}
		// — cache *scope* — while blueprint.schema.md:186 documents
		// {none, in-memory, local-storage, service-worker} — cache
		// *location*. Two different concepts share one key, and gating on
		// either would reject blueprints written against the other. A
		// blueprint authored straight from the schema table (caching.
		// strategy: in-memory) fails the Go set, so enforcing it would
		// break valid projects. Resolve the vocabulary conflict first,
		// then gate.
	}
	for _, c := range closedSets {
		if c.value == "" || len(c.vocab) == 0 {
			continue
		}
		if !c.vocab[c.value] {
			allowed := make([]string, 0, len(c.vocab))
			for k := range c.vocab {
				allowed = append(allowed, k)
			}
			sort.Strings(allowed)
			return fmt.Errorf("blueprint-strategy-unknown: %s = %q is outside the closed vocabulary (%s)",
				c.path, c.value, strings.Join(allowed, ", "))
		}
	}

	// Cross-reference: shell names in routes must exist in shells
	if bp.Navigation != nil && bp.Navigation.Routes != nil {
		seenPaths := make(map[string]bool)
		for _, route := range bp.Navigation.Routes {
			// Check for duplicate paths
			if seenPaths[route.Path] {
				return fmt.Errorf("duplicate route path %q in navigation.routes", route.Path)
			}
			seenPaths[route.Path] = true

			// Check shell reference
			if route.Shell != "" && bp.Shells != nil {
				if _, ok := bp.Shells[route.Shell]; !ok {
					return fmt.Errorf("route %q references shell %q which is not defined in shells:", route.Path, route.Shell)
				}
			}

			// Check guard reference
			if route.Guard != "" && route.Guard != "none" {
				if bp.Authorization == nil || bp.Authorization.Guards == nil {
					return fmt.Errorf("route %q references guard %q but no authorization.guards are defined", route.Path, route.Guard)
				}
				if _, ok := bp.Authorization.Guards[route.Guard]; !ok {
					return fmt.Errorf("route %q references guard %q which is not defined in authorization.guards:", route.Path, route.Guard)
				}
			}
		}
	}

	return nil
}

// deepBuildfile is the parsed structure for deep validation.
type deepBuildfile struct {
	Feature      string                   `yaml:"feature"`
	Adapter      string                   `yaml:"adapter"`
	AdapterSet   string                   `yaml:"adapter-set"`
	Targets      map[string]deepTarget    `yaml:"targets"`
	Models       map[string]interface{}   `yaml:"models"`
	Fixtures     map[string]deepFixture   `yaml:"fixtures"`
	Routes       []deepRoute              `yaml:"routes"`
	Components   map[string]deepComponent `yaml:"components"`
	CrossCutting []deepCrossCuttingEntry  `yaml:"cross-cutting"`
	Plan         *deepPlan                `yaml:"plan"`
	Decisions    []deepDecision           `yaml:"decisions"`
}

// deepDecision is one entry of the buildfile's decisions: block — an
// implementation-level judgment call codegen recorded so the next emission
// starts from it instead of re-deriving it. Only the fields the propagation
// check consults are captured; the rest of the entry rides through parse
// untouched (the block is preserved verbatim, not rewritten by validation).
type deepDecision struct {
	ID         string   `yaml:"id"`
	Component  string   `yaml:"component"`
	EnforcedBy []string `yaml:"enforced-by"`
}

// deepTarget is one entry of the v2 targets: block. Only the fields the deep
// reference/vocabulary checks consult are captured — presentation carries
// components + routes (the former top-level v1 fields); every kind carries
// its adapter slug.
type deepTarget struct {
	Adapter    string                   `yaml:"adapter"`
	Components map[string]deepComponent `yaml:"components"`
	Routes     []deepRoute              `yaml:"routes"`
}

// resolvedComponents returns the components the reference and vocabulary
// checks operate on: the v1 top-level components: for single-target
// buildfiles, or targets.presentation.components: for v2 multi-target ones.
// This lets the existing checks run unchanged against whichever shape is
// present.
func (bf deepBuildfile) resolvedComponents() map[string]deepComponent {
	if bf.AdapterSet != "" {
		if t, ok := bf.Targets["presentation"]; ok {
			return t.Components
		}
		return nil
	}
	return bf.Components
}

// resolvedRoutes mirrors resolvedComponents for the routes: block.
func (bf deepBuildfile) resolvedRoutes() []deepRoute {
	if bf.AdapterSet != "" {
		if t, ok := bf.Targets["presentation"]; ok {
			return t.Routes
		}
		return nil
	}
	return bf.Routes
}

// presentationAdapter is the adapter slug occupying the presentation slot of
// a v2 buildfile, "" if none. Vocabulary validation resolves against this
// adapter for multi-target buildfiles (widgets live only in presentation
// adapters).
func (bf deepBuildfile) presentationAdapter() string {
	if t, ok := bf.Targets["presentation"]; ok {
		return t.Adapter
	}
	return ""
}

// BuildfileComponent is the subset of a buildfile component that shared
// readers expose to callers outside this package. Only the fields the
// coverage/traceability consumers need are surfaced; the full deepComponent
// stays internal to deep validation.
type BuildfileComponent struct {
	Source string
	Widget string
}

// BuildfileRoute is the subset of a buildfile route exposed to shared readers.
type BuildfileRoute struct {
	Path string
}

// ResolveBuildfileComponents parses buildfile content and returns its
// components in a shape-agnostic way: the v1 top-level components: map for
// single-target buildfiles, or targets.presentation.components: for v2
// multi-target ones. Built on deepBuildfile.resolvedComponents(), the same
// v2-aware resolution the deep validator already trusts.
//
// This is the ONE reader every check that needs a buildfile's components must
// go through, so a v2 buildfile can never again read as "no components" to one
// validator while another sees them. That divergence was the confirmed BP1
// break: check-coverage's own struct read only top-level components:, so every
// fragment of a multi-target feature came back uncovered while validate --deep
// (which resolves the v2 shape) reported the same buildfile complete.
func ResolveBuildfileComponents(content []byte) (map[string]BuildfileComponent, error) {
	var bf deepBuildfile
	if err := yaml.Unmarshal(content, &bf); err != nil {
		return nil, err
	}
	resolved := bf.resolvedComponents()
	out := make(map[string]BuildfileComponent, len(resolved))
	for name, c := range resolved {
		out[name] = BuildfileComponent{Source: c.Source, Widget: c.Widget}
	}
	return out, nil
}

// ResolveBuildfileRoutes mirrors ResolveBuildfileComponents for the routes
// block: v1 top-level routes:, or v2 targets.presentation.routes:. Route
// ownership and section hashing both need this fallback — a v2 buildfile's
// top-level routes: is empty because the rows relocated under the presentation
// target.
func ResolveBuildfileRoutes(content []byte) ([]BuildfileRoute, error) {
	var bf deepBuildfile
	if err := yaml.Unmarshal(content, &bf); err != nil {
		return nil, err
	}
	resolved := bf.resolvedRoutes()
	out := make([]BuildfileRoute, 0, len(resolved))
	for _, r := range resolved {
		out = append(out, BuildfileRoute{Path: r.Path})
	}
	return out, nil
}

// BuildfileDeclaresPlan reports whether buildfile content carries a
// non-empty plan: section — the executable contract for which files the
// feature touches. Generate-code hard-stops without one.
//
// A member of the ResolveBuildfile* family for the same reason they are:
// one v2-aware reader, so no caller can conclude "no plan" from a buildfile
// whose rows sit under plan.targets.<kind>. It answers presence only —
// deciding what the plan MEANS stays with the readers that need the rows.
//
// Unparseable content reports false: a buildfile nothing can read cannot be
// shown to declare a plan, and check-buildfile owns saying why.
func BuildfileDeclaresPlan(content []byte) bool {
	var bf deepBuildfile
	if err := yaml.Unmarshal(content, &bf); err != nil {
		return false
	}
	if bf.Plan == nil {
		return false
	}
	if len(bf.Plan.Modifies) > 0 || len(bf.Plan.Creates) > 0 || len(bf.Plan.Deletes) > 0 {
		return true
	}
	for _, t := range bf.Plan.Targets {
		if len(t.Modifies) > 0 || len(t.Creates) > 0 || len(t.Deletes) > 0 {
			return true
		}
	}
	return false
}

type deepPlan struct {
	Modifies []deepPlanEntry `yaml:"modifies"`
	Creates  []deepPlanEntry `yaml:"creates"`
	Deletes  []deepPlanEntry `yaml:"deletes"`
	// Targets holds the per-target rows a multi-target buildfile nests
	// under plan.targets.<kind>. The top-level lists above are what a
	// presentation-only project populates; these are what the multi-target
	// shape aggregates from.
	Targets map[string]deepPlanTarget `yaml:"targets"`
}

type deepPlanTarget struct {
	Modifies []deepPlanEntry `yaml:"modifies"`
	Creates  []deepPlanEntry `yaml:"creates"`
	Deletes  []deepPlanEntry `yaml:"deletes"`
}

type deepPlanEntry struct {
	Path    string   `yaml:"path"`
	Sources []string `yaml:"sources"`
}

type deepCrossCuttingEntry struct {
	ID            string   `yaml:"id"`
	Source        string   `yaml:"source"`
	TargetFiles   []string `yaml:"target-files"`
	TargetPattern string   `yaml:"target-pattern"`
	TargetCreates []string `yaml:"target-creates"`
	Transform     string   `yaml:"transform"`
	Introduces    []string `yaml:"introduces"`
}

type deepFixture struct {
	Data map[string]interface{} `yaml:"data"`
}

type deepRoute struct {
	Path    string                `yaml:"path"`
	Regions map[string]deepRegion `yaml:"regions"`
}

type deepRegion struct {
	Components []string `yaml:"components"`
}

type deepComponent struct {
	Source   string    `yaml:"source"`
	Widget   string    `yaml:"widget"`
	Data     *deepData `yaml:"data"`
	Children []string  `yaml:"children"`
}

type deepData struct {
	Inputs []deepInput `yaml:"inputs"`
}

type deepInput struct {
	Model string `yaml:"model"`
}

// ValidationError is a structured error returned by deep validation.
// Fields are designed for agent consumption: code identifies the error class,
// context provides specifics about where it occurred, and fix suggests recovery.
//
// parlay-feature: studio-support/structured-domain-model-validation
// parlay-component: cross-cutting/json-validation-mode
// (Severity field added — findings reuse the single JSON finding convention.)
type ValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Context string `json:"context,omitempty"`
	Fix     string `json:"fix"`
	// Severity is the per-mode severity of the finding, resolved from the
	// RuleSeverity table (authoring vs build). Populated by the mode-aware
	// structured validators (e.g. ValidateDomainModelStructuredMode); left
	// blank by the legacy string-Validator paths that have no mode context.
	// Emitting it here keeps the JSON finding convention single-schema
	// (code, message, context, fix, severity) — no parallel finding shape.
	Severity string `json:"severity,omitempty"`
}

// ValidateBuildfileDeepStructured performs cross-reference validation and returns
// structured errors. Each error has a code (for programmatic handling), context
// (location), and fix (recovery hint). Single-feature mode: plannedCreates is
// nil. Project-pass mode threads the cross-feature map through
// validateBuildfileDeepCore instead — see ValidateBuildfilesProjectStructured
// in validate_project.go.
func ValidateBuildfileDeepStructured(buildfilePath, adapterPath string) []ValidationError {
	return ApplyBuildfileSeverity(validateBuildfileDeepCore(buildfilePath, adapterPath, nil))
}

// ApplyBuildfileSeverity stamps each finding with its severity from the
// shared ruleSeverityTable, so every consumer of the deep-validation finding
// set agrees on what blocks. Without it the Severity field stays empty and
// callers must invent their own classification — which is precisely how
// check-buildfile and validate --deep came to report different verdicts for
// the same buildfile at the same moment.
//
// Resolved in build mode: this is the readiness question ("can codegen run
// against this?"), not the authoring question.
func ApplyBuildfileSeverity(errs []ValidationError) []ValidationError {
	for i := range errs {
		if errs[i].Severity == "" {
			errs[i].Severity = string(RuleSeverity(errs[i].Code, ModeBuild))
		}
		// The deep-validation findings' one recording point.
		//
		// ValidationError is a different type on a different path from
		// ValidationOutcome, and nothing recorded it — so the entire deep
		// surface (buildfile cross-references, plan integrity, structured
		// domain-model checks) was invisible to a log whose stated purpose
		// is "which rules actually fire". An investigator handed that log
		// would conclude those rules never fire.
		//
		// Codes and severity only. Message, Context and Fix all carry user
		// content: Context is almost always a filesystem path, and ~15 Fix
		// sites interpolate entity or feature names.
		feedback.Record(feedback.FindingData{
			Code:     errs[i].Code,
			Mode:     string(ModeBuild),
			Severity: errs[i].Severity,
			Site:     feedback.CallerSite(1),
		})
	}
	return errs
}

// validateBuildfileDeepCore is the shared per-feature check body for both
// single-feature validation (ValidateBuildfileDeepStructured, plannedCreates
// nil) and project-pass validation (ValidateBuildfilesProjectStructured,
// plannedCreates the cross-feature union-minus-self map). Reading and
// parsing the buildfile happens once here regardless of caller.
func validateBuildfileDeepCore(buildfilePath, adapterPath string, plannedCreates map[string]string) []ValidationError {
	var errors []ValidationError

	content, err := os.ReadFile(buildfilePath)
	if err != nil {
		return []ValidationError{{
			Code:    "buildfile-not-readable",
			Message: fmt.Sprintf("cannot read buildfile: %s", err),
			Context: buildfilePath,
			Fix:     "ensure the buildfile path is correct and the file exists",
		}}
	}

	var bf deepBuildfile
	if err := yaml.Unmarshal(content, &bf); err != nil {
		return []ValidationError{{
			Code:    "invalid-yaml",
			Message: fmt.Sprintf("invalid buildfile YAML: %s", err),
			Context: buildfilePath,
			Fix:     "fix the YAML syntax errors and re-run validation",
		}}
	}

	// The reference and vocabulary checks operate on the resolved component
	// and route sets: v1 top-level components:/routes:, or v2
	// targets.presentation.components:/routes:. resolvedComponents() and
	// resolvedRoutes() collapse the difference so the checks below are shape-
	// agnostic.
	comps := bf.resolvedComponents()

	// 1. Component references in routes must exist in components
	for _, route := range bf.resolvedRoutes() {
		for regionName, region := range route.Regions {
			for _, compRef := range region.Components {
				if _, ok := comps[compRef]; !ok {
					errors = append(errors, ValidationError{
						Code:    "missing-component-reference",
						Message: fmt.Sprintf("route %q region %q references component %q which is not defined", route.Path, regionName, compRef),
						Context: fmt.Sprintf("routes[%s].regions.%s", route.Path, regionName),
						Fix:     fmt.Sprintf("either add %q to the components: section or remove it from the route", compRef),
					})
				}
			}
		}
	}

	// 2. Model references in component data.inputs must resolve to a
	// declared entity — either the deprecated top-level models: block or,
	// preferably, the project's canonical domain-model.yaml. Resolving
	// against both is what makes the documented shape (no models:) valid;
	// previously only models: was consulted, so a buildfile written the
	// way the schema and skill prescribe failed validation.
	domainEntities := domainEntityNames(buildfilePath)

	// models: was removed in v0.3 — entity declarations live in
	// domain-model.yaml, and a buildfile still carrying the block gets a
	// hard error (regenerating via build-feature drops it; there is no
	// in-place migrator because the file is tool-generated).
	if len(bf.Models) > 0 {
		errors = append(errors, ValidationError{
			Code:     "buildfile-models-unsupported",
			Message:  "top-level models: was removed in v0.3 — entity declarations belong in domain-model.yaml",
			Context:  "models",
			Fix:      "delete the models: block (or re-run /parlay-build-feature, which no longer emits it); inputs and fixtures resolve against the project's domain-model.yaml",
			Severity: string(SeverityError),
		})
	}

	knownModel := func(name string) bool {
		return domainEntities[name]
	}
	modelFix := func(name string) string {
		if len(domainEntities) == 0 {
			return fmt.Sprintf("declare %q in the project's domain-model.yaml (preferred), or add it to the buildfile's models: section", name)
		}
		return fmt.Sprintf("declare %q in the project's domain-model.yaml, or change the input to reference an existing entity", name)
	}

	for compName, comp := range comps {
		if comp.Data != nil {
			for _, input := range comp.Data.Inputs {
				if input.Model != "" {
					if !knownModel(input.Model) {
						errors = append(errors, ValidationError{
							Code:    "missing-model-reference",
							Message: fmt.Sprintf("component %q references model %q which is not defined", compName, input.Model),
							Context: fmt.Sprintf("components.%s.data.inputs", compName),
							Fix:     modelFix(input.Model),
						})
					}
				}
			}
		}

		// 3. Children references must exist in components
		for _, child := range comp.Children {
			if _, ok := comps[child]; !ok {
				errors = append(errors, ValidationError{
					Code:    "missing-child-reference",
					Message: fmt.Sprintf("component %q references child %q which is not defined", compName, child),
					Context: fmt.Sprintf("components.%s.children", compName),
					Fix:     fmt.Sprintf("either add %q to the components: section or remove it from children", child),
				})
			}
		}
	}

	// 4. Fixture data keys must match a declared entity — same dual
	// resolution as component inputs above.
	for fixtureName, fixture := range bf.Fixtures {
		for modelName := range fixture.Data {
			if !knownModel(modelName) {
				errors = append(errors, ValidationError{
					Code:    "missing-fixture-model",
					Message: fmt.Sprintf("fixture %q references model %q which is not defined", fixtureName, modelName),
					Context: fmt.Sprintf("fixtures.%s.data", fixtureName),
					Fix:     modelFix(modelName),
				})
			}
		}
	}

	// 5. Adapter vocabulary validation (if adapter path provided). For a v2
	// buildfile the caller resolves adapterPath to the presentation adapter —
	// widgets live only there — so vocabulary runs against react/angular/etc,
	// never against a backend adapter that has no widget vocabulary.
	if adapterPath != "" {
		adapterErrors := validateAdapterVocabulary(bf, adapterPath)
		errors = append(errors, adapterErrors...)
	}

	// 5b. Multi-target canonical-once + operation-ref resolution. Runs only
	// for v2 buildfiles (adapter-set: present); single-target buildfiles never
	// reach it, so their behavior is unchanged. This is the check the schema's
	// "canonical fields belong under operations: only" rule needs — without
	// it a target could name an operation the canonical block doesn't carry
	// and codegen would silently emit nothing.
	if bf.AdapterSet != "" {
		for _, o := range ValidateBuildfileCanonical(ModeBuild, buildfilePath, content) {
			errors = append(errors, ValidationError{
				Code:     o.Code,
				Message:  o.Message,
				Context:  o.Context,
				Fix:      o.Fix,
				Severity: string(o.Severity),
			})
		}
	}

	// 6. Cross-cutting entry validation
	if len(bf.CrossCutting) > 0 {
		ccErrors := validateCrossCuttingEntries(bf.CrossCutting)
		errors = append(errors, ccErrors...)
	}

	// 7. Plan section validation: every component and cross-cutting
	// entry must be represented; modify-paths must exist; create-paths
	// must not collide with existing files.
	//
	// plannedCreates is nil in single-feature mode and the cross-feature
	// union-minus-self map in project-pass mode (see the caller-selection
	// comment on validateBuildfileDeepCore above).
	//
	// parlay-extends: parlay-tool/cross-cutting-target-paths/validator-classify-entry-kind-and-route
	// parlay-extends: parlay-tool/cross-cutting-target-paths/validator-resolve-target-pattern-at-validation-time
	// parlay-extends: parlay-tool/cross-cutting-target-paths/validator-target-creates-and-two-kinded-entries
	// parlay-extends: parlay-tool/cross-cutting-target-paths/project-pass-validation-and-cli-flag
	planErrors := validatePlanSection(bf, buildfilePath, plannedCreates)
	errors = append(errors, planErrors...)

	// 8. Rationale propagation: every decisions: entry must reach the code it
	// governs. A recorded decision whose enforcing file exists but never names
	// the decision id is stranded — the reason is on disk in the buildfile but
	// absent from the file a later reader edits.
	decisionErrors := validateDecisionPropagation(bf, buildfilePath)
	errors = append(errors, decisionErrors...)

	return errors
}

// validateDecisionPropagation implements the WP7 rationale-stranded check.
// For each decisions: entry, every file named in enforced-by: that exists on
// disk must contain the decision's id verbatim. The check is lexical and
// scoped to explicitly-recorded decisions only — it never infers a decision
// from unmarked code.
//
// A file that does not yet exist is not stranded: a plan.creates path before
// codegen has run simply has not been written, and firing on it would make the
// check noise for every buildfile validated between build and generate. That
// is why the missing-file case is silent here rather than a finding — file
// existence is the plan section's concern, not this one's.
func validateDecisionPropagation(bf deepBuildfile, buildfilePath string) []ValidationError {
	if len(bf.Decisions) == 0 {
		return nil
	}
	rootDir := planRootDirFromBuildfilePath(buildfilePath)
	var errors []ValidationError
	for i, d := range bf.Decisions {
		// An entry with no id or no enforcing files records nothing the check
		// can hold anything to; the schema requires both, and the shape checks
		// for that live where the block is authored, not here.
		if d.ID == "" || len(d.EnforcedBy) == 0 {
			continue
		}
		for _, rel := range d.EnforcedBy {
			abs := rel
			if rootDir != "" && !filepath.IsAbs(rel) {
				abs = filepath.Join(rootDir, rel)
			}
			content, err := os.ReadFile(abs)
			if err != nil {
				// Unwritten (or unreadable) file: not stranded — see the
				// function comment. Existence is the plan section's job.
				continue
			}
			if !strings.Contains(string(content), d.ID) {
				errors = append(errors, ValidationError{
					Code:    "rationale-stranded",
					Message: fmt.Sprintf("decision %q names %q in enforced-by:, but that file does not contain the decision id — the recorded reason never reached the code it governs", d.ID, rel),
					Context: fmt.Sprintf("decisions[%d].enforced-by (%s)", i, rel),
					Fix:     fmt.Sprintf("reference %q in %s (a comment naming the decision is enough), or drop the file from enforced-by: if the decision no longer governs it", d.ID, rel),
				})
			}
		}
	}
	return errors
}

func validateCrossCuttingEntries(entries []deepCrossCuttingEntry) []ValidationError {
	var errors []ValidationError
	seenIDs := make(map[string]bool)

	for i, entry := range entries {
		ctx := fmt.Sprintf("cross-cutting[%d]", i)

		if entry.ID == "" {
			errors = append(errors, ValidationError{
				Code:    "missing-cross-cutting-id",
				Message: fmt.Sprintf("cross-cutting entry at index %d has no id", i),
				Context: ctx,
				Fix:     "add a unique id: field to the cross-cutting entry",
			})
		} else {
			if seenIDs[entry.ID] {
				errors = append(errors, ValidationError{
					Code:    "duplicate-cross-cutting-id",
					Message: fmt.Sprintf("cross-cutting id %q appears more than once", entry.ID),
					Context: ctx,
					Fix:     "rename one of the duplicate entries to be unique",
				})
			}
			seenIDs[entry.ID] = true
		}

		if entry.Source == "" {
			errors = append(errors, ValidationError{
				Code:    "missing-cross-cutting-source",
				Message: fmt.Sprintf("cross-cutting entry %q has no source reference", entry.ID),
				Context: ctx,
				Fix:     "add source: @feature/intent-slug for traceability",
			})
		}

		if entry.Transform == "" {
			errors = append(errors, ValidationError{
				Code:    "missing-cross-cutting-transform",
				Message: fmt.Sprintf("cross-cutting entry %q has no transform description", entry.ID),
				Context: ctx,
				Fix:     "add transform: describing what the change does",
			})
		}

		if len(entry.TargetFiles) == 0 && entry.TargetPattern == "" && len(entry.TargetCreates) == 0 {
			errors = append(errors, ValidationError{
				Code:    "missing-cross-cutting-target",
				Message: fmt.Sprintf("cross-cutting entry %q has neither target-files nor target-pattern nor target-creates", entry.ID),
				Context: ctx,
				Fix:     "add target-files: (existing-on-disk paths to modify), target-pattern: (grep pattern), or target-creates: (new paths to introduce); at least one is required",
			})
		}
	}

	return errors
}

func validateAdapterVocabulary(bf deepBuildfile, adapterPath string) []ValidationError {
	var errors []ValidationError

	// The adapter slug to fall back on: v1 top-level adapter:, or the v2
	// presentation adapter (the only kind that carries widget vocabulary).
	adapterSlug := bf.Adapter
	if adapterSlug == "" {
		adapterSlug = bf.presentationAdapter()
	}

	data, err := os.ReadFile(adapterPath)
	if err != nil {
		// Adapter file doesn't exist — try resolving from .parlay/adapters/
		resolved := filepath.Join(".parlay", "adapters", adapterSlug+".adapter.yaml")
		data, err = os.ReadFile(resolved)
		if err != nil {
			return []ValidationError{{
				Code:    "adapter-not-found",
				Message: fmt.Sprintf("cannot read adapter %q: %s", adapterPath, err),
				Context: adapterPath,
				Fix:     "verify the adapter file exists at .parlay/adapters/{name}.adapter.yaml",
			}}
		}
	}

	var adapter deepAdapter
	if err := yaml.Unmarshal(data, &adapter); err != nil {
		return []ValidationError{{
			Code:    "invalid-adapter-yaml",
			Message: fmt.Sprintf("invalid adapter YAML: %s", err),
			Context: adapterPath,
			Fix:     "fix the YAML syntax errors in the adapter file",
		}}
	}

	// Check component widgets against adapter vocabulary. The buildfile
	// contains framework-specific widget names populated from the adapter's
	// shows/actions mappings. Widgets that don't appear in ANY adapter
	// section are flagged.
	allWidgets := make(map[string]bool)
	for _, sections := range []map[string]interface{}{adapter.Shows, adapter.Actions, adapter.Flows} {
		for _, v := range sections {
			if m, ok := v.(map[string]interface{}); ok {
				if w, ok := m["widget"]; ok {
					allWidgets[fmt.Sprint(w)] = true
				}
				if p, ok := m["pattern"]; ok {
					allWidgets[fmt.Sprint(p)] = true
				}
			}
		}
	}
	for compName, comp := range bf.resolvedComponents() {
		if comp.Widget != "" && comp.Widget != "not-applicable" {
			if !allWidgets[comp.Widget] {
				errors = append(errors, ValidationError{
					Code:    "unknown-widget",
					Message: fmt.Sprintf("component %q uses widget %q which is not in adapter %q", compName, comp.Widget, adapterSlug),
					Context: fmt.Sprintf("components.%s.widget", compName),
					Fix:     fmt.Sprintf("change the widget to one defined in the adapter's shows/actions/flows sections, or add %q to the adapter", comp.Widget),
				})
			}
		}
	}

	return errors
}
