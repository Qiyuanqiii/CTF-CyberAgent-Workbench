# CyberAgent Workbench Project Memory

Last updated: 2026-07-19

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
26. `docs/adr/0021-recoverable-runtime-input-application.md`
27. `docs/adr/0022-retained-runtime-input-resource-lifecycle.md`
28. `docs/adr/0023-blocked-docker-start-gate-review.md`
29. `docs/adr/0024-strict-inert-skill-package.md`
30. `docs/adr/0025-protected-delete-command-guard.md`
31. `docs/adr/0026-run-execution-profile-selection.md`
32. `docs/adr/0027-non-authorizing-docker-production-evidence-ledger.md`
33. `docs/adr/0028-recoverable-docker-production-evidence-attempts.md`
34. `docs/adr/0029-bounded-linux-read-only-docker-evidence-harness.md`
35. `docs/adr/0030-immutable-docker-production-evidence-review.md`
36. `docs/adr/0031-content-addressed-inert-skill-registry.md`
37. `docs/adr/0032-external-skill-run-context.md`
38. `docs/adr/0033-pathless-desktop-skill-preview.md`
39. `docs/adr/0034-embedded-read-first-wails-shell.md`
40. `docs/adr/0035-desktop-lifecycle-and-event-resumption.md`
41. `docs/adr/0036-idempotent-controlled-run-creation.md`
42. `docs/adr/0037-controlled-session-message-submission.md`
43. `docs/adr/0038-idempotent-run-control-and-bounded-handoff.md`
44. `docs/adr/0039-model-plan-and-approval-controls.md`
45. `docs/adr/0040-provider-diff-wake-controls.md`
46. `docs/adr/0041-explicit-wake-file-apply-and-inert-skill-install.md`
47. `docs/adr/0042-receipts-explorer-portable-build.md`
48. `docs/adr/0043-workspace-search-evidence-attachment-receipt-history.md`
49. `docs/adr/0044-operator-action-center-evidence-inventory-command-palette.md`
50. `docs/DESKTOP_PLAN.md`
51. `docs/SKILL_PACKAGE_PLAN.md`

## Current Baseline

- Architecture completion: about 99%; the V2 run-centric control plane is about 99% complete.
- Product usability: about 84-87% for the complete Code + Cyber product.
- Generic coding-agent workflow usability: about 82%.
- Cyber autonomous-workflow usability: about 20%.
- These are engineering estimates based on tested roadmap slices, not performance benchmarks. Do not reuse the retired single-axis "overall product vision" percentage.
- Database schema: v77.
- `README.md` carries the canonical bilingual schema timeline in strict `v1 -> v77` order. `internal/store/readme_history_test.go` binds its row count and ordering to `LatestSchemaVersion`, so a future migration cannot silently leave the public history missing or out of sequence.
- Main languages: Go control plane, TypeScript React/Vite local console; Rust has not started.
- Desktop status: D0-A/D0-B and D1-R1 through D1-I1/M3/J1 pin Wails v2.13.0 and build a reproducible Windows development/portable-test binary with an embedded React bundle, in-process Go API, ephemeral memory-only tokens, resumable event polling, same-database recovery, controlled Run/Session/lifecycle/Plan/approval workflows, explicit model diagnostics/routes, a local Monaco proposal/Diff editor over Go-issued source handles, Windows Credential Manager Provider controls with status-only readback, and a default-off one-concurrent/one-step wake worker. Existing receipt, Workspace Files/search, evidence, action-center, and command-palette controls remain. Capability-only launches cannot reach sibling routes. Automated PE/hash/build diagnostics pass, but Windows 10 release coverage remains pending and `release_ready=false`. It has no installer, formal signed release, registry/startup/update behavior, terminal, real Shell/Local/Docker process execution, or install-time Skill execution. See ADR 0033 through ADR 0045 and `docs/DESKTOP_PLAN.md`.
- Custom Skill status: the five embedded `skill.v1` guides and explicitly selected external packages are Run-loadable through separate protocols. Schema v69 adds persistent content-addressed import/history; schema v70 adds a second explicitly confirmed exact Run selection and redacted user-role root/Specialist context; schema v71 adds bounded read-only provenance across HTTP/TUI/Web. D1-A adds a pathless, one-time-handle preview boundary; D1-B1 adds explicit HTTP/Desktop registration through the same inert Registry. External packages remain untrusted and grant no declared tools. Installation executes no content and still does not select a package for a Run. See ADR 0024, ADR 0031 through ADR 0033, ADR 0041, and `docs/SKILL_PACKAGE_PLAN.md`.
- Protected-delete status: explicit recursive, absolute/traversing/wildcard, environment-derived, command-substituted, current-home, PowerShell/`cmd`, and common interpreter deletion intents are permanently denied before approval across Shell, ScriptProcess, and Sandbox Policy. This is defense in depth; Local/container process execution remains disabled and a future executor still requires OS/container isolation. See ADR 0025.
- Canonical branch: `main`; do not create a branch or PR unless the user asks.
- Canonical remote: `Qiyuanqiii/CTF-CyberAgent-Workbench`.

Implemented foundations include resumable RunSupervisor turns, SQLite checkpoints and execution leases, model streaming/retry/cancellation, one Go Provider Registry with persisted routes, redacted availability, and environment/system-credential sources, WorkItems/Notes/context compaction, Tool Gateway and durable approvals, source-bound Artifacts, a stable root Agent, review-gated two-child Specialist scheduling, parent-selected minimal Specialist Skill context, separate 1/2/4/6 read-only Fan-out, immutable Finding/Evidence/Report lifecycles, SARIF/CI output, Go-owned Code/Cyber plus Plan/Deliver modes, strict three-direction Plan proposals with explicit operator selection and separate Deliver transition, safe-boundary operator steering with pending-only cancellation and explicit drain, controlled HTTP/Desktop submission into that existing queue, constrained metadata-only approval decisions, embedded Skills, strict external-package validation, a content-addressed inert user Skill Registry, explicitly confirmed external-Skill Run pinning with redacted root/Specialist delivery, pathless Desktop Skill preview plus confirmed inert registration, Go-issued Monaco FileEdit proposals followed by independently authorized review/apply, explicit foreground wake consumption plus a default-off 1 x 1-step worker, a bounded operator action center, metadata-only attached-evidence inventory, navigation/refresh-only command palette, a Windows Wails shell with embedded React, an in-process Go API, same-database lifecycle recovery, shared SSE/poll high-water cursors, fail-closed WebView2/origin handling, and idempotent closed Run creation, plus the existing non-authorizing Sandbox/Docker evidence architecture, loopback API/SSE/OpenAPI, Headless NDJSON, Run-first Bubble Tea TUI, and React/Vite console.

Schema v73 and non-schema D1-S2 additionally complete independent pending-only HTTP/Desktop steering cancellation, digest-idempotent Run start/pause/resume, and an at-most-eight-item frozen execution handoff through the existing RunSupervisor. These controls add no Desktop-native worker and no Local/Docker/Shell process authority; ADR 0038 records the boundary.

Non-schema D1-M1/P1/A1 adds one no-probe redacted Provider/model Registry projection, separate Plan-selection and Deliver controls, and a bounded metadata-only durable approval queue. Approval rechecks Policy and remains dry-run/process-disabled; it cannot create a Grant, write a file, or start Shell/Local/Docker. OpenAPI is 36 paths/84 schemas/27 GET/11 control POST. ADR 0039 records the boundary.

Non-schema D1-M2/D1-D1 and schema-v74 D1-Q1 add explicit content-free Provider diagnostics plus persist-before-memory route selection, exact body-free Diff review without apply authority, and durable bounded wake/retry intent with one generation-fenced owner. The wake API manages intent only and starts no background worker, Run, model, tool, or process. ADR 0040 records the boundary.

Schema-v75 D1-Q2, schema-v76 D1-D2, and non-schema D1-B1 add one explicit foreground wake consumer, a separately authorized current-hash/Policy-checked FileEdit apply, and confirmed HTTP/Desktop import into the inert Skill Registry. D1-U1/E1/W1 then add durable receipts, bounded Workspace exploration, and reproducible portable diagnostics. Schema-v77 D1-E2/C1/U2 adds bounded search, separately gated non-authorizing evidence attachment, and refreshable terminal receipt history. Non-schema D1-O1/C2/K1 adds a bounded operator action center, metadata-only attached-evidence inventory, and navigation/refresh-only command palette. These add no hidden worker, renderer host-path/body input, document authority, install-time execution, or general host/container process authority. ADR 0041 through ADR 0044 record the boundaries.

Non-schema D1-I1/M3/J1 adds a locally bundled Monaco proposal/Diff editor over a five-minute Go source handle, Windows Credential Manager-backed Provider setup with status-only UI responses, and a default-off process-lifetime wake worker capped at one due intent and one Supervisor step per tick. Proposal/review/apply and model/credential/wake capabilities remain independent. No renderer host path, credential readback, Tool Runner, Shell, LocalRunner, or Docker authority is added. ADR 0045 records the boundaries.

Non-schema D1-U1/E1/W1 adds `operation_receipt.v1` for apply/wake/install,
hash-and-age constrained FileEdit staging recovery, and read-bearer
`workspace_explorer.v1` with canonical Go path resolution, bounded redacted UTF-8, and
non-authorizing provenance. React can navigate only Go-issued exact child paths and
renders file bodies as plain text. `cyberagent doctor portable` plus the Windows build
scripts provide deterministic release metadata, PE/COFF/module checks, and consecutive
binary hash verification. The tested double-build SHA-256 is
`33fb9ca3064df98191ac50b2a3ef9431e1b5c81abe8c610d4be15db113cdf1ef`.
Automated checks pass, while the manual Windows 10/WebView2 matrix correctly keeps
`release_ready=false`. ADR 0042 records the boundary.

## Security Invariants

