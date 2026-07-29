// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/studio-binary-boot-and-shutdown
// parlay-extends: studio-foundation/figma-mcp-via-host-agent/cross-cutting/retract-studio-direct-mcp-source-tree
// parlay-extends: studio-foundation/studio-deployer/cross-cutting/embedded-source-and-deployer-subcommands

// Command parlay-studio is the Parlay Studio binary. main() is intentionally
// thin — it dispatches on os.Args[1] across three modes:
//
//   - parlay-studio init    → run the deployer's first-time bootstrap subcommand
//   - parlay-studio upgrade → run the deployer's idempotent re-deploy subcommand
//   - parlay-studio         → (no subcommand) boot the server harness, the
//     default invocation that preserves the pre-deployer behavior.
//
// Cobra is intentionally not in Studio's go.mod; the entry point dispatches
// via os.Args directly. The deployer subcommands parse their own flags off
// the residual os.Args[2:] slice.
//
// The boot orchestration helper lives in studio/internal/server/boot.go so
// that the sequence is unit-testable via injected fakes (config.Loader,
// http.Server.ListenAndServe, etc.).
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/parlay-tool/parlay/studio/internal/config"
	"github.com/parlay-tool/parlay/studio/internal/deployer"
	"github.com/parlay-tool/parlay/studio/internal/domain"
	"github.com/parlay-tool/parlay/studio/internal/server"
	"github.com/parlay-tool/parlay/studio/internal/ui"
)

// Set by goreleaser ldflags at release time; defaults are sentinels that
// surface as "dev" / "none" when running an unreleased build.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "init":
			if err := deployer.Init(context.Background(), args[1:]); err != nil {
				fmt.Fprintf(os.Stderr, "parlay-studio init: %v\n", err)
				os.Exit(1)
			}
			return
		case "upgrade":
			if err := deployer.Upgrade(context.Background(), args[1:]); err != nil {
				fmt.Fprintf(os.Stderr, "parlay-studio upgrade: %v\n", err)
				os.Exit(1)
			}
			return
		case "version", "--version", "-v":
			fmt.Printf("parlay-studio %s (commit %s)\n", version, commit)
			return
		case "help", "--help", "-h":
			usage(os.Stdout)
			return
		}

		// A help flag anywhere in the argument list is a request for usage,
		// not for a server. Without this, `parlay-studio domain-edit --help`
		// fell through to boot() and started listening — on a random port,
		// and with open_browser defaulting on, it also opened a browser. The
		// one invocation a person makes when they do not yet know what the
		// command does was the one that took an irreversible action.
		for _, a := range args {
			if a == "--help" || a == "-h" || a == "help" {
				usage(os.Stdout)
				return
			}
		}

		// An unrecognized subcommand is a mistake, not a request to serve.
		// Booting on it means a typo silently starts a server, and the
		// operator sees a working URL rather than "no such command".
		if !strings.HasPrefix(args[0], "-") && args[0] != "domain-edit" {
			fmt.Fprintf(os.Stderr, "parlay-studio: unknown command %q\n\n", args[0])
			usage(os.Stderr)
			os.Exit(1)
		}
	}

	// Default and `domain-edit`: boot the server harness. Both run the
	// identical boot sequence, the same registered tool route groups, and the
	// same lifecycle; only the browser-open landing path differs.
	if err := boot(context.Background(), args); err != nil {
		fmt.Fprintf(os.Stderr, "parlay-studio: %v\n", err)
		os.Exit(1)
	}
}

// boot constructs the harness dependencies (the registered tool subsystems,
// the embedded UI bundle, and the browser-open landing path) and runs the boot
// sequence. The `domain-edit` subcommand differs from the bare invocation only
// in landing the operator's browser on the editor route.
func boot(ctx context.Context, args []string) error {
	// domain-edit is an entry-point convenience, not a separate server mode:
	// strip the subcommand token from the args the boot sequence parses and
	// point the browser at the editor route.
	bootArgs := args
	browserPath := "/"
	if len(args) > 0 && args[0] == "domain-edit" {
		bootArgs = args[1:]
		browserPath = "/domain-model"
	}

	// The domain-model editor is the first tool subsystem to consume the
	// harness registration mechanism. It reads and writes the resolved project
	// root's domain-model.yaml; resolve that root the same way the boot
	// sequence does so the two agree.
	root := resolveEditorRoot(bootArgs)

	return server.Boot(ctx, server.BootDeps{
		Args:        bootArgs,
		Tools:       []server.ToolRegistration{domain.New(root)},
		UIBundle:    ui.Bundle{},
		BrowserPath: browserPath,
	})
}

// resolveEditorRoot resolves the project root the domain-model editor targets,
// using the same resolver the boot sequence uses. On failure it falls back to
// the current working directory; the boot sequence re-resolves and surfaces the
// actionable error before any request is served, so a handler never runs
// against a bad root.
func resolveEditorRoot(args []string) string {
	cwd, _ := os.Getwd()
	home := os.Getenv("HOME")
	root, _, err := config.ResolveProjectRoot(args, envMap(), cwd, home)
	if err != nil {
		return cwd
	}
	return root
}

// envMap snapshots the process environment as a map for config resolution.
func envMap() map[string]string {
	env := os.Environ()
	out := make(map[string]string, len(env))
	for _, kv := range env {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				out[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	return out
}

// usage prints the command surface. Kept here rather than delegated to a flag
// package because main dispatches on os.Args directly — the subcommands parse
// their own flags off the residual slice — so there is no single FlagSet whose
// defaults would describe the whole surface.
func usage(w io.Writer) {
	fmt.Fprint(w, `parlay-studio — the visual companion to parlay

Usage:
  parlay-studio [flags]              boot the server, landing on /
  parlay-studio domain-edit [flags]  boot the server, landing on /domain-model
  parlay-studio init [flags]         deploy studio's skills into a project
  parlay-studio upgrade [flags]      re-deploy studio's skills
  parlay-studio version              print the version
  parlay-studio help                 print this message

Flags are parsed by the boot sequence and the deployer subcommands; run a
subcommand with no arguments to see what it accepts.
`)
}
