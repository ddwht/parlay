// parlay-feature: parlay-tool
// parlay-component: validate
// parlay-extends: infrastructure-layer/InfrastructureValidationResult
// parlay-extends: studio-support/domain-model-yaml-migration/domain-model-validate-cli-type
// parlay-extends: parlay-tool/cross-cutting-target-paths/project-pass-validation-and-cli-flag

package commands

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ddwht/parlay/core/internal/agent"
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

func init() {
	validateCmd.Flags().StringVar(&validateType, "type", "", "File type: surface, buildfile, blueprint, yaml, infrastructure, domain-model, adapter, adapter-set, capabilities, coverage-review, testcases, page, layout")
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
	Note     string                 `json:"note,omitempty"`
}

func runValidate(cmd *cobra.Command, args []string) error {
	if validateProject {
		return runValidateProject(cmd)
	}

	path := args[0]

	// Structured JSON mode for domain-model (intent: json-validation-mode).
	// When --json is set with --type domain-model, emit the full per-violation
	// finding list as a bare JSON array (one entry per violation, [] for a
	// clean model) and exit 0 whether or not findings are present — a finding
	// list is a query result, not a command failure. Accepts `-` to read the
	// model from stdin. The non-JSON domain-model path below is unchanged.
	//
	// parlay-feature: studio-support/structured-domain-model-validation
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
		return fmt.Errorf("--type is required (one of: surface, buildfile, blueprint, yaml, infrastructure, domain-model, adapter-set, capabilities, coverage-review, page, layout)")
	}

	// page and layout route through the unified layout validator
	// (agent.ValidateLayoutDeep) rather than the plain Validator shape —
	// they need adapter resolution and structured, multi-error output.
	// Codes and fix messages follow layout.schema.md's "Validation pass"
	// table.
	//
	// parlay-feature: studio-support/page-layout-field
	// parlay-cross-cutting-id: layout-precheck-contract
	if validateType == "page" {
		return runValidatePageType(cmd, path)
	}
	if validateType == "layout" {
		return runValidateLayoutType(cmd, path)
	}

	var validator agent.Validator
	switch validateType {
	case "surface":
		validator = agent.ValidateSurface
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
		// parlay-feature: studio-support/domain-model-yaml-migration
		// parlay-component: validate (extends domain-model-validate-cli-type)
		validator = agent.ValidateDomainModel
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
		validator = validateTestcasesAdapter
	case "adapter":
		// Adapter files had no validate type at all before Section 10's
		// toolchain block. A validator nobody can call is the pattern this
		// consolidation found five separate instances of, so the type and
		// the rules land together.
		validator = validateAdapterFile
	case "capabilities":
		validator = wrapOutcomeValidator(agent.ValidateCapabilities)
	case "coverage-review":
		// coverage-review validation is hash-aware and needs full inputs;
		// surface a minimal YAML-shape check here.
		validator = agent.ValidateYAML
	default:
		return fmt.Errorf("unknown type %q — supported: surface, buildfile, blueprint, yaml, infrastructure, domain-model, adapter, adapter-set, capabilities, coverage-review, testcases", validateType)
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
// authoring mode, so domain-operations-deprecated surfaces at warning
// severity (the editor's context). A clean model prints "[]".
//
// The command exits 0 regardless of whether findings are present: the list
// is a query result, not a command failure. Unlike the generic
// outputValidate path, a non-empty list does NOT set a non-zero exit code.
//
// The positional path may be "-" to read the model bytes from stdin; a real
// path and "-" are mutually exclusive and, for identical bytes, produce an
// identical finding set. Even a read failure or unparseable input yields
// valid JSON on stdout (a single-element array), never a crash or non-JSON
// error.
//
// parlay-feature: studio-support/structured-domain-model-validation
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

	totalErrors := 0
	for _, v := range verdicts {
		totalErrors += len(v.Errors)
	}
	ok := totalErrors == 0

	if validateJSON {
		out := projectValidateJSONResult{
			Root:     root,
			OK:       ok,
			Features: verdicts,
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

	// Text output.
	if len(verdicts) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "OK (no buildfiles under %s/.parlay/build/)\n", root)
		return nil
	}
	if ok {
		fmt.Fprintf(cmd.OutOrStdout(), "OK (%d feature(s) validated)\n", len(verdicts))
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "FAIL: %d issue(s) across %d feature(s)\n", totalErrors, len(verdicts))
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
	if page.Layout == nil {
		return outputValidate(cmd, path, nil)
	}
	return outputValidate(cmd, path, agent.ValidateLayoutDeep(page.Layout, loadValidateAdapter()))
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

// validateAdapterFile checks an adapter file's toolchain block against
// adapter.schema.md Section 10. The rest of the adapter is validated at
// registration; this is the part that publishes a contract to third parties
// and therefore has to be enforced rather than described.
func validateAdapterFile(path string, content []byte) error {
	var doc struct {
		FileConventions struct {
			SourceRoot string `yaml:"source-root"`
		} `yaml:"file-conventions"`
		Toolchain *agent.Toolchain `yaml:"toolchain"`
	}
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return fmt.Errorf("adapter YAML parse error: %w", err)
	}
	var msgs []string
	for _, e := range agent.ValidateToolchain(doc.Toolchain, doc.FileConventions.SourceRoot) {
		msgs = append(msgs, fmt.Sprintf("%s: %s", e.Code, e.Message))
	}

	// Cross-block parity: componentVocabulary:/tokens: are the authoritative
	// declaration, vocabulary: is the Design Loop's derivation target, and an
	// author who edits one and forgets the other gets a silently stale
	// vocabulary. The check has existed since schema-consolidation and was
	// reachable from nowhere — `validate --type adapter` did not exist until
	// the toolchain rules needed it, so there was no command to attach it to.
	outcomes, perr := agent.CheckVocabularyBlockParity(agent.ModeAuthoring, path)
	if perr != nil {
		msgs = append(msgs, fmt.Sprintf("vocabulary-parity-unreadable: %v", perr))
	}
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

// validateTestcasesAdapter runs the v2 testcases validator.
//
// canonicalOperations is nil, which disables only the operation-coverage walker
// — that check needs the feature's capabilities.yaml, and this command is handed
// one file with no feature context. Passing nil is not a silent partial: with no
// declared operations there is nothing for the walker to find uncovered, so it
// reports nothing rather than reporting everything as covered. The suite-shape
// rules, which are what a standalone file check can honestly assess, all run.
func validateTestcasesAdapter(path string, content []byte) error {
	var msgs []string
	for _, o := range agent.ValidateTestcasesV2(agent.ModeAuthoring, path, content, nil) {
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
