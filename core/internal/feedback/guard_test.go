package feedback_test

// parlay-feature: parlay-tool/feedback-mode
// parlay-component: recorder
// parlay-artifact: test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ddwht/parlay/core/internal/testsupport"
)

// TestFeedbackPayloadsCarryNoFreeText is the structural half of the
// guarantee. The property test in feedback_test.go proves that the values
// this package writes today are safe; this proves that a value someone
// adds tomorrow cannot quietly be unsafe.
//
// Modelled on TestNoDirectWritePrimitives in core/internal/atomicfile,
// including its reason for using an AST rather than grep: a comment
// mentioning the forbidden shape fails a substring check, and a call
// spread over two lines passes one.
//
// The rule is a DENYLIST of constructors, not an allowlist of values, and
// the distinction is worth stating because the first version got it wrong.
// An allowlist that accepts bare identifiers accepts everything — `msg :=
// err.Error()` then `Code: msg` passes — while one that rejects them
// rejects `Code: code`, the legitimate primary use. Sound static analysis
// here would need taint tracking through arbitrary helpers, which is not
// what a guard test should be.
//
// So this catches the shapes that visibly produce free text: formatting an
// interpolated string, unwrapping an error, joining a path, reading a file
// or the environment. It will not catch a local helper that returns
// something unsafe.
//
// That limit is acceptable because it is not the guarantee. The encoder
// (feedback.encodeValue) is: every value is checked against a charset at
// the write point and replaced with a sentinel if it fails, and
// TestNothingSensitiveReachesTheLog proves that on adversarial input. This
// guard exists to make a bad call site obvious in review rather than
// discoverable only by reading the log afterwards.
func TestFeedbackPayloadsCarryNoFreeText(t *testing.T) {
	root, err := testsupport.ModuleRoot(".")
	if err != nil {
		t.Fatalf("module root: %v", err)
	}

	// The packages that construct payloads. feedback itself is exempt: it
	// is where the encoder lives, and its own tests deliberately push
	// adversarial values through to prove the encoder catches them.
	guarded := []string{
		filepath.Join("core", "internal", "agent"),
		filepath.Join("core", "internal", "commands"),
	}

	payloadTypes := map[string]bool{
		"FindingData": true, "TallyData": true,
		"SessionData": true, "AgentData": true,
	}
	// Calls whose results are safe by construction.
	sanitizers := map[string]bool{
		"Hash": true, "Bucket": true, "CallerSite": true,
	}

	var checkedFields int
	for _, pkg := range guarded {
		dir := filepath.Join(root, pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir %s: %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			fset := token.NewFileSet()
			file, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				t.Fatalf("parse %s: %v", path, perr)
			}

			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				sel, ok := lit.Type.(*ast.SelectorExpr)
				if !ok || !payloadTypes[sel.Sel.Name] {
					return true
				}
				pkgIdent, ok := sel.X.(*ast.Ident)
				if !ok || pkgIdent.Name != "feedback" {
					return true
				}

				for _, elt := range lit.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					checkedFields++
					if safePayloadValue(kv.Value, sanitizers) {
						continue
					}
					field := "?"
					if k, ok := kv.Key.(*ast.Ident); ok {
						field = k.Name
					}
					pos := fset.Position(kv.Value.Pos())
					t.Errorf("%s:%d: feedback.%s field %s is assigned a value whose content is not known at review time.\n"+
						"Payload fields take a constant, a closed-vocabulary identifier, or feedback.Hash/Bucket/CallerSite.\n"+
						"Anything else risks putting a path, a message or a user's words into a log written to be sent.",
						filepath.Join(pkg, name), pos.Line, sel.Sel.Name, field)
				}
				return true
			})
		}
	}

	if checkedFields == 0 {
		t.Fatal("inspected zero payload fields — the AST matcher has drifted from how payloads are constructed")
	}
}

// freeTextProducers are the calls that visibly manufacture free text. A
// payload field assigned from one of these is the shape this guard exists
// to reject.
var freeTextProducers = map[string]bool{
	"Sprintf": true, "Sprint": true, "Sprintln": true, "Errorf": true,
	"Error":  true, // err.Error() — the channel a parse error takes
	"Join":   true, // filepath.Join / path.Join / strings.Join
	"Getenv": true, "ReadFile": true, "String": true,
	"TrimSpace": true, // trimming a thing does not make it safe
}

// safePayloadValue reports whether an expression is free of the
// obviously-leaking constructors. See the doc comment on the test for why
// this is a denylist and what it deliberately does not catch.
func safePayloadValue(e ast.Expr, sanitizers map[string]bool) bool {
	switch v := e.(type) {
	case *ast.CallExpr:
		if fn, ok := v.Fun.(*ast.SelectorExpr); ok && freeTextProducers[fn.Sel.Name] {
			return false
		}
		// Arguments are checked even for a sanitizer, so
		// feedback.Hash(fmt.Sprintf(...)) is rejected. A hash of a sentence
		// is harmless in itself; the shape is the finding, because someone
		// wrote it believing free text belongs in a payload.
		for _, arg := range v.Args {
			if !safePayloadValue(arg, sanitizers) {
				return false
			}
		}
		return true
	case *ast.BinaryExpr:
		return safePayloadValue(v.X, sanitizers) && safePayloadValue(v.Y, sanitizers)
	}
	return true
}

// A guard nobody has seen fail is a guard nobody knows works. These pin
// both directions on the expression matcher itself.
func TestGuardRejectsTheShapesItClaimsTo(t *testing.T) {
	sanitizers := map[string]bool{"Hash": true, "Bucket": true, "CallerSite": true}
	cases := []struct {
		expr string
		safe bool
	}{
		{`"authored-field-missing"`, true},
		{`code`, true},
		{`errs[i].Code`, true},
		{`string(mode)`, true},
		{`feedback.Hash(name)`, true},
		{`feedback.Bucket(n)`, true},
		{`"custom-" + feedback.Hash(name)`, true},

		{`fmt.Sprintf("%s: %s", path, msg)`, false},
		{`err.Error()`, false},
		{`filepath.Join(root, name)`, false},
		{`os.Getenv("HOME")`, false},
		{`strings.TrimSpace(line)`, false},
		{`feedback.Hash(fmt.Sprintf("%s", x))`, false},
		{`"prefix-" + err.Error()`, false},
	}
	for _, tc := range cases {
		e, err := parser.ParseExpr(tc.expr)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.expr, err)
		}
		if got := safePayloadValue(e, sanitizers); got != tc.safe {
			t.Errorf("safePayloadValue(%q) = %v, want %v", tc.expr, got, tc.safe)
		}
	}
}
