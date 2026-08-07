SHELL := /bin/bash

# Default to `go` on PATH; override with `make GO=/path/to/go` if needed.
# Includes a common fallback for environments where Go is installed under $HOME/go/bin.
GO ?= $(shell command -v go 2>/dev/null || echo $$HOME/go/bin/go)

.PHONY: build build-noui ui test test-go test-ui vet sync-skills verify-skills

# Version stamping. Without this `make build` produced a binary reporting
# "dev (commit none)" — goreleaser injected these at release time and nothing
# injected them locally, so the version a developer tested was never the shape
# of the version a user runs.
#
# --match 'v*' is load-bearing, not decoration. A bare `git describe --tags`
# picks the newest reachable tag of any shape, and the studio-v* tags
# (studio-v0.1.0 through studio-v0.1.2) are still in this repo — so it once
# reported "studio-v0.1.2-29-g<sha>" for a parlay build. They are no longer the
# most recent tags and so no longer win on their own, but they stay reachable
# forever, and dropping the match would make correctness depend on nobody ever
# cutting a tag outside the release namespace again. Matching the release
# namespace explicitly means the version cannot be borrowed from a namespace
# that no longer names a product.
VERSION ?= $(shell git describe --tags --match 'v*' --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT)

# The UI bundle. internal/editor/ui embeds dist/ via //go:embed, and only
# dist/.gitkeep is tracked — the built assets are generated and minified. So a
# fresh checkout has an empty dist/, and a `parlay` built from it answers every
# UI route with the documented 503.
#
# That is not hypothetical: it is what shipped. Neither .goreleaser.yaml nor the
# Homebrew formula had a UI step, so every released binary embedded an empty
# directory. `build` depends on the bundle now, which is what stops the omission
# from being possible rather than merely documented.
UI_DIR    := internal/editor/ui
UI_BUNDLE := $(UI_DIR)/dist/index.html
UI_SRC    := $(shell find $(UI_DIR)/src -type f 2>/dev/null) \
             $(UI_DIR)/index.html $(UI_DIR)/vite.config.ts $(UI_DIR)/package-lock.json

# Build the UI bundle. Run this after editing anything under $(UI_DIR)/src.
# `build` picks it up automatically when sources are newer than the bundle, so
# calling this directly is only needed to force a rebuild.
ui:
	cd $(UI_DIR) && npm ci && npm run build

# The bundle is a real file target, so make rebuilds it when UI sources change
# and skips it when they have not. An .md-style phony dependency would either
# re-run npm on every `make build` or never re-run it after a source edit; the
# second is worse, because the binary would embed a stale bundle silently.
$(UI_BUNDLE): $(UI_SRC)
	cd $(UI_DIR) && npm ci && npm run build

# Build the parlay binary from current source.
# CGO is disabled: parlay has no cgo dependencies, so a pure-Go build is
# faster and works in environments without a C toolchain.
#
# One binary. The second line here built parlay-studio, a stub that printed a
# retirement notice, for one release so an already-installed copy would be
# replaced by something naming its successor. It never served that purpose:
# .goreleaser.yaml has built only ./core/cmd/parlay since v0.2.0, so the notice
# shipped to nobody and existed only in local builds. Removed along with the
# stub and the PATH probe that looked for it.
build: $(UI_BUNDLE)
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o parlay ./core/cmd/parlay

# The lean build: no UI, and therefore no Node toolchain required. Everything
# except `parlay domain-edit`'s browser surface is identical; that route answers
# the documented studio-ui-bundle-not-built 503.
build-noui:
	CGO_ENABLED=0 $(GO) build -tags noui -ldflags "$(LDFLAGS)" -o parlay ./core/cmd/parlay

# Run the test suite. One module since Stage 1 absorbed studio/, so `./...`
# reaches everything; the second `go test` from studio/'s own directory that
# used to be here would now re-run a subset of the same packages. CGO stays
# off to match `build`.
test: test-go test-ui

# Go tests. One module since Stage 1 absorbed studio/, so `./...` reaches
# everything; the second `go test` from studio/'s own directory that used to be
# here would now re-run a subset of the same packages.
test-go:
	CGO_ENABLED=0 $(GO) test ./...

# UI tests. These existed and passed and NOTHING RAN THEM — not `make test`,
# not CI — so three UI defects shipped that a green vitest run would have
# caught, including a Done control that never ended the session and a null
# collection that rendered a blank page. A test suite nobody runs is
# documentation.
#
# Skipped, with a warning rather than silently, when node_modules is absent:
# a contributor working only on the Go side should not be forced through an
# npm ci, but they should know what did not run.
test-ui:
	@if [ -d "$(UI_DIR)/node_modules" ]; then 		cd $(UI_DIR) && npm test -- --run; 	else 		echo "[WARN] skipping UI tests: $(UI_DIR)/node_modules is absent (run 'make ui' or 'cd $(UI_DIR) && npm ci')"; 	fi

