# ADR 0067: Inert Analyzer Result Staging and Product Adapter Threat Model

## Status

Accepted for P10-E1/E2/E3. This decision does not authorize a product analyzer process,
result persistence, Run/Event writes, or Artifact commit.

## Context

ADR 0065 stopped at a Disabled/Fake invocation bridge. ADR 0066 added executable identity,
preflight metadata, and real Rust subprocess lifecycle evidence only in test compilation
units. The next boundary must prove that a successful result can be bound to the complete
source chain and can survive atomic-staging failure vectors without treating test evidence as
product durability or execution authority.

A future product adapter also needs one explicit default-deny readiness model. Process startup
cannot be inferred from successful protocol parsing, a matching executable pathname, test
process cleanup, or an inert Artifact-shaped record.

## Decision

### P10-E1: validated result and inert Artifact candidate

Go owns `analyzer_validated_result_candidate.v1` and
`analyzer_artifact_candidate.v1`. A candidate can be built only from:

- one valid invocation candidate and canonical request;
- the exact executable identity and invocation preflight reconstructed from caller-supplied
  bytes;
- one product-codec-valid successful invocation outcome; and
- the exact deterministic result bytes whose length and SHA-256 match that outcome.

The result candidate binds canonical SHA-256 values for the invocation candidate, executable
identity, preflight, and outcome. It also binds the result protocol, byte count, and exact
result SHA-256. Validation re-runs the complete chain and deterministic evaluator; whitespace,
request, executable, outcome, protocol, or result drift fails closed.

The nested Artifact candidate is intentionally not `internal/artifact.Descriptor`. It records
only fixed JSON/UTF-8 metadata, result protocol, byte count, and SHA-256. Content, path,
Run/Session/Workspace binding, persistence, Run/Event writes, external publication, and
Artifact commit are all absent or false. The complete source remains
`test_conformance_only=true` because the executable preflight is still the ADR 0066 test gate.

### P10-E2: test-only atomic staging conformance

The staging implementation exists only in `_test.go`. It validates and encodes the E1
candidate, then creates a bounded test envelope containing candidate JSON and the transient
result body under a private `t.TempDir()` directory. It writes a mode-0600 pending file,
flushes that file, and publishes the complete file with a same-volume hard link. Hard-link
creation provides an atomic no-replace final name; the harness never overwrites a pre-existing
final file.

Vectors cover initial publish, exact replay without rewrite, explicit rollback, cancellation,
crash after file sync, crash after atomic publish but before pending cleanup, truncated pending
data, and foreign pending/final collision preservation. Recovery accepts only byte-for-byte
expected pending/final envelopes. Rollback removes only the exact expected pending envelope
or a valid prefix consistent with an interrupted write; unrelated same-name pending content is
preserved and reported as a collision. Receipts expose hashes, sizes, replay/recovery facts,
and fixed false authority flags, never the result body.

This is filesystem conformance evidence, not a product staging implementation. The harness has
no durable write-ahead intent, generation lease, directory fsync proof, Store/SQLite/Run/Event
integration, Artifact writer, OS sandbox, or multi-process producer protocol. Test temporary
files are removed by the test framework.

### P10-E3: product adapter readiness controls

`analyzer_product_adapter_threat_model.v1` fixes the following 20 mandatory controls. Every
control has status `required_unimplemented`, `required=true`, `implemented=false`,
`verified=false`, and `blocks_product_start=true`.

| Control ID | Category | Required product evidence |
| --- | --- | --- |
| `executable_handle_identity` | executable | Execute the same immutable handle that passed identity verification. |
| `executable_format` | executable | Parse and allow only the expected PE or ELF format. |
| `target_architecture` | executable | Prove architecture matches the reviewed target. |
| `provenance_signature` | supply chain | Verify provenance, digest, and platform signing policy. |
| `version_allowlist` | supply chain | Pin a reviewed immutable analyzer version and digest. |
| `least_privilege_identity` | isolation | Use a dedicated non-administrator OS identity. |
| `filesystem_sandbox` | isolation | Restrict filesystem access to reviewed inputs and staging. |
| `network_isolation` | isolation | Enforce no analyzer network or socket access. |
| `environment_scrubbing` | isolation | Supply an explicit environment without inherited secrets. |
| `cpu_limit` | resource | Enforce an OS CPU quota. |
| `memory_limit` | resource | Enforce a hard resident-memory ceiling. |
| `process_count_limit` | resource | Bound descendants and tree expansion. |
| `wall_clock_deadline` | lifecycle | Enforce a monotonic deadline with cleanup reserve. |
| `process_tree_termination` | lifecycle | Terminate and prove complete-tree reap. |
| `bounded_stdio_redaction` | data | Bound streams before allocation and redact evidence. |
| `operator_scope_approval` | authority | Bind explicit approval to analyzer, input, digest, and limits. |
| `atomic_result_handoff` | integrity | Publish validated output atomically without replacement. |
| `durable_intent_recovery` | recovery | Persist intent and generation fencing before startup. |
| `append_only_audit` | audit | Record bounded lifecycle, policy, approval, resource, and result events. |
| `orphan_rollback_reconciliation` | recovery | Reconcile owned residue without deleting foreign resources. |

