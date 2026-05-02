# Move Feature — Surface

---

## Move Result

**Shows**: status, message, data-value
**Actions**: invoke
**Source**: @move-feature/move-a-feature-between-locations

**Page**: move-feature
**Region**: main
**Order**: 1

**Notes**:
- Output of `parlay move-feature @feature --to <initiative>` or `parlay move-feature @feature --out`.
- `status` carries the outcome: success (feature moved), scope collision (target already has same slug), top-level namespace collision (--to value is an orphan feature), non-existent feature, wrong type (tried to move an initiative), argument error (mutually exclusive flags), no-op (already at target), or rollback (partial failure).
- `message` always shows both before and after qualified paths so the designer can see exactly what happened. For auto-created initiatives, distinguishes "initiative created" from "feature moved". For rollback, explains what failed and confirms the feature is back at its original location.
- `data-value` shows the before/after paths on all three trees (spec/intents/, spec/handoff/, .parlay/build/).
- For no-ops (feature already at target location, --out on an orphan), `message` confirms the current location and reports "no change."
- For the empty-initiative edge case (last feature moved out), `message` notes the initiative is now empty and in deferred classification, and suggests next steps (remove manually or reuse).
- Collision error messages always name the conflicting path and suggest the resolution (rename before retrying, or pick a different target).
- `invoke` as a follow-up is contextual: on success, the designer may want to run commands using the new qualified identifier.

---
