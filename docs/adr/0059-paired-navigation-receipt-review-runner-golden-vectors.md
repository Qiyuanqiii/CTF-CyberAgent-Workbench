# ADR 0059: Paired Navigation, Receipt Review, And Runner Golden Vectors

- Status: Accepted
- Date: 2026-07-20
- Scope: D1-G11, D1-V10/schema v84, and R8

## Context

Schema v83 and ADR 0058 established three narrow boundaries:

1. an operator can view the available base and head versions of one exact changed file
   through two existing redacted preview calls;
2. an operator can retain an immutable metadata receipt for one deterministic
   verification-item snapshot without accepting that snapshot or its result; and
3. the internal `NonProductOnly` Runner can hash its six post-reap metadata records
   without retaining the canonical body or exposing a product process starter.

The next usability gap was moving among the bounded changed files without repeatedly
closing the paired preview. The verification gap was an explicit operator fact about
whether retained receipt metadata was confirmed or disputed, while preserving the
separation between review, result acceptance, approval, and execution. The Runner gap
was cross-platform drift detection for the evidence-set canonical bytes.

## Decision

### D1-G11: synchronized paired-preview navigation

React derives a bounded candidate list only from the already returned exact comparison
changes. Previous and next controls select one candidate by index and rebuild the same
Workspace/base/head/path-bound state used by D1-G10. The two panes continue to call the
existing `repository_commit_file_preview.v1` endpoint independently. Added or deleted
files issue no request for the absent side.

There is no new Repository route, Git parser, revision expression, raw blob or patch
class, checkout, reference mutation, subprocess, network request, hook, host-root
projection, or authority. Navigation cannot move beyond the comparison response and
cannot make an omitted or non-regular entry previewable.

### D1-V10: immutable non-authorizing receipt review

Schema v84 adds these protocols:

- `operator_verification_plan_item_snapshot_receipt_review.v1`
- `operator_verification_plan_item_snapshot_receipt_review_inventory.v1`

One exact receipt may receive exactly one immutable decision:

- `metadata_confirmed`
- `metadata_disputed`

The request must repeat the exact receipt ID, content SHA-256, and receipt event
sequence, explicitly confirm the non-authorizing meaning, and carry a normalized
idempotency key. New writes require the current Code Session to remain active.
Committed exact intent may replay even after later state changes; a changed intent,
second operation for the same receipt, stale digest/sequence, or cross-Run binding
fails closed.

Store obtains a Run writer lock and rechecks Run, Session, Workspace, latest Code mode,
receipt identity, digest, sequence, and chronology in the same transaction that appends
the Run event and review row. SQLite binds the row to that event and receipt, enforces
one row per receipt, and rejects update or delete. Upgrading v83 creates no review.

The private reviewer identity remains Store-only. Public GET and POST views expose only
bounded metadata and fix all of the following to false:

- snapshot accepted;
- verification result accepted or inferred;
- record rewritten;
- approval;
- authority granted;
- execution started;
- private body or reviewer identity included.

The POST route reuses the existing independently enabled verification-evidence control
capability. This avoids adding a broader control class while leaving the default route
closed. TypeScript rejects unknown fields, duplicate or unordered inventory entries,
unsafe integer sequences, widened boolean semantics, or a mismatched Run/Session/
Workspace identity. The UI locks the receipt immediately from the successful mutation
response even when a later inventory refresh fails.

### R8: cross-platform evidence-set golden vectors

The repository stores two versioned JSON golden-vector descriptors for the existing
`runner_evidence_set_receipt.v1` canonical form:

- normal empty exit;
- forced timeout with bounded metadata and truncated output evidence.

Tests reconstruct the six typed records, marshal the fixed map-free canonical tuple,
and compare exact byte count and SHA-256. They also revalidate the protocol order and
the false claims for wall-clock ordering, raw output, process identity, verified OS
limits, and product execution. Ubuntu full Go tests and the Windows Desktop CI job run
the same vectors.

The vector test starts no product binary and imports no product Runner starter. It does
not preserve raw stdout/stderr, environment values, process identity, or canonical
evidence bytes in a runtime Artifact.

## Consequences

- Schema advances from v83 to v84 with one append-only review table and migration.
- Recording a snapshot receipt and reviewing its metadata remain separate operations.
- A confirmed review must never be interpreted as verification pass, approval, release
  readiness, execution authorization, or production Runner evidence.
- Go remains the sole authority for binding, persistence, capability checks, and event
  ordering. TypeScript is a strict presentation and response-validation layer.
- R8 improves deterministic compatibility evidence but does not move LocalRunner or
  Docker toward a product start gate.

## Verification

The cumulative six-slice gate passed on the final code:

- `go test ./... -count=1`: 357.6 seconds;
- `go test -race ./... -count=1`: 383.4 seconds;
- ten repeated focused race rounds across Verification, Store, HTTP, and Runner;
- ordinary and secure-Desktop tests, `go vet`, and zero-warning `staticcheck`;
- ordinary and secure-Desktop `govulncheck`: zero reachable vulnerabilities;
- `go mod verify` and a no-diff `go mod tidy`;
- 37 frontend test files and 130 tests, strict TypeScript/Vite production build, and
  zero npm vulnerabilities;
- deterministic OpenAPI/TypeScript generation and a reproducible Windows dual build.

OpenAPI contains 75 paths, 83 operations, and 180 schemas. The unsigned portable GUI
SHA-256 is `3bbf545b5ee07597d32345a8dce4f49f063475d881b164a18abf00fd5ff9bc6f`.
Automated Windows checks pass, while `release_ready=false` remains correct until the
manual Windows 10/WebView2 matrix is signed.

Review found and fixed four low-risk issues: an overly long Go event identifier caused
unrelated formatting churn; a successful UI mutation could leave stale review buttons
when refetch failed; a maximum signed event sequence needed explicit pre-overflow
rejection; and the v84 trigger needed to join the historical schema-downgrade fixture
chain before v83 objects were removed. No unresolved high- or medium-severity issue is
known on an enabled path.

No real Provider or API key, Shell, LocalRunner, Docker, hook, attack traffic, external
network request, installer, registry mutation, or product process start was used.
