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

The first P7 Skills vertical slice adds a Go-owned bounded `skill.v1` manifest, an immutable embedded Registry, and `skill list/show/validate`. Four metadata-only built-ins cover `code`, `review`, `learn`, and `script`. Every record pins a version, Profile set, narrow valid-tool prerequisite set, relative Markdown content path, UTF-8 byte count, conservative token upper bound, and SHA-256. The loader rejects unknown/duplicate/trailing JSON, invalid UTF-8, traversal, symbolic links, profile/tool escalation, oversized data, and checksum drift. Registry results are defensive copies.

This slice adds no migration, Store write, Provider call, prompt injection, external Skill path, tool execution, or capability grant. CLI output says both context injection and tool capability grants are disabled, and tests prove the commands do not create `cyberagent.db`. The focused audit found no unresolved high/medium issue and fixed low-risk strict-JSON, root-symlink, and deterministic-validation gaps.

Verified gates: uncached `go test ./...`, full `go test -race ./...`, `go vet`, clean `staticcheck`, `go mod verify`, empty `go mod tidy -diff`, `govulncheck` with zero reachable findings, OpenAPI/TypeScript drift checks, strict TypeScript, 15 Vitest tests, Vite production build, npm audit with zero findings, tracked credential/runtime-artifact scans, and an isolated real-binary Skill CLI smoke. The new `internal/skills` package has 86.3% statement coverage.

## Next Slice

Implement the second P7 Skills vertical slice:

1. Define immutable `skill_selection.v1` in Go for one Run and validated Profile.
2. Resolve exact Skill name/version/content-hash tuples from the read-only Registry.
3. Enforce an aggregate conservative token budget and deterministic ordering/fingerprint.
4. Persist only bounded selection provenance for recovery and audit.
5. Still do not inject Skill content into Provider prompts or grant tools; context assembly/redaction remains a later separately audited slice.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
