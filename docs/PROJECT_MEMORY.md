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

## Current Baseline

- Overall product vision: about 97%.
- General Agent MVP: about 99%.
- V2 run-centric runtime: about 99%.
- Database schema: v38.
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

The cross-surface lifecycle/pagination golden now uses real schema v38 SQLite state to compare CLI, TUI, authenticated HTTP/Web, and Headless for running, paused, completed, failed, and cancelled Runs. It pins complete contiguous event sequences/tails, Agent count, terminal reason, Headless exit 0/4/7, TUI latest-50 truncation over 53 Runs/Sessions, HTTP 20/20/13 opaque-cursor pages, a valid empty collection, and a zero-event resume from the durable tail. TUI exposes a defensive-copy picker projection for testing; it adds no capability.

Verified gates for this slice: uncached `go test ./...`, full `go test -race ./...`, `go vet`, clean `staticcheck`, `go mod verify`, empty `go mod tidy -diff`, `govulncheck` with zero reachable findings, OpenAPI/TypeScript drift checks, strict TypeScript, 15 Vitest tests, Vite production build, npm audit with zero findings, and scans showing neither user test key is tracked.

## Next Slice

Implement the first P7 Skills vertical slice:

1. Define a bounded, deterministic `skill.v1` manifest owned by Go.
2. Validate name/version/checksum, Profile compatibility, declared tool dependencies, paths, UTF-8, and size/token limits.
3. Add a read-only Registry and `skill list/show/validate` CLI.
4. Seed metadata for `code`, `review`, `learn`, and `script` only.
5. Do not inject Skill content into prompts or grant tools in this slice; version-pinned, token-budgeted context selection comes later.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
