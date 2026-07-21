# ADR 0065: Non-Starting Analyzer Invocation Bridge

- Status: Accepted
- Date: 2026-07-21
- Scope: non-schema Go analyzer candidate, disabled/fake transport, and failure contract

## Context

ADR 0062 fixed the Go-owned request/result/error protocol and a deterministic Rust
development fixture. ADR 0063 added an inert descriptor Registry and a bounded ZIP
central-directory analyzer. Nothing yet bound one request to one exact descriptor, and
there was no Go-owned place to define timeout, stdout, exit, cancellation, or invalid-result
behavior before reviewing a real subprocess adapter.

Connecting the Rust binary directly to Run, Store, SQLite, or Artifact code would combine
protocol validation, process lifecycle, persistence, and product authority in one change.
The safer intermediate step is a restart-verifiable invocation record plus transports that
provably cannot start a process.

## Decision

### P10-C1: canonical non-starting candidate

`analyzer_invocation.v1` embeds the exact clone-isolated descriptor from the fixed Registry
and binds these facts:

- descriptor canonical SHA-256;
- canonical request SHA-256;
- decoded inline-input SHA-256 and byte count;
- request ID, analyzer, media type, and exact limits;
- all request capabilities false; and
- product invocation, process start, file input, result persistence, and Artifact commit
  authority false.

The candidate retains neither source bytes nor Base64 and has no executable, command, path,
URL, environment, stderr, Run, Session, or persistence field. Its strict decoder rejects
unknown, duplicate, missing, malformed, future-version, and widened-authority fields. Pure
validation reconstructs the expected candidate from the fixed Registry and original request,
so validation does not depend on an in-memory registration or process state.

### P10-C2: sealed disabled/fake bridge

The `Transport` interface has an unexported marker and unexported exchange method. Outside
packages cannot implement it. This release admits only:

- `DisabledTransport`, which returns a stable disabled outcome without calling exchange; and
- `FakeTransport`, a constructor-validated deterministic script that copies bounded stdout,
  supports a bounded delay or crash, honors context cancellation, and starts nothing.

The bridge revalidates the complete candidate, canonicalizes request stdin, applies the
declared 100-30000 ms deadline, and checks request, descriptor, and global stdout limits.
Exit 0 must be an exact descriptor-selected fixture or archive result. Exit 2 must be a strict
analyzer rejection from the small runtime-rejection set. In addition to protocol validation,
both deterministic outputs must equal Go's recomputation from the current canonical request,
preventing valid success or error output from being replayed across different inputs. Exit 3
must be the strict internal-error envelope. Every other exit is a process failure.

`analyzer_invocation_outcome.v1` contains only the candidate digest, IDs, transport/status,
failure and analyzer error codes, exit code, stdout byte count/SHA-256, result protocol, limit
facts, and explicit false raw-output/product-invocation claims. Raw stdout is transient and
never retained in the outcome. Outcome decoding independently checks every false field and
recomputes whether the byte count is within the exact limits.

### P10-C3: failure and restart vectors

`invocation_failure_vectors.json` fixes eight cases: crash, in-flight cancellation, deadline,
malformed result JSON, future result protocol, wrong analyzer, oversized stdout, and unknown
exit code. Tests serialize and decode the candidate as if after restart, construct a new
bridge, execute each vector twice, and require byte-identical validated outcomes. Separate
candidate and outcome fuzz targets require every accepted envelope to decode, re-encode,
and decode idempotently.

## Security And Authority

`internal/analyzer` imports only the Go standard library. It imports no product Runner,
Store, domain Run, Artifact, Sandbox, Tool Gateway, HTTP, Desktop, or application package.
Neither admitted transport can call `os/exec`, open a file, use a network, inspect an
environment, or persist a result. There is no CLI, HTTP, OpenAPI, Desktop, Agent, Skill,
Run/Event, SQLite, or Artifact wiring.

This bridge is not evidence that a product subprocess is safe. A future real adapter still
requires an exact executable identity, bounded streaming stdout/stderr, process-tree
cancellation, TERM/KILL escalation, orphan detection and reap, privacy review, and atomic
Artifact-candidate semantics. Those requirements remain a separate release gate.

## Verification And Review

The cumulative six-slice gate passed:

- final uncached full Go and race suites in 418.5 and 459.2 seconds;
- twenty additional final-code analyzer race repetitions;
- about 4.65 million candidate, outcome, and protocol fuzz executions during this batch;
- full vet, zero-warning staticcheck, module verification/tidy with no drift, and ordinary
  plus secure-Desktop govulncheck with zero reachable vulnerabilities;
- seven Rust unit and two shared-vector tests, fmt, zero-warning clippy, and RustSec scanning
  42 locked crates against 1,166 advisories with no known vulnerability;
- deterministic OpenAPI regeneration, strict TypeScript, 137 tests across 38 Web files,
  Vite production build, and zero-vulnerability npm audit; and
- secure Desktop tests/vet plus a reproducible Windows dual build. The unsigned executable
  SHA-256 is `82a5f7b4f012c0bc39da13d3b00cc98831e8002653a4a59f54d58f63e7126b50` and
  `release_ready=false`.

Review fixed four validation/integrity issues before the final gate: explicit precedence around the
known-exit set, exact consistency between deserialized stdout byte counts and limit claims,
disabled-transport precedence over an already-cancelled caller context, and rejection of
cross-input archive success/error replay under one request ID. No unresolved
high- or medium-severity issue is known on an enabled path.

The first `npm ci` attempt failed with Windows `EPERM` because the previous Vite preview held
its native Rolldown module open. Only that repository preview PID was stopped; the Codex host
process was not touched. The clean install and complete Web gate then passed. Exact user test
key prefixes and forbidden project-package/process-entry scans returned no match.

No real Provider/key, Shell, LocalRunner, Docker, hook, attack traffic, product analyzer
process, installer, registry mutation, SQLite/Event/Artifact write, or new product authority
was used. Schema/OpenAPI remain v84 and 75 paths / 83 operations / 182 schemas.

## Consequences

- Timeout, cancellation, output-limit, exit, and result-validation semantics are fixed before
  any real process code exists.
- A candidate or outcome can be reconstructed and audited after restart without retaining
  input or output bodies.
- Architecture completion remains about 99%, product usability 95-97%, generic Coding Agent
  usability 95-96%, and Cyber automation about 20%; this internal bridge does not itself add
  an end-user workflow.
- Candidate next slices are P10-D1 executable-identity/preflight metadata without startup,
  P10-D2 a test-only non-product Rust subprocess conformance adapter, and P10-D3 real-fixture
  crash/timeout/cancel/tree-reap/stderr-bound vectors. Product invocation and persistence
  remain closed after that harness unless separately approved and audited.
