# Intent Schema — authoring digest

Derived from `intent.schema.md` at deploy time. Never hand-edit: edit the schema's
`<!-- parlay:normative -->` blocks and re-run `parlay upgrade`.

This is what you need to AUTHOR the artifact — field tables, closed
vocabularies, required shapes, invariants. It deliberately omits the
schema's rationale and history. Open the full schema when a validator
finding routes you there, when you are changing the schema itself, or
when you need to know WHY a rule exists rather than what it is.

---

```
# <Feature Name>

> <One-line description>

---

## <Intent Title>

**Goal**: <Why — what the user is trying to accomplish>
**Persona**: <Who — role performing the action>
**Priority**: <P0 | P1 | P2 — importance level; defaults to P1 if omitted>
**Context**: <When — the triggering situation>
**Action**: <How — one-line approach or method>
**Objects**: <What — domain entities involved, comma-separated>

**Constraints**:
- <Hard requirement or boundary>

**Verify**:
- <Expected outcome, observable state, or edge case behavior>

**Questions**:
- <Open design question or unresolved uncertainty>
```

---

| Field | Required | Parse rule |
|---|---|---|
| Feature Name | No | `# ` heading, first line of file |
| Intent Title | Yes | `## ` heading. Slug: lowercase, spaces → hyphens, no punctuation. Must be unique within feature. |
| Goal | Yes | `**Goal**:` line content |
| Persona | Yes | `**Persona**:` line content |
| Priority | No | `**Priority**:` line content. Values: P0 (critical), P1 (important), P2 (nice-to-have). Defaults to P1 if omitted. |
| Context | No | `**Context**:` line content |
| Action | No | `**Action**:` line content |
| Objects | No | `**Objects**:` line content, comma-separated values |
| Constraints | No | `**Constraints**:` followed by `- ` prefixed lines |
| Verify | No | `**Verify**:` followed by `- ` prefixed lines. State-based assertions — expected outcomes, observable states, and edge case behaviors. |
| Questions | No | `**Questions**:` followed by `- ` prefixed lines. Open design questions or unresolved uncertainties. |

---

## Soft boundaries

Ten fields carry an intent, and the schema above checks only that some of them
are present. What an intent SAYS is unchecked, and that is where it goes wrong:
an intent drifts toward the solution — toward the screen that will show it, the
control that will trigger it, the record the system will write — and every
artifact downstream inherits the drift, from a document that freezes at the
first green build and can never be edited again.

These are **advisory**. Nothing here blocks a build, and no validator emits
them. They are prompts for the author and for the agent guiding authoring, and
each is marked **Obligation** (follow it unless you can say why) or
**Heuristic** (a conversation, never applied automatically).

Two rules govern all of them:

**Route, do not delete.** Drifted content is usually misplaced, not wrong. A
technical mechanism belongs in `infrastructure.md`; an interaction belongs in
`dialogs.md`; a rendering belongs in the surface. Say where it goes.

**Domain is per product.** "Interface noun" is not a fixed list. `Component`,
`Route` and `Screen` are domain concepts for a UI builder, and
`command-argument` is one for a CLI. The test is whether the term is meaningful
in THIS product's domain, never whether it sounds like a widget.

| Field | | Boundary |
|---|---|---|
| Goal | Obligation | Names the user-world outcome and why it matters. Not a system operation: "create an object in the accounting system" is what the software does; "be in good standing with the tax authorities" is what the user wants. The goal need not be measurable — the evidence for it belongs in `Verify`. |
| Persona | Obligation | The situated role doing THIS job, not a job title. "accountant" is a business card; "a person sending the tax report" is a role. A title collects unrelated jobs under one label, and two intents sharing one are often different roles. Organizational or system actors are legitimate where the domain genuinely has them. |
| Priority | Heuristic | Ranks the cost of leaving the USER OUTCOME unmet — not implementation order, technical risk, or ease. "The backend must exist first, so it is P0" ranks the build, not the user. If everything is P0, ask for a relative ranking or the shared critical condition. There is no correct number of P0s. |
| Context | Heuristic | Explains why or when the task arises, without prescribing the interaction. "The quarterly reporting period has closed" is a situation; "the user opens the reports page" is a navigation gesture wearing context's clothes. Product state can be a legitimate situation — "a transfer is rejected" is fine. |
| Action | **Obligation** | The task-level act, independent of the control or navigation that will carry it. "Fill in the tax report and send it to the tax authority", not "upload the tax report to the system". The line is task versus control, NOT outside versus inside the product: approving an invoice or reconciling an account genuinely happen in software and are proper actions. What drifts is "click Upload", "open Reports", "select from the dropdown". This is the field the interface enters through, because it is the one that asks *how*. |
| Objects | **Obligation** | Concepts meaningful in this product's domain, not incidental controls. "tax report", "tax number" — not "tax modal window", "send report button". A control the user AUTHORS may itself be a domain object; a control they merely click is not. |
| Constraints | Obligation | Limits that hold in the world, stated because the domain imposes them: "the tax report must be sent before 1 March". An implementation mechanism — a framework limit, a storage bound, "the screen fits only 10 entries" — routes to `infrastructure.md`. Do not read technical vocabulary as automatic disqualification: accessibility, response time, security and interoperability are often genuine business requirements and stay with the promise. |
| Verify | Obligation | Independently testable evidence that the promised outcome happened, stated without depending on an unspecified UI mechanism. Specialist and domain-specific outcomes are fine — the test is testability, not that a layperson would recognise it. **One claim per bullet.** A bullet packing a stimulus, a contract result and visible evidence into one sentence must later be split and routed between a fragment and an operation, and a sentence routed by its dominant flavour is placed arbitrarily or duplicated wholesale (see `/parlay-create-artifacts` § Routing acceptance criteria). Splitting it here costs one rewrite; leaving it costs the routing. |
| Questions | Heuristic | Genuinely unresolved design choices. A decision already made belongs in the field that owns it; an interaction question belongs in dialogs; an architectural one in infrastructure. A question left standing after it was answered reads as open forever. |
| Intent title | Heuristic | Names what the user wants, not what the product has. "Settings page" is a screen. Several intents that differ only in a fragment are usually one intent with conditions — but variants can encode materially different obligations or actors, so this is a conversation and never an automatic merge. |

### Cohesion

**Heuristic, and the one no per-field check can reach.** Goal, Persona,
Context, Action, Objects and Verify should describe ONE causal thread. Every
field can be individually well written and still be stitched together from
different intents — `Verify` proving an outcome `Goal` never promised, or
`Objects` naming entities the `Action` never touches. Read the block as a
sentence: *this role, in this situation, does this to these things, so that
this outcome holds, and here is how you would know.* If that sentence does not
survive, the intent is describing more than one thing.

### After the freeze

`intents.md` freezes at the first green build. These boundaries are an
**authoring-time** aid and must not be applied to a frozen document: there is
no legal edit, so a finding there could only nag.

If `/parlay-doctor` or `/parlay-refine` surfaces a concern about founding
prose, treat it as a historical signal and route it to current authority — is
the CONTRACT wrong today? Then record an amendment, or supersede the intent
with `supersedes_intents:`. Is only the founding wording clumsy? Then do
nothing. History is allowed to be imperfect.
