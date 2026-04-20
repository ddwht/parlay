---
name: parlay-collect-questions
description: "Parlay: Collect open questions from intents"
---

# Collect Open Questions

Scan intents for unresolved design questions. Use this as a quality gate before running build-feature.

## Arguments

- `feature` (optional): The feature slug. If omitted, scans all features.

## Steps

1. **Collect questions** — Run: `parlay collect-questions @{feature}` (or `parlay collect-questions` for all features)

2. **Present results** — Show the user:
   - Total open question count
   - Questions grouped by intent, with priority shown
   - If count is 0: confirm the feature is ready for build

3. **If questions exist** — Ask the user:
   - A: Resolve them now (walk through each question)
   - B: Proceed anyway (acknowledge the gaps)
   - C: Skip — just the report

4. **If resolving** — For each question:
   - Present the question in context (show the intent's Goal and Constraints)
   - Ask for the designer's answer
   - Offer to update intents.md: remove the resolved question and add any new Constraints or Verify bullets based on the answer
