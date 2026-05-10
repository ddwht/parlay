// parlay-feature: parlay-tool/multi-adapter
// parlay-component: validation-mode-dispatch
// parlay-artifact: test

package agent

import "testing"

func TestRuleSeverity_DomainOperationsDeprecatedDiffersByMode(t *testing.T) {
	if got := RuleSeverity("domain-operations-deprecated", ModeAuthoring); got != SeverityWarning {
		t.Errorf("authoring: got %q, want warning", got)
	}
	if got := RuleSeverity("domain-operations-deprecated", ModeBuild); got != SeverityError {
		t.Errorf("build: got %q, want error", got)
	}
}

func TestRuleSeverity_DefaultsToError(t *testing.T) {
	if got := RuleSeverity("unknown-rule-code", ModeAuthoring); got != SeverityError {
		t.Errorf("default authoring: got %q, want error", got)
	}
	if got := RuleSeverity("unknown-rule-code", ModeBuild); got != SeverityError {
		t.Errorf("default build: got %q, want error", got)
	}
}

func TestNewOutcome_AppliesRuleSeverity(t *testing.T) {
	o := NewOutcome(ModeAuthoring, "domain-operations-deprecated", "msg")
	if o.Severity != SeverityWarning {
		t.Errorf("severity: got %q, want warning", o.Severity)
	}
	if o.Mode != ModeAuthoring {
		t.Errorf("mode: got %q", o.Mode)
	}
	if o.Message != "msg" {
		t.Errorf("message: got %q", o.Message)
	}
}

func TestFilterMode_AuthoringKeepsAll(t *testing.T) {
	outcomes := []ValidationOutcome{
		{Code: "a", Severity: SeverityWarning},
		{Code: "b", Severity: SeverityError},
	}
	out := FilterMode(ModeAuthoring, outcomes)
	if len(out) != 2 {
		t.Errorf("authoring: got %d, want 2", len(out))
	}
}

func TestFilterMode_BuildKeepsErrorsOnly(t *testing.T) {
	outcomes := []ValidationOutcome{
		{Code: "a", Severity: SeverityWarning},
		{Code: "b", Severity: SeverityError},
	}
	out := FilterMode(ModeBuild, outcomes)
	if len(out) != 1 || out[0].Code != "b" {
		t.Errorf("build: got %+v", out)
	}
}