# Vet the module. Same one-module reasoning as `test`.
vet:
	CGO_ENABLED=0 $(GO) vet ./...

# Edit-source -> build -> upgrade, in one shot.
# Use this after changing anything under core/internal/embedded/{skills,schemas}/.
# This is the dogfooding rule documented in CLAUDE.md.
sync-skills: build
	./parlay upgrade

# Verify deployed skills and schemas are in sync with the embedded source.
# Expected differences: deployer-added frontmatter and trailing whitespace.
# Anything else means someone edited a deployed copy directly (which is forbidden)
# OR the source was edited but `make sync-skills` was not run (also forbidden).
#
# Skill sources now carry their own frontmatter (parsed by
# embedded.ReadAllSkills as the description single-source) AND may
# contain deploy-time-expansion markers (`<!-- parlay:expand-... -->`)
# that ReadAllSkills expands into canonical prose before any deployer
# sees it — see core/internal/embedded/skills.go. That expansion is
# Go-side string substitution; reimplementing it in shell would just
# create a second, driftable copy of the expansion text right here.
# Instead we `go run` core/internal/embedded/dumpskills.go, which calls
# ReadAllSkills() directly, so the "expected body" side of this diff is
# always generated by the SAME code deployers actually run — no
# duplicated expansion logic to keep in sync.
#
# That helper used to be generated here with `$(file >...)` and deleted
# afterwards. `$(file ...)` is GNU Make 4.0+; macOS ships 3.81, where it
# expands to nothing — silently — and this target has never once run on a
# stock Mac. It is a committed file now: no version floor, and it shows up
# in a diff.
#
# The loop below walks the helper's manifest rather than globbing the skill
# sources, because a skill's destination is not derivable from its filename.
# The `surface:` frontmatter key decides it, and the two surfaces differ in
# both path and wrapper:
#
#   command  .claude/skills/parlay-<name>/SKILL.md   YAML frontmatter
#   module   .parlay/modules/<name>.md               "# <name>" + "_<desc>_"
#
# The glob predated modules and reported all nine of them MISSING from
# .claude/skills/ — a directory they have correctly never been deployed to.
verify-skills:
	@drift=0; \
	tmpdir=$$(mktemp -d); \
	trap 'rm -rf "$$tmpdir"' EXIT; \
	$(GO) run core/internal/embedded/dumpskills.go "$$tmpdir" > "$$tmpdir/manifest.tsv" 2>"$$tmpdir/dump.err" \
		|| { echo "verify-skills: failed to run dump helper:"; cat "$$tmpdir/dump.err"; exit 1; }; \
	while IFS=$$'\t' read -r name surface expanded; do \
		src="core/internal/embedded/skills/$$name.skill.md"; \
		if [ "$$surface" = "module" ]; then \
			dst=".parlay/modules/$$name.md"; \
			strip="NR>4"; \
		else \
			dst=".claude/skills/parlay-$$name/SKILL.md"; \
			strip=""; \
		fi; \
		if [ ! -f "$$dst" ]; then \
			echo "MISSING:  $$dst (run 'make sync-skills')"; \
			drift=1; \
			continue; \
		fi; \
		if [ -n "$$strip" ]; then \
			deployed_body=$$(awk 'NR>4' "$$dst"); \
		else \
			deployed_body=$$(awk 'BEGIN{fm=0;done=0} NR==1 && /^---$$/ {fm=1; next} fm && /^---$$/ {fm=0; done=1; next} fm {next} done && /^$$/ {done=0; next} {done=0; print}' "$$dst"); \
		fi; \
		body_diff=$$(diff <(printf '%s\n' "$$deployed_body" | sed 's/[[:space:]]*$$//') <(sed 's/[[:space:]]*$$//' "$$expanded") || true); \
		if [ -n "$$body_diff" ]; then \
			echo "DRIFT:    $$dst differs from $$src ($$surface surface, post-header-strip, post-marker-expansion)"; \
			drift=1; \
		fi; \
	done < "$$tmpdir/manifest.tsv"; \
	for src in core/internal/embedded/schemas/*.schema.md; do \
		name=$$(basename $$src); \
		dst=".parlay/schemas/$$name"; \
		if [ ! -f "$$dst" ]; then \
			echo "MISSING:  $$dst (run 'make sync-skills')"; \
			drift=1; \
			continue; \
		fi; \
		if ! diff -q <(sed 's/[[:space:]]*$$//' $$src) <(sed 's/[[:space:]]*$$//' $$dst) >/dev/null; then \
			echo "DRIFT:    $$dst differs from $$src"; \
			drift=1; \
		fi; \
	done; \
	if [ $$drift -eq 0 ]; then \
		echo "OK: skills and schemas are in sync with the embedded source."; \
	else \
		exit 1; \
	fi
