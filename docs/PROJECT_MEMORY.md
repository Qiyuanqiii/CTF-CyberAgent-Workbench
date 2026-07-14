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

## Current Baseline

- Architecture completion: about 98%; the V2 run-centric control plane is about 99% complete.
- Product usability: about 45-50% for the complete Code + Cyber product.
- Generic coding-agent workflow usability: about 40%.
- Cyber autonomous-workflow usability: about 20%.
- These are engineering estimates based on tested roadmap slices, not performance benchmarks. Do not reuse the retired single-axis "overall product vision" percentage.
- Database schema: v54.
- `README.md` carries the canonical bilingual schema timeline in strict `v1 -> v54` order. `internal/store/readme_history_test.go` binds its row count and ordering to `LatestSchemaVersion`, so a future migration cannot silently leave the public history missing or out of sequence.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, parent-selected minimal Specialist Skill context, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, safe-boundary operator steering with pending-only cancellation and explicit drain, strict metadata-only Sandbox Manifest preparation, approval-bound disabled candidates, a disabled Artifact-bound Sandbox lifecycle with independent fencing/cancellation/cleanup recovery, a required-but-unverified backend/output preflight, simulation-only backend evidence plus atomic in-memory output transactions, fixed-local-endpoint read-only Docker metadata observation, and deterministic in-memory Docker container plans plus fake write transactions, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- A Specialist receives at most one parent-selected built-in Skill guide. Assignment text, model output, and external content cannot choose or widen that subset.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local/Docker command execution is disabled.
- `sandbox_manifest.v1`, `sandbox_execution_candidate.v1`, `sandbox_execution.v1`, `sandbox_preflight.v1`, `sandbox_backend_evidence.v1`, `sandbox_output_simulation.v1`, `sandbox_docker_observation.v1`, and `sandbox_docker_container_plan.v1` are evidence, never execution permits. Schemas v48-v54 fix backend, execution, start, export, and production Artifact-commit capabilities to false even after exact operator approval.
- Sandbox execution ownership uses a separate generation-fenced lease. The initial lease can only prepare a disabled record; cleanup can recover after Run termination, but stale generations cannot commit.
- Input Artifacts are reverified by exact Run/Session/Workspace, digest, size, MIME, source, stream, order, and a 16 MiB aggregate cap. v50 stores no Artifact body or raw output path.
- The v51 backend handshake is disabled, container identity is unbound, and all 16 threat-model checks remain required/unverified/not-probed. Output slots store only opaque locator fingerprints and cannot export or commit Artifacts.
- The v52 fake client never contacts Docker. Its 16 `simulated_pass` items remain unverified and production-untrusted; the output harness commits only to an in-memory fake and must leave `run_artifacts` unchanged.
- The v53 Docker observer exposes only fixed-endpoint GET operations. It has no create/start/run/exec/pull/remove method, ignores `DOCKER_HOST`, stores no raw daemon/socket/repository identity, and cannot turn metadata observation into production verification or execution authority. Private-mount support remains explicitly unobservable through this read-only protocol.
- The v54 compiler emits a full container specification only in memory and persists metadata-only controls and fake steps. Its fake writer has no daemon transport; success, failure, crash, and cancellation all leave real containers and production Artifacts untouched. `compiled_not_applied` is not production verification.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v54 adds deterministic container compilation and fake write ordering without adding production execution. `CompileDockerContainerSpec` accepts only a complete v53 observation and fixes non-root identity, read-only root and inputs, one writable output mount, `rprivate` propagation, network default deny or exact managed allowlisting, ephemeral secrets, resource/time/kill limits, labeled orphan recovery, and stop-before-export ordering.

Compilation requires explicit `--confirm-fake-write`, exact v53 observation/Manifest bindings, and complete Application plus SQLite revalidation of the v48-v53 authority chain. Same-intent retries do not rerun the fake writer, cross-Store races converge, and one observation can bind only one immutable plan.

The full specification remains transient. SQLite/events/CLI retain only bounded counts, fingerprints, sixteen `compiled_not_applied` controls, and seven fake reconcile/create/start/wait/stop/export/remove steps. Raw command, argv, paths, network targets, environment values, secret references, labels, and container names are omitted. Failure, simulated crash, or cancellation commits no fake transaction; success still records zero daemon writes, backend contact, production submission, execution/export authorization, and Artifact commits.

Focused compiler, fake-transaction, Application, Store, migration, replay, concurrency, cancellation, authority, privacy, SQL-invariant, downgrade/re-upgrade, and CLI tests pass. The final local gate passed full ordinary/race tests in 128.2s/148.5s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, repository/privacy scans, diff checks, and an isolated real-binary v54 migration/Workspace smoke. Two-Store contention passed 20 ordinary and 10 race repetitions. The audit added explicit requester continuity across v52 evidence/output simulation in Application and SQL plus deep-copy isolation for fake-transaction snapshots. GitHub Actions run `29376503165` passed feature commit `126719f` with Go in 2m7s and TypeScript in 19s. No unresolved high/medium issue is known. No real Provider, Shell, Local/Docker execution, network action, production Artifact export, or CTF solving has been enabled.

## Next Slice

Continue P6 through a separately gated schema-v55 boundary; do not extend the v53 observer or reinterpret v54 fake evidence:

1. Define a minimal Go Docker write-transport protocol isolated from `DockerReadOnlyTransport`, fixed to the local Linux Unix socket and a closed method/path allowlist. It must accept no `DOCKER_HOST`, TCP endpoint, caller socket, image pull, exec, attach, or general-purpose Docker client.
2. Keep the transport disabled by default and require exact v54 plan identity plus a new explicit operator confirmation. Start with the narrow network-disabled, no-secret profile; unsupported plans fail closed before any daemon call.
3. Add an opt-in real-daemon integration harness that uses a pre-existing digest, deterministic labels, cancellation, and orphan cleanup. Prove replay never creates twice and every partial failure reaches bounded cleanup.
4. Continue revalidating the Manifest and all v48-v54 bindings at every boundary. v52 simulation, v53 metadata observation, and v54 compilation/fake writes are neither authorization nor proof that a production control works.
5. Keep production Artifact export, broader network/secret support, HTTP/React mutation, Rust analyzers, and CTF solving deferred until their dedicated audits pass.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so startup correctly fails closed with `migration 30 checksum or name mismatch`. The v54 slice did not modify migrations 1-53, and fresh/upgrade fixtures plus isolated `CYBERAGENT_HOME` runs pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
