# Brownfield — Dialogs

---

### Onboard Existing Codebase

**Trigger**: /parlay-onboard ==source-root==

User: /parlay-onboard src/
System (background): Reads project configuration files to detect framework.
System (condition: framework detected): Detected framework: ==framework name== (from ==config file==).
System (condition: framework unclear): I couldn't determine your UI framework automatically.
  A: ==detected option 1==
  B: ==detected option 2==
  C: ==detected option 3==
  D: Other (describe)
User: A
System (background): Loads bundled adapter template as starting point.
System (background): Scans directory structure for file conventions.
System (background): Reads representative components to extract coding conventions.
System (background): Greps source tree for widget patterns.
System: I've analyzed your codebase. Here's the drafted adapter:
System: **Framework**: ==framework name==
System: **File conventions**: source-root: ==root==, naming: ==convention==
System: **Conventions detected**:
System: - State management: "==pattern== (found in ==N== files)"
System: - Data fetching: "==pattern== (found in ==N== files)"
System: **Mount strategies detected**:
System: - ==strategy name==: Found ==pattern== in ==files==
System: How would you like to proceed?
  A: Register this adapter
  B: Let me review and edit the YAML first
  C: Re-scan with a different source root
User: Selects A
System (background): Writes adapter to .parlay/adapters/ and updates config.
System: [OK] Adapter registered. You can now add features with /parlay-add-feature.

---