- Go owns Policy, scope, budgets, state transitions, Docker/process control, API-key access, and file permissions.
- `model_availability.v1` is a deterministic no-probe projection. API keys, Base URLs, environment-variable names, clients, and raw errors never enter it; secret-like or malformed model/route identifiers fail closed or are redacted.
- `provider_diagnostic.v1` is explicit and content-free. Each invocation may make one bounded model request, but model text, secrets, endpoints, environment-variable names, clients, and raw errors never enter the result. Route persistence succeeds before the in-memory Router changes.
- `provider_credential.v1` is status-only after mutation. Windows stores exact supported Provider keys in Credential Manager with a 2,560-byte ceiling; plaintext never enters SQLite, events, logs, model context, frontend persistence, or any response. Non-Windows platforms have no plaintext fallback, environment variables keep priority, and a Registry restart is currently required.
- File-edit review exact-binds Run/Mission/Session/Workspace/proposal/approval and returns metadata plus bounded redacted Diff only. `approve_intent` never writes a file. Schema v76 apply is a separate capability that rechecks Policy, Workspace resolution, original/current hash, target hash, and idempotent result; browsers submit neither path nor body.
- `file_edit_proposal.v1` accepts only a five-minute opaque handle and replacement text after Go has issued complete, untruncated, unredacted UTF-8 for the exact running Run/active Session/Workspace. The handle is one-intent, current hash and Policy are rechecked, and the result remains pending without a file write.
- `operation_receipt.v1` is a content-free projection of an already durable apply/wake/install result. Go owns the outcome/replay/retry/cleanup tuple; TypeScript cross-checks it against the enclosing response and cannot invent recovery authority. Staging cleanup is restricted by exact directory, reserved prefix, age, ordinary-file identity, size, and approved content hash.
- `workspace_explorer.v1` treats repository content as evidence rather than authority. Go alone resolves the registered root and canonical relative path, refuses links/redirects/traversal/ambiguous names, redacts and bounds UTF-8, and emits `instruction_authorized=false`. The DTO never includes the root; React cannot submit an arbitrary host path or execute Markdown/HTML from file content.
- `workspace_search.v1` searches only bounded redacted Explorer projections. It has fixed directory/entry/file/result/read ceilings, no indexer or watcher, and returns canonical references plus false-authority provenance only.
- `session_evidence_attachment.v1` exact-binds a reprojected Workspace file to the running/paused Run and active Session. Go and SQLite require a tool-role message with `instruction_authorized=false`; document text cannot become an operator instruction, approval, Scope change, or capability grant.
- `operation_receipt_history.v1` is a bounded terminal projection with opaque IDs. It exposes no operation key/digest, path/content hash, requester, archive metadata, or private lease; FileEdit staging inspection is read-only and uncertainty remains `pending_review`.
- Schema v74 wake intent is not authority by itself. Schema v75 can consume one due intent through the existing bounded RunSupervisor handoff. D1-J1 optionally automates that same consumer only after an explicit process startup flag and control token; one serial owner may consume at most one intent and one step per tick, with no Tool Runner or Shell/Local/Docker dependency.
- Plan direction selection and Deliver transition are separate operator operations. Neither can start/resume execution, call a model/tool, acquire a lease, or grant capability.
- Desktop/Web approval can only deny or approve-once under a fresh Policy check. Shell is dry-run, ScriptProcess is process-disabled, replace-file is deny-only, permanent denial is non-overridable, and no Session Grant, file write, or real process can result.
- Core Specialist delegation is capped at two children and requires explicit operator review, application, and scheduling.
- A Specialist receives at most one parent-selected built-in Skill guide. Assignment text, model output, and external content cannot choose or widen that subset.
- `skill_package.v1` is accepted only as bounded untrusted input to a pure in-memory validator. Schema v69 may persist an explicitly confirmed validated archive and metadata, but import never selects, executes, injects context, calls a Provider/network/tool, or grants declared dependencies. Object reads revalidate archive and semantic identity; every authority bit remains false.
- Schema v69 stores external archives by SHA-256 behind non-executing write/verify and read-only loader interfaces plus immutable installation/result/removal ledgers. Code and Cyber catalogs are separate, Cyber accepts only `script`, built-in names are reserved, and removal retains bytes. See ADR 0031.
- Schema v70 requires a second explicit confirmation to pin one to four exact active packages to a created Run. At most one item is operator-designated for Specialist delivery. Every load revalidates the exact object and Manifest, redacts secrets, obeys separate root/child budgets, and appears only in a user-role untrusted-guidance envelope. SQLite/events store metadata only; first-call commits are atomic; Policy/tool/Shell/network/secret/scope/delegation authority remains false. Pinned installations cannot be removed in Go or SQL. See ADR 0032.
- Desktop exposes exactly four Wails-bound methods: connection bootstrap, native Skill selection, pathless preview, and handle-only inert installation. All other controls, including Provider credentials, FileEdit proposals, and wake automation, remain in the in-process Go HTTP Handler. It opens no TCP listener; embeds one validated production renderer; keeps tokens, retry keys, confirmation/source handles, transient password input, and its bounded 16-Run/500-frame event cache only in memory; defaults to read-only; and accepts no renderer host path or archive bytes. Independent flags gate every control class. The optional worker is process-lifetime and 1 x 1-step only; no capability creates a Grant, install hook, or general host/container process authority. See ADR 0034 through ADR 0045.
- The 1/2/4/6 Fan-out pool is separate, read-only, tool-free, network-free, write-free, and creates no Agent.
- Dangerous cyber commands remain permanently denied; approval cannot override permanent Policy denial.
- Protected or unresolved deletion through executable Shell/ScriptProcess/Sandbox intent is a critical permanent denial. Non-executable evidence is not reclassified as a command, and passing the classifier can never authorize host execution.
- External files, repository text, logs, web/mail, tool output, and memory are untrusted evidence with `instruction_authorized=false`; they never become system/assistant authority through persistence or compaction.
- Shell and ScriptProcess approval paths are dry-run only. Real Local and container-process command execution is disabled.
- `run_execution_profile.v1` records only operator intent. Every profile fixes `process_enabled=false`, `execution_authorized=false`, and `capability_grant=false`; Docker still requires its production start gate and Local still requires an unimplemented OS-sandbox gate. Selection is allowed only for `created` or quiescent `paused` Runs and cannot be widened by TypeScript, a model, a child Agent, or approval.
- `sandbox_docker_production_evidence.v1` accepts no caller-supplied conclusion, report, endpoint, socket, path, image, resource, container identity, or raw daemon response. Windows and Linux without explicit opt-in never contact a daemon. The v67 Linux harness may produce `capture_complete` only through its durable read-only protocol; all sixteen items remain insufficient and every start/process/export/Artifact authority bit remains false.
- Schema v66 commits an immutable production-evidence attempt, digest-only operation, and generation lease before collector invocation, then requires a current-generation quiescent reconciliation checkpoint. Failure and completion are fenced to the full private lease identity; released/expired attempts resume only at generation N+1. The checkpoint records zero daemon reads/resources and is not production resource verification. SQL rejects new v65 evidence operations without an attempt result. Legacy evidence gets no fabricated attempt, CLI omits lease IDs/owners, and all daemon/start/process/export/Artifact authority remains false. See ADR 0028.
- Schema v67 requires a second immutable harness intent and a daemon-aware empty-scope reconciliation before four capture GETs. The fixed Linux local transport performs exactly one labeled container-list GET plus `_ping`, `version`, `info`, and exact pre-existing digest inspect, each with a four-second bound and no pull/mutation method. Its result fixes all sixteen checks to `observed_failed`, `production_verified_count=0`, and zero start/process/output/Artifact authority. A persisted intent cannot downgrade to the v66 inert result, restart must reconcile under the current generation, and CLI/events persist no resource ID, socket, payload, path, or private lease identity. See ADR 0029.
- Schema v68 records one explicit `accepted|rejected` decision for an exact completed v67 receipt. Acceptance uses only `metadata_scope_accepted`; rejection uses one of five bounded reason codes, with no free-form body. The operation/review pair is atomic and immutable, migration fabricates no decisions, and both Go and SQL preserve zero production verification, sixteen blockers, and false start/process/output/Artifact authority. The review path has no Docker or process dependency. See ADR 0030.
- `sandbox_manifest.v1`, `sandbox_execution_candidate.v1`, `sandbox_execution.v1`, `sandbox_preflight.v1`, `sandbox_backend_evidence.v1`, `sandbox_output_simulation.v1`, `sandbox_docker_observation.v1`, `sandbox_docker_container_plan.v1`, `sandbox_docker_container_rehearsal.v1`, `sandbox_docker_container_rehearsal_attempt.v1`, `sandbox_docker_host_input_staging.v1`, `sandbox_docker_host_input_requirement.v1`, the v59 handoff, v60 projection plan, v61 runtime-input application, v62 retained-resource lifecycle, and v63 start-gate review are evidence, preparation, cleanup, or design facts, never process-execution permits. Schemas v48-v63 fix execution, start, export, and production Artifact-commit capabilities to false even after exact operator approval.
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
- The v61 application requires separate operator and daemon-write confirmations plus a durable intent and independent generation lease before Docker mutation. It revalidates v48-v60 and recaptures the exact sealed input, then uses a fixed local-Unix allowlist to create deterministic volumes/carriers, upload only to `/cyberagent-input`, verify daemon readback, and attach every input read-only/`NoCopy` to one fully inspected never-started target. Retry and bounded cleanup touch only full authority matches; foreign collisions fail closed, stale generations cannot commit, and operations stop early enough to reserve cleanup before takeover. Durable output contains no paths, targets, file/resource names, raw IDs, archives, sockets, raw keys, or private lease identities. `volumes_applied_target_never_started` grants no start, exec, output, backend, execution, or Artifact authority.
- The v62 resource lifecycle requires an explicit read-only inspection before a separately dual-confirmed cleanup. Descriptor reconstruction revalidates v48-v61 but never recaptures the input bundle. Complete never-started/read-only/`NoCopy` evidence is true only when the exact target and every volume are present. Cleanup intent and generation lease commit before Docker access; all resources are preflighted before any DELETE, a foreign collision causes zero DELETE, the target is removed by inspected ID before exact volumes, and final inspection requires total absence. Failure release, takeover, stale-worker fencing, and semantic replay are durable. No names, IDs, paths, sockets, raw keys, or private leases persist, and `exact_owned_resources_absent` grants no start, exec, output, backend, execution, or Artifact authority.
- The v63 review requires completed v62 cleanup, a resupplied Manifest, a stable digest-only operation identity, and explicit design-review confirmation. It maps all sixteen v51 checks to fixed evidence classes, sources, blockers, and future gates, while every check remains unverified and insufficient. Its eleven-transition process blueprint requires generation-fenced single ownership, write-ahead state, fixed endpoint, bounded logs, wait, TERM/KILL escalation, cancellation fan-out, and orphan reconciliation, but every transition remains unimplemented and unauthorized. The only outcome is `blocked/deny_start`; the path has no daemon transport, input capture, process, output-export, or Artifact authority.
- The Web/Desktop UI is read-mostly. Its read and optional distinct control bearers remain in memory and never belong in URLs or browser storage. The control bearer may select a non-authorizing Run execution profile, create a closed schema-v72 Run, or, under a separately enabled capability, enqueue one bounded message for an exact Run-bound Session. It cannot start/resume/drain a Run, acquire a lease, call a model/tool, approve an action, cancel/reorder steering, start a process, or read API resources.
- Provider keys are read from process environment only and must never enter Git, SQLite, events, or logs.

## Completed Sandbox Slice History (Latest: v68)

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

The final local gate passed full ordinary/race suites in 198.9s/194.0s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 17 tests across 8 frontend files, OpenAPI drift, production build, zero-vulnerability npm audit, repository privacy/artifact/encoding/Markdown scans, Linux sandbox cross-compilation, diff checks, and an isolated real-binary schema-v60 Workspace smoke. Compiler/Store/Application/CLI repetitions passed at 50/30/20/10, and the critical Sandbox/Store/Application race set passed 10 rounds. GitHub Actions run `29428011306` passed feature commit `cc92421` with Go/Linux in 2m48s and TypeScript in 24s. This Windows host cannot run the v59 handoff or now-implemented v61 application real-daemon harness; no start authority follows from v60.

