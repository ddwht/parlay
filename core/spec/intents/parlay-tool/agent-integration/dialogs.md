# Agent Integration — Dialogs

---

### Resolve Ambiguities

**Trigger**: During any skill execution when the agent encounters ambiguous content

System (background): Analyzing intents and dialogs for ambiguities.
System (condition: ambiguities found): I found some things that need your input before I can generate the surface:
System: ==context excerpt==
System: ==description of ambiguity==
  A: ==option A description==
  B: ==option B description== (recommended)
  C: ==custom input==
User: B
System: Got it. Should I update ==affected-file== to reflect this?
User: Yes
System (background): Updates source file with the resolved decision.

---

### Run an AI-Heavy Command via Skill

**Trigger**: The designer invokes an AI-heavy `/parlay-*` command

User: /parlay-==command== @==feature-name==
System (background): Reads the skill file for ==command== — plain English markdown, not embedded logic.
System (background): Follows the skill's instructions: which schemas to load from .parlay/schemas/ (on-demand, not embedded in the skill), which files to read, what analysis to perform.
System (background): Calls the parlay binary for parsing, validation, and coverage checking; the binary responds with JSON output.
System (background): Performs the AI-heavy analysis and generation the CLI cannot do alone.
System: [OK] ==command== complete.

System (condition: same command, different AI agent): ==agent-name=='s deployer packaged the identical skill content into its own format — no skill or schema content changed, only the deployer.
System (background): Runs the same steps as above; the skill markdown and schemas are agent-agnostic.
System: [OK] ==command== complete.

---
