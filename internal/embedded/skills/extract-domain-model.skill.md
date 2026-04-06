# Extract Domain Model

Analyze all features in the project and extract a domain model.

## Steps

1. **Load schemas** — Read `.parlay/schemas/intent.schema.md`, `.parlay/schemas/dialog.schema.md`, `.parlay/schemas/surface.schema.md`.

2. **Scan all features** — Read `spec/intents/*/intents.md`, `dialogs.md`, and `surface.md`.

3. **Extract entities** — From intent Objects fields and implicit references in dialogs and surfaces:
   - For each entity, derive typed properties from how it's described and used
   - Identify relationships (belongs-to, has-many, references)
   - Identify state machines from dialog conditions and intent constraints

4. **Write domain model** — Create `spec/domain-model.md` with sections:
   - Entities (with properties and relationships for each)
   - State Machines (with explicit transitions)
   - Operation Catalog (operations implied by dialogs, mapped to commands)
   - Entity Relationship Summary (tree diagram)

5. **Report** — Print the model path and a summary of what was extracted (entity count, relationships, state machines).