Schema v61 adds recoverable runtime-input application without process execution. `ApplyDockerRuntimeInputs` requires two explicit confirmations and commits an immutable intent plus an independent generation lease before recapture or daemon access. It then revalidates the complete v48-v60 chain, recompiles the same container specification, re-resolves the writable output bind, recaptures v57 bytes, and requires the v60 projection facts to reproduce exactly. Failed generations append only typed metadata, release atomically, and can be resumed; completion and failure are fenced to the current lease. A completed operation replays without provider or daemon effects.

The Unix transport is fixed to Docker API `1.40` on the local socket. For every projection it creates an authority-labelled local volume and never-started carrier, writes the canonical archive only at `/cyberagent-input`, reads it back, verifies exact expected tar entries, modes, and content, and removes the carrier. It then creates and exactly inspects one retained target whose input volumes are all read-only/`NoCopy`; the reviewed output bind is the sole writable mount. Existing exact residue is reconciled, foreign collisions are preserved and rejected, failure cleanup runs independently, and daemon work stops before lease expiry with cleanup time reserved. The transport exposes no start, exec, attach, logs, export, pull, build, network mutation, forced volume delete, or arbitrary endpoint.

SQLite v61 stores intent (including its unique digest-keyed operation binding), lease, bounded failure, immutable result, and events as metadata only; no separate operation table is needed. CLI adds `docker-runtime-input-apply`, `docker-runtime-input-apply-resume`, `docker-runtime-input-applications`, and `docker-runtime-input-application-show`; neither persistence nor output contains raw target/path/file/resource names, IDs, archives, sockets, operation keys, or private lease identities. Focused Sandbox, Store, Application, CLI, migration, SQL immutability, privacy, stale-worker, takeover, replay, collision, readback, cleanup, allowlist, and Windows unsupported tests pass. `volumes_applied_target_never_started` remains false for start, process, export, backend, execution, and Artifact authority. ADR 0021 is the recovery boundary.

The v61 final local gate passed full ordinary/race suites in 197.5s/316.8s, vet, zero-warning staticcheck, module verification, zero-finding govulncheck, strict TypeScript, 17 frontend tests, OpenAPI drift, production build, zero-vulnerability npm audit, repository privacy/process/endpoint/encoding/Markdown scans, diff checks, Linux sandbox test-binary cross-compilation, and isolated schema-v61 real-binary smoke; Sandbox race tests passed 20 repetitions. The audit tightened per-volume readback limits, lease cleanup reserve and time validity, Application-level resume confirmations, cancellation-safe typed failure persistence, narrow v55/v59/v61 transport interfaces, daemon-returned `RW`/`NoCopy` evidence, and operation-digest syntax. No unresolved high/medium issue is known. The Windows host cannot execute the opt-in Linux v59/v61 real-daemon harnesses, so start remains blocked. GitHub Actions run `29437941378` passed feature commit `f4aaf7a` with Go/Linux in 2m37s and TypeScript in 27s.

Schema v62 adds immutable metadata-only inspection for the v61 retained target and volumes, plus a separate recoverable exact-owned cleanup. Inspection requires explicit read confirmation, reconstructs the exact resource descriptor from current durable authority without input recapture, and records complete, partial/absent, or unsafe foreign-collision state. Only a complete exact target and all exact volumes establish never-started/read-only/`NoCopy` evidence; unsafe evidence is persisted and returned as a failed precondition.

Cleanup requires its own operator and daemon-write confirmations and a cleanup-eligible inspection. An immutable intent and active generation lease commit before transport use. The fixed local-Unix implementation preflights every target/volume before any DELETE, performs zero DELETE after a foreign collision, removes the target by inspected ID before exact volumes, and rechecks total absence. Bounded failure codes release the lease; later generations recover while stale workers are fenced. Completed operation and resume replay are metadata-only. Windows exposes distinct narrow unsupported inspector/cleanup capabilities and never falls back to host execution.

Focused Sandbox, Store, Application, CLI, migration, SQL immutability, privacy, replay, failure/takeover, and platform tests pass. The audit corrected read-only/`NoCopy` overclaiming for partial or unsafe inspections, made resource-cleanup event names unambiguous, exposed foreign-collision failure truthfully in CLI output, rejected future/out-of-window terminal timestamps, made v61/v62 lease rows undeletable, and extended the Linux opt-in v57/v59/v60/v61 harness through v62 cleanup. No high/medium issue is currently known. ADR 0022 records the boundary.

The v62 final local gate passed full ordinary/race suites in 313.6s/329.6s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 17 frontend tests across 8 files, OpenAPI/build/npm audit, repository privacy/capability/encoding/Markdown scans, diff checks, Linux sandbox test-binary cross-compilation, isolated schema-v62 real-binary smoke, and Sandbox/Store/Application/CLI stress repetitions of 20/15/10/10. GitHub Actions run `29444398815` passed feature commit `d250d32` with Go/Linux in 2m35s and TypeScript in 20s. The Windows host still cannot execute the opt-in Linux v59/v61/v62 real-daemon chain, so no start or process-isolation claim follows.

Schema v63 adds the immutable design-only Docker start-gate review. It requires a completed v62 cleanup, exact Manifest resubmission, and explicit operator confirmation, then revalidates the v48-v62 authority chain without input recapture or daemon contact. All sixteen v51 checks are stored with fixed evidence class/source, blocker code, and future gate; all remain `production_verified=false` and `sufficient_for_start=false`. The same transaction freezes an eleven-transition process lifecycle blueprint with per-Run generation-fenced single ownership, write-ahead intent, fixed endpoint, cancellation fan-out, bounded logs, wait, graceful/forced termination, and orphan reconciliation. Every transition is unimplemented and unauthorized, and every process/output/Artifact authority bit is false. Store migration creates no historical reviews; operation replay, cross-Store convergence, SQL immutability, fingerprint tamper detection, CLI privacy, and no-provider/no-daemon behavior have focused coverage. ADR 0023 records this boundary.

The v63 final local gate passed the final code's full ordinary/race suites in 196.9s/212.3s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 17 frontend tests across 8 files, OpenAPI/build/npm audit, repository credential/secret-file/process-entry/encoding/diff scans, Linux sandbox test-binary cross-compilation, isolated schema-v63 real-binary Workspace/Skill smoke, and Sandbox/Store/Application/CLI stress repetitions of 20/15/10/10. The audit added immediate `Rows.Err` handling for v63 child tables and lower-case internal error strings for static rules. No unresolved high/medium issue is known. Desktop and custom-Skill package changes are planning-only and add no binary dependency, import surface, installer, registry mutation, or authority. The Linux real-daemon chain remains unexecuted on this Windows host, so every v63 blocker remains unresolved. GitHub Actions run `29503856229` passed commit `e25a2ab` with Go/Linux in 2m32s and TypeScript in 24s.

The docs-only run `29444664401` exposed an npm-registry advisory-endpoint outage after `npm ci` had already reported zero vulnerabilities; GitHub then left the completed runner marked in progress. CI now retries `npm audit --audit-level=high` at most three times with bounded delay. A real high-severity finding still fails every attempt and blocks the workflow.

The first non-schema `skill_package.v1` slice adds a strict pure-memory ZIP parser, immutable metadata preview, canonical semantic fingerprint, raw archive digest, adversarial tests, and fuzzing. The only product entry is `skill package validate`: it performs a bounded regular-file read with symlink and identity-change rejection, prints no body/source path, creates no database, and keeps install, command, network, Provider, tool, and capability authority false. The accepted profile is exactly two ordered Deflate entries (`manifest.json`, `SKILL.md`) with fixed ZIP 2.0 data descriptors, zero metadata, no prefix/gaps/tail, a 64 KiB archive cap, bounded decompression/ratio, CRC/header agreement, and the existing strict `skill.v1` semantic checks. ADR 0024 records the boundary. This slice adds no migration, user Registry, import/install command, Run selection, or Desktop/HTTP upload.

The final package-validation gate passed full ordinary/race suites in 239.4s/226.8s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 20 seconds and about 26.45 million parser fuzz executions, 78.5% `internal/skills` statement coverage, 100 parser and 20 CLI repetitions, strict TypeScript, 17 frontend tests across 8 files, OpenAPI/build/npm audit, and credential/runtime-artifact/replacement-character/Markdown-link/diff scans. The audit pinned the central-directory creator version and exact Deflate-stream exhaustion to close hidden post-stream payloads, replaced a deprecated test fixture API, and wrapped filesystem causes behind stable path-free CLI errors. Synthetic redaction fixtures are the only key-pattern scan matches. No unresolved high/medium issue is known. GitHub Actions run `29512332025` passed commit `55b3fae` with Go/Linux in 3m4s and TypeScript in 20s.

The non-schema protected-delete slice adds a path-free critical permanent Policy decision before approval for explicit recursive, absolute/traversing/wildcard, environment-derived, command-substituted, current-home, PowerShell/`cmd`, and common interpreter deletion forms. Raw Shell and decoded ScriptProcess/Sandbox intents share the guard; argument-map ordering is deterministic, ordinary evidence remains non-executable, and denied proposals cannot acquire a dry-run result through operator approval. The focused audit fixed Node `require('fs').rmSync(...)`, leading `../`, and PowerShell `-Force` classification edge cases. ADR 0025 records that this classifier is only defense in depth: opaque scripts/build tools require OS/container isolation, and Local/container process execution remains disabled. That slice added no migration or authority and left schema at v63; schema v64 became the execution-profile control plane, and the Skill Registry was subsequently completed at schema v69 after the v68 evidence-review slice.

The final local gate passed the full ordinary suite in 197.0s and the full race suite in 222.6s, plus 20 repeated race runs across the Policy/Gateway/Application protected-delete paths, about 406,000 fuzz executions, 100/50/50 focused repetitions, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 17 frontend tests, OpenAPI/build/npm audit, and credential/runtime-artifact/replacement-character/Markdown-link/diff scans. The credential pattern scan found only six existing synthetic redaction fixtures. No unresolved high/medium issue is known. Local Linux cross-compilation was not repeated because the host command policy rejected the temporary-artifact cleanup command; GitHub Linux CI remains the platform proof after push.

## Completed Execution-Profile Slice (v64)

Schema v64 adds immutable `run_execution_profile.v1` snapshots and digest-only idempotency operations. New and migrated Runs default to `preview`; operators may select `preview`, `docker`, or `local` only while a Run is `created` or `paused` with no active execution lease. Domain validation, Store revalidation, SQLite checks/triggers, CLI output, HTTP DTOs, OpenAPI, generated TypeScript, and React all preserve the same fixed mapping and false authority bits. Transitions append `run.execution_profile_selected`; initial compatibility snapshots deliberately do not rewrite historical event sequences.

The local Web console accepts an optional distinct control bearer in page memory. That credential is never put in a URL, body, storage, or read request. The only newly exposed mutation is an idempotent profile selection; TypeScript submits only the profile enum and cannot submit backend, scope, network, gate, approval, process, execution, or capability fields. ADR 0026 records the boundary.

