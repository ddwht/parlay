# Brownfield

> Adopting parlay in existing codebases — analyzing projects, drafting adapters, and integrating generated features alongside existing code.

---

## Onboard Existing Codebase

**Goal**: Analyze an existing codebase and draft a framework adapter with mount-strategies, file conventions, and coding conventions populated from the project's actual structure and patterns.
**Persona**: Tech Lead
**Priority**: P1
**Context**: A team has an existing project and wants to use Parlay to add new features to it. They need an adapter that reflects their codebase conventions and declares how new components can be integrated into existing pages.
**Action**: AI agent reads representative source files, detects the framework and UI library, identifies widget patterns, extracts coding conventions, and drafts a complete adapter YAML for review.
**Objects**: adapter, mount-strategy, codebase, framework-detection

**Constraints**:
- Must produce a reviewable draft adapter, not auto-register it — the team reviews and adjusts before registering
- Must not create persistent project-level indexes — all codebase knowledge is captured in the adapter or read from live source code at generation time
- Must work for any supported framework
- Must detect common widget patterns and generate mount-strategy entries with detection patterns and code templates
- The adapter remains framework-level and reusable — project-specific file paths do not belong in it
- Mount-strategy templates use `{{placeholder}}` syntax for values that vary per integration

**Verify**:
- Adapter YAML is generated with shows, actions, flows, compositions, conventions, file-conventions, and mount-strategies sections
- Detected mount-strategy patterns match widgets that actually exist in the codebase
- The adapter is framework-level — another project using the same framework could use it with convention adjustments
- Team can review and edit the draft before registering via `/parlay-register-adapter`

---
