# ADR 0055: History Navigation, Verification Pagination, And Runner Runtime Evidence

- Status: Accepted
- Date: 2026-07-20
- Schema: unchanged at v82
- Slices: D1-G7, D1-V6, R4

## Context

Exact file history already returned immutable commit object IDs, but the React workbench
did not let an operator continue from one history row into the existing exact-commit
detail and redacted-preview boundaries. Exact verification-item drilldown stopped at
100 associations. The internal non-product Runner could prove tree reaping and bounded
output/exit facts, but did not describe stdin, descriptor inheritance, or resource
metadata.

These gaps must close without raw Git access, private verification bodies, inferred
verdicts, cursor-based authority, stdin/output content retention, or a product process
starter.

## Decision

### D1-G7: history rows reuse exact read boundaries

Every `repository_file_history.v1` row can open its exact object through the existing
`repository_commit_detail.v1` client path. A regular or executable row can also open
the existing `repository_commit_file_preview.v1` path for the exact history object and
canonical file path. Deleted, symlink, and submodule rows have no preview action.

The renderer sends only the already projected Workspace ID, exact object ID, and
canonical path. Go remains responsible for object validation, root binding, content
limits, UTF-8 checks, and secret redaction. No raw blob, patch, checkout, reference
mutation, process, network, remote, or hook surface was added.

### D1-V6: route-scoped opaque evidence pagination

`GET /api/v1/runs/{run_id}/verification-plan-coverage/{plan_id}/items/{ordinal}` now
accepts the shared strict `limit` and opaque `cursor` parameters. SQLite orders by
immutable association event sequence and ID, applies a bounded offset, and returns one
look-ahead row. Go revalidates exact Run/Session/Workspace/plan/item ownership, digests,
aggregate counts, page size, strict descending order, unique association/evidence IDs,
and explicit outcomes on every page.

The cursor is route-scoped, capped at a 100,000-row starting offset, and cannot be
reused for another plan item. This is a bounded live projection rather than a database
snapshot. React therefore compares immutable aggregate identity and event high-water
facts across loaded pages, rejects duplicate or non-descending rows, and asks the
operator to refresh if the live result changed. The UI loads 25 older references per
explicit action. It never receives plan/evidence bodies, operator identity, an
aggregate verdict, mutation, approval, command/model authority, or a resume token.

### R4: atomic non-product runtime metadata

`runner_runtime_evidence.v1` extends only the internal `NonProductOnly` lifecycle. A
backend must first prove the whole process tree reaped. It may then report:

- a bounded stdin byte count and SHA-256 while fixing stdin closed, not inherited, and
  raw input absent;
- captured stdout/stderr descriptors while fixing extra/inherited descriptor counts to
  zero and excluding names and paths;
- bounded wall time, parent user/system CPU milliseconds, and optional bounded peak
  resident bytes while excluding raw and network telemetry.

Exit evidence and runtime evidence are collected with independent bounded post-reap
contexts, validated, compared with any prior collection, and committed to `Result`
atomically. A malformed or changing runtime record yields `ErrRuntimeEvidence` and
`StopEvidenceFailed`; it does not leave a partial exit-only audit record or misclassify
the tree as orphaned. Windows Job Object and Unix process-group implementations remain
in `_test.go`, start only the current Go test binary, and provide no CLI, HTTP, Desktop,
Agent, Sandbox, LocalRunner, Docker, profile, approval, or capability wiring.

## Verification

- Uncached `go test -count=1 ./...`: passed in 377.3 seconds.
- Full `go test -race -count=1 ./...`: passed in 409.8 seconds.
- Ordinary and secure-Desktop tests, `go vet`, zero-warning `staticcheck`, module
  verification/tidy diff, and ordinary plus Desktop-tag `govulncheck`: passed with no
  reachable vulnerability.
- Web strict TypeScript, 37 Vitest files / 127 tests, and Vite production build: passed.
- OpenAPI and generated TypeScript were deterministic. OpenAPI remains 71 paths,
  77 operations, and 170 schemas. SHA-256 values are
  `7418F7CAEED0BA6A5E69E574215F22CC8AA47458A75FB70FD0679FDEDD332BA1` and
  `A3EE3B6E7E1020924B6AB1140F3EC4176A9550407187B7DDB3AA1E6FA15697CD`.
- `npm audit --audit-level=high`: zero vulnerabilities.
- Windows reproducible dual build passed. The unsigned GUI SHA-256 is
  `1d51529b1a6d7d90e121e770faa54c9f4d77b4a96d3c0d920fe091178a299da2`;
  `release_ready=false`, installer absent, and registry writes false remain correct.
- The Linux amd64 Runner test binary cross-compiled successfully. Isolated CLI smoke
  registered only `mock`; repository scans found only fixed fake credential fixtures
  and test-only OS process entry points.

The combined audit found no unresolved high or medium issue on an enabled path. No real
Provider/key, Shell, LocalRunner, Docker, Git hook, attack traffic, external network,
installer, registry mutation, or product process start was used.

## Consequences

Operators can move from file history to exact commit context and inspect arbitrarily
older explicit verification references within a bounded live window. Future Runner
backends now have a fail-closed metadata contract for stdin, descriptors, and resources,
but R4 is still conformance evidence rather than execution authority.

Candidate next slices are D1-G8 bounded exact-commit comparison, D1-V7 keyset/high-water
hardening for live verification pagination, and R5 non-product resource-limit and
termination-cause evidence. Real Local/Docker start, xterm input, installers/signing,
Rust analyzers, network grants, and CTF solving remain separately gated.