The v64 final local gate passed final-code full ordinary tests in 225.9s; the complete race suite passed in 196.9s, followed by targeted profile/HTTP race after the final DTO privacy reduction. Vet/staticcheck/module/govulncheck, strict TypeScript, 21 frontend tests, production build, npm audit, deterministic generated-contract hashes, chronology/link/privacy/artifact/encoding/diff checks, and isolated CLI smoke are green. GitHub Actions run `29523634340` passed implementation commit `8378419` with Go/Linux in 3m0s and TypeScript in 26s. Audit fixes were limited to generic control-request wording, six static error-style findings, and omission of requester/reason from browser DTOs. No unresolved high/medium issue is known. Linux real-daemon evidence was not run on this Windows host, so Docker start remains blocked.

A later real production-bundle smoke exposed one low-risk Web availability defect: Vite 8 emitted `index-D0TcvGy-.css`, whose trailing URL-safe hyphen defeated the old last-separator filename heuristic. `assetNameHasDigest` now searches backward for a bounded URL-safe digest; the primary bundle fixture uses the observed name and short/invalid suffixes remain denied. The exact built bundle then loaded under Go, served CSP-protected HTML, rendered on desktop and a 390x844 mobile viewport without horizontal overflow, and completed Docker-to-Preview UI selection while both execution authority bits remained false. The follow-up full suite passed in 201.1s, with 20 targeted race repetitions plus clean vet/staticcheck.

## Completed Docker Production-Evidence Slice (v65)

Schema v65 adds immutable `sandbox_docker_production_evidence.v1` aggregates, sixteen ordered probe items, and digest-only idempotency operations. Captures bind the exact v63 blocked review, Run/Mission/Workspace, authority and threat-model fingerprints, and the same operator. The CLI accepts only the review ID, bounded operation key, and explicit confirmation. It cannot accept evidence conclusions, JSON reports, sockets, paths, images, resources, container IDs, or raw daemon responses. One transaction stores the aggregate, all items, operation binding, and a metadata-only event; immutable SQL triggers, semantic replay, and a 32-capture-per-Run cap close mutation and unbounded-growth paths. Migration creates no historical receipts.

At the v65 delivery point, the local collector was deliberately inert. Windows returned `unsupported_platform`; Linux without `CYBERAGENT_DOCKER_PRODUCTION_EVIDENCE=1` returned `opt_in_required`; Linux with opt-in returned `harness_pending`. All three paths made zero daemon, network, Docker CLI, and process calls. The v65 Application hard-rejected `capture_complete` and `real_daemon_contacted=true` before persistence. Schema v66 later supplied the ownership boundary, and schema v67 now supplies the separately constrained read-only harness. Every probe remains `sufficient_for_start=false`, and all start/process/output/Artifact authority remains false. ADR 0027 records the ledger boundary.

Focused Domain, Store, Application, CLI, migration, SQL, idempotency, privacy, and malicious-collector tests pass. The final local ordinary suite passed in 212.3s and the full race suite in 213.9s. Vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 21 frontend tests, OpenAPI drift, production build, zero-vulnerability npm audit, credential/runtime-artifact/process-capability/diff scans, the canonical README history check, and an isolated real-CLI schema-v65 Workspace smoke are green. The audit fixed eleven static error-style findings, made multi-field identity validation deterministic, bound the SQL operation key directly to its evidence root, and rejected a future collector that could otherwise claim real-daemon contact before durable harness ownership. No unresolved high/medium issue is known. This Windows host did not run a Linux daemon harness because none exists in v65.

GitHub Actions run `29532551701` passed implementation commit `e97daf0`; Go/Linux completed in 2m47s and TypeScript in 20s.

## Completed Recoverable Production-Evidence Attempt Slice (v66)

Schema v66 adds immutable `sandbox_docker_production_evidence_attempt.v1` intents, digest-only operations, expiring generation-fenced leases, per-generation quiescent reconciliation checkpoints, bounded typed failures, and one immutable evidence result. The Application persists attempt ownership and the current checkpoint before collector invocation, derives a deadline from both the fixed 30-second capture bound and lease expiry, and releases the lease atomically with either failure or evidence completion. Released retries and expired takeovers advance generation; stale workers cannot commit. New SQL enforcement prevents the legacy Store path from creating a new v65 evidence operation without an attempt result, while legacy v65 receipts remain readable and receive no invented attempts.

CLI capture now reports the associated attempt and adds metadata-only `docker-production-evidence-attempts`, `docker-production-evidence-attempt-show`, and explicitly confirmed `docker-production-evidence-attempt-resume`. Lease IDs and owners, raw errors, sockets, paths, resource/container identities, and daemon payloads are not exposed. Focused tests cover collector-visible write-ahead ordering, active conflicts, released recovery, expired takeover, stale fencing, unsafe-contact failure, generation-two completion, SQL bypass rejection, migration compatibility, immutability, privacy, and replay without recollection. ADR 0028 records the boundary.

At the v66 delivery point, the Windows/Linux collector remained inert and no real daemon harness was executed. Its reconciliation checkpoint records only zero daemon reads and zero known resources; it is an ownership/order fact, not Docker production-resource verification. Schema v67 later adds a separate daemon-aware checkpoint while leaving every start, process, output, Artifact, backend, execution, and capability authority bit false. Architecture completion remains about 98% and product usability about 45-50%, because these slices improve evidence and recoverability rather than adding an end-user execution capability.

The final local gate passed the full ordinary/race suites in 206.9s/230.3s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 21 frontend tests, OpenAPI/build/npm audit, 50-file Markdown-link validation, repository credential/runtime-artifact/encoding/process-entry/diff scans, and focused Domain/Store/Application/CLI repetitions of 50/10/5/5 plus critical race repetitions of 5/3/3. An isolated real binary loaded only mock, migrated a separate runtime to schema v66, and initialized/listed a Workspace. Host protected-delete policy rejected recursive cleanup, so that directory remains under the OS temporary root for normal cleanup; no user action is required.

The audit fixed the v66 lease's missing immutable-delete trigger, constrained direct-SQL release to pre-expiry time and generation acquisition to the prior release/expiry chronology, and corrected trailing `--limit` parsing for the v65 capture list. No unresolved high/medium issue is known.

GitHub Actions run `29538732903` passed implementation commit `3e52b7d`; Go/Linux completed in 3m33s and TypeScript in 25s.

## Completed Linux Read-Only Production-Evidence Harness Slice (v67)

Schema v67 adds immutable harness intent, daemon-aware reconciliation, and result records behind the v66 attempt boundary. An explicitly opted-in Linux collector first proves that the exact attempt-labeled container scope is empty, then performs `_ping`, `version`, `info`, and inspect for the exact already-present image digest. The transport is fixed to the local Unix endpoint, ignores `DOCKER_HOST`, exposes no mutation method, never pulls, and permits at most five GETs with a four-second per-call bound inside the existing 30-second attempt deadline.

The v67 result is deliberately non-authorizing: all sixteen probes are `observed_failed`, `production_verified_count` is exactly zero, and every start/process/output/Artifact authority bit remains false. Go and SQLite bind intent, control reconciliation, daemon reconciliation, lease generation, evidence, and operation; a persisted v67 intent cannot fall back to the v66 inert result. Released/expired recovery uses generation N+1 and repeats daemon-aware empty-scope reconciliation. Public output keeps only bounded metadata and contact confidence, never raw daemon errors, payloads, sockets, image repository names, resource identities, paths, or private lease identity. ADR 0029 records the boundary.

Focused Domain, Store, Application, HTTP transport, migration, and CLI tests cover exact call ordering, label filtering, collision rejection, durable ordering, generation binding, zero verification, immutability, v66 fallback rejection, in-flight migration compatibility, privacy, and replay without new daemon calls. The final local gate passed full ordinary/race suites in 215.2s/233.1s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 21 frontend tests, generated-contract drift checks, production build, zero-vulnerability npm audit, 51-file Markdown validation, repository privacy/artifact/encoding/Docker-mutation/diff scans, isolated real-binary Workspace smoke, Linux test-binary cross-compilation, and focused Sandbox/Store/Application/CLI repetitions of 50/10/5/5 plus race repetitions of 10/3/3/3.

The audit forced production verification to exactly zero, recomputed the exact label selector, cross-bound the v66 control reconciliation, required evidence before lease expiry, closed a direct-SQL timestamp mismatch that could leave an immutable half-terminal record, and made daemon-contact reporting evidence-based. No unresolved high/medium issue is known. The local Windows host did not run the optional real Linux daemon path; no real Docker, container start, Shell, or host process was executed. Protected host deletion policy rejected recursive cleanup of the isolated smoke roots, so two directories remain under the OS temporary root for normal cleanup and need no user action. GitHub Actions run `29543385038` passed implementation commit `8bc0929`; Go/Linux completed in 2m50s and TypeScript in 24s.

## Completed Immutable Production-Evidence Review Slice (v68)

Schema v68 adds immutable `sandbox_docker_production_evidence_review.v1` and digest-only operation records over one exact completed v67 harness receipt. The operator explicitly confirms one accepted or rejected decision. Acceptance can only classify the bounded metadata scope; rejection uses one of five fixed codes. There is no free-form reason, uploaded report, daemon payload, resource identity, path, socket, raw operation key, or private lease identity in the request or public projection.

The Store writes operation first and review second in one transaction. A deferred foreign key and reciprocal triggers make both halves mandatory at commit, while source triggers rebind the blocked v63 review, v65 receipt/items, v66 attempt, and v67 harness result. Each evidence/attempt receives at most one immutable decision, and migration creates no historical review. Same-key/same-semantic replay returns the existing record without another event or daemon call; changed semantics conflict.

Even an accepted receipt retains `production_verified_count=0`, `sufficient_check_count=0`, `blocker_count=16`, and false start-gate/container/process/output/Artifact authority. v68 performs no Docker request, model call, Shell, or host-process start. The final ordinary/race suites passed in 247.9s/276.3s; vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 21 frontend tests, OpenAPI/build/npm audit, 57-file/74-link Markdown validation, credential/encoding/forbidden-entry/diff scans, Linux cross-compilation, and isolated real-CLI schema-v68 smoke are green. Focused Domain/Store/Application/CLI repetitions passed at 50/10/5/5 and race repetitions at 10/3/3/3.

The independent audit closed stored operation/review request-fingerprint drift at both SQL and Store read/replay boundaries, added review-only and operation immutability/source-tamper negative tests, proved two-Store convergence, and extended rejected decisions through Store/event/Application/CLI/list/show/replay. No unresolved high/medium issue is known. Protected deletion left one cross-compiled binary and one smoke root under the OS temporary directory for normal cleanup; no manual action is required. GitHub Actions run `29552080990` passed implementation commit `41583ac`; Go/Linux completed in 2m57s and TypeScript in 24s. ADR 0030 records the boundary.

## Completed Inert User Skill Registry Slice (v69)

Schema v69 adds five immutable tables for installation operations/intents/results and removal operations/tombstones. A stable raw operation key is stored only as a domain-separated digest. Deferred foreign keys and reciprocal triggers require operation/root pairs to commit together; all rows are append-only. Installation intent commits before object publication, so a same-key retry recovers an interrupted import. Same-key changed intent conflicts, same name/version under another operation conflicts, and two SQLite connections converge on one intent/result.

