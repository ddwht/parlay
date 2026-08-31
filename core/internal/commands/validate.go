// parlay-feature: parlay-tool
// parlay-component: validate
// parlay-extends: infrastructure-layer/InfrastructureValidationResult
// parlay-extends: parlay-tool/domain-model-yaml-migration/domain-model-validate-cli-type
// parlay-extends: parlay-tool/cross-cutting-target-paths/project-pass-validation-and-cli-flag

package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
	"github.com/ddwht/parlay/core/internal/config"
	"github.com/ddwht/parlay/core/internal/parser"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate --type <type> <path>",
	Short: "Validate a file against its schema",
	Args:  validateArgs,
	RunE:  runValidate,
}

var validateType string
var validateDeep bool
var validateAdapter string
var validateJSON bool
var validateProject bool

// validateTypes is the single list of accepted --type values. There were
// three hardcoded lists — the flag's help text, the "--type is required"
// message and the "unknown type" message — and they disagreed: the
// required-message omitted `adapter` and `testcases`, and the
// unknown-type message omitted `page` and `layout` despite both being
// handled sixty lines above it. A person reading either message would
// have concluded a working type did not exist. One list, three readers.
var validateTypes = []string{
	"intent", "dialog", "surface", "buildfile", "blueprint", "yaml",
	"infrastructure", "domain-model", "adapter", "adapter-set",
	"capabilities", "testcases", "page", "layout",
	"authored",
}

func validateTypeList() string { return strings.Join(validateTypes, ", ") }

func init() {
	validateCmd.Flags().StringVar(&validateType, "type", "", "File type: "+validateTypeList())
	validateCmd.Flags().BoolVar(&validateDeep, "deep", false, "Enable cross-reference validation (buildfile, infrastructure)")
	validateCmd.Flags().StringVar(&validateAdapter, "adapter", "", "Path to adapter file for vocabulary validation (used with --deep)")
	validateCmd.Flags().BoolVar(&validateJSON, "json", false, "Output structured JSON errors for agent consumption")
	// parlay-feature: parlay-tool/cross-cutting-target-paths
	// parlay-component: validate (extends project-pass-validation-and-cli-flag)
	validateCmd.Flags().BoolVar(&validateProject, "project", false, "Validate every buildfile under the resolved root in project-pass mode (no positional path; implies --type buildfile --deep)")
	// --project resolves its root via the standard active-root resolver
	// (the persistent --root flag, PARLAY_ROOT, or cwd walk-up) — the
	// same mechanism every other command uses. This command used to
	// register its own local --root flag here, which shadowed the
	// persistent --root flag's real meaning (select a registered child
	// root by name) with a different one (an arbitrary filesystem path,
	// resolved outside the multi-root machinery entirely — no parent-
	// pointer validation, no ambiguity signaling, nothing). Removed.
}

// validateArgs reconciles the legacy positional-path requirement with the
// new --project flag: --project takes zero positional arguments, the
// legacy modes still take exactly one. Combining --project with a path is
// rejected with a structured error.
func validateArgs(cmd *cobra.Command, args []string) error {
	projectFlag, _ := cmd.Flags().GetBool("project")
	if projectFlag {
		if len(args) != 0 {
			return fmt.Errorf("validate-project-takes-no-path: --project does not take a positional path argument; got %v", args)
		}
		return nil
	}
	return cobra.ExactArgs(1)(cmd, args)
}

type validateJSONResult struct {
	Path   string                  `json:"path"`
	Type   string                  `json:"type"`
	OK     bool                    `json:"ok"`
	Errors []agent.ValidationError `json:"errors,omitempty"`
}

// projectValidateJSONResult is the JSON envelope emitted when --project is
// set. It carries one entry per feature plus an aggregate ok flag.
//
// parlay-feature: parlay-tool/cross-cutting-target-paths
// parlay-component: validate (extends project-pass-validation-and-cli-flag)
type projectValidateJSONResult struct {
	Root     string                 `json:"root"`
	OK       bool                   `json:"ok"`
	Features []agent.FeatureVerdict `json:"features,omitempty"`
	// SharedConcepts lists infrastructure-concept-shared warnings: one per
	// architectural concept two or more features constrain. Cross-feature by
	// construction, so they hang off the project envelope rather than any one
	// feature's verdict. Warnings, never blocking — they never touch OK.
	SharedConcepts []agent.ValidationError `json:"shared_concepts,omitempty"`
	Note           string                  `json:"note,omitempty"`
}

