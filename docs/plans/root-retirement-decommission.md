# Decommission record: root-retirement (user-authorized one-time exception)

Date: 2026-08-31. Authorized by the project owner as one half of a two-part
decision; the other half — executing `parlay retire-root studio` — completed
first, at commit 7b36e0e, with the binary pinned at v0.7.0-29-g83cd824.

## What was removed

The `parlay retire-root` and `parlay retired-roots` commands, their
implementation (retire_root, retired_roots, retirement_journal, root_archive,
root_dispositions, root_retirement_sweep — roughly seven thousand lines with
their tests), their registrations, and the `parlay-tool/root-retirement`
feature's spec and build artifacts. Everything removed remains recoverable at
the recovery-point commit (7b36e0e) and throughout git history.

## Why this is an explicit exception, not ordinary governance

Parlay's feature-retirement flow deliberately refuses features whose build
artifacts exist (`reportNothingBuilt`), and root-retirement is a fully built
feature — so no existing mechanism can retire it, and building a
delivered-feature retirement lifecycle solely to delete a one-time lifecycle
tool would recreate the maintenance problem one level up. This record IS the
governance: a named, dated, user-authorized exception, chosen over (a)
keeping ~7k lines of destructive-operation code alive for a speculative
second user — the feature's own founding Questions set the evidence bar at "a
second retired root", which does not exist — and (b) a thin retained checker,
which would preserve a permanent archive-format obligation just to wrap
sha256.

The operation was a migrator: needed to cross a boundary once, unnecessary
when every supported state is on the far side. The studio archive's ongoing
guarantees do not depend on the removed code — git history authenticates the
committed result, and the manifest is deliberately standard SHA-256 data.

## Cutoff

An interrupted pre-removal retirement run (a `*.journal.yaml` under
`.parlay/retired/`) must be completed with the pinned binary from commit
83cd824 (`git checkout 83cd824 && make build`) before moving past it. No
later binary can resume it. As of this record, no journal exists and the one
retirement this tool ever performed is complete and verified.

## Verifying the studio archive with standard tools

The archive at `.parlay/retired/studio/` holds `contents/` (102 files),
`manifest.yaml` (per-member sha256), `dispositions.yaml` (the operator's
authorization), and `retirement-record.yaml`. To re-verify byte integrity:

```
python3 - <<'EOF'
import hashlib, os, re
base = ".parlay/retired/studio"
man = open(os.path.join(base, "manifest.yaml")).read()
members = re.findall(r"- path: (\S+)\n(?:      type: (\S+)\n)?      sha256: ([0-9a-f]{64})", man)
ok = bad = 0
for path, typ, want in members:
    full = os.path.join(base, "contents", path)
    data = os.readlink(full).encode() if typ == "symlink" else open(full, "rb").read()
    if hashlib.sha256(data).hexdigest() == want:
        ok += 1
    else:
        bad += 1
        print("MISMATCH:", path)
on_disk = {os.path.relpath(os.path.join(d, f), os.path.join(base, "contents"))
           for d, _, fs in os.walk(os.path.join(base, "contents")) for f in fs}
listed = {p for p, _, _ in members}
print(f"{ok} ok, {bad} mismatched; unlisted: {sorted(on_disk - listed) or 'none'}; missing: {sorted(listed - on_disk) or 'none'}")
EOF
```

Last verified at decommission time: 102 ok, 0 mismatched, none unlisted,
none missing.
