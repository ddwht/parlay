# Repair Project State — Infrastructure

---

## Repair Skill and Deployer Registration

**Affects**: skill deployment, deployer registration, command dispatch
**Behavior**: Register the repair command across all agent deployers so that the /parlay-repair skill is discoverable in every supported agent environment. This includes adding a skill source file that the embedded skills system bundles into the binary, a skill title entry in the shared title map used by Claude Code and Cursor deployers, and a CLI command entry in the Generic deployer's hardcoded command list. The skill file describes the repair workflow: scan three trees, detect mismatches, present interactive questions, apply confirmed repairs.
**Invariants**:
- After `parlay upgrade`, the /parlay-repair skill is available in Claude Code sessions
- After `parlay upgrade`, the Cursor deployer includes parlay-repair in its rules file
- After `parlay upgrade`, the Generic deployer's AGENT_INSTRUCTIONS.md lists the repair command
- The skill title appears in CLAUDE.md's Available Commands list
- `parlay repair --help` shows the command with --dry-run and --yes flags
**Source**: @repair-project-state/repair-project-state
**Backward-Compatible**: yes

---