func runValidate(cmd *cobra.Command, args []string) error {
	if validateProject {
		return runValidateProject(cmd)
	}

	path := args[0]

	// Structured JSON mode for domain-model (intent: json-validation-mode).
	// When --json is set with --type domain-model, emit the full per-violation
	// finding list as a bare JSON array (one entry per violation, [] for a
	// clean model). The exit code follows the findings on the same rule as
	// every other validate path — blocking findings exit 1, warnings alone
	// exit 0 — because --json chooses how the result is rendered, not whether
	// the command has a verdict. Accepts `-` to read the model from stdin.
	// The non-JSON domain-model path below is unchanged.
	//
	// parlay-feature: parlay-tool/structured-domain-model-validation
	// parlay-component: cross-cutting/json-validation-mode
	if validateJSON && validateType == "domain-model" {
		return runValidateDomainModelJSON(cmd, path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return outputValidate(cmd, path, []agent.ValidationError{{
			Code:    "file-not-readable",
			Message: fmt.Sprintf("cannot read %s: %s", path, err),
			Context: path,
			Fix:     "verify the file path is correct",
		}})
	}

	// --type is required for the non-project paths.
	if validateType == "" {
		return fmt.Errorf("--type is required (one of: %s)", validateTypeList())
	}

	// page and layout route through the unified layout validator
	// (agent.ValidateLayoutDeep) rather than the plain Validator shape —
	// they need adapter resolution and structured, multi-error output.
	// Codes and fix messages follow layout.schema.md's "Validation pass"
	// table.
	//
	// parlay-feature: parlay-tool/page-layout-field
	// parlay-cross-cutting-id: layout-precheck-contract
	if validateType == "page" {
		return runValidatePageType(cmd, path)
	}
	if validateType == "layout" {
		return runValidateLayoutType(cmd, path)
	}

	var validator agent.Validator
	switch validateType {
	// The two designer-authored artifacts. Their schemas shipped and
	// deployed while neither type was accepted here, so the only
	// hand-written files in the pipeline were the only ones nothing
	// could check.
	case "intent":
		validator = reportingOutcomeValidator(cmd.ErrOrStderr(), agent.ValidateIntentsDeep)
	case "dialog":
		validator = reportingOutcomeValidator(cmd.ErrOrStderr(), agent.ValidateDialogsDeep)
	// parlay-feature: parlay-tool/ledger-and-contract
	// parlay-component: amendment-artifact
	case "amendment":
		// Single-file shape only; ledger-level checks (sequence, supersedes,
		// affects resolution) live in `parlay internal check-amendments`.
		validator = reportingOutcomeValidator(cmd.ErrOrStderr(), agent.ValidateAmendment)
	case "surface":
		// The presence walker is cross-artifact — it reads capabilities.yaml
		// alongside the surface — so it cannot ride inside ValidateSurface,
		// whose signature sees one file's bytes. Gathered here and reported
		// beside the per-file result.
		validator = withCriteriaPresence(cmd, agent.ValidateSurface)
	case "buildfile":
		validator = agent.ValidateBuildfile
	case "blueprint":
		validator = agent.ValidateBlueprint
	case "yaml":
		validator = agent.ValidateYAML
	case "infrastructure":
		// --deep parses fragments and reports structured errors plus
		// portability warnings; the shallow form only checks that headings and
		// **Behavior**: fields exist. The deep variant had no caller at all, so
		// every rule it implements was unenforceable — the same shape as the
		// buildfile pair, which has had a --deep switch all along.
		if validateDeep {
			validator = validateInfrastructureDeepAdapter
		} else {
			validator = agent.ValidateInfrastructure
		}
	case "domain-model":
		// parlay-feature: parlay-tool/domain-model-yaml-migration
		// parlay-component: validate (extends domain-model-validate-cli-type)
		validator = validateDomainModelAdapter
	// parlay-feature: parlay-tool/multi-adapter
	// parlay-component: cli-and-deployer-registration
	case "adapter-set":
		validator = wrapOutcomeValidator(agent.ValidateAdapterSet)
	case "testcases":
		// There was no `--type testcases` at all, so nothing checked a
		// testcases.yaml against its schema: the build phase writes it and only
		// the coverage walker ever read it. Every v2 suite-shape rule —
		// discriminated kinds, source_refs presence, the legacy-ingestion
		// warning — was therefore unenforceable.
		//
		// The coverage inputs are gathered here and closed over, the same way
		// the capabilities entity list is: the operation- and criterion-coverage
		// walkers need the feature's contract artifacts, which are a fact about
		// the project the single-file adapter cannot see on its own. Resolution
		// failure is not fatal — testcasesCoverageInputs returns empty inputs and
		// only the two walkers go quiet, which is the honest answer when no
		// capabilities.yaml or surface.yaml resolves.
		cov := testcasesCoverageInputs(cmd, path)
		validator = func(p string, c []byte) error {
			cov.Path = p
			cov.Content = c
			return renderTestcasesOutcomes(agent.ValidateTestcasesV2(agent.ModeAuthoring, cov))
		}
	case "adapter":
		// The complete adapter validator — every section of adapter.schema.md,
		// kind-conditional. This used to check ONLY the toolchain block, which
		// meant an adapter with no name, no kind, no shows and no
		// file-conventions reported OK: the false green that made
		// agent-authored adapters converge on something broken.
		validator = reportingOutcomeValidator(cmd.ErrOrStderr(), agent.ValidateAdapter)
	case "capabilities":
		// The entity cross-reference needs the resolved root's domain model, so
		// the entity names are gathered here and closed over. Resolution failure
		// is not fatal: declaredCapabilityEntities returns nil, which disables
		// only the cross-reference — a project with no domain model yet is a
		// normal state, and refusing to validate capabilities at all because of
		// it would be a worse answer than checking everything else.
		//
		// The proposals are gathered the same way and for the same reason:
		// without them, a reference to an entity a sibling feature is about
		// to introduce is indistinguishable from a typo, and the validator
		// grades both as an error. That is what forced two features in the
		// regression run to ship placeholders.
		entities := declaredCapabilityEntities(cmd)
		proposed := proposedCapabilityEntities(cmd)
		// Presence runs for capabilities too, not only for surface. Wiring it
		// to one artifact type would make the same cross-artifact condition
		// appear or vanish depending on which file the author happened to
		// validate.
		validator = withCriteriaPresence(cmd, reportingOutcomeValidator(cmd.ErrOrStderr(), func(mode agent.ValidationMode, p string, c []byte) []agent.ValidationOutcome {
			return agent.ValidateCapabilitiesWithProposals(mode, p, c, entities, proposed)
		}))
	// parlay-feature: parlay-tool/hand-authored-units
	// parlay-component: authored-unit-validation
	case "authored":
		// Structural pass only — glob resolution against the filesystem is
		// a question about a project, not a file, and belongs with the
		// tracking pass that holds the emitted manifest.
		validator = reportingOutcomeValidator(cmd.ErrOrStderr(), agent.ValidateAuthoredUnit)
	default:
		return fmt.Errorf("unknown type %q — supported: %s", validateType, validateTypeList())
	}

	if err := validator(path, content); err != nil {
		return outputValidate(cmd, path, []agent.ValidationError{{
			Code:    "schema-validation-failed",
			Message: err.Error(),
			Context: path,
			Fix:     "fix the structural issues reported above",
		}})
	}

	// Deep validation for buildfiles
	if validateDeep && validateType == "buildfile" {
		adapterPath := validateAdapter
		if adapterPath == "" {
			// Auto-discover the adapter from the buildfile's own adapter:
			// field, the same way check-buildfile does. Without this,
			// omitting --adapter silently skipped every widget/action/flow
			// vocabulary check and reported ok:true — a false clean. The
			// skill's own step-10 command list omits --adapter, so the
			// documented invocation was the one that skipped the checks.
			if cfg, cfgErr := mustContext(cmd); cfgErr == nil {
				adapterPath = autoDiscoverAdapter(cfg, path)
			}
		}
		errors := agent.ValidateBuildfileDeepStructured(path, adapterPath)
		if len(errors) > 0 {
			return outputValidate(cmd, path, errors)
		}
	}

	return outputValidate(cmd, path, nil)
}

// runValidateDomainModelJSON implements `validate --type domain-model --json`.
// It emits the structured per-violation finding list as a bare JSON array,
// reusing the agent.ValidationError finding shape (code, message, context,
// fix, severity) — no parallel finding schema. Findings are resolved in
// authoring mode. (Historical note: authoring mode once downgraded
// domain-operations-deprecated to a warning for the editor; that code is gone
// — the emitted code is domain-operations-unsupported, an error in both
// modes, and the editor no longer exists.) A clean model prints "[]".
//
// The exit code agrees with the findings, on the same rule as the generic
// outputValidate path: blocking findings exit 1, warnings alone exit 0. This
// used to exit 0 unconditionally, on the reading that the list is a query
// result rather than a command failure — which made `--json` a flag that
// switched the command's contract rather than its rendering, and left parlay
// with one surface where a printed problem still reported success. That is
// R4-22, and it contradicted parlay's own stated CI rule (build-feature's
// "process exit code is the source of truth ... CI scripts MUST NOT
// pattern-match stdout/stderr text"). Rendering does not decide verdicts.
//
// This is safe for the callers that exist. Both deployed consumers —
// create-domain-model step 7 and load-domain-model steps 3 and 8 — read the
// findings array and stop on a non-empty one; neither inspects exit status,
// so neither changes behaviour. (The retired editor called the validator in
// process via the domain_validator.go seam rather than shelling out, so it
// never depended on this path either.)
//
// Warnings still exit 0, which is what keeps the authoring context usable: a
// warning-severity finding is guidance, not a reason to fail the caller.
//
// The output is unchanged either way — still the bare finding array, still
// "[]" for a clean model, still valid JSON on a read failure. A caller that
// parses stdout keeps working; a caller that trusted the exit code stops
// being misled.
//
// The positional path may be "-" to read the model bytes from stdin; a real
// path and "-" are mutually exclusive and, for identical bytes, produce an
// identical finding set. Even a read failure or unparseable input yields
// valid JSON on stdout (a single-element array), never a crash or non-JSON
// error.
//
// parlay-feature: parlay-tool/structured-domain-model-validation
// parlay-component: cross-cutting/json-validation-mode
func runValidateDomainModelJSON(cmd *cobra.Command, path string) error {
	var (
		content []byte
		err     error
	)
	// Use a stable synthetic label for stdin so the messages / whole-model
	// token stay consistent regardless of the actual source.
	label := path
	if path == "-" {
		label = "<stdin>"
		content, err = io.ReadAll(cmd.InOrStdin())
	} else {
		content, err = os.ReadFile(path)
	}

	var findings []agent.ValidationError
	if err != nil {
		findings = []agent.ValidationError{{
			Code:     "file-not-readable",
			Message:  fmt.Sprintf("cannot read %s: %s", label, err),
			Context:  wholeModelTokenForCLI,
			Fix:      "verify the file path is correct, or pass `-` to read the model from stdin",
			Severity: string(agent.SeverityError),
		}}
	} else {
		findings = agent.ValidateDomainModelStructuredMode(label, content, agent.ModeAuthoring)
	}
	if findings == nil {
		findings = []agent.ValidationError{}
	}

	data, _ := json.MarshalIndent(findings, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	if blockingCount(findings) > 0 {
		return NewExitCodeError(1)
	}
	return nil
}

// wholeModelTokenForCLI mirrors agent's wholeModelPathToken for the one
// CLI-owned finding (file-not-readable) that never reaches the validator.
// Kept in sync with agent.wholeModelPathToken ("<domain-model>").
const wholeModelTokenForCLI = "<domain-model>"

// runValidateProject implements the --project mode: walk every buildfile
// under the resolved root and validate each in project-pass mode (which
// threads cross-feature plannedCreates through validatePlanSection).
//
// Root resolution goes through mustContext(cmd), the same standard
// active-root resolver every other command uses (cwd walk-up, the
// persistent --root flag, PARLAY_ROOT) — not a bespoke PARLAY_ROOT-or-cwd
// fallback that bypasses parent-pointer validation and doesn't understand
// registered child roots at all.
//
// parlay-feature: parlay-tool/cross-cutting-target-paths
// parlay-component: validate (extends project-pass-validation-and-cli-flag)
func runValidateProject(cmd *cobra.Command) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	root := cfg.Root.Path
	verdicts, err := agent.ValidateBuildfilesProjectStructured(root)
	if err != nil {
		return fmt.Errorf("project-pass walk failed: %w", err)
	}

	// parlay-feature: parlay-tool/multi-adapter
	// parlay-component: cli-and-deployer-registration
	//
	// Multi-target gate: when the project carries an adapter-set with a
	// non-presentation slot, also walk the capabilities + supports +
	// blueprint rules. Presentation-only projects short-circuit inside
	// ValidateProjectMultiTarget and contribute zero outcomes here.
	multiOutcomes := agent.ValidateProjectMultiTarget(agent.ModeBuild, root)
	if len(multiOutcomes) > 0 {
		// Attribute them to a synthetic "_project" feature so they
		// surface alongside per-feature verdicts.
		var errs []agent.ValidationError
		for _, o := range multiOutcomes {
			if o.Severity != agent.SeverityError {
				continue
			}
			errs = append(errs, agent.ValidationError{
				Code:    o.Code,
				Message: o.Message,
				Context: o.Context,
				Fix:     o.Fix,
			})
		}
		if len(errs) > 0 {
			verdicts = append(verdicts, agent.FeatureVerdict{
				Feature:       "_project",
				BuildfilePath: filepath.Join(root, ".parlay", "adapter-set.yaml"),
				Errors:        errs,
			})
		}
	}

	// Count only findings that block, so `ok` here means what `ready` means in
	// check-buildfile. This used to be len(v.Errors) with no severity filter,
	// which made a warning fail the command outright — see blockingCount's own
	// note, which names plan-create-collision as the case it was split out
	// for, and ApplyBuildfileSeverity's call site in the project pass for how
	// these findings come to be graded at all.
	// Cross-feature infrastructure concept sharing is independent of any one
	// buildfile — one architectural concept constrained by two features is a
	// property of the pair — so it is computed here at project scope and
	// carried alongside the per-feature verdicts rather than inside one.
	sharedConcepts := sharedInfrastructureConcepts(filepath.Join(root, config.SpecDir))

	// Two-headed supersedes chains are a project-wide property of the surface
	// set — a fork no single page owns — so they are computed here and, being
	// blocking errors, carried as a synthetic verdict the way multi-target
	// errors are above.
	if conflicts := supersedesConflicts(filepath.Join(root, config.SpecDir)); len(conflicts) > 0 {
		verdicts = append(verdicts, agent.FeatureVerdict{
			Feature:       "_surface",
			BuildfilePath: filepath.Join(root, config.SpecDir, "intents"),
			Errors:        conflicts,
		})
	}

	totalBlocking := 0
	totalWarnings := len(sharedConcepts)
	for _, v := range verdicts {
		b := blockingCount(v.Errors)
		totalBlocking += b
		totalWarnings += len(v.Errors) - b
	}
	ok := totalBlocking == 0

	if validateJSON {
		out := projectValidateJSONResult{
			Root:           root,
			OK:             ok,
			Features:       verdicts,
			SharedConcepts: sharedConcepts,
		}
		if len(verdicts) == 0 {
			out.Note = fmt.Sprintf("no buildfiles under %s/.parlay/build/", root)
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		if !ok {
			return NewExitCodeError(1)
		}
		return nil
	}

	// One line per shared concept, printed in every text outcome — a warning
	// only becomes useful if it is read, and the summary count alone cannot
	// name the concept or the features. To stdout, as a non-failing finding.
	printSharedConcepts := func() {
		for _, c := range sharedConcepts {
			fmt.Fprintf(cmd.OutOrStdout(), "  [warning] [%s] %s\n", c.Code, c.Message)
		}
	}

	// Text output.
	if len(verdicts) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "OK (no buildfiles under %s/.parlay/build/)\n", root)
		printSharedConcepts()
		return nil
	}
	if ok {
		// Warnings are reported but do not fail. Staying silent about them
		// would swap one wrong answer for another: a project with 43
		// collisions and one with none would print the same line.
		if totalWarnings > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "OK (%d feature(s) validated, %d warning(s))\n", len(verdicts), totalWarnings)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "OK (%d feature(s) validated)\n", len(verdicts))
		}
		printSharedConcepts()
		return nil
	}
	printSharedConcepts()
	fmt.Fprintf(cmd.ErrOrStderr(), "FAIL: %d issue(s) across %d feature(s)\n", totalBlocking, len(verdicts))
	for _, v := range verdicts {
		if len(v.Errors) == 0 {
			continue
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "\nfeature %s (%s):\n", v.Feature, v.BuildfilePath)
		for _, e := range v.Errors {
			fmt.Fprintf(cmd.ErrOrStderr(), "  [%s] %s\n", e.Code, e.Message)
			if e.Fix != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "    fix: %s\n", e.Fix)
			}
		}
	}
	return NewExitCodeError(1)
}

