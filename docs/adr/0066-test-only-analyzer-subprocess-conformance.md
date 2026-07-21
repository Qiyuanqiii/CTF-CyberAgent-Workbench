# ADR 0066: Test-Only Analyzer Subprocess Conformance

- Status: Accepted
- Date: 2026-07-21
- Scope: inert executable identity, test-only Rust invocation, and cross-platform lifecycle evidence

## Context

ADR 0065 fixed invocation, timeout, exit, output-limit, and strict result semantics without
starting a process. Before any product adapter can be considered, Go needs evidence that the
repository Rust fixture obeys that protocol through real operating-system pipes and that the
test harness can cancel and reap a complete process tree. That evidence must not silently
create a CLI, HTTP, Desktop, Run, Store, SQLite, Artifact, Runner, or Sandbox execution path.

An executable pathname alone is not an identity. Conversely, a digest record does not prove
that a PE/ELF image matches the current platform, is signed, is safe to execute, or cannot be
replaced between validation and startup. The design therefore separates inert metadata from
the process-capable test compilation unit and records every unverified claim as false.

## Decision

### P10-D1: inert executable identity and preflight

`analyzer_executable_identity.v1` binds one fixed descriptor to its request/result/error
protocols, target `GOOS/GOARCH`, exact executable byte count, and SHA-256. It contains no raw
bytes, path, command, arguments, environment, working directory, or persistence field.
Descriptor determinism is recorded as a descriptor declaration, not binary proof.
`executable_format_verified`, `executable_semantics_verified`, `process_start_enabled`, and
`product_invocation_enabled` are false.

`analyzer_invocation_preflight.v1` binds the canonical candidate digest to the canonical
executable-identity digest and repeats the exact protocol, platform, byte-count, and SHA-256
facts. Its authority object is all false. It says `test_conformance_only=true` and does not
grant process start. Both envelopes use pointer-complete strict decoders, reject unknown,
duplicate, missing, future, digest-drifted, platform-drifted, or widened fields, and are
reconstructed from the candidate, request, and caller-supplied bytes after restart. Empty
binaries and binaries above 32 MiB fail closed.

### P10-D2: separately compiled Rust subprocess adapter

The only real analyzer process adapter is declared in `_test.go`. Public `NewBridge` still
admits only DisabledTransport and FakeTransport and explicitly rejects this test type. The
test adapter re-reads and byte-compares the fixture immediately before launch, revalidates
both D1 records, uses an absolute test-supplied path, an isolated temporary working directory,
an empty environment, no Rust arguments, a closed canonical JSON stdin stream, and bounded
stdout/stderr collectors.

Stdout retains at most the protocol maximum plus one overflow sentinel for transient strict
classification. Stderr retains at most 4 KiB plus one sentinel inside the test process; only
observed/captured byte counts, a captured-prefix SHA-256, and truncation facts enter test
evidence. No stderr body or process identity enters `analyzer_invocation_outcome.v1`.

The shared classifier accepts an internal transport predicate. Public product validation,
encoding, and decoding continue to recognize only Disabled/Fake; the `_test.go` harness
supplies its fixed conformance label through a test-only predicate, and that label cannot
cross the product outcome codec. No non-test implementation or constructor can supply the
transport, and a source-level regression test rejects process imports from every non-test
analyzer Go file.

### P10-D3: lifecycle, tree, adverse-output, and privacy vectors

The real Rust fixture is exercised for success, deterministic runtime rejection, a blocked-
stdin deadline, cancellation after observed process start, and forced termination. Linux
uses a dedicated process group and sends TERM before KILL. Windows places the process in a
Job Object with `KILL_ON_JOB_CLOSE`; because a hidden process group has no portable SIGTERM,
the common harness records the terminate request, waits 200 ms, then terminates the Job.

A separate Go test executable fixture covers normal descendant reap, a terminate-to-hard-stop
path, parent-exit orphan detection and cleanup, malformed/future/wrong-analyzer stdout, and a
private stderr marker. It exists only in test binaries and is not a product analyzer. Both
Linux and Windows run this gate in GitHub Actions; ordinary tests continue to run all Go
helper/tree/privacy vectors while the real Rust vectors skip unless
`CYBERAGENT_ANALYZER_CONFORMANCE_BINARY` is explicitly set by CI or a developer.

