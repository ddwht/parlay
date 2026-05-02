# Add Feature

Create a new feature folder with intents.md and dialogs.md. Optionally place it inside an initiative.

## Arguments

- `name`: The feature name (e.g., `upgrade plan creation`)
- `initiative` (optional): The initiative to create the feature inside (e.g., `auth overhaul`). Auto-creates the initiative if it doesn't exist.

## Steps

1. If the user specified an initiative: run `parlay add-feature {name} --initiative {initiative}`
2. Otherwise: run `parlay add-feature {name}`
3. Tell the user to start authoring intents.md.
