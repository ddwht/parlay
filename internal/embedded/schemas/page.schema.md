# Page Schema

File: `spec/pages/<page-name>.page.md`
Optional. Locks the layout of a page when the derived view from feature surfaces isn't enough. Created via `/parlay lock-page ==page-name==`.

By default, pages are derived views assembled on demand from feature surfaces. A manifest is only needed when cross-feature layout needs an explicit owner.

## Template

```
# <Page Name>

> <One-line description>

**Owner**: <Team or person responsible>
**Status**: <draft | reviewed | locked>

## <Region Name>

1. @feature-a/fragment-name
2. @feature-b/fragment-name

## <Region Name>

1. @feature-c/fragment-name
```

## Fields

| Field | Required | Parse rule |
|---|---|---|
| Page Name | Yes | `# ` heading. Must match Page values in feature surfaces. |
| Description | No | `> ` line after heading |
| Owner | No | `**Owner**:` line content |
| Status | No | `**Status**:` value — `draft`, `reviewed`, or `locked` |
| Region | Yes (at least one) | `## ` heading. Must match Region values in feature surfaces. |
| Fragment list | Yes | Numbered list (`1.`, `2.`, ...) of `@feature/fragment-name` references. Order overrides feature surface Order values. |

## Behavior

- Manifest order overrides feature surface Order values
- Unlisted fragments targeting this page appear after manifest-ordered ones in `/parlay view-page`
- Tool warns on drift (new/removed fragments) but never auto-updates a locked manifest
- Does not define layout dimensions or styling — that's the prototype framework's job

## Parsing

- Page identity: `# ` heading
- Metadata: `**Field**:` pattern
- Regions: `## ` headings
- Fragment ordering: numbered list items
- Fragment references: `@feature/fragment-name` pattern