## Security And Authority

Production `internal/analyzer` still imports no `os/exec`, `syscall`, Windows process API,
Runner, Store, Run, Event, SQLite, Artifact, Tool, Sandbox, HTTP, CLI, or Desktop package. The
test adapter cannot be constructed through the product bridge. It does not use a Provider,
LLM, network, user key, external target, LocalRunner, Docker, hook, registry, or installer.

This decision is conformance evidence, not product authorization. In particular:

- byte re-read before a pathname launch does not eliminate a validation/start TOCTOU race;
- target GOOS/GOARCH is recorded, but PE/ELF format, architecture, signature, and provenance
  are explicitly unverified;
- the fixture has no product CPU/memory/network/filesystem sandbox or OS resource ceiling;
- raw stdout/stderr exists transiently inside a bounded test collector; and
- no validated result is persisted or committed as an Artifact.

A future product adapter requires a separate threat model and release gate for executable
selection/handle identity, provenance/signing, resource limits, sandboxing, user control,
atomic result handoff, recovery, and audit persistence.

## Verification And Review

The three-slice functional gate passed:

- final uncached full Go tests in 321.1 seconds;
- five focused analyzer race repetitions plus two 10-second identity/preflight fuzz targets
  in 61.3 seconds, with about 1.89 million fuzz executions;
- Go vet, zero-warning staticcheck, module verification/tidy without drift, Linux test-
  binary cross-compilation, and a zero-finding Analyzer govulncheck;
- seven Rust unit tests plus two shared-vector tests, locked dependencies, fmt, and clippy;
- 38 Web test files / 137 tests, deterministic OpenAPI TypeScript generation, strict build,
  Vite production build, and zero-vulnerability npm audit;
- secure Desktop tests/vet and a reproducible Windows dual build. The unsigned GUI SHA-256
  is `649c7107fdc6e8bad3b718e705d7ce9a5003ea7891c649606695286adf61bf93` and
  `release_ready=false`; and
- exact key-prefix, production analyzer process-entry, diff, and repository hygiene scans.

Combined review fixed five integrity/test-harness defects before the final gate. The first
draft labeled descriptor determinism as if it certified arbitrary executable behavior; the
record now separates `descriptor_deterministic=true` from
`executable_semantics_verified=false`. Copying an empty
environment through a nil slice accidentally selected `exec.Cmd` parent-environment
inheritance; the harness now always supplies a non-nil isolated slice. Race-instrumented Go
helper cold starts could be confused with the dedicated deadline vector, and a one-consumer
wait channel made cleanup ownership fragile after an earlier waiter. Dedicated timeout tests
remain at 100 ms; adverse-output helpers use a 5-second startup allowance, and tree cleanup
now observes one durable completion signal plus a locked wait result. No unresolved high- or
medium-severity issue is known on an enabled product path. Finally, the first draft allowed
the fixed test transport label through public outcome validation; product validation and the
codec now reject it while the separately compiled harness retains ephemeral classifier
coverage.

This is the first three-slice batch in a new six-slice cycle, so it does not claim a new full-
repository race/govulncheck/RustSec robustness gate. That complete gate remains due after the
next three slices.

## Consequences

- Go and Rust protocol compatibility now has real pipe/process evidence on Linux and Windows.
- Product execution authority and all user-facing analyzer surfaces remain unchanged.
- Schema/OpenAPI remain v84 and 75 paths / 83 operations / 182 schemas.
- Architecture completion remains about 99%, complete product usability 95-97%, generic
  Coding Agent usability 95-96%, and Cyber automation about 20%.
- The next batch should add an inert validated-result/Artifact candidate, test-only atomic
  handoff and crash-recovery vectors, and a product-adapter threat model for executable
  identity/TOCTOU/signing/resource ceilings. It must not enable product startup by implication.
