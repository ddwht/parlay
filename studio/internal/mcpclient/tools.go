// parlay-feature: studio-foundation/figma-mcp-client
// parlay-component: cross-cutting/official-mcp-sdk-adoption
// parlay-component: cross-cutting/bounded-figma-tool-surface
// parlay-extends: studio-foundation/web-server-harness/cross-cutting/figma-mcp-phase-0-wiring

package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// supportedTools is the enumerated allowlist of Figma MCP tool names Studio
// is permitted to call. Adding a tool is a spec change against
// studio/spec/intents/studio-foundation/figma-mcp-client — not a code-only PR.
var supportedTools = map[string]struct{}{
	"use_figma":                    {},
	"create_new_file":              {},
	"add_code_connect_map":         {},
	"send_code_connect_mappings":   {},
	"get_metadata":                 {},
	"get_code_connect_map":         {},
	"get_code_connect_suggestions": {},
	"whoami":                       {},
}

// v1-excluded tool names — rejected by dispatch with figma-mcp-tool-unsupported.
// Each exclusion cites the figma-mcp-client intent:
//
//   - get_design_context         : lossy React+Tailwind output; breaks
//                                  component-identity round-trip
//   - get_screenshot             : visual-only, not load-bearing for the loop
//   - generate_diagram           : FigJam, out of scope
//   - get_figjam                 : FigJam, out of scope
//   - create_design_system_rules : authoring helper, out of scope
//   - search_design_system       : authoring helper, out of scope
//   - generate_figma_design      : overlaps use_figma; use_figma is the v1
//                                  write entry — switching is a v2-or-later
//                                  spec revision, not a runtime fallback
//
// Adding a wrapper method for any of these is a v1-excluded change; reviewers
// reject such PRs with a citation back to figma-mcp-client.

// --- Wrapper-domain input/output types ---
//
// These types are wrapper-domain only — they MUST NOT reference Studio-domain
// types like typed-tree nodes. Callers (internal/designloop and elsewhere)
// translate between wrapper types and Studio-domain types at the boundary.

// UseFigmaInput is the input to the use_figma tool wrapper.
type UseFigmaInput struct {
	NodeID string
	Params map[string]any
}

// UseFigmaOutput is the response from the use_figma tool wrapper.
type UseFigmaOutput struct {
	NodeID string
	Raw    map[string]any
}

// CreateNewFileInput is the input to the create_new_file tool wrapper.
type CreateNewFileInput struct {
	Name string
}

// CreateNewFileOutput is the response from the create_new_file tool wrapper.
type CreateNewFileOutput struct {
	FileKey string
	Raw     map[string]any
}

// AddCodeConnectMapInput is the input to the add_code_connect_map tool wrapper.
type AddCodeConnectMapInput struct {
	Mapping map[string]string
}

// AddCodeConnectMapOutput is the response from the add_code_connect_map tool wrapper.
type AddCodeConnectMapOutput struct {
	Raw map[string]any
}

// SendCodeConnectMappingsInput is the input to the send_code_connect_mappings tool wrapper.
type SendCodeConnectMappingsInput struct {
	FileKey  string
	Mappings []map[string]any
}

// SendCodeConnectMappingsOutput is the response from the send_code_connect_mappings tool wrapper.
type SendCodeConnectMappingsOutput struct {
	Raw map[string]any
}

// GetMetadataInput is the input to the get_metadata tool wrapper.
type GetMetadataInput struct {
	NodeID string
}

// GetMetadataOutput is the response from the get_metadata tool wrapper.
type GetMetadataOutput struct {
	XML string
	Raw map[string]any
}

// GetCodeConnectMapInput is the input to the get_code_connect_map tool wrapper.
type GetCodeConnectMapInput struct {
	FileKey string
}

// GetCodeConnectMapOutput is the response from the get_code_connect_map tool wrapper.
type GetCodeConnectMapOutput struct {
	Mapping map[string]string
	Raw     map[string]any
}

// GetCodeConnectSuggestionsInput is the input to the get_code_connect_suggestions tool wrapper.
type GetCodeConnectSuggestionsInput struct {
	NodeID string
}

// GetCodeConnectSuggestionsOutput is the response from the get_code_connect_suggestions tool wrapper.
type GetCodeConnectSuggestionsOutput struct {
	Suggestions []string
	Raw         map[string]any
}

// WhoamiOutput is the response from the whoami tool wrapper. The startup
// probe in probe.go consumes this shape directly.
type WhoamiOutput struct {
	Email string
	Plans []Plan
}

// Plan is one entry in the whoami response's plans[] array.
type Plan struct {
	Key  string
	Name string
	Seat string // "Dev", "Full", "View", or "Collab"
	Tier string
}

// --- Named wrapper methods (one per supported Figma tool) ---

