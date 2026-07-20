# ADR 0058: Paired Comparison, Snapshot Receipts, And Runner Evidence-Set Digest

- Status: Accepted
- Date: 2026-07-20
- Schema: v83
- Slices: D1-G10, D1-V9, R7

## Context

Comparison rows could open either exact redacted side, but operators could not retain
both sides in one stable review workspace. Verification snapshot exports were
deterministic but ephemeral: there was no append-only fact recording that an operator
retained one exact digest. The internal non-product Runner had six independently
validated evidence records but no single canonical receipt binding the complete set.

These gaps must close without raw Git access, snapshot content persistence, inferred
verification results, acceptance semantics, operator identity disclosure, new approval
or execution authority, raw process output, or a product Runner starter.

## Decision

### D1-G10: bounded paired redacted preview workspace

React adds an explicit paired-preview command to each comparison row with at least one
regular or executable side. The selection is bound to Workspace, exact base object,
exact head object, and canonical path. Two independent calls reuse the existing
`repository_commit_file_preview.v1` endpoint; an added or deleted file renders an
explicit absent side rather than inventing content. Each available pane repeats the
exact hash/path returned by Go and retains the existing redaction and size limits.

This is renderer composition only. It adds no Git route, symbolic-ref parser, raw blob
or patch access, checkout, mutation, process, network, hook, or authority.

### D1-V9: immutable deterministic snapshot receipt history

Schema v83 adds append-only `operator_verification_snapshot_receipts` and the
`verification.snapshot_receipt_recorded` event. A controlled POST first rebuilds the
current deterministic JSON or Markdown snapshot and requires an exact protocol,
Run/plan/item binding, association-event high-water, content SHA-256, explicit
`confirm_metadata_snapshot=true`, and a normalized idempotency key. The Store obtains a
Run writer lock and rechecks the active Code Session, Workspace, plan/item digests,
current high-water, counts, and truncation before inserting the event and receipt in one
transaction. A later association therefore makes a new stale receipt fail closed;
an already committed identical operation can still replay exactly once.

The table stores metadata only: format, exact bindings, counts, truncation, digest,
byte count, private recorder identity, event sequence, and time. It never stores the
snapshot body. SQLite rejects update and delete, validates the matching metadata-only
event, and requires every acceptance/authority flag in that event to remain false.
Migration v82 to v83 creates no synthetic receipt.

`GET|POST /api/v1/runs/{run_id}/verification-snapshot-receipts` returns at most 100
newest receipts. Public projections omit the private recorder identity and force
content included, private bodies, operator identity included, snapshot accepted,
result accepted/inferred, rewrite, approval, authority, and execution to false. POST
reuses the existing default-off verification control capability; no new Desktop flag
or mutation class is introduced. TypeScript validates exact keys, bindings, counts,
descending event order, and all closed safety fields. The UI keeps download and receipt
commands separate and labels persisted entries as `record only`.

Recording proves only that an operator retained metadata for exact deterministic bytes.
It is not acceptance of the snapshot, its observations, or a verification result. Any
future acceptance/rejection decision needs a separate immutable protocol and review.

### R7: canonical six-record evidence-set receipt

`runner_evidence_set_receipt.v1` canonicalizes the fixed six-record tuple: exit,
runtime, configured resource-limit, termination-cause, logical lifecycle timeline, and
independent deadline-budget evidence. A fixed Go struct with no maps produces bounded
JSON bytes; only SHA-256, byte count, exact protocol inventory, and closed semantic
flags remain in the Result. Canonical bytes are neither retained nor exposed.

All six records and the receipt validate before the seven Result fields are assigned.
Recollection drift or receipt tampering fails closed without partial replacement. The
receipt explicitly claims no cross-record wall-clock order, raw output, process
identity, verified OS limits, or product execution. Concrete process adapters remain
test-only and no CLI, HTTP, Desktop, Agent, Sandbox, LocalRunner, Docker, or product
starter imports the harness.

## Verification

- Focused domain, application, schema-v83 Store, HTTP/OpenAPI, React, and Runner tests
  passed, including tamper, stale-snapshot, idempotent replay, immutable update/delete,
  v82 upgrade, absent comparison side, and no-partial-replacement cases.
- Uncached `go test ./... -count=1` passed in 394.1 seconds with its exit code
  explicitly propagated; `go vet ./...` passed.
- Desktop boundary tests with `desktop,wv2runtime.error` and reproducible Windows dual
  build passed. The unsigned GUI SHA-256 is
  `d5e37e193223a41939598edceb77a92637430b0c87c52233cdafb9c2fda10bb5`;
  `release_ready=false`, installer absent, and registry writes false remain correct.
- Web strict TypeScript, 37 Vitest files / 129 tests, and Vite production build passed.
- OpenAPI and generated TypeScript reproduced byte-for-byte across two generations.
  The contract contains 74 paths, 81 operations, and 176 schemas. SHA-256 values are
  `7E50A343391F167989E871828B1494F45E3A02581198D5B880C3FC3E795B521D` and
  `A693C4E62D65B7D39A5E5668EA319F57E97613AA61234B0658ABC9CBF80F9334`.

Combined review found and fixed three low-risk contract/verification issues before
delivery: the reflection generator does not flatten an embedded control DTO, so receipt
control fields are now explicit; the inventory protocol is constrained to its sole v1
enum; and the authenticated OpenAPI live-route fixture now builds one exact valid
snapshot-receipt request. The first remote Go run exposed the missing fixture body.
After correction the focused route passed five consecutive runs and the full local
suite returned exit code 0. No known unresolved high or medium issue remains on an
enabled path. No real
Provider/key, Shell, LocalRunner, Docker, Git hook, attack traffic, installer, registry
mutation, or product process start was used.

This is a three-slice functional gate. The next batch, after six cumulative slices,
must run the full race/staticcheck/govulncheck/dependency/privacy robustness gate.

## Consequences

Operators can review both exact redacted comparison sides together and retain an
auditable, immutable metadata receipt for one deterministic verification snapshot.
Runner design tests can bind all six post-reap evidence records to one digest without
turning test evidence into production execution evidence.

Candidate next slices are D1-G11 bounded synchronized paired-preview navigation,
D1-V10 an independently designed explicit non-authorizing receipt review decision, and
R8 cross-platform canonical receipt golden vectors. Real Local/Docker start, xterm
input, installer/signing, Rust analyzers, network grants, and CTF workflows remain
separate release gates.
