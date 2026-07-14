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

## Current Baseline

- Architecture completion: about 98%; the V2 run-centric control plane is about 99% complete.
- Product usability: about 45-50% for the complete Code + Cyber product.
- Generic coding-agent workflow usability: about 40%.
- Cyber autonomous-workflow usability: about 20%.
- These are engineering estimates based on tested roadmap slices, not performance benchmarks. Do not reuse the retired single-axis "overall product vision" percentage.
- Database schema: v50.
- `README.md` carries the canonical bilingual schema timeline in strict `v1 -> v50` order. `internal/store/readme_history_test.go` binds its row count and ordering to `LatestSchemaVersion`, so a future migration cannot silently leave the public history missing or out of sequence.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, parent-selected minimal Specialist Skill context, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, safe-boundary operator steering with pending-only cancellation and explicit drain, strict metadata-only Sandbox Manifest preparation, approval-bound disabled candidates, and a disabled Artifact-bound Sandbox lifecycle with independent fencing/cancellation/cleanup recovery, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- A Specialist receives at most one parent-selected built-in Skill guide. Assignment text, model output, and external content cannot choose or widen that subset.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local/Docker command execution is disabled.
- `sandbox_manifest.v1`, `sandbox_execution_candidate.v1`, and `sandbox_execution.v1` are validation/lifecycle facts, never execution permits. Schemas v48-v50 fix `backend_enabled`, `execution_authorized`, and `backend_started` to false even after exact operator approval.
- Sandbox execution ownership uses a separate generation-fenced lease. The initial lease can only prepare a disabled record; cleanup can recover after Run termination, but stale generations cannot commit.
- Input Artifacts are reverified by exact Run/Session/Workspace, digest, size, MIME, source, stream, order, and a 16 MiB aggregate cap. v50 stores no Artifact body or raw output path.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v50 adds a disabled Sandbox lifecycle on top of the v48 Manifest and v49 approval/candidate facts. `run sandbox begin` must resupply the complete Manifest and revalidate the candidate, non-terminal Run, Mission, Workspace root, Scope, current Policy, exact approval, `os.Root` mount binding, aggregate model/tool budgets, and Run execution-lease quiescence. It then creates an immutable `sandbox_execution.v1` root whose backend, authorization, and started flags are all false.

Every Manifest input Artifact is loaded and hash-verified, then pinned by exact Run/Session/Workspace, ordinal, ID, SHA-256, byte size, MIME, stream, source, and redaction state. Aggregate input is capped at 16 MiB. The output capture plan stores only stdout/stderr flags, output-path count, byte ceiling, and a fingerprint; it never stores output paths. SQLite triggers independently bind the v49 candidate, v48 preparation counts, current usage, quiescent Run, input rows, and initial lease.

Sandbox ownership now has an independent generation-fenced lease. The generation-one preparation lease is released without starting a backend. Later cleanup can acquire a successor even after the Run is terminal; expired-worker takeover increments the generation, and stale workers cannot release or commit. Cancellation and cleanup facts plus their digest-only operations are immutable and replayable. The only cleanup result is `backend_disabled`: no backend started, no orphan existed or was reaped, inputs were reverified, output Artifact count is zero, and cleanup is complete.

Focused protocol, Application, Store, migration, fencing, cross-Run Artifact, terminal cleanup, immutability, event-privacy, and CLI tests pass. The uncached full Go suite, full-repository race suite, `go vet`, zero-warning `staticcheck`, module verification/tidy diff, `govulncheck`, frontend tests/typecheck/OpenAPI drift check/production build, npm audit, credential/runtime-artifact/Runner-call scans, diff checks, and an isolated real-binary lifecycle smoke also pass. GitHub Actions run `29353239789` passed commit `ff4846a` with Go in 2m6s and TypeScript in 25s. No unresolved high- or medium-severity issue was found, and no real Provider, Shell, Local, Docker, network, or CTF execution was enabled.

## Next Slice

Continue P6 without enabling process execution:

1. Threat-model Docker mount propagation, network default-deny, secret materialization, container identity, timeout, kill, and orphan cleanup before importing the Docker client into the application path.
2. Define the real bounded stdout/stderr and output-file Artifact capture transaction, including truncation, MIME, symlink/special-file rejection, partial failure, and restart behavior; do not store raw host paths.
3. Add a backend capability handshake and container identity record while keeping `backend_enabled=false` until a separate operator-visible audit accepts every invariant.
4. Revalidate the Manifest and all v48-v50 bindings at any future execution boundary; no preparation, approval, candidate, or disabled lifecycle record is authorization by itself.
5. Keep HTTP/React mutation, Rust analyzers, real Local/Docker execution, and CTF solving deferred until their dedicated audits pass.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so startup correctly fails closed with `migration 30 checksum or name mismatch`. The v50 slice did not modify migrations 1-49, and fresh/upgrade fixtures plus isolated `CYBERAGENT_HOME` runs pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
