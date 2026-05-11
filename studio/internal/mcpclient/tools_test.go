// parlay-feature: studio-foundation/figma-mcp-client
// parlay-component: cross-cutting/bounded-figma-tool-surface

package mcpclient

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestDispatchRejectsExcludedTools asserts that the dispatch helper rejects
// every v1-excluded Figma tool name with figma-mcp-tool-unsupported, even
// though the underlying SDK would happily forward the call.
func TestDispatchRejectsExcludedTools(t *testing.T) {
	excluded := []string{
		"get_design_context",
		"get_screenshot",
		"generate_diagram",
		"get_figjam",
		"create_design_system_rules",
		"search_design_system",
		"generate_figma_design",
	}

	c := &Client{}
	for _, tool := range excluded {
		t.Run(tool, func(t *testing.T) {
			_, err := c.dispatch(context.Background(), tool, nil)
			if !errors.Is(err, ErrToolUnsupported) {
				t.Fatalf("dispatch(%q): want ErrToolUnsupported, got %v",
					tool, err)
			}
		})
	}
}

// TestDispatchAcceptsSupportedTools asserts that every tool on the supported
// allowlist passes the dispatch enforcement gate.
func TestDispatchAcceptsSupportedTools(t *testing.T) {
	c := &Client{}
	supported := []string{
		"use_figma", "create_new_file", "add_code_connect_map",
		"send_code_connect_mappings", "get_metadata", "get_code_connect_map",
		"get_code_connect_suggestions", "whoami",
	}
	for _, tool := range supported {
		t.Run(tool, func(t *testing.T) {
			_, err := c.dispatch(context.Background(), tool, nil)
			if errors.Is(err, ErrToolUnsupported) {
				t.Fatalf("dispatch(%q): supported tool was rejected as unsupported",
					tool)
			}
		})
	}
}

// TestWrapperTypesIsolation asserts that tools.go does not import any
// Studio-domain package. Wrapper input/output structs are wrapper-domain
// only; translation between wrapper types and Studio-domain types happens
// at the caller boundary.
func TestWrapperTypesIsolation(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "tools.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse tools.go: %v", err)
	}
	forbidden := []string{
		"studio/internal/domain",
		"studio/internal/designloop",
	}
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		for _, f := range forbidden {
			if strings.Contains(path, f) {
				t.Fatalf("tools.go imports Studio-domain package %q "+
					"(wrapper types must be isolated)", path)
			}
		}
	}
}
