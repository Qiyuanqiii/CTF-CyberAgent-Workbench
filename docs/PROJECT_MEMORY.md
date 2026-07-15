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
21. `docs/adr/0016-recoverable-docker-rehearsal-attempt.md`
22. `docs/adr/0017-descriptor-sealed-host-input-staging.md`

## Current Baseline

- Architecture completion: about 98%; the V2 run-centric control plane is about 99% complete.
- Product usability: about 45-50% for the complete Code + Cyber product.
- Generic coding-agent workflow usability: about 40%.
- Cyber autonomous-workflow usability: about 20%.
- These are engineering estimates based on tested roadmap slices, not performance benchmarks. Do not reuse the retired single-axis "overall product vision" percentage.
- Database schema: v57.
- `README.md` carries the canonical bilingual schema timeline in strict `v1 -> v57` order. `internal/store/readme_history_test.go` binds its row count and ordering to `LatestSchemaVersion`, so a future migration cannot silently leave the public history missing or out of sequence.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, parent-selected minimal Specialist Skill context, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, safe-boundary operator steering with pending-only cancellation and explicit drain, strict metadata-only Sandbox Manifest preparation, approval-bound disabled candidates, a disabled Artifact-bound Sandbox lifecycle with independent fencing/cancellation/cleanup recovery, a required-but-unverified backend/output preflight, simulation-only backend evidence plus atomic in-memory output transactions, fixed-local-endpoint read-only Docker metadata observation, deterministic in-memory Docker container plans plus fake write transactions, default-disabled Docker create-inspect-remove rehearsals, durable pre-daemon attempts with recoverable stopped-container stage/cleanup checkpoints, descriptor-pinned and kernel-sealed host-input capture evidence, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- A Specialist receives at most one parent-selected built-in Skill guide. Assignment text, model output, and external content cannot choose or widen that subset.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local and container-process command execution is disabled.
- `sandbox_manifest.v1`, `sandbox_execution_candidate.v1`, `sandbox_execution.v1`, `sandbox_preflight.v1`, `sandbox_backend_evidence.v1`, `sandbox_output_simulation.v1`, `sandbox_docker_observation.v1`, `sandbox_docker_container_plan.v1`, `sandbox_docker_container_rehearsal.v1`, `sandbox_docker_container_rehearsal_attempt.v1`, and `sandbox_docker_host_input_staging.v1` are evidence, never execution permits. Schemas v48-v57 fix execution, start, export, and production Artifact-commit capabilities to false even after exact operator approval.
- Sandbox execution ownership uses a separate generation-fenced lease. The initial lease can only prepare a disabled record; cleanup can recover after Run termination, but stale generations cannot commit.
- Input Artifacts are reverified by exact Run/Session/Workspace, digest, size, MIME, source, stream, order, and a 16 MiB aggregate cap. v50 stores no Artifact body or raw output path.
- The v51 backend handshake is disabled, container identity is unbound, and all 16 threat-model checks remain required/unverified/not-probed. Output slots store only opaque locator fingerprints and cannot export or commit Artifacts.
- The v52 fake client never contacts Docker. Its 16 `simulated_pass` items remain unverified and production-untrusted; the output harness commits only to an in-memory fake and must leave `run_artifacts` unchanged.
- The v53 Docker observer exposes only fixed-endpoint GET operations. It has no create/start/run/exec/pull/remove method, ignores `DOCKER_HOST`, stores no raw daemon/socket/repository identity, and cannot turn metadata observation into production verification or execution authority. Private-mount support remains explicitly unobservable through this read-only protocol.
- The v54 compiler emits a full container specification only in memory and persists metadata-only controls and fake steps. Its fake writer has no daemon transport; success, failure, crash, and cancellation all leave real containers and production Artifacts untouched. `compiled_not_applied` is not production verification.
- The v55 writer is a separate default-disabled transport fixed to the Linux local Unix socket and Docker API `1.40`. Its closed allowlist permits exact image/container inspection, create, and non-forced delete with fixed anonymous-volume cleanup. The image RepoDigest must match and declare no `VOLUME` before create. Its first profile is network-, environment-, and secret-free; it never starts a container, pulls an image, exports output, or grants backend/execution/Artifact authority. Raw container IDs and host paths remain transient, semantic replay does not contact Docker, and cancellation/uncertain-create cleanup re-inspects under an independent bounded context before deleting only an exact authority match.
- The v56 attempt is durable before daemon mutation and fenced by an expiring monotonically generated SQLite lease. Stage can create once or adopt only an exact stopped authority match, then freezes 19 configuration controls with `execution_evidence=false`. Cleanup deletes only the exact request/configuration/authority/container-ID-fingerprint match or accepts absence. Stale generations fail closed, failure codes are bounded and append-only, attempt-ID resume requires full Manifest resubmission and fresh confirmation, and the raw operation key is not required or exposed. Image and container environments must both be empty.
- The v57 host-input intent is recorded after the v56 stopped-container stage and before cleanup. Linux uses `openat2` no-symlink/no-magic-link/beneath/no-cross-device resolution and `O_PATH` special-file preflight, supports directory and single-file mounts, rechecks descriptor identity and metadata, writes a deterministic sanitized tar to `memfd`, applies write/grow/shrink/seal kernel seals, and rereads the bundle for digest verification. SQLite blocks completion while an intent is pending and retains metadata only. The bundle is not passed to Docker, so `daemon_consumed=false` and `execution_evidence=false`; v57 closes descriptor-capture replacement but does not yet prove daemon consumption or process isolation.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v57 adds a default-disabled host-input capture gate to the recoverable v56 never-started rehearsal. It requires separate operator confirmation, binds an immutable intent to the exact attempt, stopped-container fingerprint, plan, input digest, requester, and current lease generation, and makes SQLite completion depend on a matching result.

