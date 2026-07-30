package commands

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ddwht/parlay/internal/editor/domain"
	"github.com/ddwht/parlay/internal/editor/server"
	"github.com/ddwht/parlay/internal/editor/ui"
	"github.com/spf13/cobra"
)

// domainEditCmd is the whole of what absorbing Studio adds to parlay's surface.
//
// It was tempting to relocate Studio as a namespace — `parlay studio serve`,
// `parlay studio init`, `parlay studio domain-edit` — and that would have been
// wrong twice over. `parlay init` and `parlay upgrade` already deploy the agent
// surface, so a second pair under a `studio` prefix would be two commands doing
// one job. And a bare `serve` verb has no user: the server exists to host the
// editor, so "open the editor" is the operation and serving is how it happens.
//
// So: one command, no namespace, and the word "studio" stops appearing in the
// CLI at all — with one binary it names nothing a user can act on.
var domainEditCmd = &cobra.Command{
	Use:   "domain-edit",
	Short: "Open the domain-model editor in a browser",
	Long: `Boot the local editor and open the project's domain model in a browser.

Reads and writes the resolved root's domain-model.yaml. Validation runs against
the same rules ` + "`parlay validate --type domain-model`" + ` applies — the editor
refuses to save a model that would fail the build.

The server shuts down on an idle timeout so a forgotten tab does not hold a
port indefinitely.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEditor(cmd, "/domain-model")
	},
}

// serveCmd is the bare harness with no landing route, for exercising the server
// without the editor. Hidden and under `internal` because it is a testing
// affordance, not something a designer has a reason to run.
var serveCmd = &cobra.Command{
	Use:    "serve",
	Short:  "Boot the editor harness without opening a landing route (JSON APIs only)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEditor(cmd, "/")
	},
}

var (
	editorPort        int
	editorIdleTimeout string
	editorNoBrowser   bool
)

func init() {
	for _, c := range []*cobra.Command{domainEditCmd, serveCmd} {
		c.Flags().IntVar(&editorPort, "server-port", 0, "Port to bind (0 = let the OS choose)")
		c.Flags().StringVar(&editorIdleTimeout, "idle-timeout", "", "Shut down after this much inactivity (e.g. 30m; 0 disables)")
		c.Flags().BoolVar(&editorNoBrowser, "no-browser", false, "Do not open a browser on boot")
	}
}

// runEditor boots the harness with the editor subsystem registered.
//
// The flags are reassembled into the argument slice the studio config loader
// already parses, rather than reimplemented against its Config struct. That
// loader carries the five-source precedence rule (CLI > env > project file >
// user file > default) and is well covered; rewriting it to take a cobra
// FlagSet would trade tested behaviour for a rewrite that only looks tidier.
func runEditor(cmd *cobra.Command, browserPath string) error {
	cfg, err := mustContext(cmd)
	if err != nil {
		return err
	}
	root := cfg.Root.Path

	// --project pins the editor to the root parlay already resolved, so the
	// two resolvers cannot disagree about which project is being edited.
	bootArgs := []string{"--project", root}
	if cmd.Flags().Changed("server-port") {
		bootArgs = append(bootArgs, "--server-port", strconv.Itoa(editorPort))
	}
	if cmd.Flags().Changed("idle-timeout") && editorIdleTimeout != "" {
		bootArgs = append(bootArgs, "--idle-timeout", editorIdleTimeout)
	}
	if editorNoBrowser {
		bootArgs = append(bootArgs, "--no-browser")
	}

	err = server.Boot(cmd.Context(), server.BootDeps{
		Args:        bootArgs,
		Tools:       []server.ToolRegistration{domain.New(root, domainValidator)},
		UIBundle:    ui.Bundle{},
		BrowserPath: browserPath,
	})
	if err != nil {
		return fmt.Errorf("domain-edit: %w", err)
	}
	return nil
}

// OpenDomainEditor boots the editor in-process and blocks until the session
// ends. It is what the trio hook calls on confirmation.
//
// This replaces invokeStudioSubprocess, which shelled out to a second binary,
// normalized a start failure to exit code 127, and printed a launch-failure
// line when that binary exited non-zero. None of that has an analogue here: a
// function either returns an error or it does not.
func OpenDomainEditor(ctx context.Context, root string) error {
	return server.Boot(ctx, server.BootDeps{
		Args:        []string{"--project", root},
		Tools:       []server.ToolRegistration{domain.New(root, domainValidator)},
		UIBundle:    ui.Bundle{},
		BrowserPath: "/domain-model",
	})
}
