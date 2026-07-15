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
18. `docs/adr/0013-read-only-docker-observation.md`
19. `docs/adr/0014-deterministic-docker-container-plan.md`
20. `docs/adr/0015-bounded-docker-write-rehearsal.md`

## Current Baseline

- Architecture completion: about 98%; the V2 run-centric control plane is about 99% complete.
- Product usability: about 45-50% for the complete Code + Cyber product.
- Generic coding-agent workflow usability: about 40%.
- Cyber autonomous-workflow usability: about 20%.
- These are engineering estimates based on tested roadmap slices, not performance benchmarks. Do not reuse the retired single-axis "overall product vision" percentage.
- Database schema: v55.
- `README.md` carries the canonical bilingual schema timeline in strict `v1 -> v55` order. `internal/store/readme_history_test.go` binds its row count and ordering to `LatestSchemaVersion`, so a future migration cannot silently leave the public history missing or out of sequence.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, parent-selected minimal Specialist Skill context, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, safe-boundary operator steering with pending-only cancellation and explicit drain, strict metadata-only Sandbox Manifest preparation, approval-bound disabled candidates, a disabled Artifact-bound Sandbox lifecycle with independent fencing/cancellation/cleanup recovery, a required-but-unverified backend/output preflight, simulation-only backend evidence plus atomic in-memory output transactions, fixed-local-endpoint read-only Docker metadata observation, deterministic in-memory Docker container plans plus fake write transactions, and default-disabled Docker create-inspect-remove rehearsals that never start a container, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- A Specialist receives at most one parent-selected built-in Skill guide. Assignment text, model output, and external content cannot choose or widen that subset.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local and container-process command execution is disabled.
- `sandbox_manifest.v1`, `sandbox_execution_candidate.v1`, `sandbox_execution.v1`, `sandbox_preflight.v1`, `sandbox_backend_evidence.v1`, `sandbox_output_simulation.v1`, `sandbox_docker_observation.v1`, `sandbox_docker_container_plan.v1`, and `sandbox_docker_container_rehearsal.v1` are evidence, never execution permits. Schemas v48-v55 fix execution, start, export, and production Artifact-commit capabilities to false even after exact operator approval.
- Sandbox execution ownership uses a separate generation-fenced lease. The initial lease can only prepare a disabled record; cleanup can recover after Run termination, but stale generations cannot commit.
- Input Artifacts are reverified by exact Run/Session/Workspace, digest, size, MIME, source, stream, order, and a 16 MiB aggregate cap. v50 stores no Artifact body or raw output path.
- The v51 backend handshake is disabled, container identity is unbound, and all 16 threat-model checks remain required/unverified/not-probed. Output slots store only opaque locator fingerprints and cannot export or commit Artifacts.
- The v52 fake client never contacts Docker. Its 16 `simulated_pass` items remain unverified and production-untrusted; the output harness commits only to an in-memory fake and must leave `run_artifacts` unchanged.
- The v53 Docker observer exposes only fixed-endpoint GET operations. It has no create/start/run/exec/pull/remove method, ignores `DOCKER_HOST`, stores no raw daemon/socket/repository identity, and cannot turn metadata observation into production verification or execution authority. Private-mount support remains explicitly unobservable through this read-only protocol.
- The v54 compiler emits a full container specification only in memory and persists metadata-only controls and fake steps. Its fake writer has no daemon transport; success, failure, crash, and cancellation all leave real containers and production Artifacts untouched. `compiled_not_applied` is not production verification.
- The v55 writer is a separate default-disabled transport fixed to the Linux local Unix socket and Docker API `1.40`. Its closed allowlist permits exact image/container inspection, create, and non-forced delete with fixed anonymous-volume cleanup. The image RepoDigest must match and declare no `VOLUME` before create. Its first profile is network-, environment-, and secret-free; it never starts a container, pulls an image, exports output, or grants backend/execution/Artifact authority. Raw container IDs and host paths remain transient, semantic replay does not contact Docker, and cancellation/uncertain-create cleanup re-inspects under an independent bounded context before deleting only an exact authority match.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v55 adds a bounded Docker create-inspect-remove rehearsal without adding process execution. It accepts only the strict network-disabled, environment-free, secret-free subset of an exact current v54 plan, and only after a new explicit operator confirmation.

