# Feature retirement

> Intent supersession retires one promise; a feature that should stop existing entirely is a different operation, and `amendment-supersedes-last-intent` refuses to let the ledger fake it. That refusal is correct — a feature promising nothing is a question about what still depends on it, not a question the ledger can answer — but it currently points nowhere. This feature is where it points. Retirement is recorded as a terminal decision in the feature's own ledger, and is permitted only when nothing anywhere still needs the feature.

---

## Retire a feature nothing depends on

**Goal**: Close a feature that should no longer exist, recording whether its work moved somewhere named or simply stopped being needed, so the decision is legible later and nothing is left pointing at something that is gone.
**Persona**: Parlay tool maintainer
**Priority**: P1
**Context**: `amendment-supersedes-last-intent` refuses a ledger entry that would retire a feature's last live promise, and routes to "a lifecycle operation with its own dependency checks" that does not exist. Retirement is recorded in the feature's own ledger rather than in a new artifact, keeping the one "this replaces that" model the project already applies at fragment and amendment level, and inheriting the decision gate, the reasoning and successor requirements, and the ledger's integrity walks. It is marked explicitly rather than inferred from an amendment naming every intent: that shape is already an error, carries none of retirement's obligations, and an inferred lifecycle transition is one nobody chose.
**Action**: Add `retires_feature: true` to the amendment front matter, with a closed `outcome:` of `replaced` or `obsolete`, and `replacement_feature:` required when replaced. Permit such an amendment to retire every remaining live intent, which `amendment-supersedes-last-intent` otherwise refuses.
**Objects**: feature, terminal-amendment, retirement, outcome, replacement-feature, lifecycle

**Constraints**:
- Retirement is declared, never inferred. An amendment that happens to name every live intent without `retires_feature: true` remains the error it is today; the marker is what carries the obligations.
- `outcome:` is closed at `replaced | obsolete`. `replaced` requires `replacement_feature:`; `obsolete` forbids it. A reader months later cannot recover from silence whether work moved or stopped mattering, and that difference is the whole content of the decision.
- A named `replacement_feature:` must resolve to a feature that exists and is **active under applied authority** — not merely a directory on disk, and not one carrying an authored-but-unapplied retirement of its own. Directing a reader at something that is also gone is worse than naming nothing. A feature may not name itself.
- `replacement_feature:` is metadata about the outcome, never permission. Naming a successor does not license retiring a feature something still references: `replaced` faces the same zero-inbound rule as `obsolete`, because a reference pointing at this feature does not start pointing at the replacement by being told about it.
- `supersedes_intents:` on a retirement must name **exactly** the feature's live intents — every one, and only ones currently live. Computed against the ledger's earlier amendments and excluding the terminal amendment itself, so the set is what is live at the moment of retirement. Fewer would close a feature while a promise stands; a historical intent listed in place of a current one would look complete while missing what actually remains.
- Retirement inherits every obligation of an intent-superseding amendment: non-empty `## Why` and `## Acceptance`, and the decision gate with no safe default and no unattended path.
- A feature may not be retired while its ledger carries unapplied amendments other than the retirement itself. Retiring on top of changes nobody applied closes a feature over a specification that was never true.
- The retirement takes effect only once applied, exactly as any other amendment does.
- A feature may be retired only when it has **nothing built**: no contract artifacts, no buildfile or testcases, and no generated code recorded against it. This is what makes the narrow cut sound rather than merely narrow. Retirement does not delete anything, so a feature with artifacts would keep them on disk and readable by every consumer that enumerates features, and a feature with generated code would keep shipping it. Refusing that case is honest; claiming to have handled it would not be.
- Exactly one record in a ledger may carry the retirement marker, and it must be the last record in the ledger. A retirement followed by further changes is a feature that did not end where it said it did.

**Verify**:
- An amendment with `retires_feature: true`, a valid `outcome:`, and every live intent named validates, where the same amendment without the marker fails with `amendment-supersedes-last-intent`.
- A retirement on a feature that has contract artifacts, a buildfile, testcases, or generated code fails, naming what is still there.
- A second retirement record in one ledger fails, and so does a retirement followed by any later record.
- A retirement that has not been applied is reported as pending rather than as the feature having ended.
- `outcome: replaced` with no `replacement_feature:` fails, and so does `outcome: obsolete` with one.
- An `outcome:` outside the closed set fails.
- A `replacement_feature:` naming a feature that does not exist fails, and so does one naming a feature that is itself retired.
- A retirement authored while the ledger carries other unapplied amendments fails.
- A retirement naming an intent an earlier amendment already retired fails, as does one missing a live intent: the set is exactly the live intents, and a set padded with history reads as complete while a live promise goes unnamed.
- A retirement amendment missing `## Why` or `## Acceptance` fails, as any superseding amendment does.

