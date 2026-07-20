# ADR 0057: Comparison Preview, Verification Snapshot, And Runner Timeline Evidence

- Status: Accepted
- Date: 2026-07-20
- Schema: unchanged at v82
- Slices: D1-G9, D1-V8, R6

## Context

Exact commit comparison exposed safe changed-path metadata, but an operator still had
to leave the comparison flow to inspect either side through the existing redacted
exact-file preview. Exact verification-item pagination produced a stable snapshot, but
there was no deterministic downloadable representation that another review step could
digest-check. The internal non-product Runner had bounded post-reap evidence, configured
request limits, and a Go termination classification, but not one canonical logical
timeline or an explicit inventory of the independent context ceilings used by the
lifecycle harness.

These gaps must close without a raw Git surface, private verification text, inferred
verdict, durable acceptance fact, wall-clock timing claim, OS resource-enforcement
claim, or product process starter.

## Decision

### D1-G9: comparison rows reuse exact redacted preview

React comparison rows now expose separate base and head preview commands only when the
corresponding tree entry is a regular or executable file. Added rows therefore expose
only head, deleted rows only base, and modified regular/executable rows may expose both.
Symlinks, submodules, and absent sides remain non-previewable.

The commands submit only the Workspace, exact object ID, and canonical path already
projected by Go and reuse `repository_commit_file_preview.v1`; no route or Repository
authority was added. Preview selection is bound to the Workspace and exact selected
object rather than the currently open detail commit. The preview header repeats the
returned exact hash and path so base/head context cannot be hidden by later navigation.

### D1-V8: deterministic verification snapshot download receipt

`GET /api/v1/runs/{run_id}/verification-plan-coverage/{plan_id}/items/{ordinal}/snapshot-export`
requires exactly one unmodified `format=json|markdown` query value. Missing, duplicate,
blank, whitespace-padded, and unknown values fail closed.

The service reuses the exact verification-item detail boundary to freeze the current
association-event high-water. It emits
`operator_verification_plan_item_snapshot_export.v1` around deterministic
`operator_verification_plan_item_snapshot.v1` content with:

- exact Run, Session, Workspace, plan, item, and SHA-256 bindings;
- explicit pass, fail, unknown, returned, and total association counts;
- at most 100 descending immutable association references and explicit truncation;
- content SHA-256, UTF-8 byte count, safe filename, fixed MIME type, and a 256 KiB cap.

No generated timestamp is added, so unchanged source facts reproduce identical bytes.
The export includes no private plan/evidence body or operator identity, infers no
result, grants no approval or authority, rewrites no record, and starts no model,
command, or execution. It is a regenerable download receipt, not a persisted acceptance
decision. TypeScript verifies exact keys, bindings, digests, counts, ordering,
uniqueness, suffix/MIME agreement, and every false-authority field before download.

The same review found that the existing Code Handoff export accepted a whitespace-
padded format because a shared helper normalized the value. Both export handlers now
require one byte-exact format value; tests cover missing, duplicate, and padded forms.

### R6: logical lifecycle timeline and independent deadline budgets

`runner_lifecycle_timeline_evidence.v1` records a canonical sequence of control facts:
start accepted, stop trigger, optional terminate, optional kill, tree reaped, exit
evidence, runtime evidence, and evidence-set commit. Its sequence numbers are logical
ordinals only. Wall-clock timestamps, backend-call timing, and process identity are
structurally absent. The timeline binds the existing Go control trigger and final
wait/terminate/kill mechanism.

`runner_deadline_budget_evidence.v1` records the independent Go context ceilings for
the run, terminate call and post-wait, kill call and post-wait, tree inspection, exit
evidence, and runtime evidence. Applied flags must match the observed path. It explicitly
does not claim one cumulative wall deadline, CPU or memory limits, or verified OS
resource enforcement.

Exit, runtime, configured-limit, termination-cause, timeline, and deadline-budget
records all validate before atomic assignment. Recollection drift fails closed with a
dedicated evidence error and cannot leave a partial replacement. Concrete Windows and
Unix process-tree adapters remain in `_test.go` and start only the current Go test
binary. No CLI, HTTP, Desktop, Agent, Sandbox, approval, profile, LocalRunner, Docker,
or product process-start path imports this harness.

## Verification

- Uncached `go test ./... -count=1`: passed in 387.3 seconds.
- Full `go test -race ./... -count=1`: passed in 395.8 seconds.
- Full `go vet ./...`, secure-Desktop vet, zero-warning ordinary and secure-Desktop
  `staticcheck`, `go mod verify`, and no-drift `go mod tidy`: passed.
- Ordinary and secure-Desktop tests plus both `govulncheck` paths: zero reachable or
  imported-package vulnerabilities.
- Web strict TypeScript, 37 Vitest files / 129 tests, Vite production build, and
  `npm audit --audit-level=high`: passed with zero high vulnerabilities.
- OpenAPI and generated TypeScript were byte-stable. The contract contains 73 paths,
  79 operations, and 172 schemas. SHA-256 values are
  `8FF7E6A39132ED46DA828009D6D7A603D05B862AEA734E4D8C3E13838DD8A8AE` and
  `FE852EEC8B561D14BD5C3FD1411B2F1930C8A80F15551D071DD3356236BA3503`.
- The reproducible unsigned Windows GUI SHA-256 is
  `7aa5c3bf67a0af12e51e396977632e5dcc21c74dc04411d3fec7b6f09719aeef`;
  `release_ready=false`, installer absent, and registry writes false remain correct.
- Mock-only CLI smoke, credential/privacy/product-entry scans, and Linux amd64 Runner
  test-binary cross-compilation passed.

The six-slice robustness review found and fixed the low-risk export-format normalization
regression described above. No known unresolved high or medium issue remains on an
enabled path. No real Provider/key, Shell, LocalRunner, Docker, Git hook, attack traffic,
external network, installer, registry mutation, or product process start was used.

## Consequences

Operators can inspect either available side of a comparison without widening the Git
surface, and can download a reproducible metadata-only record of one frozen verification
item. Runner design reviews can distinguish logical lifecycle ordering and configured
Go context ceilings from timing measurements or kernel-enforced limits.

Candidate next slices are D1-G10 a bounded paired base/head preview workspace, D1-V9 an
independently designed durable snapshot-receipt history, and R7 a canonical digest over
the six-record non-product evidence set. Any persisted acceptance, real Local/Docker
start, xterm input, installer/signing, Rust analyzer, network grant, or CTF workflow
still requires its own authority and release gate.
