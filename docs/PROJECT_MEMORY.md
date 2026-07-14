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

## Current Baseline

- Overall product vision: about 97%.
- General Agent MVP: about 99%.
- V2 run-centric runtime: about 99%.
- Database schema: v48.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, parent-selected minimal Specialist Skill context, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, safe-boundary operator steering with pending-only cancellation and explicit drain, a strict metadata-only Sandbox Manifest preparation boundary, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- A Specialist receives at most one parent-selected built-in Skill guide. Assignment text, model output, and external content cannot choose or widen that subset.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local/Docker command execution is disabled.
- `sandbox_manifest.v1` is descriptive input, never an execution permit. Schema v48 fixes both `backend_enabled` and `execution_authorized` to false even when an exact approval is approved.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v48 adds the Go-owned `sandbox_manifest.v1` preparation boundary. The strict JSON protocol hard-bounds backend description, executable/ordered argv, workspace-relative mounts, sandbox working directory, exact network targets, CPU/memory/PID/output resources, literal or secret-reference environment bindings, input Artifact identities, output capture/paths, timeout, and cancellation grace. It rejects unknown or duplicate fields, trailing data, invalid UTF-8, traversal, overlapping mounts, wildcard targets, non-canonical CIDRs, literal secret variables, credential-shaped argv, and values outside protocol limits.

Go normalizes the Manifest and binds its fingerprint to one non-terminal Run, Mission, persisted Workspace root, normalized Mission Scope, Policy decision, optional exact approval, requester, and generated cancellation identity. Manifest network access can only narrow the Mission allowlist. Docker/Local intent, write mounts, network, or secret references require approval if Policy permits them; permanent denial cannot be overridden. SQLite stores immutable preparation, validation, and digest-only operation rows plus metadata-only events. It stores no executable, argv, path, environment value, secret reference, network target, or Manifest JSON. Same-key replay and two-store concurrency converge; changed intent conflicts.

This slice does not execute anything. `NoopRunner` is the only validator used by the service. Local and Docker validation/run paths fail closed, and Go types, SQLite CHECK/trigger rules, CLI, and events all hold `backend_enabled=false` and `execution_authorized=false`, including when an exact approval is approved. Focused tests cover strict parsing, credential rejection, scope widening, Policy denial persistence without raw intent, exact approval binding, immutable tables, schema v47 upgrade, SQL-level write-capability approval enforcement, no workspace side effects, and cross-store concurrency. Uncached full tests, full-repository race detection, vet, staticcheck, module verification/tidy diff, govulncheck, credential/runtime scans, OpenAPI/TypeScript drift, 17 frontend tests, production build, npm audit, isolated Manifest validation, and an isolated real schema-v48 Run/preparation/event smoke all pass. No unresolved high- or medium-severity issue is known.

## Next Slice

Continue P6 with an exact Sandbox approval-request and revalidation bridge, still without enabling process execution:

1. Create a first-class approval request from one preparation's `authorization_fingerprint`, with no raw Manifest content in the approval ledger or events.
2. Require callers to resupply the Manifest for every later step; normalize it again and require the exact stored fingerprint, Run, Workspace root, Scope, Policy, and approval binding.
3. Resolve mount sources beneath the persisted Workspace root with symlink-safe Go filesystem APIs and record only bounded identity metadata. Do not mount or launch anything.
4. Add a metadata-only execution-candidate/recovery ledger whose state still fixes `execution_authorized=false`; separately threat-model Docker client, cancellation, cleanup, output Artifact capture, and secret materialization.
5. Do not add Docker process creation until a dedicated audit proves lease fencing, cancellation, cleanup, network default-deny, and host-path isolation. Rust analyzers and CTF solving remain deferred.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so startup correctly fails closed with `migration 30 checksum or name mismatch`. The v47 code did not modify migrations 1-46, and fresh/upgrade fixtures plus isolated `CYBERAGENT_HOME` runs pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
