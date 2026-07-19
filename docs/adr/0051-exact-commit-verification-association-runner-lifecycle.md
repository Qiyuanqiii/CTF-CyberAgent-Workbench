# ADR 0051: Exact Commit Detail, Verification Association, And Runner Lifecycle

Date: 2026-07-20

Status: Accepted

## Context

The Code workbench could show bounded local history but could not explain the changed-file metadata for one exact commit. Verification plans and operator evidence were deliberately separate, but there was no immutable operator action linking one observation to one planned check. The runtime also needed a process-lifecycle contract that could detect cancellation, timeout, and orphan failures before any real Local or Docker execution was exposed.

These additions must preserve Go as the sole control plane. Git objects are untrusted local evidence, an evidence association is not an inferred aggregate result, and a lifecycle contract is not permission to start a host or container process.

## Decision

1. `repository_commit_detail.v1` accepts only an exact lowercase 40-character SHA-1 object ID at the registered Workspace root. A bounded pure-Go walk compares the commit tree with its first parent and returns at most 200 canonical changed paths plus added, modified, deleted, content-change, and mode-change metadata. It reads no author, email, commit body, blob body, remote, host root, checkout, process, network, or hook data. Redirected Git metadata, linked entries, malformed trees, and missing objects fail closed.
2. Schema v81 adds immutable `operator_verification_plan_evidence_association.v1` facts. One evidence record may be linked to exactly one earlier plan item; one item may retain multiple explicit and even contradictory observations. Go, the transactional Store, and SQLite triggers bind the exact Code Run, active Session, Workspace, plan, item, evidence, operation, event, and digest. Command execution, model assertion, result inference, approval, and authority remain false.
3. `operator_verification_plan_coverage.v1` is a bounded read projection of explicit associations. It reports per-item `pass|fail|unknown` counts and unobserved state. It never computes an overall pass, rewrites evidence, or promotes a plan to an execution fact.
4. `runner_lifecycle_contract.v1` defines start, wait, cancel, timeout, TERM grace, KILL grace, final inspection, and orphan cleanup around a `SimulationOnly` backend. It enters the shared wait graph before start, rejects pre-cancelled or cyclic requests, treats partial start as a started process that requires cleanup, and returns stable timeout, cancellation, start, wait, invalid-identity, and orphan errors.
5. The Runner lifecycle package has no CLI, HTTP, Desktop, Agent, LocalRunner, Docker, `os/exec`, or product capability wiring. A concrete OS backend and a product authorization mapping require separate release gates.

## Consequences

- Repository history can open exact changed-file metadata without exposing commit identity text or file contents.
- Operators can state which observation answers which planned check while preserving conflicting evidence and avoiding aggregate result inference.
- Future Local and Docker adapters have one testable lifecycle contract for process-tree cleanup and wait-graph integration.
- Exact commit blob previews, side-parent merge traversal, automatic verification execution, association reassignment, real process launch, terminal streaming, and product Runner enablement remain deferred.

## Verification

The cumulative six-slice gate passed the final uncached full Go suite in 509 seconds and the full race suite in 341 seconds, ordinary and secure-Desktop tests/vet, zero-warning staticcheck, module verification/tidy, and zero reachable govulncheck findings. The module graph retains only the unimported and uncalled transitive `GO-2026-5932` openpgp advisory.

All 127 React tests across 37 files, strict TypeScript, deterministic OpenAPI generation, the Vite production build, zero-vulnerability npm audit, isolated mock-only CLI smoke, privacy/artifact checks, and the reproducible Windows build passed. OpenAPI has 68 paths, 74 operations, and 163 schemas. The unsigned GUI SHA-256 is `77fb4d6fede1c1e3a0c3f3e9d39581e28f7a6880e0e25b222dcf0d3c701d1213` and remains `release_ready=false`.

Chrome-extension verification against the Go-hosted production bundle recorded a plan, a pass observation, and their explicit association, then recovered `1/1 observed` and `1 linked` after reload. Desktop and 390x844 mobile layouts had no page-level horizontal overflow or console warning/error. The audit replaced a Git tree walker that could silently omit missing subtree objects, fixed v81 downgrade-fixture trigger ordering, saturated redaction counters, constrained the new control operation in OpenAPI tests, and ensured partial/invalid Runner starts are cleaned. No unresolved high or medium issue is known on an enabled path.
