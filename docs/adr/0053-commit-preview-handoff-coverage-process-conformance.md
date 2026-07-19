# ADR 0053: Exact Commit Preview, Handoff Coverage, And Process Conformance

- Status: Accepted
- Date: 2026-07-20
- Schema: unchanged at v82
- Slices: D1-G5, D1-V4, R2

## Context

The previous batch could describe an exact commit but could not safely inspect one
changed text file. Verification coverage existed as a separate read projection but
was absent from portable Code Handoff output. The Runner lifecycle contract had only
deterministic simulation evidence, leaving OS process-tree behavior unverified.

These gaps must close without adding a raw Git blob endpoint, inferring an aggregate
verification result, or enabling product host/container execution.

## Decision

### D1-G5: redacted exact-commit file preview

`repository_commit_file_preview.v1` accepts one exact lowercase 40-character commit
object ID and one canonical relative path at the exact registered Workspace root.
It reads only regular or executable UTF-8 text, refuses links, binary data, missing
objects, and input above 64 KiB, then secret-redacts before returning at most 128 KiB.
The response includes projected-content SHA-256 and non-authorizing provenance.

The protocol exposes no raw blob, host root, Git remote, checkout, ref update,
subprocess, network access, or hook execution. The strict TypeScript client repeats
identity, byte-count, SHA-256, and every false-authority check before rendering.

### D1-V4: verification coverage in Code Handoff

`code_handoff.v1` and its Markdown/JSON exports now include bounded
`operator_verification_plan_coverage.v1` metadata. At most 100 flat plan-item
references carry plan/item digests and explicit pass/fail/unknown counts. Private plan
titles, expected observations, evidence summaries, and message bodies are omitted.

Contradictory pass and fail observations remain visible as a count. No aggregate pass,
completion, report acceptance, resume, mutation, or execution fact is inferred.
Go validates plan objects, exact Run/Session/Workspace bindings, positive aggregate
rows, duplicate plans/counts, digest syntax, bounded totals, and event sequences.
TypeScript independently validates the same totals and authority fields.

### R2: test-only platform process-tree conformance

The lifecycle backend marker is renamed from `SimulationOnly` to `NonProductOnly` so
the contract can admit deterministic simulations and OS conformance adapters without
misrepresenting either as a product Runner.

Windows tests create a private Job Object with kill-on-close. Unix tests create a
private process group. Each test starts only the current Go test binary and validates:

1. timeout followed by cooperative tree termination;
2. ignored termination followed by forced tree kill;
3. parent exit followed by orphan-child detection and cleanup.

All concrete adapters and `os/exec` calls are in `_test.go`. No CLI, HTTP, Desktop,
Agent, Sandbox, LocalRunner, Docker, approval, profile, or capability path imports or
constructs them. Cleanup targets only the private Job Object/process group created by
the current test.

## Verification

- Uncached `go test -count=1 ./...`: passed in 380 seconds.
- `go test -race -count=1 ./...`: passed in 411.2 seconds.
- Post-audit focused ordinary and race regressions: passed.
- Windows Job Object conformance: all three lifecycle cases passed; Linux test binary
  cross-compilation passed.
- `go vet`, zero-warning `staticcheck`, module verify/tidy diff: passed.
- `govulncheck`: zero reachable vulnerabilities; one required-module advisory remains
  unimported/uncalled.
- Web strict TypeScript, 37 Vitest files / 127 tests, and Vite build: passed.
- OpenAPI/TypeScript regeneration was byte-deterministic. OpenAPI is 69 paths,
  75 operations, and 167 schemas.
- `npm audit --audit-level=high`: zero vulnerabilities.
- Secure Desktop tests/vet/staticcheck/govulncheck and reproducible Windows build:
  passed. The unsigned binary SHA-256 is
  `44d54bf9d50b7cd99b89f5089833823ce0337bb0e0158ec16ef6aa9a5b415614`;
  `release_ready=false`, installer absent, and registry writes false remain correct.
- Isolated mock-only CLI version/provider/Workspace smoke: passed.
- Credential, tracked secret-file, runtime-artifact, diff, and product process-entry
  scans: passed. User test keys are absent from the repository.

The audit fixed platform-width integer addition, negative/zero aggregate event facts,
and duplicate plan/count acceptance at the narrow Store boundary. No unresolved high
or medium issue is known on an enabled path.

## Consequences

Exact commit text can now be inspected safely and verification coverage survives a
handoff, while real OS lifecycle behavior has evidence on Windows and compile coverage
for Unix. None of this authorizes end-user process execution.

The next batch should add bounded exact-file history, a read-only verification
coverage drill-down, and an R3 output/exit-evidence contract. Product Local/Docker
start, xterm input, installers/signing, Rust analyzers, network grants, and CTF solving
remain independently gated.