// wrapOutcomeValidator adapts an agent.ValidationOutcome-returning
// validator to the legacy agent.Validator signature. Outcomes with severity
// error are joined into a single error string; warnings are dropped silently
// (the structured JSON path uses ValidationOutcome directly when needed).
//
// parlay-feature: parlay-tool/multi-adapter
// parlay-component: cli-and-deployer-registration
func wrapOutcomeValidator(fn func(agent.ValidationMode, string, []byte) []agent.ValidationOutcome) agent.Validator {
	return func(path string, content []byte) error {
		outcomes := fn(agent.ModeAuthoring, path, content)
		var msgs []string
		for _, o := range outcomes {
			if o.Severity == agent.SeverityError {
				msgs = append(msgs, fmt.Sprintf("%s: %s", o.Code, o.Message))
			}
		}
		if len(msgs) == 0 {
			return nil
		}
		return fmt.Errorf("%s", strings.Join(msgs, "\n"))
	}
}

// reportingOutcomeValidator is wrapOutcomeValidator with the warnings kept.
// The designer-authored artifacts lean on the authoring/build severity
// split — an empty intents.md and a half-typed turn are both normal
// mid-authoring states that block the build — so dropping warnings here
// would leave `parlay validate --type intent` silent about the two things
// it most needs to say while a person is still writing the file. Warnings
// go to the command's stderr; the exit code still turns on errors alone.
func reportingOutcomeValidator(warnings io.Writer, fn func(agent.ValidationMode, string, []byte) []agent.ValidationOutcome) agent.Validator {
	return func(path string, content []byte) error {
		outcomes := fn(agent.ModeAuthoring, path, content)
		var msgs []string
		for _, o := range outcomes {
			if o.Severity == agent.SeverityError {
				msgs = append(msgs, fmt.Sprintf("%s: %s", o.Code, o.Message))
				continue
			}
			fmt.Fprintf(warnings, "[warning] [%s] %s\n", o.Code, o.Message)
			if o.Fix != "" {
				fmt.Fprintf(warnings, "          %s\n", o.Fix)
			}
		}
		if len(msgs) == 0 {
			return nil
		}
		return fmt.Errorf("%s", strings.Join(msgs, "\n"))
	}
}

