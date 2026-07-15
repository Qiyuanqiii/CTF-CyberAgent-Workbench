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
23. `docs/adr/0018-durable-pre-stage-host-input-requirement.md`
24. `docs/adr/0019-daemon-owned-host-input-handoff.md`
25. `docs/adr/0020-deterministic-runtime-input-projection.md`

## Current Baseline

- Architecture completion: about 98%; the V2 run-centric control plane is about 99% complete.
- Product usability: about 45-50% for the complete Code + Cyber product.
- Generic coding-agent workflow usability: about 40%.
- Cyber autonomous-workflow usability: about 20%.
- These are engineering estimates based on tested roadmap slices, not performance benchmarks. Do not reuse the retired single-axis "overall product vision" percentage.
- Database schema: v60.
- `README.md` carries the canonical bilingual schema timeline in strict `v1 -> v60` order. `internal/store/readme_history_test.go` binds its row count and ordering to `LatestSchemaVersion`, so a future migration cannot silently leave the public history missing or out of sequence.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, parent-selected minimal Specialist Skill context, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, safe-boundary operator steering with pending-only cancellation and explicit drain, strict metadata-only Sandbox Manifest preparation, approval-bound disabled candidates, a disabled Artifact-bound Sandbox lifecycle with independent fencing/cancellation/cleanup recovery, a required-but-unverified backend/output preflight, simulation-only backend evidence plus atomic in-memory output transactions, fixed-local-endpoint read-only Docker metadata observation, deterministic in-memory Docker container plans plus fake write transactions, default-disabled Docker create-inspect-remove rehearsals, durable pre-daemon attempts with recoverable stopped-container stage/cleanup checkpoints, descriptor-pinned and kernel-sealed host-input capture, immutable pre-stage requirements, daemon-owned fixed-volume handoff with exact archive readback and complete cleanup, strict operator-confirmed runtime-input projection plans, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- A Specialist receives at most one parent-selected built-in Skill guide. Assignment text, model output, and external content cannot choose or widen that subset.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local and container-process command execution is disabled.
- `sandbox_manifest.v1`, `sandbox_execution_candidate.v1`, `sandbox_execution.v1`, `sandbox_preflight.v1`, `sandbox_backend_evidence.v1`, `sandbox_output_simulation.v1`, `sandbox_docker_observation.v1`, `sandbox_docker_container_plan.v1`, `sandbox_docker_container_rehearsal.v1`, `sandbox_docker_container_rehearsal_attempt.v1`, `sandbox_docker_host_input_staging.v1`, `sandbox_docker_host_input_requirement.v1`, the v59 handoff requirement/intent/result, and the v60 runtime-input projection plan are evidence, never execution permits. Schemas v48-v60 fix execution, start, export, and production Artifact-commit capabilities to false even after exact operator approval.
- Sandbox execution ownership uses a separate generation-fenced lease. The initial lease can only prepare a disabled record; cleanup can recover after Run termination, but stale generations cannot commit.
- Input Artifacts are reverified by exact Run/Session/Workspace, digest, size, MIME, source, stream, order, and a 16 MiB aggregate cap. v50 stores no Artifact body or raw output path.
- The v51 backend handshake is disabled, container identity is unbound, and all 16 threat-model checks remain required/unverified/not-probed. Output slots store only opaque locator fingerprints and cannot export or commit Artifacts.
- The v52 fake client never contacts Docker. Its 16 `simulated_pass` items remain unverified and production-untrusted; the output harness commits only to an in-memory fake and must leave `run_artifacts` unchanged.
- The v53 Docker observer exposes only fixed-endpoint GET operations. It has no create/start/run/exec/pull/remove method, ignores `DOCKER_HOST`, stores no raw daemon/socket/repository identity, and cannot turn metadata observation into production verification or execution authority. Private-mount support remains explicitly unobservable through this read-only protocol.
- The v54 compiler emits a full container specification only in memory and persists metadata-only controls and fake steps. Its fake writer has no daemon transport; success, failure, crash, and cancellation all leave real containers and production Artifacts untouched. `compiled_not_applied` is not production verification.
- The v55 writer is a separate default-disabled transport fixed to the Linux local Unix socket and Docker API `1.40`. Its closed allowlist permits exact image/container inspection, create, and non-forced delete with fixed anonymous-volume cleanup. The image RepoDigest must match and declare no `VOLUME` before create. Its first profile is network-, environment-, and secret-free; it never starts a container, pulls an image, exports output, or grants backend/execution/Artifact authority. Raw container IDs and host paths remain transient, semantic replay does not contact Docker, and cancellation/uncertain-create cleanup re-inspects under an independent bounded context before deleting only an exact authority match.
- The v56 attempt is durable before daemon mutation and fenced by an expiring monotonically generated SQLite lease. Stage can create once or adopt only an exact stopped authority match, then freezes 19 configuration controls with `execution_evidence=false`. Cleanup deletes only the exact request/configuration/authority/container-ID-fingerprint match or accepts absence. Stale generations fail closed, failure codes are bounded and append-only, attempt-ID resume requires full Manifest resubmission and fresh confirmation, and the raw operation key is not required or exposed. Image and container environments must both be empty.
- The v57 host-input intent is recorded after the v56 stopped-container stage and before cleanup. Linux uses `openat2` no-symlink/no-magic-link/beneath/no-cross-device resolution and `O_PATH` special-file preflight, supports directory and single-file mounts, rechecks descriptor identity and metadata, writes a deterministic sanitized tar to `memfd`, applies write/grow/shrink/seal kernel seals, and rereads the bundle for digest verification. SQLite blocks completion while an intent is pending and retains metadata only. The bundle is not passed to Docker, so `daemon_consumed=false` and `execution_evidence=false`; v57 closes descriptor-capture replacement but does not yet prove daemon consumption or process isolation.
- The v59 handoff is default-disabled and requires daemon-write, capture, and handoff confirmation. It uses only fixed API `1.40` archive/volume/container operations, a deterministic local-volume carrier, fixed `/cyberagent-input/bundle.tar`, exact daemon readback, a final read-only never-started target check, and complete resource deletion. User mounts cannot overlap the reserved destination. Retry removes only exact owned residue, while foreign collisions fail closed. Durable evidence grants no start, exec, output, backend, execution, or Artifact authority.
- The v60 projection plan requires a separately persisted operator confirmation and a completed v59 handoff. It recaptures the exact sealed input, accepts only byte-for-byte canonical v57 PAX tar, maps directory-root read-only mounts and fixed Artifact input in memory, and binds deterministic future volume identity to the handoff fingerprint. Tables/events/CLI retain no raw target, path, file name/content, volume name, or archive bytes. `compiled_not_applied` grants no daemon, start, exec, output, backend, execution, or Artifact authority.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v57 adds a default-disabled host-input capture gate to the recoverable v56 never-started rehearsal. It requires separate operator confirmation, binds an immutable intent to the exact attempt, stopped-container fingerprint, plan, input digest, requester, and current lease generation, and makes SQLite completion depend on a matching result.

