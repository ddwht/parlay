# Feature retirement — Infrastructure

---

## Terminal retirement record shape

**Affects**: amendment ledger record shape and its retirement vocabulary
**Behavior**: A ledger record may declare that it retires the whole feature, using an explicit marker rather than being recognized by what it happens to name. It carries an outcome drawn from a closed set of exactly two values — the work moved to a named successor, or the need is gone — and a successor reference that is required by the first and forbidden by the second. Such a record may retire every remaining live promise, which an ordinary record may not, and it must name exactly the promises live at that moment: all of them, and none that an earlier record already retired. Completeness is computed against the ledger's earlier records while excluding the terminal record itself.
**Invariants**:
- A record carrying the retirement marker, a valid outcome, and every live promise validates, where the same record without the marker is refused for retiring the last live promise.
- A record naming fewer than all live promises is refused, and so is one naming a promise an earlier record already retired in place of one still live.
- An outcome outside the closed set is refused; the moved outcome without a successor is refused; the gone outcome with one is refused.
- A record that happens to name every live promise without the marker keeps the refusal it has today.
**Source**: @feature-retirement/retire-a-feature-nothing-depends-on
**Backward-Compatible**: yes

**Notes**:
- Retirement is declared rather than inferred because an inferred lifecycle transition is one nobody chose, and because the shape it would be inferred from is already an error carrying none of retirement's obligations.

---

## Successor validity

**Affects**: retirement record validation, cross-feature resolution
**Behavior**: A named successor must identify a feature that exists and still stands under applied authority — not merely a directory present on disk, and not one already carrying a retirement of its own, whether applied or authored and waiting. A feature may not name itself as its own successor. Naming a successor changes nothing about what else the retirement must satisfy: it records where the work went and grants no permission, because a reference pointing at the retiring feature does not begin pointing at the successor by being told about it.
**Invariants**:
- A successor that names no feature in the project is refused.
- A successor that is itself retired is refused, and so is one whose own retirement is recorded but not yet applied.
- A feature naming itself as successor is refused.
- A retirement naming a valid successor is still refused when anything references the retiring feature.
**Source**: @feature-retirement/retire-a-feature-nothing-depends-on
**Backward-Compatible**: yes

---

## Inbound reference inventory

**Affects**: project-wide specification scan across features and global artifacts
**Behavior**: Retirement is permitted only when nothing anywhere still points at the feature. The inventory reads specifications rather than build outputs, and never treats an unbuilt feature as incapable of depending on something — that assumption is sound for deciding what to rebuild and wrong here, because the work not yet built is exactly the work a silent removal would strand. Artifacts that belong to no feature are walked separately from features, since a walk over features alone cannot see them. What counts as pointing at the feature is a closed, documented set of machine-readable positions; prose that merely mentions the feature, and provenance recording what prompted a change, are not references and never block. Every finding names the artifact that owns it, the position within that artifact, and the reference itself.
**Invariants**:
- A reference from another feature's specification blocks retirement, including from a feature that has never been built.
- A reference from an artifact belonging to no feature blocks retirement.
- A feature named only in prose or in provenance does not block retirement.
- An illustrative reference naming no real feature is not reported.
- Each finding carries owning artifact, position and exact reference; a clean result is auditable without repeating the scan.
- A feature nothing references retires cleanly.
**Source**: @feature-retirement/refuse-to-retire-a-feature-something-still-needs
**Caching**: none
**Backward-Compatible**: yes

**Notes**:
- The existing rebuild-scoping probe may inform this inventory but is never its authority: it answers what must be rebuilt, searches build outputs, and skips anything unbuilt.
- Structural matching against the feature's qualified and bare forms, rather than substring containment, is what keeps illustrative references in prose from being reported as dependents.

---

## Retirement over an unapplied ledger

**Affects**: retirement record validation against the applied tail
**Behavior**: A feature may not be retired while its ledger carries recorded changes nobody has applied, other than the retirement itself. Closing a feature on top of unapplied changes closes it over a specification that was never true of anything. The retirement itself takes effect only once applied, as any record does, so until then the feature still makes its promises and every advancing boundary refuses to pass.
**Invariants**:
- A retirement authored while the ledger carries another unapplied record is refused, naming that record.
- A retirement authored on a fully applied ledger is accepted.
- Before the retirement is applied the feature's promises remain in force and the boundary blocks.
**Source**: @feature-retirement/retire-a-feature-nothing-depends-on
**Backward-Compatible**: yes
