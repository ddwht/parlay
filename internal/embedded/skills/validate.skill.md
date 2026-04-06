# Validate

Validate a spec file against its schema.

## Arguments

- `type`: File type — `surface`, `buildfile`, `yaml`, or `analysis`
- `path`: Path to the file

## Steps

1. Run: `parlay validate --type {type} {path}`
2. Report OK or the validation errors.
