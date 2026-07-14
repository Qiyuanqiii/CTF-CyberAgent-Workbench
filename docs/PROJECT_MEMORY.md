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
13. `docs/adr/0008-sandbox-manifest-boundary.md`
14. `docs/adr/0009-sandbox-approval-candidate.md`

## Current Baseline

- Architecture completion: about 97%; the V2 run-centric control plane is about 99% complete.
- Product usability: about 45-50% for the complete Code + Cyber product.
- Generic coding-agent workflow usability: about 40%.
- Cyber autonomous-workflow usability: about 20%.
- These are engineering estimates based on tested roadmap slices, not performance benchmarks. Do not reuse the retired single-axis "overall product vision" percentage.
- Database schema: v49.
- `README.md` now carries the canonical bilingual schema timeline in strict `v1 -> v49` order. `internal/store/readme_history_test.go` binds its row count and ordering to `LatestSchemaVersion`, so a future migration cannot silently leave the public history missing or out of sequence.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, parent-selected minimal Specialist Skill context, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, safe-boundary operator steering with pending-only cancellation and explicit drain, strict metadata-only Sandbox Manifest preparation plus approval-bound disabled candidates, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- A Specialist receives at most one parent-selected built-in Skill guide. Assignment text, model output, and external content cannot choose or widen that subset.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local/Docker command execution is disabled.
- `sandbox_manifest.v1` and `sandbox_execution_candidate.v1` are validation facts, never execution permits. Schemas v48-v49 fix both `backend_enabled` and `execution_authorized` to false even after exact operator approval.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v49 completes the first Sandbox approval and revalidation loop on top of v48. `run sandbox request` derives a pending record in the shared `tool_approvals` ledger from one preparation's exact authorization fingerprint; `review` records an immutable operator approve/deny fact. Policy denial remains final. A candidate must resupply the full Manifest, normalize it again, and exactly rematch the preparation, non-terminal Run, Mission, persisted Workspace root, normalized Scope, current Policy result, and approved per-call record when required.

Mount sources are opened through Go `os.Root`, so symlinks cannot leave the trusted Workspace root; special files are rejected. Candidate creation acquires the Run write lock and rechecks root/Specialist/Fan-out token and model-time totals, tool-call usage, and execution-lease quiescence in the same SQLite transaction. SQL triggers independently enforce those bindings. Immutable candidate/operation rows and metadata-only events store fingerprints, counts, usage snapshots, and status only, never Manifest content, command/argv, paths/root, environment values, secret references, or network targets. Same-key cross-process retries converge and changed intent conflicts.

This slice still executes nothing. Every candidate fixes `backend_enabled=false` and `execution_authorized=false`; Local and Docker remain fail-closed. Focused tests cover pending/approved/denied approval states, Policy and Manifest drift, terminal replay, active lease, exhausted tool budget, symlink escape, mount-source change, immutable SQL rows, v48 upgrade, CLI lifecycle, and two-store convergence. Final uncached full tests, full-repository race detection, vet, staticcheck, module verification/tidy diff, govulncheck, OpenAPI drift, strict TypeScript, 17 Vitest cases, production build, npm audit, credential/runtime scans, and an isolated real schema-v49 CLI lifecycle all pass. No unresolved high- or medium-severity issue is known.

## Next Slice

Continue P6 without enabling process execution:

1. Define an immutable Sandbox lifecycle/cancellation/cleanup protocol and prove lease-fenced ownership and restart recovery with a disabled backend.
2. Validate input Artifact ownership/integrity and design bounded stdout/stderr/output-path Artifact capture without persisting raw host paths.
3. Threat-model Docker mount propagation, network default-deny, secret materialization, container identity, timeout, kill, and orphan cleanup before importing the Docker client into the application path.
4. Revalidate the Manifest, approval, Scope, Policy, budgets, mount sources, and lease again at any future execution boundary; a v49 candidate alone must never be consumed as authorization.
5. Keep HTTP/React write surfaces, Rust analyzers, real Local/Docker execution, and CTF solving deferred until their dedicated audits pass.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so startup correctly fails closed with `migration 30 checksum or name mismatch`. The v47 code did not modify migrations 1-46, and fresh/upgrade fixtures plus isolated `CYBERAGENT_HOME` runs pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
