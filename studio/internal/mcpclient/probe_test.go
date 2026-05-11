// parlay-feature: studio-foundation/figma-mcp-client
// parlay-component: cross-cutting/figma-mcp-startup-probe

package mcpclient

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestProbeRejectsDesktopShape asserts that endpoints matching the desktop
// MCP server signature (localhost-bound, non-HTTPS) are rejected at probe
// time with the stable code figma-mcp-endpoint-unsupported.
func TestProbeRejectsDesktopShape(t *testing.T) {
	cases := []string{
		"http://localhost:1234",
		"http://127.0.0.1:1234",
		"https://localhost:1234",
		"https://127.0.0.1:1234",
		"ftp://example.com",
	}
	for _, endpoint := range cases {
		t.Run(endpoint, func(t *testing.T) {
			_, err := Probe(context.Background(), endpoint)
			if !errors.Is(err, ErrEndpointUnsupported) {
				t.Fatalf("Probe(%q): want ErrEndpointUnsupported, got %v",
					endpoint, err)
			}
		})
	}
}

// TestProbeErrorsCarrySetupDocLink asserts that every stable error message
// the probe can emit carries the canonical setup-doc tail. Operators who
// hit a failure mode always have a single landing page.
func TestProbeErrorsCarrySetupDocLink(t *testing.T) {
	const docPath = "studio/docs/figma-mcp-setup.md"
	for _, e := range []error{
		ErrEndpointUnsupported,
		ErrEndpointUnreachable,
		ErrSeatInsufficient,
	} {
		t.Run(e.Error(), func(t *testing.T) {
			if !strings.Contains(e.Error(), docPath) {
				t.Fatalf("error %q lacks setup-doc tail %q",
					e.Error(), docPath)
			}
		})
	}
}

// TestPickCanvasWriteSeat exercises the seat-tier check the probe uses to
// distinguish canvas-write-capable seats (Dev, Full) from lower tiers
// (View, Collab).
func TestPickCanvasWriteSeat(t *testing.T) {
	cases := []struct {
		name string
		in   []Plan
		want string
		ok   bool
	}{
		{"empty", nil, "", false},
		{"view only", []Plan{{Seat: "View"}}, "", false},
		{"collab only", []Plan{{Seat: "Collab"}}, "", false},
		{"dev seat", []Plan{{Seat: "Dev"}}, "Dev", true},
		{"full seat", []Plan{{Seat: "Full"}}, "Full", true},
		{"mixed dev first", []Plan{{Seat: "Dev"}, {Seat: "View"}}, "Dev", true},
		{"mixed view first", []Plan{{Seat: "View"}, {Seat: "Full"}}, "Full", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pickCanvasWriteSeat(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("pickCanvasWriteSeat: got (%q, %v), want (%q, %v)",
					got, ok, tc.want, tc.ok)
			}
		})
	}
}
