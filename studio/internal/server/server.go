// parlay-feature: studio-foundation/web-server-harness
// parlay-component: cross-cutting/http-server-harness
// parlay-extends: studio-foundation/figma-mcp-via-host-agent/cross-cutting/retract-studio-direct-mcp-source-tree

// Package server is Parlay Studio's HTTP harness. It owns the chi router,
// the middleware stack, the two universal endpoints (/api/health and
// /api/shutdown), the tool-registration interface, and the SPA fallback
// for the embedded UI bundle.
//
// The harness package does NOT import any Studio tool package directly.
// Tools plug in via the ToolRegistration interface and are passed to
// New() as a slice. This keeps the harness independent of any concrete
// tool, so it can be tested in isolation with a fake tool registration.
//
// All HTTP requests bind to 127.0.0.1 only — see boot.go for the
// loopback-enforcing listener. The loopback bind is the trust boundary
// that lets /api/shutdown skip authentication.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/parlay-tool/parlay/studio/internal/config"
)

// Stable codes surfaced by the harness.
var (
	// ErrToolNameCollision (studio-tool-name-collision) — two ToolRegistration
	// values with the same Name() were passed to New(); the harness refuses
	// to construct because a tool would be silently shadowed otherwise.
	ErrToolNameCollision = errors.New("studio-tool-name-collision")

	// ErrUIBundleNotBuilt (studio-ui-bundle-not-built) — the embedded UI
	// bundle is not packaged into this binary; the SPA fallback returns
	// 503 with this code.
	ErrUIBundleNotBuilt = errors.New("studio-ui-bundle-not-built")
)

// ToolRegistration is the plug-in surface used by Studio tools (e.g. the
// domain-model editor, the design-loop coordinator) to mount their handlers
// onto the harness router. Each tool's package exports a value satisfying
// this interface; the harness package does not import any tool package.
type ToolRegistration interface {
	// Name returns the unique tool subsystem name (e.g. "domain-model",
	// "design-loop"). Two registrations with the same Name() are rejected
	// at New() time with studio-tool-name-collision.
	Name() string
	// Mount attaches the tool's handlers under the tool's prefix on the
	// supplied router. The harness invokes Mount during construction; the
	// tool may call any chi routing primitive on the supplied router.
	Mount(r chi.Router)
}

// UIBundle is the optional embedded UI bundle handler. When non-nil, the
// SPA fallback serves index.html for unmatched non-/api paths. When nil,
// the SPA fallback returns 503 with studio-ui-bundle-not-built.
type UIBundle interface {
	// ServeIndex writes the UI's index.html to w; the harness calls it as
	// the SPA fallback handler.
	ServeIndex(w http.ResponseWriter, r *http.Request)
}

// Deps are the runtime dependencies the harness needs at construction.
// The HTTP server itself is built from these; the listener and the
// process-level shutdown channel live in package main / boot.go.
type Deps struct {
	// Config is the merged Studio configuration. The harness reads
	// IdleTimeout to drive the idle-tracker middleware.
	Config config.Config

	// Tools is the list of registered Studio tools. Order determines the
	// route-mount order; collisions on Name() are rejected at New().
	Tools []ToolRegistration

	// IdleTracker is the activity tracker that observes /api/* requests
	// (excluding /api/health). May be nil when IdleTimeout is zero.
	IdleTracker *IdleTracker

	// ShutdownChan is the unified shutdown channel; POST /api/shutdown
	// writes onto it with reason "explicit: /api/shutdown".
	ShutdownChan chan<- string

	// UIBundle is the optional embedded UI bundle. nil means "bundle not
	// built"; the SPA fallback returns 503 in that case.
	UIBundle UIBundle
}

// Server wraps the configured *http.Server and the chi router. The caller
// (boot.go) supplies the net.Listener — the harness does not bind directly.
type Server struct {
	Router *chi.Mux
	HTTP   *http.Server
	deps   Deps
}

// New constructs the harness. The construction order is fixed and is the
// invariant the middleware-stack tests assert on:
//
//  1. middleware.RequestID
//  2. PanicRecovery
//  3. IdleTimeoutReset (applied via chi.Group on /api/* paths with /api/health excluded)
//  4. middleware.Logger
//  5. ErrorEnvelopeTranslate
//
// Duplicate tool names are rejected with ErrToolNameCollision before any
// handler is mounted, so a half-mounted router never escapes the constructor.
func New(deps Deps) (*Server, error) {
	if err := assertUniqueToolNames(deps.Tools); err != nil {
		return nil, err
	}

	r := chi.NewRouter()

	// Stage 1 — request-ID is the outermost middleware so every other
	// stage observes the assigned X-Request-ID.
	r.Use(chimw.RequestID)
	// Stage 2 — panic recovery wraps every downstream handler.
	r.Use(PanicRecovery)
	// Stage 4 — structured request log.
	r.Use(Logger)
	// Stage 5 — error-envelope translation; converts handler-returned
	// sentinels into the harness JSON envelope.
	r.Use(ErrorEnvelopeTranslate)

	// /api/health is mounted at the top level so it does NOT pick up the
	// idle-timeout-reset middleware (health checks must not advance the
	// idle timer).
	r.Get("/api/health", HealthHandler)

	// All other /api/* routes pick up the idle-timeout-reset middleware
	// via a chi.Group. Stage 3 is applied here so /api/health is excluded
	// by routing rather than by per-request introspection.
	r.Group(func(api chi.Router) {
		if deps.IdleTracker != nil {
			api.Use(IdleTimeoutReset(deps.IdleTracker))
		}
		api.Post("/api/shutdown", ShutdownHandler(deps.ShutdownChan))
		for _, tool := range deps.Tools {
			tool.Mount(api)
		}
	})

	// SPA fallback — non-/api paths serve the embedded UI bundle's
	// index.html when one is embedded, or return 503 otherwise.
	r.NotFound(spaFallback(deps.UIBundle))

	httpSrv := &http.Server{
		Handler: r,
		// ReadHeaderTimeout protects against slow-loris reads even on the
		// loopback interface; chi's defaults do not cover this.
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}

	return &Server{Router: r, HTTP: httpSrv, deps: deps}, nil
}

// Shutdown drains in-flight requests within the supplied context's deadline
// and then closes the underlying *http.Server. Callers (boot.go) wrap the
// context with the harness's 5-second drain deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.HTTP.Shutdown(ctx); err != nil {
		return fmt.Errorf("server: shutdown: %w", err)
	}
	return nil
}

// assertUniqueToolNames returns ErrToolNameCollision when two registrations
// produce the same Name().
func assertUniqueToolNames(tools []ToolRegistration) error {
	seen := make(map[string]struct{}, len(tools))
	for _, t := range tools {
		name := t.Name()
		if _, dup := seen[name]; dup {
			return fmt.Errorf("%w: %q", ErrToolNameCollision, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}
