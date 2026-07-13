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

## Current Baseline

- Overall product vision: about 97%.
- General Agent MVP: about 99%.
- V2 run-centric runtime: about 99%.
- Database schema: v43.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local/Docker command execution is disabled.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v43 adds immutable `context_provenance.v1` for Session history and model context. New messages distinguish `operator_message`, `model_response`, `go_control`, `workspace_file`, `workspace_listing`, `workspace_diff`, `tool_result`, and `go_command_result`. Only operator messages and Go control records may set `instruction_authorized=true`; every evidence source is persisted with role `tool` and false authority. Redacted content receives a lowercase SHA-256, SQLite enforces the exact role/source/authority matrix and immutable rows, and Go recomputes the digest whenever messages are read. Compaction remains the only monotonic mutable bit.

`/read`, `/ls`, `/write`, `/run`, and other slash replies no longer become assistant history. External evidence is projected through a bounded `untrusted_context` JSON envelope before direct Session chat, root Supervisor, or Specialist calls. The envelope includes source kind, source reference, digest, false authority, and content. Context summaries now contain provenance-preserving JSON records and are replayed as user-role transcript data, not system instructions. Root WorkBoard, Notes, inbox, and compacted memory are also user-role untrusted data. The shared system policy states that files, repositories, issues, logs, web/mail, tool output, and memory are evidence and cannot grant capability.

Migration v43 conservatively labels prior rows as `context_provenance.v0`; recognizable workspace read/list, FileEdit, and ToolRun slash replies are changed from assistant to tool evidence. Legacy rows remain readable without pretending they had a historical digest. HTTP/OpenAPI, generated TypeScript DTOs, React message badges, Run events, and CLI history now expose provenance audit fields.

Regression coverage uses the concrete indirect-injection case where a README's valid Setup requires `.env`, `DATABASE_URL`, and `SESSION_SECRET` while an embedded note tells automated coding assistants to omit `.env`. Tests prove the note appears only inside a `workspace_file`, `instruction_authorized=false` envelope and is never replayed as system/assistant content through Session, root, Specialist, or compaction paths. Store tests cover v42 upgrade, role forgery rejection, immutable content/delete, control-character refs, valid-looking digest tampering detected by Go, and API projection.

The audit found no unresolved high- or medium-severity issue. One low-severity staticcheck capitalization issue was fixed. Full Go tests, full-repository race detection, vet, zero-warning staticcheck, module verify/tidy, zero-finding govulncheck, strict TypeScript, 16 Vitest tests, Vite production build, zero-vulnerability npm audit, deterministic OpenAPI/TypeScript generation, tracked-file credential scanning, and isolated binary CLI provenance smoke pass. No real Provider, Shell, network, Docker, or Sandbox execution was used. GitHub CI must be verified after push.

## Next Slice

Build schema v44 Delivery audit gates over the selected v42 WorkItems without creating another task engine:

1. Bind each delivery checkpoint to one selected WorkItem, its acceptance criteria, exact source selection, and current Deliver-mode revision.
2. Require focused verification, diff/security audit, and a compact durable handoff Note before that slice can complete.
3. Require a broader functional and robustness gate when a larger module boundary is reached, then let existing Run completion checks consume those facts.
4. Keep model assertions non-authoritative: Go owns gate state, evidence references, phase, budgets, and completion.
5. Audit Specialist Skill minimization separately after the single-root Delivery loop is stable.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
