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

## Current Baseline

- Architecture completion: about 98%; the V2 run-centric control plane is about 99% complete.
- Product usability: about 45-50% for the complete Code + Cyber product.
- Generic coding-agent workflow usability: about 40%.
- Cyber autonomous-workflow usability: about 20%.
- These are engineering estimates based on tested roadmap slices, not performance benchmarks. Do not reuse the retired single-axis "overall product vision" percentage.
- Database schema: v53.
- `README.md` carries the canonical bilingual schema timeline in strict `v1 -> v53` order. `internal/store/readme_history_test.go` binds its row count and ordering to `LatestSchemaVersion`, so a future migration cannot silently leave the public history missing or out of sequence.
- Main languages: Go control plane, TypeScript React/Vite read console; Rust has not started.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, parent-selected minimal Specialist Skill context, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with operator selection, safe-boundary operator steering with pending-only cancellation and explicit drain, strict metadata-only Sandbox Manifest preparation, approval-bound disabled candidates, a disabled Artifact-bound Sandbox lifecycle with independent fencing/cancellation/cleanup recovery, a required-but-unverified backend/output preflight, simulation-only backend evidence plus an atomic in-memory output transaction, and fixed-local-endpoint read-only Docker metadata observation, loopback read API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and a React/Vite read console.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- A Specialist receives at most one parent-selected built-in Skill guide. Assignment text, model output, and external content cannot choose or widen that subset.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local/Docker command execution is disabled.
- `sandbox_manifest.v1`, `sandbox_execution_candidate.v1`, `sandbox_execution.v1`, `sandbox_preflight.v1`, `sandbox_backend_evidence.v1`, `sandbox_output_simulation.v1`, and `sandbox_docker_observation.v1` are evidence, never execution permits. Schemas v48-v53 fix backend, execution, start, export, and production Artifact-commit capabilities to false even after exact operator approval.
- Sandbox execution ownership uses a separate generation-fenced lease. The initial lease can only prepare a disabled record; cleanup can recover after Run termination, but stale generations cannot commit.
- Input Artifacts are reverified by exact Run/Session/Workspace, digest, size, MIME, source, stream, order, and a 16 MiB aggregate cap. v50 stores no Artifact body or raw output path.
- The v51 backend handshake is disabled, container identity is unbound, and all 16 threat-model checks remain required/unverified/not-probed. Output slots store only opaque locator fingerprints and cannot export or commit Artifacts.
- The v52 fake client never contacts Docker. Its 16 `simulated_pass` items remain unverified and production-untrusted; the output harness commits only to an in-memory fake and must leave `run_artifacts` unchanged.
- The v53 Docker observer exposes only fixed-endpoint GET operations. It has no create/start/run/exec/pull/remove method, ignores `DOCKER_HOST`, stores no raw daemon/socket/repository identity, and cannot turn metadata observation into production verification or execution authority. Private-mount support remains explicitly unobservable through this read-only protocol.
- The Web UI is read-first. Its bearer remains in memory and never belongs in URLs or browser storage.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Latest Completed Slice

Schema v53 adds a production-observation protocol without adding production execution. `DockerReadOnlyTransport` contains only endpoint metadata, ping, daemon version/info, and digest-bound image inspection. Linux uses the fixed `/var/run/docker.sock` endpoint with proxies and redirects disabled and an allowlist of four GET shapes; Windows returns a bounded unsupported-transport observation. The code does not call the Docker CLI, accept `DOCKER_HOST` or arbitrary endpoints, pull an image, or expose any container mutation method.

An observation requires explicit CLI confirmation and exact v52 evidence/output-simulation/Manifest bindings. Application and SQLite independently revalidate the full v48-v52 authority chain, current Policy/approval, cumulative budgets, Run/Sandbox leases, input Artifacts, and cancellation/cleanup state before an immutable root, six ordered items, and digest-only operation are committed. Same-intent retries never reprobe, cross-Store races converge, and one simulation accepts at most eight observations.

Results are limited to complete observation, daemon unavailable, or image unavailable. Complete means only that daemon and image metadata were read. Private-mount support is fixed to `not_observable_read_only`; production verification, backend availability/enabling, execution, and Artifact authority remain false. Raw daemon ID/name/root, socket, security options, image identity/RepoDigests, Manifest, command, operation key, and private lease data do not enter SQLite events or CLI output.

Focused transport, Application, Store, migration, concurrency, cancellation, authority, privacy, limit, and CLI tests pass. The final local gate also passes full ordinary/race tests in 125.4s/140.7s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, repository scans, diff checks, and an isolated real-binary full-chain smoke. The smoke rejects an unconfirmed probe, records only `transport_unsupported` on Windows, converges replay without a second probe, creates zero production Artifacts, and leaks no private Docker field. The audit fixed low-risk concurrent-result identity comparison and added a defense-in-depth HTTP path/final-request/media-type allowlist; two-Store contention passes 20 ordinary and 10 race repetitions. Initial GitHub Actions run `29368112083` had TypeScript pass in 22s but Linux Go fail because the CLI unit test contacted the runner's real daemon. The test now injects a deterministic unavailable observer through an in-process App configuration hook; production defaults and the opt-in integration test remain real. Local full ordinary/race and static gates pass after the fix; CI awaits its commit. No unresolved high/medium issue is known. No real Provider, Shell, Local/Docker execution, network action, production Artifact export, or CTF solving has been enabled.

## Next Slice

Continue P6 without contacting a write-capable daemon path:

1. Define a schema-v54 deterministic Docker container-spec compiler that converts an exact v53-complete authority chain into a bounded, reviewable create/start/stop/export plan in memory only.
2. Prove the compiled specification fixes non-root identity, read-only root and inputs, one dedicated output mount, private propagation requirements, network default-deny/exact allowlist, secret lifecycle, CPU/memory/PID/time/kill limits, labels, and orphan-recovery identity without submitting it.
3. Add a fake write-transport transaction and crash/cancel rollback tests while keeping real create/start/pull/exec/remove unreachable.
4. Continue revalidating the Manifest and all v48-v53 bindings at every boundary. Neither v52 simulation nor v53 metadata observation is authorization or proof that a container control works.
5. Keep HTTP/React mutation, Rust analyzers, real Local/Docker execution, production Artifact export, and CTF solving deferred until their dedicated audits pass.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so startup correctly fails closed with `migration 30 checksum or name mismatch`. The v53 slice did not modify migrations 1-52, and fresh/upgrade fixtures plus isolated `CYBERAGENT_HOME` runs pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically.

## Delivery Loop

For every completed slice: run focused and full tests, race/vet/static analysis, security and credential checks, update README/status/progress/task memory, audit the diff, commit on `main`, push to GitHub, and verify CI. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
