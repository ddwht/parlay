# Feature retirement — Dialogs

---

### Retiring a feature nothing needs

**Trigger**: A maintainer decides a feature should stop existing, and runs `/parlay-refine` against it.

User: /parlay-refine @parlay-tool/loop-coverage-review-phase
User: This whole feature goes away. Mechanical readiness replaced what it was for.
System (background): Resolves live intents; the change would retire all of them, which an ordinary supersession refuses.
System (background): Scans every feature's specification, the page manifests and the project domain model for references to this feature.
System: This retires the feature, not just a promise. It has nothing built — no contract artifacts, no buildfile, no generated code — and nothing references it.
System: Live promises being retired: ==insert-a-coverage-review-step-between-the-build-and-code-phase-groups== — every promise the feature still makes.
System: Did this work move somewhere, or stop being needed?
    a. Moved — name the feature that carries it now
    b. Obsolete — the need itself is gone
User: b
System: Retire ==@parlay-tool/loop-coverage-review-phase== as obsolete?
    a. Retire — write the terminal amendment
    b. Cancel — change nothing
User: a
System (background): Writes the amendment with `retires_feature: true`, `outcome: obsolete`, and every live intent named. `intents.md` is not opened for writing.
System: Wrote amendment 001. The feature is retired once the amendment is applied; until then it still makes its promises and the boundary is blocked.

---

### Refusing a retirement something still stands on

**Trigger**: The same flow, on a feature another feature's specification references.

User: /parlay-refine @catalog/pricing
User: Retire this, it is finished.
System (background): Scans specifications rather than builds, so a feature that has never been built is still visible.
System: Cannot retire ==@catalog/pricing== — ==3== things still reference it:
    a. ==@catalog/checkout== · surface.yaml · fragment ==order-summary== · `supersedes: @pricing/price-panel`
    b. ==@catalog/reporting== · capabilities.yaml · operation ==quote.render== · `source: @pricing/price-quoting`
    c. ==pages/catalog.page.md== · fragment list · `@pricing/price-panel`
System: ==@catalog/reporting== has never been built, so its reference lives only in its specification and a rebuild-scoping probe would not have found it. Retiring on that answer would have removed ground from underneath the work not yet done.
System: Resolve each of these, then retire.

---

### Refusing a feature that still has something built

**Trigger**: The maintainer tries to retire a feature that has been built.

User: /parlay-refine @catalog/pricing
User: Retire this.
System: Cannot retire ==@catalog/pricing== — it still has ==surface.yaml==, ==capabilities.yaml==, a buildfile and ==7== generated files recorded against it.
System: Retirement records a decision; it does not remove artifacts or generated code, so those would stay on disk and keep being read after the feature was declared gone. Deciding what becomes of them is work this operation does not yet do, and refusing is the honest answer rather than a partial one.

---

### Naming a replacement that is not there

**Trigger**: The maintainer says the work moved, and names a feature that is gone or going.

User: a
User: It moved to @catalog/pricing-v2.
System (condition: the named feature does not exist): ==@catalog/pricing-v2== is not a feature in this project. Name the feature that carries this work now, or record the outcome as obsolete.
System (condition: the named feature is itself retired): ==@catalog/pricing-v2== is itself retired, by amendment ==003-folded-into-billing==. Pointing a later reader at something also gone is worse than naming nothing — name the feature that carries the work now, or record this as obsolete.
System (condition: the named feature is the retiring feature): A feature cannot replace itself.

---

### Retiring over changes nobody applied

**Trigger**: The feature's ledger carries an unapplied amendment when the retirement is authored.

System (background): Reads the ledger tail against the baseline.
System: Cannot retire — amendment ==002-tighten-quote== is recorded but not applied. Retiring on top of it would close the feature over a specification that was never true. Apply it, or withdraw it, then retire.

---

### Declining to answer for a person who is not there

**Trigger**: A retirement is reached during an unattended run.

System (background): `/parlay-refine --non-interactive` reaches a change that would retire a feature.
System: Blocked — retiring a feature ends what it commits to, and there is no safe default for that. Nothing was written.
System: Re-run interactively, or record the amendment by hand with `retires_feature: true`, an `outcome:`, and every live intent named.