The Linux production constructor is fixed to `/var/run/docker.sock`, ignores `DOCKER_HOST`, disables proxying and redirects, and pins Docker API `1.40`. A closed HTTP allowlist permits exact volume-free image inspection, create by deterministic name, exact container inspection, and non-forced delete with fixed `v=1`. There is no start, exec, attach, pull, log, archive, export, network, volume management, image-build, or general request operation.

Application revalidates v48-v54 before transport access, and SQLite revalidates the same authority chain before commit. The transport recompiles and exactly matches the v54 specification, resolves non-symlink regular host sources under the trusted workspace, rejects image-declared volumes before create, creates a stopped digest-pinned non-root/read-only/capability-dropped container, inspects attachment/device/port/security settings and mounts, and removes it with anonymous-volume cleanup. Only an exact stopped stale rehearsal container may be reconciled. Cancellation, partial failure, and an uncertain create response use an independent five-second cleanup context and never delete before another exact match.

Durable rehearsal rows, five steps, operations, events, and CLI expose metadata and fingerprints only. Raw container IDs, host paths, command/argv, environment values, secrets, sockets, and full specifications are omitted. Same-intent replay never reaches the daemon, and two concurrent Stores converge to one immutable result. The normal path records three daemon reads and two writes, or three writes when one exact stale rehearsal is removed, while `container_never_started`, `process_never_executed`, `image_never_pulled`, and `output_never_exported` remain true and all production authority remains false.

Focused protocol, transport, uncertain-create and cancellation cleanup, no-blind-delete, image-volume rejection, collision, symlink, Application, Store, migration, replay, concurrency, privacy, SQL-invariant, downgrade/re-upgrade, and CLI tests pass. The final full ordinary/race suites completed in 163.3s/168.7s; vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, repository/privacy scans, diff checks, and an isolated real-binary schema-v55 Workspace smoke are green. Transport tests passed 20 ordinary and 10 race repetitions; two-Store convergence passed 10 repetitions. The audit fixed uncertain-create orphan cleanup, blind-ID deletion, image-declared anonymous-volume side effects, and incomplete attachment/device/port/capability checks. No unresolved high/medium issue is known. The Linux real-daemon integration harness requires an already-present immutable volume-free digest and was skipped on this Windows host without Docker; it can never pull or start. GitHub Actions run `29382661971` passed feature commit `69d81d6` with Go in 2m32s and TypeScript in 25s. No real Provider, Shell, Local/container-process execution, network action, production Artifact export, or CTF solving has been enabled.

## Next Slice

Continue P6 through a separately gated schema-v56 boundary; do not reinterpret v55 rehearsal evidence as process-execution authority:

1. Add a durable pre-daemon attempt/intention record and recovery state so a process crash between daemon mutation and v55 result commit can be reconciled by exact authority labels without depending only on a repeated CLI request.
2. Preserve the default-disabled constructor, explicit operator confirmation, strict no-network/no-secret profile, fixed local socket, and closed HTTP operation set. Do not add start, exec, attach, pull, logs, export, or a generic Docker client in the same slice.
3. Add an immutable production-control matrix derived from exact post-create inspection, while keeping every item non-execution evidence. Replay and takeover must never create twice or remove an unrelated container.
4. Before any future start boundary, close host-mount time-of-check/time-of-use replacement with descriptor-pinned Linux resolution or daemon-side immutable staging, then separately audit termination, orphan recovery, output collection, and atomic Artifact commit.
5. Keep production Artifact export, broader network/secret support, HTTP/React mutation, Rust analyzers, and CTF solving deferred until their dedicated audits pass.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so startup correctly fails closed with `migration 30 checksum or name mismatch`. The v55 slice did not modify migrations 1-54, and fresh/upgrade fixtures plus isolated `CYBERAGENT_HOME` runs pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
