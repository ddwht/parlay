---
amendment: sweep-scope-is-lexical-not-parsing
date: 2026-08-31
trigger: "adversarial review of the built implementation found the intent claiming the sweep fails closed on 'unparseable' input, but the sweep parses nothing: it is a line-based lexical scan, so 'unparseable' promised a detection that cannot exist"
affects:
  - "@parlay-tool/root-retirement/infrastructure:project-wide-source-aware-inbound-sweep"
---

## Change

The fail-closed constraint on the inbound sweep is restated to match what the
sweep actually is. Old claim: "A file that is present but unreadable or
unparseable is recorded as a scan failure and refuses the retirement." The
word "unparseable" is retired. The honest scope: the sweep is a line-based
lexical scan over text; it parses no source, YAML, or schema into structure,
so there is no parse step at which "unparseable" could be observed. What
fails closed is exactly: a file that cannot be read, a directory that cannot
be listed — either refuses the retirement. Binary files (NUL in the leading
bytes) are passed over as non-text by documented detection, not silently
skipped by error. A syntactically broken YAML file is still text and is still
scanned; its references are still found.

## Why

A guarantee is only as honest as its observation point. "Unparseable fails
closed" reads as a promise that malformed artifacts cannot hide a reference,
but the sweep never parses, so nothing is ever observed to be unparseable —
the promise was unfalsifiable and therefore empty. The lexical framing is
strictly stronger where it matters: a broken YAML file that a parser would
reject is still scanned line by line, so a reference inside it is FOUND
rather than lost behind a parse error. The infrastructure prose and the
implementation already state the lexical scope; this amendment brings the
founding intent's wording to the same truth rather than leaving a frozen
sentence promising a detection the feature deliberately does not perform.

## Acceptance

- The sweep refuses the retirement when any file in its corpus is unreadable
  or any directory unlistable, naming the path.
- A syntactically invalid YAML/text file is scanned and its inbound
  references are reported like any other file's.
- Binary detection is documented, and a binary file produces neither a
  finding nor a failure.
- No behavior, spec, or test claims "unparseable" detection anywhere in the
  feature.
