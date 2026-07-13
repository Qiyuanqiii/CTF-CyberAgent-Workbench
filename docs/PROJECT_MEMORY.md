# CyberAgent Workbench Project Memory

Last updated: 2026-07-13

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

## Current Baseline

- Overall product vision: about 97%.
- General Agent MVP: about 99%.
- V2 run-centric runtime: about 99%.
- Database schema: v41.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- Shell and ScriptProcess approval paths are dry-run only. Real Local/Docker command execution is disabled.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v41 establishes Go-owned `run_mode.v1` before the Plan/Delivery Skill itself. Every Run receives an append-only mode snapshot with two orthogonal axes: execution surface `code|cyber` and execution phase `plan|deliver`. Scope, Profile, policy version, and surface are immutable for the lifetime of the Run. Phase changes require an explicit operator operation key and are accepted only while the Run is `created` or `paused` with no active execution lease. Legacy v40 rows and omitted create fields backfill/default to `code/deliver`.

Run creation, script-process Run creation, and legacy Task adaptation now persist the initial snapshot and `run.mode_selected` event in their existing transaction. Phase replay is digest-keyed and converges across Store connections; changed intent conflicts, transition records are immutable, and events expose metadata rather than scope targets or raw keys. The Store recomputes operation fingerprints and independently checks lifecycle and active leases, so application-only validation cannot be bypassed.

RunSupervisor loads the authoritative snapshot inside its turn transaction and places a Go-generated, bounded mode contract before selected Skill guidance. Plan phase permits reasoning and the existing create-only WorkItem/Note tools, but rejects both model and operator completion. A model `finish` uses the existing one-repair path and must converge to `continue` or `wait`. Persistence rechecks the current phase during finalization to close a mode-change time-of-check/time-of-use window. Code/Cyber changes require a new Run; mode is never a capability grant.

CLI adds `run create --surface/--phase`, `run mode`, and replay-safe `run phase`. Run list/show, Run-first TUI, HTTP/OpenAPI, generated TypeScript DTOs, and the React Run overview project the same revision. This slice intentionally adds no HTTP mutation, model-selected mode, Shell, network, file-write, Sandbox execution, or extra child authority.

The slice audit found no unresolved high- or medium-severity issue. Hardening during review replaced migration IDs derived from unbounded legacy Run strings, made lease checks use Store/SQLite time rather than caller timestamps, blocked phase changes while an execution lease is active, enforced Plan completion denial in both Supervisor completion transactions and a SQLite trigger, rejected unredacted mode metadata at the Store boundary, recomputed operation fingerprints at persistence, included the complete mode tuple in script-process idempotency, and clamped phase-transition time against clock rollback.

Release verification passed full `go test ./...`, full `go test -race ./...`, `go vet`, clean `staticcheck`, module verification/tidy diff, zero-finding `govulncheck`, deterministic OpenAPI-to-TypeScript regeneration, strict TypeScript, 15 Vitest cases, Vite production build, zero-finding npm audit, credential-pattern scanning, and an isolated real-binary `cyber/plan -> deliver -> step -> TUI` smoke. No real Provider key, host command, network target, or Sandbox execution was used.

## Next Slice

Build the fourth P7 Skills vertical slice on schema v41 as an explicit, bounded Plan/Delivery workflow inspired by strong coding-agent product patterns without copying proprietary implementation:

1. Add one embedded cross-Profile planning Skill with three concise operator-selectable directions and a strict bounded plan document.
2. Persist operator choice and project accepted modules into existing WorkItems and durable handoff Notes rather than creating a second task engine.
3. Advance one slice at a time in `deliver`; require focused verification, diff/security audit, and compact handoff state after each slice.
4. Require broader functional and robustness gates after each larger module before allowing completion.
5. Keep Go authoritative for workflow state, phase transitions, budgets, and completion. Skill text remains guidance, and Specialist Skill minimization follows as a separate audited slice.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