// runValidatePageType handles --type page: parses the page artifact
// (page.schema.md shape, including any embedded ## Layout block per
// layout.schema.md) and, when a Layout is present, runs the deep layout
// validator against the resolved --adapter. Codes and fix messages match
// layout.schema.md's "Validation pass" table; a page with no Layout
// block validates clean by definition (nothing layout-shaped to check).
func runValidatePageType(cmd *cobra.Command, path string) error {
	page, err := parser.ParsePageFile(path)
	if err != nil {
		return outputValidate(cmd, path, agent.ValidateLayoutParseError(err, loadValidateAdapter()))
	}

	// Cross-check the manifest's references against the surfaces.
	//
	// This checked nothing at all before: a manifest naming a page no feature
	// targets, a region nothing declares and a fragment no surface produces
	// validated OK. A manifest is a set of references; a validator that never
	// resolves them is checking that the file is well-formed YAML, which the
	// parser already established.
	errs := validatePageReferences(cmd, path, page)

	if page.Layout != nil {
		errs = append(errs, agent.ValidateLayoutDeep(page.Layout, loadValidateAdapter())...)
	}
	return outputValidate(cmd, path, errs)
}

// validatePageReferences resolves every @feature/fragment reference in a page
// manifest against the surfaces that would produce it.
//
// Findings are warnings, not errors, and deliberately so: a manifest listing a
// fragment a feature has not written yet is the normal state of a page being
// designed ahead of its features, and blocking it would make the manifest
// unusable for the thing it is for. view-page reports the same drift at
// assembly time. What was missing was any report at all.
func validatePageReferences(cmd *cobra.Command, path string, page *parser.Page) []agent.ValidationError {
	cfg, err := mustContext(cmd)
	if err != nil {
		// No resolved root — nothing to resolve references against. The
		// well-formedness checks above still ran.
		return nil
	}
	fragments, err := parser.ScanAllSurfaces(filepath.Join(cfg.Root.Path, config.SpecDir))
	if err != nil {
		return nil
	}

	// The filename stem is the page's identity, not the `name:`/heading. That
	// is what view-page keys on (spec/pages/<page>.page.md) and what a
	// surface fragment's `page:` names. Comparing against the heading instead
	// reports "no fragment targets this page" for every manifest whose title
	// is capitalised — which is all of them.
	pageName := strings.TrimSuffix(filepath.Base(path), ".page.md")

	produced := map[string]bool{}
	targetsThisPage := 0
	for _, f := range fragments {
		produced[fmt.Sprintf("@%s/%s", f.Feature, parser.Slugify(f.Name))] = true
		if f.Page == pageName {
			targetsThisPage++
		}
	}

	var errs []agent.ValidationError
	if targetsThisPage == 0 {
		errs = append(errs, agent.ValidationError{
			Code:     "page-has-no-fragments",
			Message:  fmt.Sprintf("no surface fragment targets page %q, so this manifest orders nothing", pageName),
			Context:  "page",
			Fix:      "set `page: " + pageName + "` on the surface fragments this page is meant to assemble, or delete the manifest",
			Severity: "warning",
		})
	}

	for _, region := range page.Regions {
		for _, ref := range region.Components {
			if produced[ref] {
				continue
			}
			errs = append(errs, agent.ValidationError{
				Code:     "page-fragment-unresolved",
				Message:  fmt.Sprintf("%s is listed under region %q but no surface produces it", ref, region.Name),
				Context:  "regions." + region.Name,
				Fix:      "correct the reference, or add the fragment to the owning feature's surface",
				Severity: "warning",
			})
		}
	}

	errs = append(errs, sharedRegionWarnings(fragments, page, pageName)...)
	return errs
}

