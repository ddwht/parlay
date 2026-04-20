---
name: parlay-load-domain-model
description: "Parlay: Load and integrate external domain model"
---

# Load Domain Model

Load an external domain model and integrate it with the current project's model.

## Arguments

- `path`: Path to the external domain model file

## Steps

1. **Read both models**:
   - External model at `{path}`
   - Current project model at `spec/domain-model.md` (may not exist yet)

2. **Compare entities** — For each entity in the external model:
   - If it only exists in the external model → will be added
   - If it only exists in the current model → will be kept
   - If it exists in both with different definitions → conflict

3. **If conflicts found** — Present each one to the user:
   - Show the entity name and both definitions
   - Offer options:
     - A: Keep current project definition
     - B: Use external definition
     - C: Merge properties from both
     - D: Custom mapping (user describes)
   - Wait for the user's response for each conflict

4. **Merge models** — Apply the user's decisions and write the merged result to `spec/domain-model.md`.

5. **Report** — Confirm integration and summarize what changed.
