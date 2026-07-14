# CyberAgent Workbench Project Memory

Last updated: 2026-07-14

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

## Current Baseline

- Overall product vision: about 97%.
- General Agent MVP: about 99%.
- V2 run-centric runtime: about 99%.
- Database schema: v47.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, parent-selected minimal Specialist Skill context, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, safe-boundary operator steering with pending-only cancellation and explicit drain, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- A Specialist receives at most one parent-selected built-in Skill guide. Assignment text, model output, and external content cannot choose or widen that subset.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local/Docker command execution is disabled.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v47 adds `specialist_skill_context.v1`. Go derives at most one guide for each active Specialist Attempt from the parent Run's immutable Skill selection and current Run mode. Code uses only the matching Profile guide; Cyber gets an empty set except for the narrow Script Profile. `plan-delivery` is root-only. Child assignment state is fingerprinted for provenance but cannot choose or widen Skills, and the child must already hold delegated `model.chat`.

The exact body is reconstructed from the embedded versioned Registry, revalidated, redacted, and delivered only in the in-memory Provider request under a separate 1,024-token default budget and 2,048-token hard limit. SQLite schema v47 stores one immutable metadata-only preparation per AgentAttempt and one commit bound atomically to the first durable Specialist model start. Events contain aggregate counts, budget, mode, and Agent identity but no Skill body, path, name, version, or content hash. Runs without a historical Skill selection preserve their previous no-Skill child behavior; an in-flight selected Attempt without provenance fails closed and must recover as a fresh Attempt.

The audit found no unresolved high- or medium-severity issue. It added direct proof that an unprepared selected child cannot start a model call, a later event failure rolls back both model-call and Skill-commit state, concurrent preparation converges to one row, both tables are immutable, event order is `prepared -> committed -> model.started`, and the schema/events remain metadata-only. The real-binary smoke also found and fixed stale CLI text that still claimed context was root-only. Full ordinary and race tests, vet, staticcheck, module verification, Go/npm vulnerability scans, API generation, 17 frontend tests, production build, credential/runtime scans, and isolated schema-v47 smoke pass. GitHub Actions run `29321708904` passed release commit `d7e269b`: Go control plane completed in 1m51s and TypeScript console in 20s.

## Next Slice

Start P6 with a Go-owned Sandbox Manifest and immutable execution-intent boundary, without enabling process execution:

1. Define versioned Manifest, Mount, NetworkScope, ResourceLimit, command/argv, input/output, timeout, and cancellation identities with hard bounds.
2. Separate descriptive intent from authorization. Only Go can bind a manifest to Run, Workspace, Policy, approval, and an execution backend.
3. Persist metadata-only preparation and validation facts with idempotent recovery; keep command content and credentials out of events.
4. Make Noop validation deterministic and keep Local/Docker runners fail-closed. Do not start host or container processes in this slice.
5. Use this envelope as the later TS -> Go -> Docker and Go -> Rust JSON boundary; Rust analyzers and CTF solving remain deferred.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so startup correctly fails closed with `migration 30 checksum or name mismatch`. The v47 code did not modify migrations 1-46, and fresh/upgrade fixtures plus isolated `CYBERAGENT_HOME` runs pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