// sharedRegionWarnings reports every region on the named page that two or more
// different features contribute fragments to. The full cross-feature fragment
// set is already in hand (ScanAllSurfaces), so the collision is a grouping, not
// a second walk.
//
// Two features stacking in one region is not by itself a defect — but it is the
// exact shape behind the "a working component never appears" mystery, where the
// first feature's assembly silently wins over the second's. WP3 named the stack
// as a `surface-region-shared` warning; WP8 escalates the *unresolved* subset
// to a blocking `surface-region-conflict` error, because by now the tool offers
// two ways to order a legitimate stack and a stack using neither is a defect,
// not a heads-up:
//
//   - a page manifest that lists the region (the designer locked its order), or
//   - a `supersedes:` annotation on one occupant naming another (this replaces
//     that).
//
// A resolved stack stays a warning: the reviewer already annotated it, but the
// stack is still worth seeing. An exact-slot collision (same order, different
// features) with no resolution gets a sharper conflict message, because there
// the assembler picks a winner with nothing to separate the two; assembleRegions
// stays the view-time reporter for that same case.
func sharedRegionWarnings(fragments []parser.Fragment, page *parser.Page, pageName string) []agent.ValidationError {
	type occupant struct {
		feature    string
		fragment   string
		order      int
		ref        string
		supersedes string
	}
	byRegion := map[string][]occupant{}
	for _, f := range fragments {
		if f.Page != pageName {
			continue
		}
		region := f.Region
		if region == "" {
			region = "main"
		}
		ref := fmt.Sprintf("@%s/%s", f.Feature, parser.Slugify(f.Name))
		byRegion[region] = append(byRegion[region], occupant{f.Feature, f.Name, f.Order, ref, strings.TrimSpace(f.Supersedes)})
	}

	// Regions the manifest orders — a region heading listing at least one
	// fragment is the designer locking that region's sequence.
	orderedByManifest := map[string]bool{}
	if page != nil {
		for _, r := range page.Regions {
			name := strings.ToLower(strings.TrimSpace(r.Name))
			if name == "" {
				name = "main"
			}
			if len(r.Components) > 0 {
				orderedByManifest[name] = true
			}
		}
	}

	regionNames := make([]string, 0, len(byRegion))
	for name := range byRegion {
		regionNames = append(regionNames, name)
	}
	sort.Strings(regionNames)

	var errs []agent.ValidationError
	for _, region := range regionNames {
		occ := byRegion[region]
		sort.Slice(occ, func(i, j int) bool {
			if occ[i].feature != occ[j].feature {
				return occ[i].feature < occ[j].feature
			}
			if occ[i].fragment != occ[j].fragment {
				return occ[i].fragment < occ[j].fragment
			}
			return occ[i].order < occ[j].order
		})

		features := map[string]bool{}
		refs := map[string]bool{}
		for _, o := range occ {
			features[o.feature] = true
			refs[o.ref] = true
		}
		if len(features) < 2 {
			continue
		}

		// Resolution: a manifest that orders the region, or a supersedes:
		// among the occupants naming another occupant in the same region.
		resolvedBySupersedes := false
		for _, o := range occ {
			if o.supersedes != "" && refs[o.supersedes] {
				resolvedBySupersedes = true
				break
			}
		}
		resolved := orderedByManifest[region] || resolvedBySupersedes

		exactSlot := 0
		for i := 0; i < len(occ); i++ {
			for j := i + 1; j < len(occ); j++ {
				if occ[i].order > 0 && occ[i].order == occ[j].order && occ[i].feature != occ[j].feature {
					exactSlot = occ[i].order
				}
			}
		}

		parts := make([]string, 0, len(occ))
		for _, o := range occ {
			parts = append(parts, o.ref)
		}

		if resolved {
			msg := fmt.Sprintf("region %q on page %q is contributed to by %d features (%s) — ordered by a manifest or a supersedes: annotation, so the stack is intentional", region, pageName, len(features), strings.Join(parts, ", "))
			errs = append(errs, agent.ValidationError{
				Code:     "surface-region-shared",
				Message:  msg,
				Context:  "regions." + region,
				Fix:      "none required — this is a note that an intentional cross-feature stack lives here",
				Severity: "warning",
			})
			continue
		}

		msg := fmt.Sprintf("region %q on page %q is contributed to by %d features (%s) with nothing ordering them — one feature's assembly silently wins over the other", region, pageName, len(features), strings.Join(parts, ", "))
		if exactSlot > 0 {
			msg = fmt.Sprintf("region %q on page %q holds fragments from different features at the same order %d (%s) with nothing to separate them — the assembler picks a winner blind", region, pageName, exactSlot, strings.Join(parts, ", "))
		}
		errs = append(errs, agent.ValidationError{
			Code:     "surface-region-conflict",
			Message:  msg,
			Context:  "regions." + region,
			Fix:      "order the fragments with a page manifest (parlay lock-page), or add supersedes: @feature/fragment to the occupant that replaces the other",
			Severity: "error",
		})
	}
	return errs
}

// supersedesConflicts reports two-headed supersedes chains across every
// feature's surface: two fragments that both name the SAME @feature/fragment in
// their supersedes:. The composition forks — no single winner exists — exactly
// the way two amendments carrying the same sequence number collide, so it is an
// error rather than a warning. Reported project-wide because supersedes crosses
// feature boundaries and no one page owns the pair.
// Delegates to the resolver rather than re-deriving the answer. This function
// used to detect two-headed forks with its own walk and nothing applied the
// result; now the same walk that RESOLVES the composition reports it, so a
// shape the validator calls clean cannot be one the resolver silently treats
// differently. It also picks up the three refusals a detect-only pass had no
// reason to look for — an unknown target, a cycle, and a cross-slot
// annotation — because you only need those once you try to apply the edge.
func supersedesConflicts(specDir string) []agent.ValidationError {
	fragments, err := parser.ScanAllSurfaces(specDir)
	if err != nil {
		return nil
	}
	return agent.ResolveActiveView(fragments).Errors
}