The Linux stager opens the absolute workspace root and every read-only mount with `openat2` no-symlink/no-magic-link/beneath/no-cross-device resolution. It preflights entries through `O_PATH`, reopens only matching ordinary files/directories, and therefore rejects FIFOs and other special files before a potentially blocking read. Directory and single-file mounts are both valid; hard links, traversal, excessive depth, entry limits, and byte limits fail closed. Once the whole tree is descriptor-pinned it rechecks device, inode, mode, link count, size, mtime, and ctime, then builds a deterministic sanitized tar with exact revalidated input Artifacts. Directory inode size is excluded from the content digest. The tar exists only in a sealable `memfd`; write/grow/shrink/seal bits are applied and the bundle is reread to verify its digest. Windows reports `staging_unsupported` before a container can be created.

Application verifies returned Artifact bytes and payload digest, stores only bounded counts/digests/security flags, and performs best-effort stopped-container cleanup before releasing the lease on staging failure. A later generation resumes a pending intent without another create, including when cleanup was already checkpointed. CLI adds the opt-in flags plus metadata-only list/show commands. Raw paths, file content, descriptors, raw container IDs, commands, environment values, secrets, sockets, operation keys, and private lease identities stay out of v57 tables and events.

Focused tests cover separate confirmation, default-disabled and unsupported behavior, deterministic replay, rename/replace/delete detection after pin, symlink/hard-link/FIFO rejection, single-file mounts, bounded directory enumeration, cancellation, report mismatch, cleanup-first failure, restart/takeover without a second create, stale-generation fencing, SQL completion gating, immutability, privacy, and v56-to-v57 migration. Final ordinary/race suites pass in 155.0s/168.1s; vet/staticcheck/module/govulncheck, 17 frontend tests, OpenAPI/build/npm audit, repository scans, isolated schema-v57 binary smoke, focused repetition, and Linux test-binary cross-compilation pass. GitHub Actions supplies the Linux runtime proof. The audit tightened root-parent symlink rejection, public report construction, Artifact byte/digest revalidation, stage-to-intent chronology, resource-limit errors, ambiguous confirmation, file-mount SQL/report constraints, filesystem-independent directory digests, special-file preflight, bounded/cancellable reads, independent-ID semantic convergence, and pre-acquire rejection of missing resume confirmation. No high/medium issue is currently known. The bundle is deliberately not passed to Docker, so this slice adds no execution usability and does not satisfy a future start gate.

