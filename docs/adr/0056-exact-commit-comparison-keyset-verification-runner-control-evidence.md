# ADR 0056: Exact Commit Comparison, Keyset Verification, And Runner Control Evidence

- Status: Accepted
- Date: 2026-07-20
- Schema: unchanged at v82
- Slices: D1-G8, D1-V7, R5

## Context

The read-only Repository surface could inspect one exact commit and navigate one exact
file through history, but could not compare two operator-selected commit trees. Exact
verification-item pagination used an offset over a live descending list, so a new
association could shift later pages and force the client to discard the projection.
The internal non-product Runner described observed output and runtime metadata, but did
not separately state which request-level limits were configured or why the Go lifecycle
selected wait, terminate, or kill.

These gaps must close without raw Git data, long-lived SQLite read transactions,
cursor-derived authority, private verification bodies, inferred verdicts, fabricated OS
resource enforcement, signal inference, or a product process starter.

## Decision

### D1-G8: compare two exact local commit trees

`GET /api/v1/workspaces/{workspace_id}/repository-commit-comparison` requires exactly one
`base_object_id` and one `head_object_id`. Both are lowercase 40-character local object
IDs in the exact registered Workspace repository. Symbolic revisions and ancestor
requirements are rejected or absent.

The pure-Go reader uses the existing bounded tree walk and changed-path projection. It
returns `repository_commit_comparison.v1` with bounded, redacted commit subjects and
times plus canonical added/modified/deleted path, kind, content-change, and mode-change
metadata. It returns no author, body, blob, patch, remote, host root, or rename inference
and performs no checkout, reference update, subprocess, network request, or hook.

React uses a two-step operator flow: set the currently inspected exact commit as the
comparison base, then open another exact commit to load the comparison. Comparison
state is Workspace-bound; TypeScript strictly validates both requested identities,
counts, ordering, false-authority fields, and truncation.

### D1-V7: snapshot high-water plus keyset pagination

The first exact verification-item page freezes the latest association event sequence
and recomputes aggregate pass/fail/unknown counts at or below that high-water. Later
pages use the strict descending tuple `(event_sequence, association_id)` and include in
their route-scoped opaque cursor:

- the snapshot high-water;
- the previous page's final tuple; and
- the number of rows already consumed.

SQLite counts the anchor's rank inside the frozen range, and Go requires it to equal the
cursor's consumed count before reading the next page. This rejects forged positions,
cross-item reuse, missing anchors, and snapshot conflicts. Associations appended after
the first page have higher event sequences and cannot move or alter the frozen pages.
No long-running transaction is held between requests.

The existing 100,000-association read window remains a hard bound. If a snapshot has
more rows beyond that window, the final permitted response sets `page.truncated=true`
and emits no invalid continuation cursor. Private plan/evidence bodies, operator
identity, aggregate verdict, mutation, model/command execution, approval, and authority
remain absent.

### R5: configured-limit and control-cause evidence

`runner_resource_limit_evidence.v1` extends only `NonProductOnly`. It binds the
normalized run timeout, termination grace, and kill grace to the request and records
that a wall deadline is configured. It deliberately fixes CPU-time limit configured,
memory limit configured, and OS resource limits verified to false. It is metadata, not
proof of kernel- or container-enforced quotas.

`runner_termination_cause_evidence.v1` classifies the Go control path as one of:

- `process_exit`;
- `caller_cancelled`;
- `run_deadline`;
- `wait_failure`;
- `orphan_after_exit`; or
- `partial_start_failure`.

It binds that trigger to a final `wait`, `terminate`, or `kill` mechanism and the same
exit/reap/lifecycle flags already proven by the result. It explicitly excludes inferred
OS cause and signal identity. Exit, runtime, resource-limit, and termination-cause
records are validated before all four are assigned atomically. Recollection drift fails
closed with `StopEvidenceFailed` and leaves no partial replacement.

Windows Job Object and Unix process-group adapters remain in `_test.go` and start only
the current Go test binary. No CLI, HTTP, Desktop, Agent, Sandbox, approval, profile,
LocalRunner, Docker, or capability path can construct a product Runner.

## Verification

- Uncached `go test -count=1 ./...`: passed in 391.1 seconds.
- Focused race tests for Repository, Runner, Application, Store, and HTTP pagination:
  passed.
- `go vet ./...`, zero-warning `staticcheck ./...`, and `go mod verify`: passed.
- Ordinary and secure-Desktop tests: passed.
- Web strict TypeScript, 37 Vitest files / 128 tests, and Vite production build: passed.
- OpenAPI and generated TypeScript were deterministic. The contract contains 72 paths,
  78 operations, and 171 schemas. SHA-256 values are
  `839C731B4D96B9F60A1EB26A0178D0C6212282C0E1287CFC233E7C6AA9520373` and
  `741D882B9213C7AE8244FA90648D08FC351A547334E44CCCA3319C439D4E4D9F`.
- `npm audit --audit-level=high`: zero vulnerabilities.
- Windows reproducible dual build passed. The unsigned GUI SHA-256 is
  `748411c3b3dfd56768c814fd06b6da7e5e81dcd636ad69b658d862afca313e01`;
  `release_ready=false`, installer absent, and registry writes false remain correct.

The combined review found no known unresolved high or medium issue on an enabled path.
No real Provider/key, Shell, LocalRunner, Docker, Git hook, attack traffic, external
network, installer, registry mutation, or product process start was used.

## Consequences

Operators can compare exact local repository states without granting a raw Git or file
content surface. Verification pages remain stable under later appends while retaining a
strict finite read window. Future Runner implementations have explicit evidence fields
that distinguish configured request controls from OS-enforced limits and Go lifecycle
classification from kernel termination identity.

Candidate next slices are D1-G9 comparison-to-exact-file-preview navigation, D1-V8 a
bounded verification snapshot receipt/export that does not infer a verdict, and R6 a
non-product lifecycle timeline/deadline-budget evidence projection. Real Local/Docker
start, xterm input, installers/signing, Rust analyzers, network grants, and CTF solving
remain separately gated.