`LocalPackageObjectStore` stores only strictly validated deterministic ZIP bytes below `$CYBERAGENT_HOME/skill-registry/objects/sha256/<prefix>/<digest>.zip`. It exposes `Put` and `Verify` only, publishes through an exclusive same-directory temporary file plus file sync and atomic hard link, and revalidates size, SHA-256, ZIP structure, semantic package fingerprint, and file identity on every completed replay/list/show. Symlinks, replacement, corruption, forged receipts, and cancellation fail closed. The package body and source path never enter SQLite, Run events, or CLI output.

CLI now supports explicitly confirmed `skill import`, metadata-only `skill installed` and `installed show`, and explicitly confirmed `skill remove`. External packages are always `operator_installed_untrusted`; all command, hook, network, Provider, tool-grant, Run-selection, and context-injection authority is false. Built-in names cannot be shadowed. Code/Cyber catalogs are separate and Cyber accepts exactly the `script` Profile. Removal appends a tombstone, retains the content object, blocks an exact Run-pinned version in Go and SQL, and has no implicit reinstall/restore behavior. ADR 0031 records the boundary.

The final local gate passed ordinary/race suites in 259.7s/275.3s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, OpenAPI drift checks, 21 frontend tests, production build, and zero-vulnerability npm audit. Focused v69 race tests passed three repetitions, and real two-Service import/removal convergence with independently generated IDs/timestamps passed 20 ordinary and 10 race repetitions. Initial Linux CI run `29556933994` exposed concurrent nested-directory preparation through `os.Root.MkdirAll`; the object store now creates each component independently, accepts an existing component only after `Lstat` proves it is a real directory, and rejects symlink redirection. Twelve independent Stores passed 100 ordinary and 20 race repetitions, and the Linux test binary cross-compiled. GitHub Actions run `29557803407` passed fix commit `d28b100` with Go/Linux in 3m21s and TypeScript in 23s. The broader audit fixed migration downgrade ordering, static error text, redundant temporary cleanup state, object-receipt interface binding, cancellation immediately before publication, semantic replay comparison of generated identities, and credential redaction for the free-text Manifest description before SQLite persistence. No unresolved high/medium issue is known. No model, network request, Shell, Docker operation, installation hook, or host process was run by this slice.

## Completed External Skill Run Context Slice (v70)

Schema v70 adds immutable external selection/item/operation ledgers and separate root/Specialist context preparation/commit ledgers. Selection requires a created Run, current mode, exact active v69 installation/result/object identity, stable digest-only operation, and a second explicit untrusted-context confirmation. It allows one to four packages under a 4096 aggregate budget and at most one operator-designated Specialist package. Code/Cyber/Profile constraints remain fixed, same-intent replay converges, and Go plus SQL prevent removal of a pinned installation.

`PackageObjectLoader` is separate from object publication. It opens only the expected content-addressed path and rechecks ordinary-file identity, size, archive SHA-256, deterministic ZIP structure, semantic package fingerprint, and Manifest/content binding before returning a defensive in-memory copy. Root and Specialist assembly redact secrets and apply separate budgets; the child defaults to 1024 and is capped at 2048. Content is serialized only as a user-role `external_skill_guidance.v1` envelope with Policy/tool/Shell/network/secret/scope/delegation authority fixed false. System control text explicitly treats external content as optional workflow guidance and document claims as evidence, not instructions.

Root context preparation commits with the corresponding first `model.started`; Specialist preparation commits with its first Specialist model-call start. SQLite and events retain counts, budgets, redaction totals, selection/context fingerprints, and closed authority facts, never package content, source paths, raw keys, secrets, or model text. CLI supports `skill select-external` and `skill external-selection`; HTTP/TUI/Web mutation and upload remain absent. Focused tests cover confirmation, replay, exact binding, cross-surface/Profile rejection, secret redaction, object mismatch, root/child user-role provenance, no-tool child delivery, immutable SQL, direct-SQL removal protection, and v69-to-v70 migration without fabricated state. ADR 0032 records the boundary.

The final local gate passed the full ordinary/race suites in 197.6s/264.4s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 21 frontend tests across nine files, OpenAPI drift checks, production build, zero-vulnerability npm audit, repository credential/runtime-artifact/encoding/43-file Markdown-link/diff scans, and an isolated real-CLI schema-v70 smoke. The audit fixed one medium-severity schema defect that had made `installation_id` globally unique, incorrectly preventing one verified installation from being independently selected by later Runs; uniqueness is now scoped to a selection and covered by a second-Run regression. Further hardening binds Specialist provenance to the latest mode in Go and SQL, rejects Specialist packages above 2048 before persistence, dynamically accepts the valid 1024-2048 range, preserves same-operation replay across Plan-to-Delivery drift, pairs selection and operation with a deferred reciprocal foreign key, closes cancellation windows around object parsing, and emits every non-guidance authority field explicitly false. No unresolved high/medium issue is known. No real model, network, Shell, Docker, installer hook, or host process ran. Protected cleanup left the isolated smoke root under the OS temporary directory for normal cleanup; no user action is required. GitHub Actions run `29566538449` passed implementation commit `edc4073` with Go/Linux in 3m42s and TypeScript in 21s.

## Completed External Skill Read Projection Slice (v71)

Schema v71 adds two SQLite read-only views and a separate `external_skill_projection.v1` Go contract. Existing v70 selections become visible without backfill or new events. Runs without an external selection remain absent. The root projection contains Run/mode/surface/Profile, bounded token and item totals, closed authority facts, and root/Specialist prepared/committed counts. Item rows contain only ordinal, name, version, token upper bound, trust class, declared-tool count, and Specialist eligibility.

The safe type cannot represent package bodies, paths, byte sizes, digests/fingerprints, selection/installation/mode-snapshot IDs, requester/operation identities, or attempt/Agent identities. Store tests inspect the view columns, reject writes, verify v70 in-place upgrade without fabricated facts, and scan serialized values for private identities. The same DTO is optional in Run detail and available from read-only `GET /api/v1/runs/{run_id}/external-skills`; the OpenAPI contract now has 26 paths and 61 schemas. TUI adds a read-only `Skills` activity and stable count projection. React adds a button-free Run-overview panel. No control capability, model call, package load, Shell, network, Docker, installer hook, or Agent-controlled host process was added or run.

Final local gates: full ordinary/race suites passed in 227.1s/301.1s; vet, zero-warning staticcheck, module verify/tidy diff, zero-finding govulncheck, deterministic OpenAPI/TypeScript generation, strict TypeScript, 9 files/22 frontend tests, production build, zero-vulnerability npm audit, credential/runtime-artifact/production-process-entry/encoding/54-file and 78-relative-link Markdown/diff scans, and an isolated real-binary schema-v71 Workspace smoke are green. The audit added explicit Run matching to all four preparation/commit count subqueries and a separate ordinary/race HTTP regression for a valid Run without a selection. No unresolved high/medium issue is known. No real Provider, Agent-controlled Shell/host process, Docker, installer hook, or external network call ran. The smoke root remains below the OS temporary directory for normal cleanup and requires no user action. GitHub Actions run `29574167659` passed implementation commit `3947bea`; Go/Linux completed in 2m56s and TypeScript in 25s.

## Completed Pathless Desktop Skill Preview Boundary (non-schema D1-A)

The CLI package validation/import paths now share `skills.ReadPackageFile` and `skills.ValidatePackageFile`. The reader accepts only one bounded non-symlink ordinary file, rejects leading/trailing whitespace rewrites, compares pre-open/opened/post-read identity, size, and modification time, honors cancellation, and returns path-free errors. The existing strict deterministic ZIP parser remains the sole semantic validator.

`desktop.NewSkillPackagePreviewBoundary` returns a native-only Go selector function and a separately bindable renderer bridge. The selector validates immediately and stores only a constrained projection; it never stores the path or body. The renderer receives a cryptographically random 256-bit URL-safe handle with a five-minute TTL, sixteen-entry process cap, and atomic single-consumption rule. The DTO excludes file/path/body, Manifest description/content path/content digest, and fixes command/hook/network/Provider/tool/install authority false. At D1-A delivery there was no Wails registration; ADR 0034 now binds only this pathless flow in the read-first shell. The preview still creates no database/event/model/network/process/Docker/installation mutation.

The final full ordinary/race suites passed in 255.8s/314.0s. Desktop passed 100 ordinary and 25 race repetitions, the Skill file boundary passed 100 repetitions, and the CLI package chain passed 10. Vet, zero-warning staticcheck, module verify/tidy diff, zero-finding govulncheck, OpenAPI drift checks, 22 frontend tests, production build, zero-vulnerability npm audit, 60-file/79-relative-link Markdown validation, and repository privacy scans are green. Tests cover path-free JSON allowlists, source replacement after selection, missing/directory/symlink/empty/oversized/malformed input, cancellation without consumption, expiry/capacity, entropy failure, replay, and 32 concurrent consumers with exactly one success. No unresolved high/medium issue is known. GitHub Actions run `29578985787` passed implementation commit `45c047c` with Go/Linux in 3m13s and TypeScript in 27s. ADR 0033 records the boundary. Schema remains v71 and product-usability estimates remain unchanged because no end-user Desktop UI exists.

## Completed Embedded Read-First Windows Desktop Shell (non-schema D0-A)

Wails v2.13.0 is now a pinned direct Go dependency. `cmd/cyberagent-desktop` builds only on Windows with the `desktop` tag and embeds the Vite production bundle through `web/assets_desktop.go`. `webui.LoadEmbeddedFS` snapshots only a bounded UTF-8 index and content-hashed allowlisted assets. The existing `httpapi.API` Handler runs directly behind the Wails AssetServer, with no TCP listener or second business API. Its narrow adapter pins loopback identity and handles the two exact Wails v2.13 request-shape differences observed in real startup.

The complete Wails renderer binding has exactly `Bootstrap`, `SelectSkillPackage`, and `PreviewSkillPackage`. Bootstrap returns same-origin `/api/v1`, bounded app/UI metadata, and ephemeral memory-only tokens with every process/Shell/Docker/Skill-install/path-input authority bit false. The default launch has no control token. Explicit `--enable-profile-control` creates a distinct control token only for the existing v64 non-authorizing profile selection. The native `.zip` dialog is serialized and path-free at the renderer boundary; valid selections reuse ADR 0033's one-time handle, while cancellation and native errors reveal no path.

React auto-connects without showing or persisting tokens. Ordinary Web retains SSE, while Windows Desktop uses bounded opaque-cursor event polling because Wails v2 does not stream AssetServer responses on Windows. The top bar exposes only package preview and refresh in the default shell; it contains no ineffective disconnect, install, terminal, process, approval, or queue mutation control. Single-instance restore, file/drop denial, context-menu denial, CSP, renderer code integrity, and a path-free typed startup failure dialog are active. No installer, registry, auto-start, updater, protocol, service, signed release, LocalRunner, Docker start, Shell, model call, network Scope mutation, or new SQLite fact exists.

Focused Go and TypeScript tests cover the exact binding/JSON allowlists, closed authority, token distinction, dialog serialization, path privacy, one-time handles, embedded asset bounds, Wails request compatibility, auto-connection, polling, cancellation, and no-install preview. A production-tag binary built successfully and was launched against an isolated schema-v71 home. Visual checks covered the 1440x900 shell, automatic API connection, empty Run/Session states, the Skill modal, and native `.zip` dialog without overlap or blank rendering.

