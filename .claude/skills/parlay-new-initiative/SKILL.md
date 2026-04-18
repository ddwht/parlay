---
name: parlay-new-initiative
description: "Parlay: Create an empty initiative directory"
---

# New Initiative

Create an empty initiative directory across the three parallel trees (spec/intents/, spec/handoff/, .parlay/build/).

## Arguments

- `name`: The initiative name (e.g., `auth overhaul`)

## Steps

1. Run: `parlay new-initiative {name}`
2. If the user wants to add features immediately, suggest: `/parlay-add-feature <name> --initiative {slugified-name}`