GitHub Actions run `29396264276` passed commit `8719dff` with Go/Linux in 3m55s and TypeScript in 23s, providing the Linux runtime proof. The preceding run `29395980413` failed only because the single-file test fixture no longer covered its directory working path; the corrected mixed directory/file fixture now exercises the intended report constraint.

Schema v58 closes the v57 post-stage/pre-intent downgrade window for all new attempts. `sandbox_docker_host_input_requirement.v1` is created atomically with the v56 attempt, initial lease, and audit events before daemon stage. It binds the required/confirmed choice to attempt, plan, Run, Mission, Workspace, requester, digest-only operation identity, complete authority fingerprints, and bounded input counts. Generated row IDs and timestamps are excluded from its semantic fingerprint.

Recovery treats that durable choice as authoritative. Required attempts automatically resume v57 capture without repeating staging flags and cannot complete without matching evidence; false requirements cannot be widened. Go and SQLite independently enforce binding, immutability, false-to-staging rejection, and completion gating. Migration intentionally leaves legacy v57 attempts without a requirement because historical operator intent cannot be invented, but copies their IDs into an immutable compatibility set before new marker inserts are disabled. Every later stage/staging/completion must have a requirement or that migration marker. Tables, events, and CLI projections remain metadata-only. Focused tests cover migration, SQL mutation/deletion, privacy, completion gating, false widening, two-Store candidate convergence, completed and pending operation replay, generation-two crash recovery without flags, and CLI output.

The v58 audit rejected direct archive upload into the read-only target: Docker rejects archive writes to read-only rootfs/volumes, and weakening the target is outside authority. No archive, volume, start, exec, pull, build, export, or Artifact surface was added. The v57 bundle remains daemon-unconsumed and every production flag remains false. ADR 0018 reserves schema v59 for a separately audited daemon-owned carrier, exact upload/readback verification, carrier removal, and read-only final attachment.

Final local gates pass: full ordinary/race suites took 158.1s/168.4s; vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 17 tests in 8 frontend files, OpenAPI generation, production build, zero-vulnerability npm audit, repository privacy/artifact/process/encoding/Markdown scans, Linux sandbox test-binary cross-compilation, diff checks, and isolated schema-v58 real-binary workspace smoke are green. Domain requirement tests passed 50 repetitions, Store convergence/missing-requirement tests 30, Application pending-recovery/no-widen tests 20, and Store/Application race repetitions 10 each. The audit fixed pending operation-key recovery selecting a new candidate, unmatched explicit flags beside a durable requirement, direct-SQL post-migration attempts without requirements, and false-requirement zero-input compatibility. No unresolved high/medium issue is known. Linux real-daemon handoff evidence remains pending because this Windows host has no Docker.

GitHub Actions run `29400696276` passed feature commit `4b570f7` with Go/Linux in 2m39s and TypeScript in 23s.

Schema v59 completes the daemon-owned immutable host-input handoff gate. Every new attempt receives an immutable handoff requirement with its v58 capture requirement before daemon stage. A write-ahead intent binds the exact v57 bundle report/digest, attempt/plan, stopped-container fingerprint, active generation, requester, and full authority before archive or volume mutation. Required cleanup and completion are blocked until a matching immutable result exists; migrated v58 attempts keep explicit legacy compatibility without invented intent.

The fixed Linux transport wraps the sealed archive as `bundle.tar`, creates one deterministic daemon-owned local volume and never-started writable carrier, uploads only to `/cyberagent-input`, reads the file back through Docker, and verifies exact bytes and SHA-256. It removes the carrier and original stopped target, recreates and inspects the target with the volume read-only, then removes target and volume. Exact carrier/volume/final-target crash residue converges; foreign resources are protected. Early bundle/image failures clean only fingerprint-matched owned resources under an independent context. The reserved destination tree cannot overlap Manifest mounts. No start, exec, attach, pull, build, network mutation, export, forced volume delete, arbitrary endpoint/path, or Artifact writer exists.