The final local gate passed the full ordinary/race suites in 205.1s/293.9s, ordinary and desktop-tag vet/staticcheck, ordinary and desktop-tag zero-finding govulncheck, module verification/tidy, deterministic OpenAPI generation, strict TypeScript, 31 tests across 12 frontend files, the production build, zero-vulnerability npm audit, 61-file/82-relative-link Markdown validation, and credential/runtime-artifact/encoding/forbidden-execution/diff scans. Desktop-focused packages passed 50 ordinary and 10 race repetitions. The final unsigned Windows GUI is 21,022,208 bytes with SHA-256 `6b355cfa72b41d225e62ed58ac24cb9493bbf2a71f4d45120e6f0dbf5308ad0c`; it is ignored by Git. The audit fixed the real Wails empty-root/no-body-GET compatibility cases, hid the ineffective Desktop disconnect action, rejected equal read/control tokens, added a path-free startup error, and finally made nil-URL/missing-RequestURI adaptation fail closed. No unresolved high/medium issue is known. GitHub Actions run `29602281365` passed implementation commit `2c0b81c`; Go/Linux completed in 4m57s, TypeScript in 26s, and the new Windows Desktop job in 4m27s. ADR 0034 records the boundary.

Metrics after D0-A: architecture remains about 98% (V2 about 99%); complete-product usability is about 52-56%, generic coding-agent workflow usability about 46%, and Cyber autonomous workflow usability about 20%. The gain is a real read-first Windows client and native package risk preview, not execution authority.

## Completed Desktop Lifecycle And Event Resumption Hardening (non-schema D0-B)

`desktop.OpenControlPlane` now owns one Desktop SQLite connection and the existing in-process `httpapi.API` without opening a socket. Same-database tests prove an independent CLI connection can append events while Desktop is open, Desktop can close and reopen without losing its high-water cursor, six control planes can open the initialized database concurrently, and close is idempotent. `desktop.Lifecycle` coalesces early second-instance signals, discards their arguments and working directory, restores only the existing window, serializes native restore with shutdown, cancels its context, and cannot restart after stop.

The read-only `run-event-poll.v1` endpoint returns real `run-events.v1` frames and the same Run-bound opaque cursor as SSE. It rejects unknown/duplicate query fields, invalid limits, cross-Run cursors, and SSE-only headers; Go validates a contiguous batch before returning it. React no longer synthesizes Desktop cursors from ordinary event pages. It keeps at most 16 Runs and 500 frames per Run in module memory, resumes across component remount, performs one stale-cursor reset per mount, honors abort after every response, and never uses Local or Session Storage.

Secure Windows builds require `desktop,production,wv2runtime.error`. WebView2 `94.0.992.31` or newer is checked read-only before bundle/database initialization; missing, old, or failed probes return bounded path-free guidance with no URL, download, or installer. The in-process adapter accepts exact `http://wails.localhost`, canonicalizes `RequestURI`, pins loopback identity, and rejects alternate origins. A Desktop-only renderer guard blocks external links, forms, and popup calls while ordinary browser navigation remains unchanged. Wails start-origin validation remains the native binding authority.

Final local validation passed the 256.6-second full ordinary suite, 273.5-second full race suite, ordinary and secure-Desktop vet/staticcheck/govulncheck, deterministic OpenAPI generation, strict TypeScript, 37 frontend tests across 13 files, production build, and zero-vulnerability npm audit. A focused 20-round lifecycle race also passed after the audit serialized native restore and shutdown. The first Desktop-tag vulnerability scan found five newly reachable `x/net/html@v0.54.0` advisories; `x/net` was upgraded to fixed v0.55.0, `x/sys` resolved to v0.45.0, and both final scans report no vulnerabilities. Windows 11 Pro x64 10.0.26200 real-process smoke of the final 19,572,224-byte binary (SHA-256 `f26ea87f42701a7eba8efa789900ea6953ef3c1533ff95106ec4b8e6b02b1160`) proved second-instance handoff, forced termination, same-database reopen, and zero residual Desktop processes. GitHub Actions run `29609621468` passed implementation commit `c9b1c66` with Go/Linux in 5m00s, Windows Desktop in 4m21s, and TypeScript in 23s. The unsigned binary remains ignored and non-distributable; Windows 10 real-machine coverage is pending. No unresolved high/medium issue is known, and no Agent-controlled Shell/Local/Docker, Provider, Skill installation, or external network operation ran. ADR 0035 records the boundary.

Metrics after D0-B: architecture remains about 98% (V2 about 99%); complete-product usability is about 53-57%, generic coding-agent workflow usability about 47%, and Cyber autonomous workflow usability about 20%. The increase reflects a recoverable read-first Desktop client, not new execution authority.

## Completed Idempotent Controlled Run Creation (schema v72 / D1-R1)

Schema v72 adds immutable digest-only `run_creation.v1` operations. The
Application service requires a registered Workspace and creates one redacted
Mission, interactive created Run, active Session, initial mode, preview/noop
execution profile, root Agent, and exact initial events in one immediate SQLite
transaction. Network is disabled, targets are empty, the default budget is
fixed, model route equals Profile, and the goal is bounded to 4096 UTF-8 bytes.
Same-key/same-intent replay returns the original graph across restart or
independent connections; changed intent conflicts. SQL triggers independently
rebind the entire closed initial graph and make operations immutable.

HTTP adds a strict control `POST /api/v1/runs` and read-only paginated
`GET /api/v1/workspaces`; the latter omits root paths. OpenAPI has 28 paths,
65 schemas, 25 GET operations, and four control POST operations. Desktop adds
an independent `--enable-run-creation` capability while keeping the native
bridge at exactly three methods. A creation-only token cannot reach existing
cancellation/profile controls. React adds a responsive New Run dialog with
Workspace, Profile, surface, and phase selection, in-memory uncertain-failure
key reuse, UTF-8 byte preflight, closed response validation, query refresh, and
new Run selection. Neither token nor operation key enters browser storage.

The implementation and audit cover immutable SQL, direct-SQL default-budget
rejection, cross-connection convergence, strict HTTP JSON/header/body handling,
Workspace path privacy, Desktop capability separation, forged Goal/Workspace/
mode/authority response rejection, strict UTF-8 JSON, and multibyte limits.
Historical downgrade fixtures now remove v72 triggers before deleting older
profile tables; SQL binds exact initial timestamps, root state, and event count;
and the creation service uses a narrow Store contract instead of inheriting
unrelated Run-transition authority. No model, tool, Shell, host process, Docker, network
request, Skill installer hook, or execution lease is invoked. ADR 0036 records
the boundary.

The final local gate passed full ordinary/race Go suites in 271.5s/257.9s,
ordinary and secure-Desktop tests plus vet/staticcheck/govulncheck, module
verification/tidy, deterministic OpenAPI/TypeScript generation, 45 frontend
tests across 14 files, strict TypeScript, the production builds, zero-finding
dependency audits, 63-file/86-relative-link Markdown validation, privacy and
forbidden-entry scans, and an isolated schema-v72 CLI smoke. No unresolved
high/medium issue is known.

Metrics after D1-R1: architecture remains about 98% (V2 about 99%);
complete-product usability is about 55-59%, generic coding-agent workflow
usability about 49%, and Cyber autonomous workflow usability about 20%.

## Completed Controlled Session Message Submission (non-schema D1-S1)

D1-S1 is one three-slice product batch over the existing schema-v45/v46
operator-steering queue; database schema remains v72. Slice one adds the narrow
`SessionMessageSubmissionService` and strict
`POST /api/v1/sessions/{session_id}/messages`. Go reloads the Session and exact
bound Run, redacts and bounds content, and reuses the existing digest-idempotent
enqueue operation and event semantics. The response is metadata-only and fixes
execution/model/tool/capability facts false.

Slice two adds independent Desktop `--enable-session-messages` bootstrap and
in-process Handler wiring. It neither expands the exact three-method Wails
bridge nor couples profile, creation, cancellation, or Session capabilities.
Slice three adds the React Session composer for running or paused bound Runs,
a 16 KiB UTF-8 preflight, memory-only same-intent uncertain-failure replay, and
queue-status feedback without echoing submitted content. Browser storage stays
empty and Go remains authoritative.

The integrated batch functional gate passed a direct 255.6-second full ordinary
Go suite on final code, the 80.5-second Desktop-tag focused suite, final focused Application/HTTP/
Desktop regression, 52 frontend tests across 15 files, strict TypeScript, Vite
production build, and Windows production Desktop build. The path invokes no
Provider, execution lease, tool, Shell, Docker, external network, or process.
The agreed six-slice robustness gate deliberately moves full race, vet,
staticcheck, govulncheck, dependency, and extended privacy analysis to the end
of the next three-slice batch. GitHub Actions run `29633205163` passed
implementation commit `3ecb22a`: Go/Linux 3m06s, Windows Desktop 1m38s, and
TypeScript 25s, including remote vet, govulncheck, and dependency audit. ADR
0037 records the boundary.

Metrics after D1-S1: architecture remains about 98% (V2 about 99%);
complete-product usability is about 57-61%, generic coding-agent workflow
usability about 52%, and Cyber autonomous workflow usability about 20%.

## Completed Run Control And Bounded Handoff (schema v73 / D1-S2/D1-L1/D1-X1)

D1-S2 adds non-schema `session_steering_cancellation.v1` and strict HTTP/Desktop/React control over the existing v46 ledger. Cancellation is exact Session -> Run -> message bound and pending-only. Public metadata now derives whether a pending message already has a prepared delivery, and React hides cancellation in that state; prepared, committed, or cancelled facts remain immutable.

Schema v73 adds `run_lifecycle_control.v1` and `run_execution_handoff.v1`. Lifecycle start/pause/resume uses an immutable digest-idempotency operation and exact quiescence/lease/Agent/Supervisor gates. A delayed replay returns the original operation plus the current Run state, so later legal transitions do not invalidate retry. The execution operation freezes at most eight pending identities before work, then uses the existing RunSupervisor, Policy, cumulative budgets, model/tool ledgers, private execution lease, checkpoints, and events. Later appends cannot enter the batch; cancellation before delivery is skipped; an empty selection completes without a lease or model call. Terminal completion is fenced to the exact lease generation and exact result intent.

HTTP adds independent strict control routes for message cancellation, Run lifecycle, and bounded execution. Desktop adds `--enable-session-steering-control`, `--enable-run-lifecycle`, and `--enable-run-execution`; the Wails bridge remains three methods. React adds pending cancellation, Start/Pause/Resume, and an at-most-eight-step Run Queue control with memory-only intent-bound retry keys. Responses contain counts/status only and omit message/model/tool content, raw keys, and private lease identities.