The top-level model fixes required/open controls at 20, implemented/verified controls at zero,
and all product authority flags to false. `operator_override_allowed=false` means no CLI flag,
UI toggle, environment variable, Provider response, or approval can bypass an open control.

## Security and Authority

Production `internal/analyzer` still contains no `os`, `os/exec`, syscall, platform process API,
Artifact, Store, SQLite, Run, Event, CLI, HTTP, Desktop, Runner, Sandbox, Provider, network, or
secret integration. E1 is a pure protocol projection, E2 exists only in test compilation, and
E3 is a metadata-only blocker list.

The following claims remain prohibited:

- a pathname plus SHA-256 is not TOCTOU-safe executable identity;
- target GOOS/GOARCH metadata is not PE/ELF format or architecture proof;
- descriptor determinism is not arbitrary binary semantic proof;
- test process-tree cleanup is not a product sandbox or resource ceiling;
- test hard-link staging is not durable product recovery; and
- an Artifact-shaped candidate is not a committed Artifact or vulnerability finding.

## Verification

Required tests include strict exact-field round trips; missing, duplicate, unknown, future, and
authority-widened envelope rejection; cross-source/result tamper rejection; candidate fuzzing;
ten-round and race staging repetitions; crash/replay/rollback/collision vectors; ADR/control-ID
alignment; production import-boundary scans; and Linux/Windows CI execution of the test-only
staging gate.

This batch completes the six-slice P10-D/P10-E cycle, so its release gate additionally requires
full ordinary and race Go tests, vet, zero-warning staticcheck, module verification/tidy,
govulncheck, Rust fmt/test/clippy, RustSec, Web tests/strict TypeScript/OpenAPI/Vite/npm audit,
secure Desktop tests, a reproducible Windows dual build, cross-platform compilation, and
credential/runtime-artifact/encoding/link/diff scans.

The final gate passed uncached full ordinary/race Go in 397.6/462.5 seconds, twenty final
staging race repetitions, a ten-round real Rust/process-tree/staging race gate during review,
and about 2.17 million new fuzz executions. Vet/staticcheck/module, 7+2 locked Rust tests,
fmt/clippy/RustSec, 38 files/137 Web tests, strict TypeScript/OpenAPI, Vite/npm, secure Desktop,
Linux Analyzer cross-compilation, repository scans, and a reproducible Windows dual build all
passed. The unsigned GUI SHA-256 is
`10effa0de5f5fc159e43f99aa97f45fc7579e4413b4ec0f3c7051dd4e217dabf` and
`release_ready=false`. `govulncheck` reports no reachable or imported-package issue; the one
module-level residual is GO-2026-5932 in unimported `golang.org/x/crypto/openpgp`, for which
the advisory has no fixed version.

Review fixed one low-risk rollback ownership issue before the final gate. One initial full Go
run also hit a transient Windows GCC/ld failure without a linker diagnostic; the affected
package and both complete final ordinary/race gates subsequently passed. No enabled product
path has a known unresolved high/medium issue.

## Consequences

- Go can now prove which exact deterministic analyzer result an inert Artifact candidate refers
  to without retaining or committing the body.
- Atomic staging and restart vectors have real local-filesystem test evidence, but no product
  storage or durability claim.
- Product startup remains mechanically blocked by 20 open controls and zero override paths.
- Schema/OpenAPI remain v84 and 75 paths / 83 operations / 182 schemas.

Candidate next slices are pure caller-byte PE/ELF format and architecture inspection, a signed
release-manifest/provenance allowlist candidate, and an operator-reviewed resource/sandbox
launch-plan candidate. Those slices must remain non-starting until this threat model is
separately advanced and re-audited.