// sharedInfrastructureConcepts scans every feature's infrastructure.md and
// reports each architectural concept that two or more different features
// constrain — one infrastructure-concept-shared warning per concept, named by
// its normalized Affects: value. Two features holding invariants over the same
// concept (a cache, a boundary, a probe) is the third composition mystery: two
// individually-correct invariants that jointly forbid something, with every
// per-feature signal green. No lexical check can decide whether the invariants
// actually contradict, so this names the pair for a human rather than judging
// it — the same warning-not-error stance the whole detection tier takes.
//
// The trigger is a concept constrained by more than one distinct FEATURE, not
// merely more than one fragment: a single feature legitimately splitting its
// own constraints across two fragments is not the cross-feature hazard this
// exists to surface, and warning on it would be noise on ordinary specs.
func sharedInfrastructureConcepts(specDir string) []agent.ValidationError {
	fragments, err := parser.ScanAllInfrastructure(specDir)
	if err != nil {
		return nil
	}

	type contributor struct {
		feature  string
		fragment string
	}
	// Preserve first-seen concept order for a stable listing, and keep the
	// human-readable form of the concept alongside its normalized key.
	byConcept := map[string][]contributor{}
	display := map[string]string{}
	var order []string
	for _, f := range fragments {
		concept := strings.Join(strings.Fields(strings.ToLower(f.Affects)), " ")
		if concept == "" {
			continue
		}
		if _, seen := byConcept[concept]; !seen {
			order = append(order, concept)
			display[concept] = strings.TrimSpace(f.Affects)
		}
		byConcept[concept] = append(byConcept[concept], contributor{f.Feature, f.Name})
	}

	var errs []agent.ValidationError
	for _, concept := range order {
		contribs := byConcept[concept]
		sort.Slice(contribs, func(i, j int) bool {
			if contribs[i].feature != contribs[j].feature {
				return contribs[i].feature < contribs[j].feature
			}
			return contribs[i].fragment < contribs[j].fragment
		})
		features := map[string]bool{}
		for _, c := range contribs {
			features[c.feature] = true
		}
		if len(features) < 2 {
			continue
		}
		parts := make([]string, 0, len(contribs))
		for _, c := range contribs {
			parts = append(parts, fmt.Sprintf("%s/%s", c.feature, c.fragment))
		}
		errs = append(errs, agent.ValidationError{
			Code:     "infrastructure-concept-shared",
			Message:  fmt.Sprintf("concept %q is constrained by %d features (%s) — one implementation must satisfy every invariant they place on it", display[concept], len(features), strings.Join(parts, ", ")),
			Context:  "affects:" + concept,
			Fix:      "read the fragments together; if their invariants cannot both hold, record which supersedes the other",
			Severity: "warning",
		})
	}
	return errs
}

// runValidateLayoutType handles --type layout: validates a standalone
// *.layout.yaml file (see layout.schema.md's "Top-level structure" —
// a layout block carries the same three top-level keys whether embedded
// in a page artifact or standalone). internal/parser only exposes a
// loader for the embedded-in-page-markdown form (ParsePageFile), so a
// standalone file is validated by wrapping its raw YAML in a minimal
// synthetic page body and running it through that same, fully-tested
// parser path — this reuses every parser-level shape check (missing
// componentVocabulary/schema_version, wiring rejection, raw-spacing
// format) instead of re-implementing a second, divergent decode path.
func runValidateLayoutType(cmd *cobra.Command, path string) error {
	layout, err := loadStandaloneLayout(path)
	if err != nil {
		return outputValidate(cmd, path, agent.ValidateLayoutParseError(err, loadValidateAdapter()))
	}
	return outputValidate(cmd, path, agent.ValidateLayoutDeep(layout, loadValidateAdapter()))
}

// loadStandaloneLayout reads a standalone *.layout.yaml file and parses
// it via parser.ParsePageFile by wrapping its raw content in a minimal
// synthetic page body under a ## Layout heading, then returns the parsed
// Layout. See runValidateLayoutType for why the wrap exists.
func loadStandaloneLayout(path string) (*parser.Layout, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read layout file %s: %w", path, err)
	}
	wrapped := "---\nname: " + filepath.Base(path) + "\n---\n\n## Layout\n\n```yaml\n" + strings.TrimRight(string(raw), "\n") + "\n```\n"
	tmp, err := os.CreateTemp("", "parlay-layout-*.md")
	if err != nil {
		return nil, fmt.Errorf("create temp file for layout validation: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(wrapped); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("write temp file for layout validation: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close temp file for layout validation: %w", err)
	}
	page, err := parser.ParsePageFile(tmp.Name())
	if err != nil {
		// The parser's error text embeds the temp file's ephemeral path;
		// callers (and anyone reading the surfaced message) care about
		// the original standalone layout file, not the synthetic wrapper.
		return nil, fmt.Errorf("%s", strings.ReplaceAll(err.Error(), tmp.Name(), path))
	}
	return page.Layout, nil
}

// loadValidateAdapter loads the --adapter flag's file (if set) for
// --type page/layout. Returns nil when --adapter is unset or fails to
// load — adapter-dependent checks (component-type membership, variant,
// property, disallowed-child, vocabulary match, token membership)
// degrade gracefully to "skipped" in that case; adapter-independent
// checks (schema_version, wiring, raw-spacing format) still run.
func loadValidateAdapter() *agent.Adapter {
	if validateAdapter == "" {
		return nil
	}
	adapter, err := agent.LoadAdapterFile(validateAdapter)
	if err != nil {
		return nil
	}
	return adapter
}

// blockingCount returns how many findings are error-severity. A finding
// with no severity set is treated as blocking, preserving the behaviour of
// every legacy validator path that never populated the field.
//
// Splitting this out is what makes `ok` mean the same thing here as
// `ready` does in check-buildfile: previously every finding landed under
// "errors" with no severity, so a warning-severity rule (e.g.
// plan-create-collision, which fires on any buildfile whose code has been
// generated) failed the command outright and no caller could tell the
// difference.
func blockingCount(findings []agent.ValidationError) int {
	n := 0
	for _, f := range findings {
		if f.Severity != string(agent.SeverityWarning) {
			n++
		}
	}
	return n
}

func outputValidate(cmd *cobra.Command, path string, errors []agent.ValidationError) error {
	blocking := blockingCount(errors)

	if validateJSON {
		result := validateJSONResult{
			Path:   path,
			Type:   validateType,
			OK:     blocking == 0,
			Errors: errors,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		if blocking > 0 {
			return NewExitCodeError(1)
		}
		return nil
	}

	// Text output (default)
	if len(errors) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "OK")
		return nil
	}
	if blocking == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "OK (%d warning(s))\n", len(errors))
		for _, e := range errors {
			fmt.Fprintf(cmd.ErrOrStderr(), "  [warning] [%s] %s\n", e.Code, e.Message)
		}
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "FAIL: %d issue(s)\n", len(errors))
	for _, e := range errors {
		fmt.Fprintf(cmd.ErrOrStderr(), "  [%s] %s\n", e.Code, e.Message)
		if e.Fix != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "    fix: %s\n", e.Fix)
		}
	}
	return NewExitCodeError(1)
}