The cumulative six-slice gate passed the final ordinary suite in 268.2 seconds and race suite in 295.3 seconds, ordinary and secure-Desktop vet/staticcheck, both zero-finding govulncheck paths, module verify/tidy diff, deterministic contract generation, strict TypeScript, 66 frontend tests across 16 files, Vite and Windows production builds, zero-vulnerability npm audit, and repository privacy/artifact/forbidden-entry scans. The unsigned GUI is 20,849,664 bytes with SHA-256 `ce3ff2b4609068de996b6362e3a5008c4d2348eae73c48ad0661c4e22739eba5`.

The combined audit fixed delayed lifecycle replay, misleading cancellation for prepared items, stale-lease/changed-intent terminal handoff replay, execution retry-key reuse after `max_steps` changes, a false-positive frontend assertion, and two static-analysis findings. The first ordinary timing wrapper was also found not to propagate a child exit code; the suite was rerun directly and passed. This was a test-orchestration issue, not a product defect. No unresolved high/medium issue is known. ADR 0038 records the authority boundary.

Metrics after D1-X1: architecture remains about 98% (V2 about 99%); complete-product usability is about 61-65%, generic coding-agent workflow usability about 56%, and Cyber autonomous workflow usability about 20%. The gain is an explicit resumable mock/Provider Agent loop, not host/container process execution or an automatic background scheduler.

## Completed Model, Plan, And Approval Controls (non-schema D1-M1/P1/A1)

D1-M1 replaces separate CLI/Desktop router construction with one Go-owned `modelregistry.Registry`. It registers mock plus valid environment-backed Mimo, DeepSeek, and Anthropic-compatible Providers, then loads the five persisted routes. `GET /api/v1/models` returns only bounded names, models, status, credential-source class, network-required/configuration-error booleans, and route availability. It contains no key, Base URL, environment-variable name, client, or raw error and performs no Provider request. Secret-like or malformed model/route identifiers are rejected or projected as unavailable/redacted.

D1-P1 exposes `plan_delivery_control.v1` as two distinct digest-idempotent POST operations. Direction selection reloads the exact proposal/Run and reuses the existing atomic selection, WorkItem, Note, and event path without changing phase. Deliver transition requires that immutable selection and reuses the Run-mode ledger. Neither operation starts/resumes the Run, obtains a lease, calls a model/tool, or grants authority.

D1-A1 exposes `approval_queue.v1` and `approval_control.v1`. The read queue contains at most 100 pending metadata items and excludes command, arguments, paths, file content, fingerprints, reasons, operations, and private authority. Decision reloads the exact Run, approval, and source; only pending nonterminal requests may change. Approve-once is limited to current-Policy-approved dry-run Shell and process-disabled ScriptProcess sources; replace-file can only be denied, permanent Policy denial cannot be overridden, and all process/Shell/Docker/write/Grant/capability outputs stay false. A committed same-key decision remains replayable after the Run becomes terminal, while a new terminal decision remains rejected.

Desktop adds independent `--enable-plan-delivery` and `--enable-approvals`; model availability remains read-only. The native Wails bridge stays exactly three methods. React adds a model dialog, explicit direction/Deliver controls, and an Approval tab with memory-only intent-bound retry keys and strict response authority validation. OpenAPI now has 36 paths, 84 schemas, 27 GET operations, and 11 control POST operations.

The ordinary integrated gate passes the final full Go suite in 310.1 seconds, focused Windows Desktop tags, all 73 frontend tests across 18 files, strict TypeScript, Vite and Windows production builds, and zero-vulnerability npm audit. The combined audit fixed secret-like model identifier projection, terminal replay of an already committed approval, missing frontend Session/Workspace approval binding, and one encoding defect. No Provider network call, real Shell/Local/Docker process, file write, Session Grant, installer, or external Skill execution occurred. No unresolved high/medium issue is known. ADR 0039 records the boundary.

Metrics after D1-A1: architecture remains about 98% (V2 about 99%); complete-product usability is about 64-68%, generic coding-agent workflow usability about 60%, and Cyber autonomous workflow usability about 20%.

## Completed Provider, Diff, And Wake Controls (schema v74 / D1-M2/D1-D1/D1-Q1)

D1-M2 adds explicit `provider_diagnostic.v1` plus persisted `model_route_control.v1`.
Route changes commit to SQLite before the concurrent-safe Router changes. Diagnostics run
only after an operator action and make at most one 15-second content-free/tool-disabled
request. Their DTO contains status metadata only and no model text, key, endpoint,
environment-variable name, client, or raw error.

D1-D1 adds exact Run/Mission/Session/Workspace FileEdit metadata, bounded redacted Diff,
and review-only `approve_intent|deny`. It never selects file bodies into HTTP and never
writes the workspace. The audit found the approval/edit two-transaction crash window;
same-outcome retry now repairs edit state after a committed approval, while opposite or
cross-bound decisions fail closed.

Schema v74 D1-Q1 adds digest-idempotent wake schedule/cancel, bounded attempts/backoff/
deadline, and one generation-fenced owner. Public state omits owner/lease identity and
fixes background/model/tool/execution authority false. No goroutine, service, automatic
Run transition, or Run execution lease exists. OpenAPI is 43 paths, 96 schemas, 30 GET,
and 16 control POST operations. ADR 0040 records the boundary.

The final six-slice robustness gate passes on the audited code. Full ordinary/race suites
took 278.6s/296.1s. Ordinary and secure-Desktop vet/staticcheck/govulncheck, module
verification/tidy, deterministic OpenAPI, 80 React tests across 20 files, strict
TypeScript, Vite/Windows production builds, zero-vulnerability npm audit, isolated CLI
smoke, and UTF-8/local-link/changed-credential/runtime-artifact/new-process-entry scans
are green. Focused route and wake race repetitions also pass. Audit fixes cover the
approval/edit crash window, body-free exact Diff SQL projection, expired-final-lease
event ordering, invalid delay/deadline combinations, and concurrent durable/in-memory
route ordering. No live key, network Provider, Shell, LocalRunner, Docker, or file apply
was used. No unresolved high/medium issue is known.

GitHub Actions run `29649564643` passed for implementation commit `37fbfbf`:
TypeScript console 30s, Windows Desktop shell 1m58s, and Go control plane 3m44s.

Metrics after implementation: architecture remains about 98% (V2 about 99%);
complete-product usability is about 67-71%, generic coding-agent workflow usability
about 63%, and Cyber autonomous workflow usability about 20%.

## Completed Foreground Wake, File Apply, And Inert Skill Installation (schema v75-v76 / D1-Q2/D1-D2/D1-B1)

Schema v75 adds `run_wake_consumer.v1`. An operator action claims one due intent and
routes at most eight steps through the existing durable handoff and RunSupervisor. The
claim, exact handoff binding, completion/failure, and events are restart-safe. A
crash-uncertain handoff without a result remains prepared, cannot be reclaimed after
lease expiry, and cannot be cancelled or failed as though no model call occurred.
There is no background goroutine, service, startup task, or hidden polling loop.

Schema v76 adds `file_edit_apply.v1`. Go reloads the exact Run/Mission/Session/
Workspace/proposal/approval, rechecks the running Run, active Session, current Policy,
and original/current SHA-256 at the final write boundary, then uses same-directory
staging and atomic replacement before verifying the proposed digest. One Edit admits
one apply operation and persists one idempotent result. Run-bound edits must use
`review-approve` followed by `apply`; the
legacy approve command cannot bypass the separate capability. HTTP and React receive no
path or file body.

Non-schema D1-B1 adds a fourth narrow native method, `InstallSkillPackage`, and one
independent HTTP control. Desktop consumes a short-lived confirmation handle; HTTP
accepts strict bounded canonical base64. Both call the existing content-addressed inert
Registry and require explicit untrusted-package confirmation. No content, script, hook,
command, tool, Provider, network request, Run selection, or context delivery runs during
installation. ADR 0041 records all three boundaries.

The final ordinary Go suite passes in 333.1s, along with focused race, Windows Desktop
tags, 85 React tests, strict TypeScript, deterministic OpenAPI/TypeScript, Vite/Windows
production builds, vet, module verification/tidy, npm audit, isolated CLI smoke, and
privacy/UTF-8/link/artifact/entry scans. The audit fixed prepared-wake reclaim/cancel,
failed-call fact binding, stale FileEdit recovery authority, duplicate apply operations,
and direct-truncation writes. No high/medium issue remains known. A forced kill before
atomic replacement may leave one redacted hidden staging file; D1-U1 owns this low-risk
recovery receipt and cleanup design. Current
metrics are architecture about 98% (V2 about 99%), complete-product usability about
70-74%, generic coding-agent workflow usability about 66%, and Cyber autonomous
workflow usability about 20%.

GitHub Actions run `29655417908` passed implementation commit `79f07fb`: the
TypeScript console completed in 28s, the Windows Desktop shell in 2m5s, and the Go
control plane in 3m57s. Remote checks included API drift, frontend tests/build/audit,
Desktop build/boundaries, module verification, the full Go suite, vet, and govulncheck.

## Completed Receipts, Workspace Explorer, And Portable Diagnostics (D1-U1/E1/W1)

D1-U1 adds one strict `operation_receipt.v1` projection to FileEdit apply, foreground
wake consumption, and inert Skill installation. It contains no operation key/digest,
path/body, model content, requester, or private lease. Go owns the closed outcome,
replay, retry, recovery, and cleanup tuple. FileEdit replays attempt conservative
cleanup only for an old ordinary reserved staging file in the exact target directory
whose full bytes match the approved proposal SHA-256; uncertainty is reported without
changing the durable apply result.

D1-E1 adds read-only `workspace_explorer.v1` and the Run Files tab. Go resolves the
registered root, accepts only canonical slash-separated relative paths, follows no
link or redirected component, scans at most 400 entries, returns at most 200, reads at
most 64 KiB of UTF-8, and caps redacted output at 128 KiB. The root and internal staging
names stay out of the DTO. Each result has non-authorizing `context_provenance.v1`;
React accepts only the exact child path derived from the current parent and name and
renders content as plain text.

D1-W1 adds `cyberagent doctor portable`, reproducible linker metadata, a repository-
contained/no-child-reparse-point build output boundary, consecutive SHA-256 builds,
and PE architecture/executable/zero-COFF-timestamp/hash/trimpath/module/non-installing
checks. Automated success and release approval are separate; the manual Windows 10,
WebView2, display, launch, and recovery matrix keeps `release_ready=false`.

The cumulative six-slice gate is green. Full ordinary/race suites passed in
294.0s/338.3s; ordinary and secure-Desktop test/vet, zero-warning staticcheck,
zero-finding govulncheck, module verification/tidy, 88 React tests across 22 files,
strict TypeScript, deterministic OpenAPI/TypeScript, Vite build, zero-vulnerability
npm audit, isolated mock-only CLI smoke, privacy/artifact scans, and the real Windows
double build passed. OpenAPI is 47 paths/106 schemas/31 GET/19 control POST. The double
build SHA-256 is
`33fb9ca3064df98191ac50b2a3ef9431e1b5c81abe8c610d4be15db113cdf1ef`.
No unresolved high/medium issue is known. No real Provider, LocalRunner, Shell, Docker,
network attack, installer, registry mutation, startup task, or updater was used.
GitHub Actions run `29658783000` passed implementation commit `5f0f397`: Go control
plane 5m49s, TypeScript console 32s, and Windows Desktop shell 2m11s.
Current estimates are architecture about 98% (V2 about 99%), complete-product usability
about 74-78%, generic Coding Agent workflow about 70%, and Cyber autonomous workflow
about 20%.

