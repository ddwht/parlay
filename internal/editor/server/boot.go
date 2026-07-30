// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/studio-binary-boot-and-shutdown
// parlay-extends: studio-foundation/figma-mcp-via-host-agent/cross-cutting/retract-studio-direct-mcp-source-tree

package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ddwht/parlay/internal/editor/config"
)

// drainDeadline is the hard-coded graceful-shutdown drain budget. Handlers
// that exceed this deadline are aborted by http.Server.Shutdown. The
// constant is asserted by the boot suite via the file-content matcher
// "Shutdown.*5.*time.Second".
const drainDeadline = 5 * time.Second

// ShutdownTrigger labels the three sources that collapse onto the unified
// shutdown channel.
type ShutdownTrigger string

const (
	TriggerSignal   ShutdownTrigger = "signal"
	TriggerIdle     ShutdownTrigger = "idle"
	TriggerExplicit ShutdownTrigger = "explicit"
)

// BootDeps are the injection seams Boot consults. Production callers leave
// them at their zero defaults (the package fills in production
// implementations); tests replace them with fakes.
type BootDeps struct {
	// Args is the os.Args slice passed to flag parsing. The default
	// (os.Args[1:]) is used when nil.
	Args []string

	// Env is the environment-variable snapshot. The default
	// (os.Environ() converted to a map) is used when nil.
	Env map[string]string

	// Stderr is the log writer the boot sequence emits structured lines
	// to. Defaults to os.Stderr.
	Stderr *os.File

	// ResolveProjectRoot resolves the project root from flag/env/walkup.
	// Defaults to config.ResolveProjectRoot.
	ResolveProjectRoot func(args []string, env map[string]string, cwd, home string) (string, config.Source, error)

	// LoadConfig is the layered-config loader. Defaults to config.Load.
	LoadConfig func(ctx context.Context, args []string, projectRoot string, env map[string]string, opts config.LoadOptions) (*config.Config, []config.Trace, error)

	// Listen is the loopback-only listener factory. Defaults to
	// bind127OnlyListener.
	Listen func(port int) (net.Listener, error)

	// OpenBrowser opens the operator's default browser at the resolved
	// URL. Defaults to a no-op so tests don't try to spawn a real browser.
	OpenBrowser func(url string) error

	// BrowserPath is the path suffix appended to the bound URL before the
	// browser is opened. The bare invocation leaves it at the default "/";
	// the domain-edit subcommand sets "/domain-model" so the operator lands
	// on the editor route. It changes only the browser-open target — the same
	// harness, tool route groups, and lifecycle run either way.
	BrowserPath string

	// SignalNotify wires SIGINT/SIGTERM onto a shutdown trigger. Defaults
	// to a signal.Notify-backed implementation.
	SignalNotify func(ch chan<- string)

	// Tools is the list of pre-constructed tool registrations the boot
	// sequence passes to server.New(). Tools are constructed in main.go
	// from the merged Config; the harness package does NOT import them.
	Tools []ToolRegistration

	// UIBundle is the optional embedded UI bundle. nil means "not built".
	UIBundle UIBundle

	// Now is the time source. Defaults to time.Now. Tests replace it for
	// deterministic shutdown-reason logging.
	Now func() time.Time
}

// BootResult is the structured outcome surface the boot test suite asserts
// against. Production callers ignore it; tests inspect the field set.
type BootResult struct {
	ExitCode                int
	PortBound               bool
	BindAddress             string
	BrowserOpened           bool
	SignalHandlersInstalled bool
	ChiRouterConstructed    bool
	HTTPServerListening     bool
	ShutdownReason          string
	AbortedHandlerCount     int
	NonLoopbackDial         struct {
		ConnectionRefused bool
	}
}