// validateInfrastructureDeepAdapter bridges ValidateInfrastructureDeep's
// (errors, portabilityWarnings) return into the agent.Validator shape the
// command's dispatch table expects. Portability warnings are advisory and do
// not fail the command.
func validateInfrastructureDeepAdapter(path string, content []byte) error {
	errs, warnings := agent.ValidateInfrastructureDeep(path)
	var msgs []string
	for _, e := range errs {
		// capabilities-prose-only is advisory, not a failure. An
		// infrastructure.md containing only architectural prose — boundaries,
		// probes, allowlists, dependency pins — is one of the documented valid
		// artifact shapes; capabilities.yaml and infrastructure.md are
		// co-equal, not alternatives with a preferred one. The code exists to
		// say migrate-capabilities has nothing to extract here, which is
		// information, and grading it blocking would reject a correct file.
		//
		// It carries no entry in ruleSeverityTable, so it never got a severity
		// at all; that is why wiring the deep validator surfaced it as an error
		// on the first real file it saw.
		if e.Code == "capabilities-prose-only" {
			fmt.Fprintf(os.Stderr, "[INFO] %s: %s\n", e.Code, e.Message)
			continue
		}
		msgs = append(msgs, fmt.Sprintf("%s: %s", e.Code, e.Message))
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "[WARN] portability: %s / %s — %s\n", w.Fragment, w.Field, w.Suggestion)
	}
	if len(msgs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(msgs, "\n"))
}