The Linux stager opens the absolute workspace root and every read-only mount with `openat2` no-symlink/no-magic-link/beneath/no-cross-device resolution. It preflights entries through `O_PATH`, reopens only matching ordinary files/directories, and therefore rejects FIFOs and other special files before a potentially blocking read. Directory and single-file mounts are both valid; hard links, traversal, excessive depth, entry limits, and byte limits fail closed. Once the whole tree is descriptor-pinned it rechecks device, inode, mode, link count, size, mtime, and ctime, then builds a deterministic sanitized tar with exact revalidated input Artifacts. Directory inode size is excluded from the content digest. The tar exists only in a sealable `memfd`; write/grow/shrink/seal bits are applied and the bundle is reread to verify its digest. Windows reports `staging_unsupported` before a container can be created.

Application verifies returned Artifact bytes and payload digest, stores only bounded counts/digests/security flags, and performs best-effort stopped-container cleanup before releasing the lease on staging failure. A later generation resumes a pending intent without another create, including when cleanup was already checkpointed. CLI adds the opt-in flags plus metadata-only list/show commands. Raw paths, file content, descriptors, raw container IDs, commands, environment values, secrets, sockets, operation keys, and private lease identities stay out of v57 tables and events.

Focused tests cover separate confirmation, default-disabled and unsupported behavior, deterministic replay, rename/replace/delete detection after pin, symlink/hard-link/FIFO rejection, single-file mounts, bounded directory enumeration, cancellation, report mismatch, cleanup-first failure, restart/takeover without a second create, stale-generation fencing, SQL completion gating, immutability, privacy, and v56-to-v57 migration. Final ordinary/race suites pass in 155.0s/168.1s; vet/staticcheck/module/govulncheck, 17 frontend tests, OpenAPI/build/npm audit, repository scans, isolated schema-v57 binary smoke, focused repetition, and Linux test-binary cross-compilation pass. GitHub Actions supplies the Linux runtime proof. The audit tightened root-parent symlink rejection, public report construction, Artifact byte/digest revalidation, stage-to-intent chronology, resource-limit errors, ambiguous confirmation, file-mount SQL/report constraints, filesystem-independent directory digests, special-file preflight, bounded/cancellable reads, independent-ID semantic convergence, and pre-acquire rejection of missing resume confirmation. No high/medium issue is currently known. The bundle is deliberately not passed to Docker, so this slice adds no execution usability and does not satisfy a future start gate.

GitHub Actions run `29396264276` passed commit `8719dff` with Go/Linux in 3m55s and TypeScript in 23s, providing the Linux runtime proof. The preceding run `29395980413` failed only because the single-file test fixture no longer covered its directory working path; the corrected mixed directory/file fixture now exercises the intended report constraint.

## Next Slice

Continue P6 through a separately gated schema-v58 daemon-owned immutable input handoff; do not reinterpret v57 capture evidence as bytes consumed by Docker:

1. Persist the host-input-staging choice before the v56 daemon stage. Today, a crash after that checkpoint but before the v57 intent requires both flags to be resubmitted and can otherwise finish an optional v56-only rehearsal; this is a low-risk recovery gap because start and all production authority remain false.
2. Select a closed Docker mechanism that consumes the exact v57 bundle without weakening read-only root/input guarantees or adding caller-selected endpoints, arbitrary archive paths, image pulls, generic volume control, or start.
3. Bind daemon-consumption evidence to the v57 bundle digest, current v56/v57 attempt authority, generation, and deterministic container identity. Crash, retry, cleanup, and unrelated-resource collisions must fail closed or converge.
4. Keep the target container stopped and all execution/backend/Artifact flags false. A Docker API archive write that is incompatible with read-only targets must not be “fixed” by making the target writable.
5. Only after daemon handoff is proven should termination, running-container orphan recovery, output collection, and atomic Artifact commit be designed as separate gates. Broader network/secrets, HTTP/React mutation, Rust analyzers, and CTF solving remain deferred.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so startup correctly fails closed with `migration 30 checksum or name mismatch`. The v57 slice did not modify migrations 1-56, and fresh/upgrade fixtures plus isolated `CYBERAGENT_HOME` runs pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
