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

## Current Baseline

- Overall product vision: about 97%.
- General Agent MVP: about 99%.
- V2 run-centric runtime: about 99%.
- Database schema: v45.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, safe-boundary operator steering, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

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

Schema v45 adds a durable operator steering queue over the existing RunSupervisor and Session pipeline. `run steer enqueue` accepts a stable operation key, while an ordinary Run-bound `session send` automatically queues when the Run has an active lease, a started/failed pending attempt, or an existing queue. Input is redacted, normalized, limited to 16 KiB, ordered by a per-Run sequence, and bounded to 64 pending items/256 KiB. Raw operation keys are never stored.

At a safe root-turn boundary, `BeginSupervisorTurn` prepares only the oldest item and binds it to the exact attempt/turn. Successful lifecycle commit writes the authorized Session user message, assistant response, delivery commit, and queue state in one transaction. A failed attempt supersedes its delivery and retries the same item without early Session history. If more input remains, model `finish`/`wait` is deferred to a Go-owned `continue`; final Run completion is blocked while pending input exists. Failed/cancelled Runs cancel outstanding queue items transactionally.

CLI provides enqueue/list/show, and Session/TUI input can queue without interrupting the active action. HTTP/OpenAPI, React, and TUI expose bounded status/sequence metadata only. They omit content, hashes, operator identity, Session IDs, and Session-message IDs; no public mutation or capability was added. The local CLI detail is the intentional trusted surface that can display content.

The release audit found no unresolved high- or medium-severity issue. It fixed three low-risk boundaries: a paused Run with an existing queue returns the durable queued fact without a fallible post-commit auto-resume; queue events no longer expose requester identity; and `run steer list` validates that its parent Run exists instead of treating an unknown Run as an empty queue. Full Go tests, full race detection, vet, staticcheck, module verification, govulncheck, strict TypeScript, 17 Vitest tests, production build, npm audit, and an isolated real-binary queue smoke pass locally. GitHub Actions run `29310437643` passed for release commit `022b083`: Go control plane completed in 3m10s and TypeScript console in 23s. A follow-up CI audit disabled the govulncheck composite action's redundant checkout and cache restore because the job already owns both; the vulnerability scan remains enabled.

## Next Slice

Build schema v46 queue-control hardening before widening autonomy:

1. Add digest-idempotent, operator-only cancellation for `pending` steering; never edit, reorder, or cancel the currently prepared item.
2. Define an explicit wake/drain policy for queued work on idle or paused Runs without allowing a post-commit API error to hide a successful enqueue.
3. Add caller-supplied idempotency to ordinary Session steering so client retries across process boundaries can converge as precisely as `run steer enqueue`.
4. Keep HTTP and React read-only; expose no queue mutation to models or child Agents.
5. Follow with the independent Specialist Skill-minimization audit, preserving distinct Code and Cyber Skill sets. Rust analyzers remain behind the later Sandbox/process JSON protocol.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so startup correctly fails closed with `migration 30 checksum or name mismatch`. The v45 code did not modify migrations 1-44, and fresh/upgrade fixtures plus isolated `CYBERAGENT_HOME` runs pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