// renderTestcasesOutcomes splits v2 testcases findings by severity: errors fail
// the command, warnings go to stderr. The suite-shape rules a standalone file
// check can honestly assess all run regardless of the coverage inputs; the two
// coverage walkers are as thorough as those inputs allow.
func renderTestcasesOutcomes(outcomes []agent.ValidationOutcome) error {
	var msgs []string
	for _, o := range outcomes {
		if o.Severity == agent.SeverityError {
			msgs = append(msgs, fmt.Sprintf("%s: %s", o.Code, o.Message))
		} else {
			fmt.Fprintf(os.Stderr, "[WARN] %s: %s\n", o.Code, o.Message)
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(msgs, "\n"))
}

// testcasesCoverageInputs derives the operation- and criterion-coverage inputs
// for a testcases.yaml at path from the feature's contract artifacts.
//
// The path lives under .parlay/build/<feature>/, so the feature is the build
// path's parent relative to the active root's BuildRoot(). From it the feature's
// capabilities.yaml supplies every canonical operation (the operation walker's
// subjects) and every verify: bullet those operations carry; surface.yaml
// supplies every bullet its fragments carry; coverage-decisions.yaml supplies the
// criteria a person deliberately excused, applied only while the ledger is
// still bound to the contract it was granted against.
//
// Criteria are gathered per BULLET, not per entry. Gathering them per entry —
// one ref for an operation with five bullets — is what let a single case mark
// all five discharged.
//
// Every resolution failure returns whatever was gathered so far rather than an
// error. Absence of a domain artifact is a normal state — a feature with no
// capabilities.yaml has no operations to cover, not a broken one — so the
// walker it feeds simply reports nothing. Passing empty inputs is not a silent
// partial: with no declared operations or criteria there is nothing to find
// uncovered, which is a different answer from reporting everything as covered.
// criteriaFor expands one contract entry's verify: list into its individual
// criteria, each carrying the entry's ref and its own text.
func criteriaFor(ref string, verify []string) []agent.CriterionRef {
	out := make([]agent.CriterionRef, 0, len(verify))
	for _, v := range verify {
		text := agent.CanonicalCriterionText(v)
		if text == "" {
			continue
		}
		out = append(out, agent.CriterionRef{Ref: ref, Text: text})
	}
	return out
}

func testcasesCoverageInputs(cmd *cobra.Command, path string) agent.TestcasesV2Input {
	in := agent.TestcasesV2Input{Path: path}

	pctx := config.FromCtx(cmd.Context())
	if pctx == nil {
		return in
	}

	// Feature = the build path's parent, relative to BuildRoot(). A path that
	// is not under BuildRoot (a standalone file handed by absolute path, say)
	// yields no feature, and the walkers stay quiet.
	// Symlinks resolved on both sides, for the reason criteriaPresenceInputs
	// documents: on macOS /tmp is a symlink to /private/tmp, so a file named
	// through one against a root resolved through the other yields "../../.."
	// and the feature fails to resolve — disabling both coverage walkers with
	// no sign they were skipped.
	abs, err := resolvePath(path)
	if err != nil {
		return in
	}
	feature, err := filepath.Rel(resolvedOrSelf(pctx.BuildRoot()), filepath.Dir(abs))
	if err != nil || feature == "." || strings.HasPrefix(feature, "..") {
		return in
	}

	featureDir := pctx.FeaturePath(feature)

	// capabilities.yaml → canonical operations + operation criteria. The ref is
	// normalized from the file's own feature: field, which is what the operation
	// suites' operation: and any operation criterion.ref cite; a capabilities.yaml
	// with no feature: cannot form a reliable ref, so its operations are skipped.
	if caps, capErr := parser.ParseCapabilities(filepath.Join(featureDir, "capabilities.yaml")); capErr == nil && caps.Feature != "" {
		in.ContractResolved = true
		// The declared revision is what lets the transitional diagnostics
		// graduate: a file at the current shape is one where these facts could
		// have been recorded, so omitting them is an error rather than a
		// warning. Absent reads as legacy, which keeps the warning.
		in.Revisions.Capabilities = caps.SchemaVersion
		for _, op := range caps.Operations {
			if op.ID == "" {
				continue
			}
			ref := parser.NormalizeOperationID(caps.Feature, op.ID)
			in.CanonicalOperations = append(in.CanonicalOperations, ref)
			in.Criteria = append(in.Criteria, criteriaFor(ref, op.Verify)...)
		}
	}

	// surface.yaml (or legacy surface.md) → fragment criteria. A fragment
	// carrying verify: is a criterion the criterion walker requires a case for,
	// cited as @<feature>/fragment:<name>.
	if surfacePath := parser.ResolveSurfacePath(featureDir); surfacePath != "" {
		if fragments, fErr := parser.ParseSurfaceFile(surfacePath); fErr == nil {
			in.ContractResolved = true
			for _, f := range fragments {
				if f.Name == "" || f.Feature == "" {
					continue
				}
				ref := fmt.Sprintf("@%s/fragment:%s", f.Feature, f.Name)
				in.Criteria = append(in.Criteria, criteriaFor(ref, f.Verify)...)
			}
		}
	}

	// Exceptions → ExemptCriteria, freshness-bound.
	//
	// The legacy coverage-review.yaml read that used to sit here folded
	// cr.Exemptions in without touching either hash, so nothing bound an
	// exemption to the artifacts it was granted against — the blanket gate was
	// the only thing enforcing that, and it is going. Left as it was, removing
	// the gate would have converted every recorded exemption into a permanent
	// unconditional waiver, aimed precisely at the criteria a person once said
	// needed no test, and silently.
	//
	// A stale ledger now excuses NOTHING and reports why, rather than being
	// dropped: dropping turns each waiver back into an uncovered criterion,
	// which under warning severities may still proceed, leaving freshness
	// advisory.
	if pctx != nil {
		if exceptions, exErr := loadCoverageExceptions(pctx, feature); exErr == nil && exceptions != nil {
			if current, cErr := CurrentCriteria(pctx, feature); cErr == nil {
				verdict := EvaluateCoverageExceptions(pctx.Root.Path, exceptions, current)
				in.ExemptCriteria = verdict.Exempt
			}
		}
	}

	return in
}

// declaredCapabilityEntities returns the entity names declared in the resolved
// root's domain-model.yaml, or nil when there is no resolvable model.
//
// It exists so `validate --type capabilities` can perform the cross-artifact
// check capabilities.schema.md documents: subject.entity and output.entity name
// declared entities. Nothing loaded the model before, so the reference was
// declared and never checked.
//
// Every failure path returns nil rather than an error. A missing or unparseable
// domain model is a condition with its own diagnostics under `--type
// domain-model`; surfacing it a second time here, as a capabilities failure,
// would report one problem as two and point the author at the wrong file.
func declaredCapabilityEntities(cmd *cobra.Command) []string {
	pctx := config.FromCtx(cmd.Context())
	if pctx == nil {
		return nil
	}
	model, err := pctx.LoadDomainModel()
	if err != nil || model == nil {
		return nil
	}
	names := make([]string, 0, len(model.Entities))
	for _, e := range model.Entities {
		if e.Name != "" {
			names = append(names, e.Name)
		}
	}
	return names
}

// proposedCapabilityEntities maps each entity a feature's contribution
// proposes to the feature proposing it. Nil when the project has no
// contributions, which restores the previous behaviour exactly — an
// undeclared entity is an undeclared entity when nothing proposes it.
func proposedCapabilityEntities(cmd *cobra.Command) map[string]string {
	pctx := config.FromCtx(cmd.Context())
	if pctx == nil {
		return nil
	}
	return agent.ProposedEntities(loadContributions(pctx))
}

// validateDomainModelAdapter runs the domain-model validator and splits its
// findings by severity: warnings go to stderr, errors fail the command.
//
// This is where the severity filter belongs, and its absence is what shaped the
// validator above. agent.ValidateDomainModel used to aggregate every finding into
// one error with no reference to severity, so a warning failed the build like an
// error — and the only way to keep a legacy operations: block from failing every
// project was to stop emitting the diagnostic on this path entirely, via a boolean
// threaded into the validator. The result was that `--type domain-model` said
// nothing about a deprecated block while `--type domain-model --json` reported it.
//
// Rendering is the command layer's job. With the filter here, the validator has
// one entry point and no mode flags, and both CLI paths agree about what the model
// contains — this one prints the deprecation as a warning rather than hiding it.
//
// Same shape as renderTestcasesOutcomes, deliberately: two commands rendering
// structured findings should not invent two conventions for it.
func validateDomainModelAdapter(path string, content []byte) error {
	var msgs []string
	for _, e := range agent.ValidateDomainModelStructuredMode(path, content, agent.ModeAuthoring) {
		if e.Severity == string(agent.SeverityWarning) {
			fmt.Fprintf(os.Stderr, "[WARN] %s: %s\n", e.Code, e.Message)
			continue
		}
		msgs = append(msgs, fmt.Sprintf("[%s] %s", e.Code, e.Message))
	}
	if len(msgs) == 0 {
		return nil
	}
	return fmt.Errorf("domain-model.yaml validation failed: %d issue(s)\n  %s",
		len(msgs), strings.Join(msgs, "\n  "))
}

// withCriteriaPresence wraps a per-file validator so that validating a
// feature's surface.yaml or capabilities.yaml also reports contract entries
// carrying no verify: at all.
//
// The findings go to stderr as warnings rather than into the wrapped
// validator's error, because they are facts about the FEATURE's contract
// rather than about the file's own shape — a fragment missing criteria does
// not make surface.yaml invalid, and failing the file for it would block a
// validate the author ran for an unrelated reason.
//
// When the file is not under a resolvable feature (a standalone path, no
// project context), presence is skipped: the walker would have no contract to
// read and its silence would be an artifact of the missing input.
func withCriteriaPresence(cmd *cobra.Command, inner func(string, []byte) error) func(string, []byte) error {
	return func(path string, content []byte) error {
		if in, ok := criteriaPresenceInputs(cmd, path); ok {
			for _, o := range agent.ValidateCriteriaPresence(agent.ModeAuthoring, in) {
				fmt.Fprintf(cmd.ErrOrStderr(), "[WARN] %s: %s\n", o.Code, o.Message)
			}
		}
		return inner(path, content)
	}
}

// criteriaPresenceInputs resolves the feature owning path and loads its
// contract. The bool reports whether a feature resolved at all.
func criteriaPresenceInputs(cmd *cobra.Command, path string) (agent.CriteriaPresenceInput, bool) {
	var in agent.CriteriaPresenceInput

	pctx := config.FromCtx(cmd.Context())
	if pctx == nil {
		return in, false
	}
	// Both sides go through EvalSymlinks before Rel. On macOS /tmp is a symlink
	// to /private/tmp, so a file named through one and a root resolved through
	// the other produce a Rel of "../../..." and the feature silently fails to
	// resolve — the check would then skip with no sign it had.
	abs, err := resolvePath(path)
	if err != nil {
		return in, false
	}
	feature, err := filepath.Rel(resolvedOrSelf(pctx.IntentsRoot()), filepath.Dir(abs))
	if err != nil || feature == "." || strings.HasPrefix(feature, "..") {
		return in, false
	}
	featureDir := pctx.FeaturePath(feature)

	in.Feature = feature
	if surfacePath := parser.ResolveSurfacePath(featureDir); surfacePath != "" {
		in.HasSurface = true
		if frags, fErr := parser.ParseSurfaceFile(surfacePath); fErr == nil {
			in.Fragments = frags
		}
	}
	if caps, cErr := parser.ParseCapabilities(filepath.Join(featureDir, "capabilities.yaml")); cErr == nil {
		in.Operations = caps.Operations
	}
	return in, true
}

// resolvePath makes a path absolute and resolves any symlinks in it.
func resolvePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return resolvedOrSelf(abs), nil
}

// resolvedOrSelf resolves symlinks, falling back to the input when it cannot —
// a path that does not exist yet is not a reason to give up on comparing it.
func resolvedOrSelf(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}
