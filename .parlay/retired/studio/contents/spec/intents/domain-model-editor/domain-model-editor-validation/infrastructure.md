# Domain-model-editor-validation — Infrastructure

---

## Out-of-process validation through the Core CLI

**Affects**: the validation query endpoint added to the editor's route group; the out-of-process boundary between Studio and Core's validator; the no-Core-import package boundary; the closed error-code vocabulary passed through unchanged; the parity guard between the endpoint and the build path

**Behavior**: The editor obtains validation findings for a draft by invoking Core's `parlay validate --type domain-model --json` mode as a subprocess — serializing the in-memory draft to YAML, feeding it on standard input, and parsing the emitted finding list. Studio and Core are separate modules in different namespaces, and Studio deliberately does not import Core as a library; it reaches the validator only out of process, so the editor and the build path run the *same binary* and cannot drift even in principle. The `parlay` executable is located once by the subsystem (resolved from the running Studio process's own binary or an explicit configured path) rather than searched per request. This behavior is exposed as one validation query endpoint mounted in the existing editor route group at the `/api/domain-model` prefix, alongside the MVP's two persistence endpoints; it is a query, not a persistence operation, so it does not widen the persistence surface. It accepts a model draft and returns the findings as a `fields[]`-shaped payload with HTTP 200 — including an empty list when the draft is clean, because a finding list is a query result, not an error. Each finding carries the closed error code exactly as Core emits it, an element path anchoring it to an entity, field, enum, enum value, or relationship (or the distinguished top-level path for whole-model findings), the schema's actionable message, and its severity taken from Core's authoring-mode table (`domain-operations-deprecated` is the sole warning; every other code is an error). Studio adds no validation rule of its own, invents or renames no code, and recomputes no severity. The endpoint is a pure function over the submitted draft: it reads nothing from disk, mutates nothing, and returns the same findings regardless of the on-disk model. A malformed request (unparseable draft bytes or a bad request envelope) is the one case that returns `validation-failed` at the HTTP-error level — distinct from a well-formed but schema-invalid draft, which is always a 200 finding list.

**Invariants**:
- Findings are obtained only by invoking Core's `parlay validate --type domain-model --json` subprocess; no validation rule is reimplemented in Studio's backend or in the UI bundle, and Studio imports no Core package — a guard test fails the build if any Studio package imports Core.
- The `parlay` executable is located once at subsystem startup, not searched or re-resolved per validate request.
- The validation endpoint is mounted inside the existing `/api/domain-model` route group as a query; it does not add a persistence endpoint and does not widen the MVP's two-persistence-endpoint surface.
- The endpoint returns HTTP 200 with the finding list for every well-formed draft, empty list included; it never returns a 4xx to signal that a well-formed draft has findings.
- A malformed request returns `validation-failed` at the HTTP-error level; this is the only case that is an HTTP error, and it is distinct from a 200 finding-list response for a well-formed invalid draft.
- Each finding carries the closed error code unchanged, a machine-usable element path (the distinguished top-level path for whole-model findings), the schema's actionable message, and a severity taken from Core's authoring-mode table; Studio neither invents, renames, nor reclassifies.
- The endpoint is side-effect-free: validating a draft writes nothing to disk and mutates nothing, and returns findings computed from the submitted draft bytes alone, independent of the on-disk model.
- A parity suite runs a shared fixture corpus (one fixture per closed code plus clean fixtures) through both the direct CLI mode and the endpoint and asserts identical finding sets; because both run the same binary it guards the Studio wrapper (request shaping, stdin transport, path mapping), and a mismatch fails the build.

**Source**: @domain-model-editor-validation/validation-parity-with-cores-deep-validation, @domain-model-editor-validation/live-in-editor-validation-surfacing

**Caching**: none

**Backward-Compatible**: yes

**Notes**:
- Depends on the Core-side feature `studio-support/structured-domain-model-validation`, which delivers the `--json` structured output mode, the per-finding element path, and the `domain-operations-deprecated` emission this endpoint consumes verbatim.
- Out-of-process invocation was chosen over sharing source precisely because running the same binary makes editor-vs-build drift impossible, which an in-process import could not guarantee.
- The live revalidation that drives the surface is triggered on load and on every committed draft mutation, debounced so a rapid burst produces one trailing call; a validate response superseded by a newer draft is discarded, never rendered over fresher findings. This is a presentation-layer contract over this endpoint — the endpoint itself is stateless and per-call.

---

## Server-side save gate ordered before the compare-and-swap

**Affects**: the write-path validation gate layered onto the MVP save endpoint; the error-blocks / warning-passes rule; the ordering of the gate relative to the etag compare-and-swap; the absence of any force-save path

**Behavior**: The save path validates the draft server-side before writing, using the same out-of-process validator the validation endpoint uses. A draft carrying any error-severity finding fails the save with the `validation-failed` envelope listing the findings, and nothing touches disk. Warning-severity findings (`domain-operations-deprecated`) never block a save, alone or alongside other findings' errors. Enforcement is server-side and authoritative: a client that skipped, staled, or bypassed the UI's blocked-save affordance still cannot write a draft the build path would reject, and there is no force-save or override path through the editor — the escape hatch for deliberately writing an invalid file is a text editor, which the editor's contract is precisely not to be. The gate orders before the compare-and-swap etag check: an invalid draft fails with `validation-failed` even when its etag is also stale, because validity is designer-actionable first and conflict resolution matters only for a savable draft. A valid draft then proceeds into the MVP's unchanged compare-and-swap, where a stale etag yields the conflict envelope as before.

**Invariants**:
- Every save validates the draft server-side before any write; a save whose draft carries any error-severity finding is rejected with the `validation-failed` envelope listing the findings, and the on-disk file is untouched.
- Warning-severity findings never block a save, alone or alongside the block for other findings' errors.
- The gate is enforced server-side regardless of client state; a save submitted directly to the API bypassing the UI is gated identically.
- There is no force-save or override path through the editor.
- The validation gate runs before the compare-and-swap: an invalid draft with a stale etag returns `validation-failed` (not `conflict`); once the draft is valid, the same stale etag then returns `conflict`.
- The gate reuses the out-of-process validator; it introduces no second validation code path and no Studio-side rule.

**Source**: @domain-model-editor-validation/save-gating-on-validation-findings

**Backward-Compatible**: yes

**Notes**:
- The gate augments the MVP's compare-and-swap save rather than replacing it; the etag mechanics, deterministic serialization, and deprecated-operations passthrough are unchanged and still owned by the MVP infrastructure fragments.
- Findings for a freshly-loaded file populate before any edit (the editor validates on load), so a designer opening an already-invalid file sees the blocked save state and the repair scope up front rather than discovering a blocked save later. The load-time surfacing itself is a presentation concern; this fragment pins only the server-side write gate.
- The no-force-save posture is deliberate; whether a future revision should permit strictly-improving saves (error set a strict subset of the load-time error set) on large hand-broken models is the open question carried from the intent, deferred until real usage shows the need.

---