// UseFigma is the canonical v1 write entry point — component instantiation,
// layout edits, frame creation. Switching to generate_figma_design is a
// v2-or-later spec revision, not a runtime fallback.
func (c *Client) UseFigma(ctx context.Context, in UseFigmaInput) (*UseFigmaOutput, error) {
	return callTyped[UseFigmaOutput](ctx, c, "use_figma", in)
}

// CreateNewFile creates an ephemeral frame container via the create_new_file tool.
func (c *Client) CreateNewFile(ctx context.Context, in CreateNewFileInput) (*CreateNewFileOutput, error) {
	return callTyped[CreateNewFileOutput](ctx, c, "create_new_file", in)
}

// AddCodeConnectMap establishes a Code Connect binding via the add_code_connect_map tool.
func (c *Client) AddCodeConnectMap(ctx context.Context, in AddCodeConnectMapInput) (*AddCodeConnectMapOutput, error) {
	return callTyped[AddCodeConnectMapOutput](ctx, c, "add_code_connect_map", in)
}

// SendCodeConnectMappings sends a batch of Code Connect bindings via send_code_connect_mappings.
func (c *Client) SendCodeConnectMappings(ctx context.Context, in SendCodeConnectMappingsInput) (*SendCodeConnectMappingsOutput, error) {
	return callTyped[SendCodeConnectMappingsOutput](ctx, c, "send_code_connect_mappings", in)
}

// GetMetadata reads node metadata via the get_metadata tool.
func (c *Client) GetMetadata(ctx context.Context, in GetMetadataInput) (*GetMetadataOutput, error) {
	return callTyped[GetMetadataOutput](ctx, c, "get_metadata", in)
}

// GetCodeConnectMap reads the Code Connect mapping via the get_code_connect_map tool.
func (c *Client) GetCodeConnectMap(ctx context.Context, in GetCodeConnectMapInput) (*GetCodeConnectMapOutput, error) {
	return callTyped[GetCodeConnectMapOutput](ctx, c, "get_code_connect_map", in)
}

// GetCodeConnectSuggestions requests identity-inference suggestions for new
// nodes via the get_code_connect_suggestions tool.
func (c *Client) GetCodeConnectSuggestions(ctx context.Context, in GetCodeConnectSuggestionsInput) (*GetCodeConnectSuggestionsOutput, error) {
	return callTyped[GetCodeConnectSuggestionsOutput](ctx, c, "get_code_connect_suggestions", in)
}

// Whoami invokes the identity probe tool. The startup probe in probe.go is
// the canonical caller.
func (c *Client) Whoami(ctx context.Context) (*WhoamiOutput, error) {
	return callTyped[WhoamiOutput](ctx, c, "whoami", struct{}{})
}

// dispatch is the internal enforcement helper. It checks the requested tool
// name against supportedTools and rejects anything outside the allowlist with
// the stable error ErrToolUnsupported (figma-mcp-tool-unsupported). On
// success it forwards the call to the persistent MCP session and returns
// the SDK's StructuredContent verbatim.
//
// dispatch is package-internal. The exported wrapper methods above call it
// with hard-coded tool names — there is no exported generic name+args entry
// point.
//
// The return type is `any` to match the SDK's StructuredContent field;
// callTyped performs the JSON round-trip into the typed Out shape.
func (c *Client) dispatch(ctx context.Context, toolName string, in any) (any, error) {
	if _, ok := supportedTools[toolName]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrToolUnsupported, toolName)
	}
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("%w: client not connected", ErrEndpointUnreachable)
	}
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: in,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEndpointUnreachable, err)
	}
	if result.IsError {
		return nil, fmt.Errorf("%w: tool=%s content=%s",
			ErrToolCallFailed, toolName, contentText(result.Content))
	}
	return result.StructuredContent, nil
}

// callTyped is the typed wrapper around dispatch. It marshals the raw
// StructuredContent value back to JSON and unmarshals into Out so the
// wrapper-method callers receive a typed, validated value.
func callTyped[Out any](ctx context.Context, c *Client, toolName string, in any) (*Out, error) {
	raw, err := c.dispatch(ctx, toolName, in)
	if err != nil {
		return nil, err
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: tool=%s marshal=%v",
			ErrResponseMalformed, toolName, err)
	}
	var out Out
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("%w: tool=%s unmarshal=%v",
			ErrResponseMalformed, toolName, err)
	}
	return &out, nil
}

// contentText flattens a CallToolResult.Content slice into a single
// human-readable string for inclusion in the wrapped error detail.
// Non-text content kinds (image, audio) are summarised by their type
// name so the operator sees something instead of "" when a tool errors
// with no text content.
func contentText(content []mcp.Content) string {
	if len(content) == 0 {
		return ""
	}
	out := ""
	for i, c := range content {
		if i > 0 {
			out += "; "
		}
		if t, ok := c.(*mcp.TextContent); ok {
			out += t.Text
			continue
		}
		out += fmt.Sprintf("<%T>", c)
	}
	return out
}
