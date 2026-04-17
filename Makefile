SHELL := /bin/bash

# Default to `go` on PATH; override with `make GO=/path/to/go` if needed.
# Includes a common fallback for environments where Go is installed under $HOME/go/bin.
GO ?= $(shell command -v go 2>/dev/null || echo $$HOME/go/bin/go)

.PHONY: build sync-skills verify-skills

# Build the parlay binary from current source.
# CGO is disabled: parlay has no cgo dependencies, so a pure-Go build is
# faster and works in environments without a C toolchain.
build:
	CGO_ENABLED=0 $(GO) build -o parlay ./cmd/parlay

# Edit-source -> build -> upgrade, in one shot.
# Use this after changing anything under internal/embedded/{skills,schemas}/.
# This is the dogfooding rule documented in CLAUDE.md.
sync-skills: build
	./parlay upgrade

# Verify deployed skills and schemas are in sync with the embedded source.
# Expected differences: deployer-added frontmatter and trailing whitespace.
# Anything else means someone edited a deployed copy directly (which is forbidden)
# OR the source was edited but `make sync-skills` was not run (also forbidden).
verify-skills:
	@drift=0; \
	for src in internal/embedded/skills/*.skill.md; do \
		name=$$(basename $$src .skill.md); \
		dst=".claude/skills/parlay-$$name/SKILL.md"; \
		if [ ! -f "$$dst" ]; then \
			echo "MISSING:  $$dst (run 'make sync-skills')"; \
			drift=1; \
			continue; \
		fi; \
		body_diff=$$(diff <(awk 'BEGIN{fm=0;done=0} NR==1 && /^---$$/ {fm=1; next} fm && /^---$$/ {fm=0; done=1; next} fm {next} done && /^$$/ {done=0; next} {done=0; print}' $$dst | sed 's/[[:space:]]*$$//') <(sed 's/[[:space:]]*$$//' $$src) || true); \
		if [ -n "$$body_diff" ]; then \
			echo "DRIFT:    $$dst differs from $$src"; \
			drift=1; \
		fi; \
	done; \
	for src in internal/embedded/schemas/*.schema.md; do \
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