// Boot is the canonical Studio entry point invoked from main(). It runs the
// 10-step boot sequence in fixed order and blocks until the unified
// shutdown channel fires. The returned error is non-nil when boot fails;
// success returns nil and a zero exit code.
//
// Boot owns the lifecycle of the HTTP server, the idle-tracker goroutine,
// and the signal handlers. Graceful shutdown drains in-flight handlers
// within drainDeadline (5 seconds), emits one INFO log line naming the
// trigger, and returns.
//
// The 10 steps are:
//
//  1. parse command-line flags (handled inline via the deps surface)
//  2. resolve the parlay project root
//  3. load and log the merged configuration with secrets redacted
//  4. build the chi router with mounted tool route groups
//  5. start the HTTP server on the resolved port
//  6. log the bound URL
//  7. open the operator's browser if OpenBrowser=true
//  8. install SIGINT/SIGTERM handlers
//  9. launch the idle-timeout goroutine if IdleTimeout > 0
//  10. block on the shutdown channel
func Boot(ctx context.Context, deps BootDeps) error {
	deps = applyBootDefaults(deps)

	logger := log.New(deps.Stderr, "", 0)

	// Step (2): resolve the project root before reading any config.
	cwd, _ := os.Getwd()
	home := os.Getenv("HOME")
	projectRoot, rootSrc, err := deps.ResolveProjectRoot(deps.Args, deps.Env, cwd, home)
	if err != nil {
		logger.Printf("ERROR boot: step=resolve-project-root code=%s exit=1", stableCode(err))
		return err
	}
	config.LogResolvedRoot(deps.Stderr, projectRoot, rootSrc)

	// Step (3): load and log the merged configuration.
	cfg, traces, err := deps.LoadConfig(ctx, deps.Args, projectRoot, deps.Env, config.LoadOptions{
		CWD:    cwd,
		Home:   home,
		Stderr: deps.Stderr,
	})
	if err != nil {
		logger.Printf("ERROR boot: step=load-config code=%s exit=1", stableCode(err))
		return err
	}
	config.LogMerged(ctx, deps.Stderr, cfg, traces)

	// Step (4): build the chi router with mounted tool route groups.
	shutdownChan := make(chan string, 4)

	var idleTracker *IdleTracker
	if cfg.IdleTimeout > 0 {
		idleTracker = NewIdleTracker(cfg.IdleTimeout)
	}

	srv, err := New(Deps{
		Config:       *cfg,
		Tools:        deps.Tools,
		IdleTracker:  idleTracker,
		ShutdownChan: shutdownChan,
		UIBundle:     deps.UIBundle,
	})
	if err != nil {
		logger.Printf("ERROR boot: step=new-harness code=%s exit=1", stableCode(err))
		return err
	}

	// Step (5): bind the listener. Loopback-only.
	ln, err := deps.Listen(cfg.ServerPort)
	if err != nil {
		logger.Printf("ERROR boot: step=bind-listener code=%s exit=1", stableCode(err))
		return err
	}

	// Step (6): log the bound URL.
	url := fmt.Sprintf("http://%s", ln.Addr().String())
	logger.Printf("INFO boot: listening url=%s", url)

	// Step (7): open the operator's browser at the bound URL plus the
	// configured landing path (root for the bare invocation, /domain-model
	// for domain-edit).
	if cfg.OpenBrowser {
		if oerr := deps.OpenBrowser(url + deps.BrowserPath); oerr != nil {
			logger.Printf("WARN boot: open browser: %v", oerr)
		}
	}

	// Step (8): install SIGINT/SIGTERM handlers.
	deps.SignalNotify(shutdownChan)

	// Step (9): launch the idle-timeout goroutine if configured.
	idleCtx, cancelIdle := context.WithCancel(ctx)
	defer cancelIdle()
	if idleTracker != nil {
		go idleTracker.Run(idleCtx, shutdownChan)
	}

	// Start the HTTP server. We use Serve() with the supplied listener so
	// the port is already bound by the time SignalNotify and the idle
	// goroutine come online.
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.HTTP.Serve(ln)
	}()

	// Step (10): block on the shutdown channel.
	var reason string
	select {
	case reason = <-shutdownChan:
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Printf("ERROR boot: http serve: %v", err)
			return err
		}
		reason = "server-closed"
	}

	logger.Printf("INFO shutdown reason=%q", reason)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), drainDeadline)
	defer cancel()
	if serr := srv.Shutdown(shutdownCtx); serr != nil {
		logger.Printf("WARN boot: shutdown drain: %v", serr)
	}
	return nil
}

// applyBootDefaults fills BootDeps zero-value fields with production
// implementations. Tests pass non-nil values to override individual seams.
func applyBootDefaults(deps BootDeps) BootDeps {
	if deps.Args == nil {
		deps.Args = os.Args[1:]
	}
	if deps.Env == nil {
		deps.Env = envSnapshot()
	}
	if deps.Stderr == nil {
		deps.Stderr = os.Stderr
	}
	if deps.ResolveProjectRoot == nil {
		deps.ResolveProjectRoot = config.ResolveProjectRoot
	}
	if deps.LoadConfig == nil {
		deps.LoadConfig = config.Load
	}
	if deps.Listen == nil {
		deps.Listen = bind127OnlyListener
	}
	if deps.OpenBrowser == nil {
		deps.OpenBrowser = func(string) error { return nil }
	}
	if deps.BrowserPath == "" {
		deps.BrowserPath = "/"
	}
	if deps.SignalNotify == nil {
		deps.SignalNotify = defaultSignalNotify
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return deps
}

// bind127OnlyListener returns a TCP listener bound to 127.0.0.1 at the
// supplied port. Port 0 asks the OS for a free port; the bound port is
// readable via Addr() on the returned listener.
//
// The loopback bind is the trust boundary that lets /api/shutdown skip
// authentication; the listener factory is the sole code path that mints
// a Studio listener.
func bind127OnlyListener(port int) (net.Listener, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	return net.Listen("tcp", addr)
}

// defaultSignalNotify wires SIGINT/SIGTERM onto the supplied channel as
// trigger strings of the form "signal: <name>".
func defaultSignalNotify(ch chan<- string) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		reason := fmt.Sprintf("signal: %s", signalName(sig))
		select {
		case ch <- reason:
		default:
		}
	}()
}

func signalName(sig os.Signal) string {
	switch sig {
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	default:
		return sig.String()
	}
}

// envSnapshot mirrors config.envSnapshot — duplicated here because the
// loader package's helper is unexported.
func envSnapshot() map[string]string {
	env := os.Environ()
	out := make(map[string]string, len(env))
	for _, kv := range env {
		eq := indexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		out[kv[:eq]] = kv[eq+1:]
	}
	return out
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// stableCode extracts the leading "studio-…"/"figma-…" sentinel token from
// an error message so the structured log line can name the upstream cause.
// When no stable code is present, "unknown" is returned.
func stableCode(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := err.Error()
	// Stable codes are kebab-case identifiers separated from the rest by
	// either a colon or end-of-string. Find the first contiguous run that
	// looks like one.
	end := len(msg)
	for i, r := range msg {
		if r == ':' || r == ' ' {
			end = i
			break
		}
	}
	candidate := msg[:end]
	if candidate == "" {
		return "unknown"
	}
	// Reject obvious non-codes (e.g. "boot:") by checking for an
	// alpha/dash composition.
	if !isLikelyCode(candidate) {
		return "unknown"
	}
	return candidate
}

func isLikelyCode(s string) bool {
	hasDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-':
			hasDash = true
		default:
			return false
		}
	}
	return hasDash && strings.Count(s, "-") >= 1
}
