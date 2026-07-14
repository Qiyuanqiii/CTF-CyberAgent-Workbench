# CyberAgent Workbench Project Memory

Last updated: 2026-07-15

## Resume First

CyberAgent Workbench is a local-first, resumable, auditable AI Agent workbench. Go is the only control plane. TypeScript consumes Go-owned HTTP/OpenAPI contracts, and future Rust analyzers must run as deterministic JSON tools behind Go. CTF-specific solving remains deferred until the generic runtime, Skills, Sandbox, and analyzer boundaries are stable.

Read in this order after a long context break:

1. `README.md`
2. This file
3. `docs/PROJECT_STATUS.md`
4. `docs/PROGRESS_BOOK.md`
5. `docs/TASK_BOOK.md`
6. `docs/adr/0001-go-control-plane.md`
7. `docs/adr/0002-run-centric-runtime.md`
8. `docs/adr/0003-run-execution-modes.md`
9. `docs/adr/0004-plan-delivery-workflow.md`
10. `docs/adr/0005-operator-steering-queue.md`
11. `docs/adr/0006-operator-steering-controls.md`
12. `docs/adr/0007-specialist-skill-context.md`
13. `docs/adr/0008-sandbox-manifest-boundary.md`
14. `docs/adr/0009-sandbox-approval-candidate.md`
15. `docs/adr/0010-disabled-sandbox-lifecycle.md`
16. `docs/adr/0011-disabled-sandbox-preflight.md`
17. `docs/adr/0012-simulation-only-sandbox-evidence.md`

## Current Baseline

- Architecture completion: about 98%; the V2 run-centric control plane is about 99% complete.
- Product usability: about 45-50% for the complete Code + Cyber product.
- Generic coding-agent workflow usability: about 40%.
- Cyber autonomous-workflow usability: about 20%.
- These are engineering estimates based on tested roadmap slices, not performance benchmarks. Do not reuse the retired single-axis "overall product vision" percentage.
- Database schema: v52.
- `README.md` carries the canonical bilingual schema timeline in strict `v1 -> v52` order. `internal/store/readme_history_test.go` binds its row count and ordering to `LatestSchemaVersion`, so a future migration cannot silently leave the public history missing or out of sequence.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, parent-selected minimal Specialist Skill context, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, safe-boundary operator steering with pending-only cancellation and explicit drain, strict metadata-only Sandbox Manifest preparation, approval-bound disabled candidates, a disabled Artifact-bound Sandbox lifecycle with independent fencing/cancellation/cleanup recovery, a required-but-unverified backend/output preflight, and simulation-only backend evidence plus an atomic in-memory output transaction, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- A Specialist receives at most one parent-selected built-in Skill guide. Assignment text, model output, and external content cannot choose or widen that subset.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local/Docker command execution is disabled.
- `sandbox_manifest.v1`, `sandbox_execution_candidate.v1`, `sandbox_execution.v1`, `sandbox_preflight.v1`, `sandbox_backend_evidence.v1`, and `sandbox_output_simulation.v1` are evidence, never execution permits. Schemas v48-v52 fix backend, execution, start, export, and production Artifact-commit capabilities to false even after exact operator approval.
- Sandbox execution ownership uses a separate generation-fenced lease. The initial lease can only prepare a disabled record; cleanup can recover after Run termination, but stale generations cannot commit.
- Input Artifacts are reverified by exact Run/Session/Workspace, digest, size, MIME, source, stream, order, and a 16 MiB aggregate cap. v50 stores no Artifact body or raw output path.
- The v51 backend handshake is disabled, container identity is unbound, and all 16 threat-model checks remain required/unverified/not-probed. Output slots store only opaque locator fingerprints and cannot export or commit Artifacts.
- The v52 fake client never contacts Docker. Its 16 `simulated_pass` items remain unverified and production-untrusted; the output harness commits only to an in-memory fake and must leave `run_artifacts` unchanged.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v52 adds simulation-only Sandbox backend evidence and output transactions on top of the complete v48-v51 chain. `SimulationBackendClient` has no daemon transport. It binds a canonical Docker OCI image digest plus separate daemon-capability, mount, network, secret, container-configuration, resource, termination, orphan, and output-plan fingerprints to exactly 16 ordered `simulated_pass` items. Every item remains `verified=false`; production verification, backend availability, execution authorization, and Artifact authorization are fixed false.

`sandbox_output_fixture.v1` is strict duplicate-aware bounded UTF-8. `InMemoryOutputHarness` requires exact slot order and kind, aggregate byte limits, MIME detection, redaction, regular files for file slots, and rejection of symlinks and special files. It stages all redacted outputs before one fake atomic commit; injected failure or cancellation leaves zero fake outputs. The Store fixes production Artifact count to zero and never writes `run_artifacts` from this path.

The Application and SQLite independently revalidate the Manifest, all v48-v51 identities, current Scope/Policy/approval, mounts, cumulative model/tool budgets, Run and Sandbox leases, input Artifact content, and output-plan binding at both new boundaries. Root/item/operation rows are immutable, same-intent retries converge across Stores, and changed intent conflicts. Events, CLI, and persistence omit fixture bodies, raw locators and host paths, commands, Manifest content, secrets, container IDs, operation digests, and private leases.

Focused protocol, Application, Store, migration, concurrency, cancellation, rollback, event-privacy, and CLI tests pass. The uncached full ordinary suite completed in 120.7s and the full race suite in 155.7s. `go vet`, zero-warning `staticcheck`, module verification/tidy diff, zero-finding `govulncheck`, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, credential/runtime/Runner/Markdown-link scans, diff checks, and an isolated real-binary full simulation smoke also pass. The smoke created zero Docker processes and zero production Artifacts. No unresolved high/medium issue is known; GitHub Actions results must be recorded after the feature commit. No real Provider, Shell, Local, Docker, network, production Artifact, or CTF execution has been enabled.

## Next Slice

Continue P6 without enabling process execution:

1. Add a schema-v53 production-observation protocol and a read-only Docker capability-inspector boundary; container create/start APIs must remain unreachable.
2. Keep production observations distinct from v52 simulation rows and bind daemon identity/API capabilities, image inspection, rootless/private-mount support, and platform limits to independently reviewable evidence.
3. Add deterministic fake transport tests plus a clear unavailable-daemon result. Any real-daemon integration gate must be opt-in and must not create a container.
4. Revalidate the Manifest and all v48-v52 bindings at every future boundary; no preparation, approval, candidate, lifecycle, preflight, simulated evidence, or fake output record is authorization by itself.
5. Keep HTTP/React mutation, Rust analyzers, real Local/Docker execution, production Artifact export, and CTF solving deferred until their dedicated audits pass.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so startup correctly fails closed with `migration 30 checksum or name mismatch`. The v52 slice did not modify migrations 1-51, and fresh/upgrade fixtures plus isolated `CYBERAGENT_HOME` runs pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