---

## Refuse to retire a feature something still needs

**Goal**: Make a retirement that would break something impossible rather than merely discouraged, so closing a feature can never silently remove ground another feature is standing on.
**Persona**: Parlay tool maintainer
**Priority**: P0
**Context**: The project already has a reverse-dependency probe, `parlay internal affected-set`, and it answers a different question: what must be rebuilt if this changes. It searches **built** buildfiles and skips any feature whose buildfile it cannot read, on the reasoning that nothing can depend through an unbuilt feature. That is sound for rebuild scoping and wrong here — a feature nobody has built yet can reference the retired one in its specification, and retiring under that check would remove it from underneath precisely the work not yet done. The existing scope accounting is also the wrong direction: it walks contract entries inside a feature whose `source:` names a retired intent, which is outbound, while retirement is a question about inbound references owned elsewhere and about artifacts no feature owns at all.
**Action**: Scan the whole project's specifications for references to the retiring feature and refuse the retirement while any remain, naming each one and where it lives.
**Objects**: dependency-scan, inbound-reference, page-manifest, domain-model, buildfile-reference, provenance

**Constraints**:
- The scan reads specifications, not builds, and never skips a feature because it has not been built. Being unbuilt is what makes a dependent invisible to the existing probe and is exactly the case retirement must not miss.
- Project-global artifacts are scanned separately from features: page manifests and the singleton domain model are not any feature's specification, and a scan that only walks features would miss them entirely.
- The reference positions that count are a **closed, documented set** of machine-readable fields in **specifications**, not wherever the name appears. Included: surface fragment references, capability operation references, infrastructure `Source:` citations, amendment `affects:`, page manifest fragment lists, the project domain model, and buildfile and testcase references. Excluded: narrative prose, dialogs, source comments, and `trigger:` provenance. A rule that blocks on any occurrence of a string is one people learn to route around, and a closed set is what makes "nothing depends on this" a claim rather than a hope.
- Generated ownership markers are **not** scanned, and the claim is scoped to match: what is established is that no supported specification reference remains, not that nothing anywhere does. Generated ownership is instead excluded by the precondition above — a feature with generated code cannot be retired at all — so the narrower claim is sufficient rather than convenient.
- A refusal reports the **owning artifact, the field or path within it, and the exact reference** — enough for a reader to verify the finding without repeating the scan, and enough that a clean result is auditable rather than merely asserted.
- References are matched structurally against the feature's qualified and bare forms, not by substring. Buildfiles contain illustrative refs in prose that name no real feature, and a substring scan reports them as dependents.
- A narrative mention is not a dependency. A `trigger:` naming a feature, and prose that happens to contain its name, are provenance and must not block a retirement — a rule that blocks on any occurrence of a string is one people learn to work around.
- Refusal names every blocking reference and where it lives. A refusal that reports only a count leaves the person to find them, which is the work the scan just did.
- The scan is the authority; `affected-set` may inform it but never substitutes for it.

**Verify**:
- A feature referenced by another feature's specification cannot be retired, and the refusal names the referring feature and the reference.
- A reference from another feature's amendment `affects:` blocks retirement.
- A reference appearing only in a non-reference field — a `Verify:` bullet, a `Behavior:` paragraph, a page description — does not block retirement.
- The same holds when the referring feature has never been built, which the existing probe would have missed.
- A reference from a page manifest or from the project domain model blocks retirement and is named.
- A feature named only in a `trigger:` or in prose is not treated as a dependent.
- A feature nothing references retires cleanly.
- An illustrative buildfile ref naming no real feature is not reported as a dependent.

**Questions**:
- Dependency freedom and replacement validity are checked when the retirement is authored, not again when it is applied, which leaves **two** ways the approval can go stale. A new inbound reference can appear in between, so the retirement lands on a feature something needs. And a `replacement_feature:` active at authoring can itself retire before this one applies, so the record directs a reader at something gone — the exact failure the replacement rule exists to prevent. Re-validating both at apply time closes them; it was deliberately deferred with the rest of the disposition machinery, and is the first thing to add when a real case demands it.
- Retiring a feature that has anything built is refused rather than handled, because handling it means deciding what happens to artifacts and generated code that retirement does not itself remove — and ownership is not per-file, since files are shared, extended and hand-maintained. The motivating case has nothing built, so the restriction costs nothing today. The first feature worth retiring that does have output will need the disposition and removal work this defers, and until then the refusal is the honest answer rather than a silent partial one.
