## Summary

- Describe the user-visible or architectural change.
- Explain why this slice is needed now.

## Validation

- [ ] `go test -count=1 ./...`
- [ ] `go vet ./...`
- [ ] Relevant CLI smoke tests

## Audit

- [ ] No credentials or local runtime data are included.
- [ ] Policy, workspace, sandbox, and persistence boundaries were reviewed.
- [ ] `README.md` or project memory was updated when behavior or progress changed.