## Completed Workspace Search, Evidence Attachment, And Receipt History (schema v77 / D1-E2/C1/U2)

D1-E2 adds deterministic `workspace_search.v1` over redacted Explorer projections.
Queries are normalized and bounded to 128 Unicode code points; one request scans at
most 128 directories, 1,000 entries, 64 regular files, 50 results, and the declared
Explorer byte ceiling including UTF-8 look-ahead. Paths are canonical relative
references, links and internal staging are skipped, snippets are plain text, and all
provenance remains `instruction_authorized=false`. There is no index, watcher, daemon,
host root, raw-byte search, model call, or renderer filesystem authority.

Schema v77 D1-C1 adds immutable `session_evidence_attachment.v1`. The independently
gated HTTP/Desktop route accepts only an exact Run, Workspace reference, projected
SHA-256, protocol version, and memory-held idempotency key. Go reloads and exact-binds
Run/Mission/active Session/registered Workspace, reprojects the current file, then one
transaction stores the tool-role evidence message, event, and attachment. Go and the
SQLite trigger both require matching `context_provenance.v1` and false instruction
authority. On model projection the text is untrusted user evidence, so README content
addressed to automated assistants cannot become an operator instruction, approval, or
capability. Same-operation replay returns the durable snapshot; a new stale operation
conflicts.

D1-U2 adds `operation_receipt_history.v1`, a newest-first at-most-100 terminal view of
FileEdit apply, foreground wake, and inert Skill installation. It supports an exact Run
filter and emits opaque domain-separated IDs. Keys/digests, paths/content hashes,
requester/owner identity, archive metadata, and private leases stay internal. FileEdit
staging inspection is read-only; uncertainty stays `pending_review` and listing never
deletes a file.

The ordinary gate passed the uncached Go suite in 297.9s, Windows Desktop tag tests,
`go vet`, module verification/tidy, 92 React tests across 23 files, strict TypeScript,
deterministic OpenAPI/TypeScript, Vite build, zero-vulnerability npm audit, and a
isolated mock-only CLI smoke plus a reproducible Windows double build. OpenAPI is 50
paths/53 operations/112 schemas. The
unsigned binary SHA-256 is
`d187601e9e9d8cb0d4ee644e3c9aa1c7617905580b001ef7955dbc35b8c47af3`;
automated compatibility passed and release readiness remains false. The audit fixed
Unicode case-mapping offsets, the true UTF-8 look-ahead read ceiling, and schema-level
canonical source-reference enforcement. No unresolved high/medium issue is known and
no real Provider, Shell, LocalRunner, Docker, network request, API key, installer,
registry mutation, startup task, or updater was used. Current estimates: architecture
about 98% (V2 about 99%), complete-product usability about 78-82%, generic Coding Agent
about 74%, and Cyber automation about 20%. ADR 0043 is authoritative.

GitHub Actions run `29661764283` passed implementation commit `ffbdc72`: TypeScript
console 34s, Windows Desktop shell 2m21s, and Go control plane including govulncheck
3m48s.

## Completed Operator Actions, Evidence Inventory, And Command Palette (D1-O1/C2/K1)

D1-O1 adds read-only `operator_action_center.v1` for one exact Run. Go joins bounded
indexed pending steering, approval, FileEdit review/apply readiness, and due wake facts,
then revalidates Run/Mission/Session/Workspace and due time. At most 100 closed metadata
items use domain-separated opaque public IDs. Source rows, operations, requesters,
messages, commands, paths, Diff/content, leases, and authority fields remain private;
listing performs no decision or execution.

D1-C2 adds `session_evidence_inventory.v1` for immutable evidence already attached to
the exact Run-bound active Session and Workspace. It returns only closed source kind,
canonical reference, SHA-256, attachment time, and fixed false instruction authority.
It omits message identity/body, attaching operator, event sequence, private operations,
and capability state. React may send a Go-issued source reference only to the existing
Explorer, which rechecks its own Workspace/path boundary.

D1-K1 adds a static `Ctrl+K` command palette. Commands navigate existing Run tabs or
refresh current Run queries only. They submit no path, content, approval, operation,
capability, process, network, or secret value and persist no browser state.

The cumulative six-slice gate is green. Full ordinary/race suites passed in
319.6s/299.8s; ordinary/secure-Desktop tests, vet, zero-warning staticcheck,
zero-finding govulncheck, module verification/tidy, deterministic OpenAPI/TypeScript,
97 React tests across 26 files, strict TypeScript, Vite build, zero-vulnerability npm
audit, isolated mock-only CLI smoke, repository hygiene scans, and the real Windows
reproducible double build passed. OpenAPI is 51 paths/55 operations/116 schemas with
SHA-256 `B9CD79254D9AE09A2DB4BCC6268F04CA8F4ADD6C638E6BAA4DA42FC223A10181`.
The unsigned Desktop binary SHA-256 is
`a89b2357a5f1e7376ea8a533356028ccd5ea5eaec388b14d7623343fd041f520`.

Real-browser desktop/mobile auditing verified action navigation, evidence empty/source
states, palette filtering/Enter/Escape behavior, responsive containment, live SSE, and
stable connection count. It discovered and fixed a canonical event-version mismatch
(`event.v1` versus Go's `v1`) plus response-body leakage on failed reconnect. OpenAPI
now imports the Go version constant and generates a literal TypeScript type; every
parse/transport failure cancels the reader before reconnect. No unresolved high/medium
issue is known. No real Provider, API key, Shell, LocalRunner, Docker, external network,
installer, registry mutation, startup task, or updater was used. Current estimates:
architecture about 98% (V2 about 99%), complete-product usability about 80-84%, generic
Coding Agent workflow about 77%, and Cyber automation about 20%. ADR 0044 is
authoritative.

GitHub Actions run `29665187925` passed implementation commit `1151aaf`: TypeScript
console 36s, Windows Desktop shell 2m23s, and Go control plane 3m35s.

## Completed Go-Issued Editor, System Credentials, And Bounded Wake (D1-I1/M3/J1)

D1-I1 adds `file_edit_proposal.v1`. Go issues complete, untruncated, unredacted UTF-8
for an exact running Run/active Session/registered Workspace behind a 256-bit,
five-minute, one-intent source handle. The locally bundled lazy Monaco editor receives
no host path and submits only the handle plus proposed text. Go reloads bindings,
current hash, secret redaction, and Policy before creating a pending FileEdit. Proposal,
review, and apply remain independent; the editor cannot write a file.

D1-M3 adds `provider_credential.v1`. Windows stores exact `mimo`, `deepseek`, and
`anthropic` secrets in Credential Manager with the documented 2,560-byte generic
credential ceiling. TypeScript receives only configured/store/restart status with
`plaintext_returned=false`. Keys do not enter SQLite, events, logs, diagnostics, model
context, or browser persistence. Non-Windows builds fail closed without a plaintext
fallback; environment variables remain higher priority. A restart is intentionally
required before the Provider Registry uses a changed system credential.

D1-J1 adds `run_wake_worker.v1`. The worker starts only with
`--enable-wake-worker` and a distinct control token, owns one serial process-lifetime
loop, and consumes at most one due intent with `max_steps=1` through the existing
Foreground Wake Consumer/RunSupervisor. Durable wake ownership, budgets, Policy,
leases, checkpoints, and cancellation are unchanged. It owns no Tool Runner and has no
Shell, LocalRunner, Docker, or service/startup authority.

The ordinary integrated gate passed the final uncached full Go suite in 327.6s,
`go vet`, secure Desktop-tag tests, deterministic OpenAPI/TypeScript, strict
TypeScript, 102 React tests across 28 files, the Vite production build, and a
zero-vulnerability npm audit. A reproducible Windows double build produced SHA-256
`a0e6aa0a3d15ccc39712f8a0a64d7de06e4a6af426e060b6378b1011c93a1cf6` and correctly
kept `release_ready=false`.

The combined review fixed the Windows credential-size limit, exact Provider-name
normalization, one bad credential read disabling the whole application, worker restart
after Desktop close, FileEdit ID/intent drift after an uncertain save, internal errors
on post-review replay, a `models:null` contract failure for unconfigured Providers, a
secret-clearing frontend test race, and Monaco CDN/dependency risk. Monaco 0.53.0 and
all five workers now ship locally and lazily; desktop and 390x844 mobile UI smoke pass,
and npm reports zero known vulnerabilities. No unresolved high/medium issue is known.
No real key, Provider
request, Shell, LocalRunner, or Docker operation was used. Current estimates are
architecture about 99% (V2 about 99%), complete-product usability about 84-87%, generic
Coding Agent workflow about 82%, and Cyber automation about 20%. ADR 0045 is
authoritative.

## Next Slice

The recommended next three-slice batch is:

1. D1-I2: add safe editor recovery for an expired source handle or a durable pending proposal. Recovery must reissue from the current Go projection, surface stale-file conflicts, and never let renderer draft state bypass proposal/review/apply separation.
2. D1-M4: add an explicit Go-owned Provider Registry reload after credential changes. A generation swap must not race or cancel an active model call, leak old/new keys, or make one unavailable Provider deny mock and unrelated Providers.
3. D1-J2: add a bounded read-only browser-capability and wake-worker health projection plus explicit process-local drain/shutdown status. It must reveal no owner/lease/token/private error and must not become a runtime enable endpoint, persistent service, or wider scheduler. This closes the current low-risk gap where ordinary `api serve` React conservatively hides D1-I1/M3/J1 controls while direct authorized routes and Desktop bootstrap work.
4. This batch reaches six slices, so finish with the complete ordinary/race/vet/staticcheck/govulncheck/dependency/privacy gate plus Desktop/browser recovery checks. The manual Windows 10 matrix, signed/package distribution, real Sandbox/host processes, Rust analyzers, xterm, and CTF solving remain separate work requiring their own gates and, where noted, operator hardware/signing input.

## Local Machine Note

The default `~/.cyberagent-workbench/cyberagent.db` currently carries a historical schema-v30 checksum that differs from this repository's immutable migration definition, so CLI startup correctly fails closed with `migration 30 checksum or name mismatch` and Desktop shows a bounded `FAILED_PRECONDITION`/startup code instead of silently resetting it. The v75-v77 and D1-Q2 through D1-I1/M3/J1 slices did not rewrite migrations 1-74, and fresh/upgrade fixtures pass. Preserve that local database for backup/diagnosis; do not delete it or rewrite `schema_migrations` automatically. Desktop visual and recovery tests use separate `CYBERAGENT_HOME` directories under the OS temporary root.

## Delivery Loop

Work in batches of three focused slices. During implementation, run focused compile and regression checks; after the third slice, run one integrated functional gate, review the combined behavior/diff, update README/status/progress/task memory, commit on `main`, push, and verify CI. Every second batch, after six slices, additionally run the complete race/vet/staticcheck/govulncheck/dependency/privacy robustness gate. Keep real Sandbox execution and CTF automation closed until their dedicated audits pass.