Focused sandbox, Store, Application, and CLI tests cover fixed endpoint/path allowlists, successful cleanup, exact crash-residue convergence, foreign-volume protection, invalid-bundle early cleanup, destination overlap, four confirmations, sealed-handle closure, metadata privacy, write-ahead intent, generation-two retry, immutability, migration, and cleanup/completion gates. The Linux sandbox test binary cross-compiles with the new opt-in real-daemon handoff harness. This Windows host cannot execute that harness, so no Linux real-daemon runtime claim is made locally. Final ordinary/race suites passed in 183.1s/185.1s, and GitHub Actions run `29406403201` passed feature commit `fb1daca` with Go/Linux in 2m37s and TypeScript in 28s.

Schema v60 adds the separately confirmed deterministic runtime-input projection plan. Application accepts only a completed v59 handoff and completed attempt, revalidates the complete v48-v59 authority, recompiles the exact Manifest/container specification, and recaptures the v57 sealed bundle. A frozen report view prevents mutable provider metadata from changing during parsing. Report fingerprint, bundle digest/length, source and Artifact counts, and Artifact payload identity must match durable evidence.

The compiler permits only byte-for-byte canonical v57 PAX tar and rejects links, devices, traversal, duplicates, missing parents, unexpected roots, empty Artifacts, trailing bytes, and non-canonical headers. The first profile requires directory-root read-only mounts; each root becomes a separate relative tar projection, while Artifacts map to fixed `/cyberagent-input/artifacts`. Transient future volume names include the v59 handoff fingerprint, so restart retries are deterministic and identical input across different Runs remains isolated.

SQLite schema v60 atomically commits one operator-confirmed plan, ordered digest-only items, an aggregate completion marker, operation binding, and metadata event under the Run write lock. Go and SQL enforce contiguous item sets, aggregate sums, immutable records, exact handoff/attempt/plan binding, and false daemon/start/exec/export/backend/execution/Artifact authority. CLI adds `docker-runtime-input-plan`, `docker-runtime-input-plans`, and `docker-runtime-input-plan-show`; output contains no raw target, host path, file name/content, volume name, or archive bytes. Migration from v59 creates no projection facts. The audit fixed missing durable confirmation, cross-Run future volume-name collision, an incorrect global item-fingerprint uniqueness constraint, non-canonical trailing tar acceptance, deprecated tar xattr inspection, duplicate/out-of-range mount ordinals, incomplete plan chronology, and canonical long-PAX-path compatibility. No unresolved high/medium issue is known.

The final local gate passed full ordinary/race suites in 198.9s/194.0s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 17 tests across 8 frontend files, OpenAPI drift, production build, zero-vulnerability npm audit, repository privacy/artifact/encoding/Markdown scans, Linux sandbox cross-compilation, diff checks, and an isolated real-binary schema-v60 Workspace smoke. Compiler/Store/Application/CLI repetitions passed at 50/30/20/10, and the critical Sandbox/Store/Application race set passed 10 rounds. This Windows host cannot run the v59 real-daemon or future v61 volume harness; no start authority follows from v60.

## Next Slice

Continue P6 with a separately gated schema-v61 projection-application transport and never-started lifecycle design; do not reinterpret v60 compilation as process-isolation or execution proof:

1. Add a write-ahead, generation-fenced transport intent before creating any v60-derived local volume or uploading a projection archive.
2. Apply only handoff-bound deterministic volumes, attach every one read-only with no-copy semantics, inspect the complete never-started container configuration, and recover exact owned residue without deleting foreign resources.
3. Keep container start absent from v61. Design start/wait/TERM/KILL as the following independent fixed-endpoint state machine with cancellation fan-out and orphan ownership.
4. Run the v59 opt-in real-daemon handoff harness and the future v61 volume harness on Linux with an already-present exact image before any start gate depends on them.
5. Output export/Artifact commit, broader network/secrets, HTTP/React mutation, Rust analyzers, and CTF solving remain deferred.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so startup correctly fails closed with `migration 30 checksum or name mismatch`. The v60 slice did not modify migrations 1-59, and fresh/upgrade fixtures plus isolated `CYBERAGENT_HOME` runs pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
