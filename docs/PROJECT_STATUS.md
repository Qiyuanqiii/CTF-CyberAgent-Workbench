# Project Status

Last updated: 2026-07-20

## Resume Context

Current database schema is v82. Schemas v78-v81 add immutable operator verification evidence, livelock recovery, verification plans, and explicit plan-item/evidence associations; schema v82 adds conservative complete-request model-context planning and immutable cumulative handoff memory. Non-schema Code/Desktop work now reaches D1-G5/V4, while R2 adds OS-specific process-tree conformance adapters only to the Go test boundary. Root and Specialist requests use a 32K conservative fallback with explicit output/safety reservations, repeated compaction preserves a predecessor-bound cumulative handoff chain, arbitrary repository documents remain untrusted evidence instead of instructions, exact-commit preview is bounded and redacted, and Code Handoff exposes verification coverage without inventing an aggregate verdict. None grants general Shell, LocalRunner, Docker, child scheduling, install hooks, renderer host-path authority, credential readback, document instruction authority, automatic Skill selection, or product process execution.

Schema v63 remains the blocked Sandbox start-gate review; schemas v48-v68 keep Local and container-process execution disabled. Schema v64 records only backend preference, schema v65 records non-authorizing machine-capture receipts, schema v66 adds recoverable ownership, schema v67 permits only five fixed read-only daemon GETs after explicit Linux opt-in, and schema v68 records a non-authorizing receipt decision without contacting Docker. No product path starts a Runner, container, Shell, or host process. ADR 0024 through ADR 0053 record the Skill, Sandbox, Desktop, Run-control, foreground-wake/worker, FileEdit proposal/recovery/review/apply, Provider credential/generation, inert-install, receipt, Explorer/search, Repository, Verification, Handoff, process-conformance, and portable-build boundaries.

CyberAgent Workbench is a local-first Go agent runtime for cyber-oriented work. The CLI-first implementation has resumable Runs, a durable root Agent Coordinator, bounded review-gated Specialist delegation, a separate read-only 1/2/4/6 Fan-out pool, persisted sessions and model calls, context compaction, WorkItems/Notes/Artifacts, a unified Tool Gateway, embedded and inert user Skills, Finding/Evidence/Report lifecycles with SARIF/CI output, loopback HTTP/SSE/OpenAPI, a Run-first TUI, a React/Vite console, and a Windows Wails shell with independently gated Run/Session/Plan/approval, FileEdit proposal/review/apply, Provider credentials, foreground/bounded wake, inert Skills, actions/evidence, and navigation. Core delegation remains capped at two children and only the original application operator can schedule it; models, ordinary tools, HTTP, and the Desktop native bridge cannot autonomously spawn or schedule children.

Schemas v48-v68 form the current Sandbox chain: strict Manifest preparation, exact approval and disabled candidates, generation-fenced lifecycle recovery, fixed threat checks, simulation evidence, read-only Docker observation, deterministic plans, never-started rehearsals, sealed host-input capture and handoff, read-only runtime-input projection, retained-resource cleanup, a blocked start-gate review, non-authorizing execution-profile selection, production-evidence receipts and recoverable attempts, the bounded opt-in Linux read-only daemon harness, and an immutable operator receipt review. These records, including an accepted v68 receipt, grant no execution authority. Local and container-process execution remain disabled, and TypeScript, future Rust analyzers, Skills, models, documents, and approval facts cannot bypass Go Policy, Scope, budgets, or the Tool Gateway.

Every Finding lifecycle decision is an additive immutable overlay that leaves v35 model assertions and their source projection digest unchanged. SARIF and GitHub annotations expose only Gate-selected source Finding data, never private lifecycle narratives; additional CI-platform adapters remain pending.

Schema v41 gives every Run a Go-owned immutable `run_mode.v1`: `code|cyber` is the execution surface and `plan|deliver` is the execution phase. Surface, Profile, Scope, and policy binding cannot change within a Run. Phase changes are explicit, digest-keyed operator operations allowed only for `created` or quiescent `paused` Runs. Plan completion fails closed at Supervisor, application, Store, and SQLite boundaries. Mode is projected consistently through CLI, TUI, HTTP/OpenAPI, and React, and it grants no capability.

Schema v42 now adds strict Go-owned `plan_delivery.v1`. A root Plan turn may persist exactly three bounded directions but cannot choose or execute one. An operator may choose 1/2/3 only after the Run pauses and releases its lease; one transaction creates the immutable selection, selected WorkItem dependency graph, pinned decision Note, and metadata-only events. The Run remains in Plan until a separate phase operation. CLI is the only selection surface; HTTP, TUI, and React are read-only and project no capability grant.

Schema v43 adds immutable `context_provenance.v1`. New Session rows bind redacted content hashes to one strict source/role/authority tuple; only operator messages and Go control text are instruction-authorized. Workspace reads/listings/diffs, tool results, slash-command results, Notes, WorkBoard, inbox payloads, and compacted history are model-visible only as user-role untrusted evidence. Go verifies each v1 digest on read, SQLite prevents source/content mutation or deletion, and legacy v42 data is conservatively backfilled as v0. The concrete README indirect-injection regression is covered across direct Session, root Supervisor, Specialist, compaction, Store migration/tampering, HTTP/OpenAPI, and React types.

Schema v44 adds immutable `delivery_checkpoint.v1` over accepted Plan WorkItems. A quiescent paused Deliver Run and an exact `in_progress` WorkItem version are required before the operator can attest focused verification, diff/security review, and a compact handoff; the final selected module also requires full functional and robustness review. One transaction freezes the checkpoint, digest-only idempotency operation, pinned handoff Note, relations, and metadata-only events. Existing WorkItem and Run completion paths consume those facts in Go and SQLite. HTTP, TUI, and React remain read-only and omit evidence, digests, operation identity, and requester identity. Partially completed pre-v44 selections remain explicitly unenrolled rather than receiving invented history.

Schema v45 adds ordered, exactly-once operator steering at safe root-turn boundaries. Schema v46 adds immutable pending-only cancellation facts, digest-only cancellation operations, a steering-only Supervisor begin path, lease-owned wake/drain, and caller-supplied Session retry identity. A prepared message cannot be cancelled; queue order and content remain immutable. Non-schema D1-S1 exposes enqueue/replay for an exact Run-bound Session; D1-S2 separately exposes exact pending-only cancellation and derives `prepared` for fail-closed UI eligibility. Schema v73 D1-L1 exposes idempotent start/pause/resume, while D1-X1 freezes at most eight pending identities and drains only that set through the existing RunSupervisor under a private lease. Later appends do not enter the batch, selected cancellations are skipped, and HTTP/Desktop still cannot reorder or edit input or grant process/tool authority.

Schema v47 adds minimal `specialist_skill_context.v1` delivery. Go binds the parent selection, current mode, child assignment fingerprint, and active AgentAttempt, then selects at most one already-pinned guide. Code receives only its matching Profile guide; Cyber receives none except `script` for the Script Profile; `plan-delivery` remains root-only. A metadata-only preparation commits atomically with the first Specialist model start. Bodies, paths, names, versions, and content hashes remain out of SQLite and Run events, while the two-child, no-tool, Policy, Scope, and Fan-out boundaries remain unchanged.

Schema v48 adds strict `sandbox_manifest.v1` plus immutable preparation, validation, and digest-keyed operation ledgers. Go hard-bounds command/argv, workspace-relative mounts, sandbox paths, exact network targets, resources, environment literals or secret references, input Artifacts, outputs, timeout, and cancellation grace; it rejects ambiguous JSON, path/capability ambiguity, credential-shaped values, and Scope widening. The normalized fingerprint is bound to one non-terminal Run, Mission, persisted Workspace root, Mission Scope, Policy decision, optional exact approval, and a generated cancellation identity. Events and tables retain metadata only. Same-key and cross-store replay converge, and Policy denial remains auditable without command content. No process is enabled: Noop is the sole validator, Local/Docker fail closed, and approved intent still has `backend_enabled=false` and `execution_authorized=false`.

Schema v49 adds a shared-ledger Sandbox approval request/review and `sandbox_execution_candidate.v1`. Every candidate resupplies the Manifest, exactly rematches the v48 fingerprints and current Policy/Scope/approval, resolves mount sources through Go `os.Root`, and rechecks aggregate model usage, tool budget, and execution-lease quiescence in the candidate write transaction. Candidate rows/events remain metadata-only, immutable, and digest-replayable across stores. They still fix `backend_enabled=false` and `execution_authorized=false`; no Runner call or process start exists.

Schema v50 adds `sandbox_execution.v1` without enabling a backend. Lifecycle creation resupplies the Manifest yet again, rechecks every v48-v49 binding, verifies exact input Artifact content and scope under a 16 MiB aggregate cap, and persists only metadata for output capture. A separate generation-fenced Sandbox lease supports restart takeover and cleanup after a terminal Run. Cancellation and cleanup are immutable digest-idempotent facts; the only cleanup outcome is `backend_disabled`, with zero output Artifacts and no backend/orphan claim. CLI can begin, inspect, cancel, and clean these records but never receives lease tokens or owners.

Schema v51 adds immutable `sandbox_preflight.v1` without probing or enabling a backend. It resupplies the Manifest, rechecks the complete v48-v50 authority chain and live budgets/leases/Artifacts, then freezes exactly 16 required-but-unverified backend checks, an unavailable handshake, an unbound container identity, and a metadata-only output-export plan. Output slots retain opaque locator fingerprints and kinds only; all-or-nothing commit, aggregate byte limits, MIME/redaction, regular-file-only handling, link/special-file rejection, and restart reconciliation are fixed while export and Artifact commit remain unauthorized. CLI can create/list/show the fact but omits locators, paths, commands, Manifest content, container identity, and private leases.

Schema v52 adds immutable `sandbox_backend_evidence.v1` and `sandbox_output_simulation.v1` facts without adding a daemon transport or production Artifact writer. The in-memory client binds a canonical OCI image digest and independent daemon, mount, network, secret, container-configuration, resource, termination, orphan, and output-plan fingerprints to the 16 v51 checks. Every result is explicitly `simulation_only/simulated_pass`, remains unverified, and fixes production verification, backend availability, execution, export, and Artifact authority to false. The strict output harness validates ordered slots, aggregate bytes, MIME, redaction, regular-file type, and link/special-file rejection before one atomic fake commit; failure or cancellation rolls back to zero and production `run_artifacts` remain unchanged. Both boundaries revalidate the complete v48-v51 authority chain and live budgets, leases, mounts, and input Artifacts. CLI/events/persistence omit fixture bodies, locators, paths, commands, Manifest content, secrets, container IDs, operation digests, and private leases.

Schema v53 adds immutable `sandbox_docker_observation.v1` facts without adding a write-capable daemon path. The minimal transport has only ping, daemon version/info, and exact-digest image inspection. Linux uses a fixed local Unix socket and four allowlisted GET shapes; Windows records an explicit unsupported result. It ignores `DOCKER_HOST`, accepts no arbitrary endpoint, follows no redirect, calls no Docker CLI, and exposes no create/start/run/exec/pull/remove method. Explicit CLI confirmation and the exact v52 evidence/simulation/Manifest are required. Application and SQLite revalidate v48-v52 authority, Policy, approval, budgets, leases, input Artifacts, cancellation, and cleanup before storing complete, daemon-unavailable, or image-unavailable facts. Raw daemon/socket/repository identity is not persisted. Complete metadata observation is still `production_verified=false`; private mounts remain unobservable and all backend/execution/Artifact authority remains false.

Schema v54 adds deterministic `sandbox_docker_container_spec.v1`, immutable metadata-only `sandbox_docker_container_plan.v1`, and in-memory-only `sandbox_docker_write_transaction.v1`. Compilation accepts only a complete v53 observation and resupplied exact Manifest after Application and SQLite revalidate the full v48-v53 chain. It fixes non-root identity, read-only root and inputs, one writable output, private propagation, default-deny/exact-allowlist networking, ephemeral secrets, resource/time/kill limits, authority-derived orphan labels, and stop-before-export ordering. The full specification remains transient. Sixteen controls are `compiled_not_applied`, and seven fake steps commit only after all staging succeeds; failure, simulated crash, or cancellation leaves zero fake transactions. Durable rows, events, and CLI omit command, argv, paths, targets, environment values, secret references, labels, and container names. Daemon writes, backend contact, production submission, execution/export, and Artifact authority remain false.

Schema v55 adds default-disabled `sandbox_docker_write_transport.v1` and immutable metadata-only `sandbox_docker_container_rehearsal.v1`. An explicit operator confirmation and exact current v54 plan are required before the Linux-only transport can connect to fixed `/var/run/docker.sock`. It pins Docker API `1.40`, disables proxy/redirect/environment endpoint selection, and exposes only exact image inspect, create, exact container inspect, and non-forced delete with fixed `v=1` anonymous-volume cleanup. The accepted profile has no network, environment, or secrets; the local RepoDigest must match and the image must declare no `VOLUME` before create. The container uses the digest-pinned v54 image and exact non-root/read-only/capability/resource/private-mount controls, but it is never started. A stopped name collision is removed only after exact configuration and authority-label matching. Cancellation, partial failure, or an uncertain create response uses an independent bounded re-inspection and never blindly deletes a returned ID. Application and SQLite revalidate v48-v54 before transport and commit. Replay does not contact Docker and concurrent Stores converge. Persistence/events/CLI omit raw IDs, host paths, commands, environment values, secrets, socket paths, and specifications. The normal path records three daemon reads and two writes, or three writes for one exact stale rehearsal, while image pull, process execution, output export, production verification, backend enablement, execution authority, and Artifact authority remain false.

Schema v56 persists `sandbox_docker_container_rehearsal_attempt.v1` before any daemon mutation. A bounded SQLite lease with monotonically increasing generation fences stage, cleanup, failure, completion, takeover, and replay. `Stage` creates at most one deterministic-name stopped container or adopts an exact prior authority match, then stores 19 immutable inspected controls with `execution_evidence=false`; image and container inspection now both require an actually empty environment. `Cleanup` re-inspects and removes only the exact request/configuration/authority/container-ID-fingerprint match, while absence is idempotent and mismatched same-name containers are protected. Failures append bounded codes and release the lease. An operator can list, inspect, and resume by durable attempt ID only with the full Manifest and a fresh explicit daemon-write confirmation; the raw operation key is neither required nor exposed. Legacy rehearsal, operation, completion, and lease release commit atomically. The v55 fixed endpoint and closed operation set are unchanged, and host-mount TOCTOU remains a required independent gate before any future start boundary.

Schema v57 implements that independent capture gate without opening start. `DockerHostInputStager` is default-disabled and needs separate operator confirmation. Linux resolves the absolute workspace root and every read-only input tree through `openat2` no-symlink/no-magic-link/beneath/no-cross-device constraints. `O_PATH` preflight means FIFOs and other special files fail before a potentially blocking content open; directory and single-file mounts are supported, while hard links and excess depth/entries/bytes fail closed. Descriptor identity and metadata are rechecked after all entries are pinned. A deterministic sanitized tar is written to a sealable `memfd`, receives write/grow/shrink/seal kernel seals, and is reread for digest verification. Exact input Artifacts are reverified immediately before capture. The immutable intent/result is bound to the active v56 attempt, stopped-container fingerprint, v54 plan, input digest, requester, and lease generation; SQL prevents completion while evidence is pending. Failure performs bounded best-effort stopped-container cleanup and preserves a resumable intent. CLI list/show remains metadata-only. The bundle is not handed to Docker, so the durable trust class is explicitly local descriptor rehearsal, `daemon_consumed=false`, and `execution_evidence=false`; a daemon-owned immutable handoff remains required before start.

Schema v58 closes the remaining post-stage/pre-intent recovery downgrade for new attempts without opening a Docker operation. Application derives an immutable host-input requirement before daemon stage and commits it atomically with the attempt, first lease, and audit events. The fact binds required/confirmed choice, attempt, plan, Run, Mission, Workspace, requester, digest-only operation identity, complete authority fingerprints, and bounded input counts. Recovery treats it as authoritative: required capture proceeds without resubmitted staging flags, false cannot be widened, and completion cannot pass without matching v57 evidence. Go and SQLite independently enforce binding, immutability, and completion/staging gates; legacy v57 attempts are not backfilled with invented operator intent. Queries and events remain metadata-only. The audit rejected direct archive upload into the read-only target and added no archive, volume, start, exec, pull, build, export, or Artifact capability. The bundle remains daemon-unconsumed; schema v59 is reserved for a daemon-owned carrier, exact readback, removal, and final read-only attachment. See ADR 0018.

Schema v59 implements that separate handoff without opening start. Every new attempt receives an immutable handoff requirement with its v58 requirement before daemon stage. A write-ahead intent binds the exact v57 bundle, stopped target, lease generation, plan, requester, and authority before a fixed archive/volume operation can occur. The Linux-only, default-disabled transport uses Docker API `1.40`, deterministic resource names, one local volume, one never-started writable carrier, and only `/cyberagent-input/bundle.tar`. It verifies exact daemon readback, removes the carrier and original stopped target, inspects a never-started target with the volume read-only, then removes target and volume. Manifest mounts cannot overlap the reserved destination. Exact crash residue converges; foreign collisions fail closed; early failure cleanup deletes only exact owned resources. SQLite gates required cleanup/completion on the immutable result. CLI list/show remains metadata-only, while start, exec, attach, pull/build, network mutation, export, backend, execution, and Artifact authority remain false. See ADR 0019.

Schema v60 adds an operator-confirmed runtime-input projection plan without a Docker transport. Application revalidates v48-v59, recompiles the exact Manifest and v54 specification, and recaptures the sealed input through the v57 provider. Its report, digest, length, counts, and Artifact payload must match v57/v59. Go accepts only byte-for-byte canonical PAX tar, requires directory roots for the first read-only-mount profile, maps Artifacts to fixed `/cyberagent-input/artifacts`, and compiles one relative archive per target. Future volume identity includes the immutable handoff fingerprint, preventing identical cross-Run inputs from colliding. SQLite atomically stores operator confirmation, digest-only ordered items, aggregate completion, operation, and event. CLI plan/list/show is metadata-only. Raw targets, host paths, file names/content, volume names, and archive bytes are transient; status remains `compiled_not_applied` and every daemon/start/exec/export/backend/execution/Artifact flag is false. See ADR 0020.

Schema v61 adds a second, dual-confirmed application gate. An immutable intent plus independent generation lease commits before daemon mutation and binds the exact v60 projection, v59 handoff, attempt, plan, Manifest, endpoint, requester, and operation digest. Application revalidates the complete v48-v60 chain, recompiles the exact specification, resolves the output bind again, recaptures the sealed bundle, and requires an identical projection compilation. The fixed local-Unix Docker transport creates deterministic local volumes and never-started carriers, uploads only at `/cyberagent-input`, reads every archive back for complete semantic verification, removes carriers, and creates one final target with all input volumes read-only/`NoCopy` and the reviewed output bind as its sole writable mount. Exact owned residue converges; foreign collisions are never deleted. Current-generation fencing, typed failures, released-generation resume, independent bounded cleanup, and a pre-expiry cleanup reserve cover recovery. Operation replay returns durable metadata without recapture or daemon contact. Tables, events, and CLI omit raw paths, targets, names, IDs, archives, sockets, keys, and lease identities. Status is `volumes_applied_target_never_started`; start, exec, export, backend, execution, and Artifact authority remain false. Windows returns unsupported. See ADR 0021.

Schema v62 adds a third, separately authorized retained-resource lifecycle. Read-only inspection reconstructs the exact descriptor from the completed v61 result and current v48-v61 authority without recapturing input, then classifies the target and every volume as exact-owned, absent, or foreign. Complete read-only/`NoCopy` evidence requires the exact never-started target and all volumes. Cleanup requires its own operator and daemon-write confirmations, persists an immutable intent and generation lease before Docker access, preflights every resource before any DELETE, rejects a foreign collision with zero DELETE, removes the target by inspected ID before exact volumes, and rechecks total absence. Typed failure release, takeover, stale-worker fencing, undeletable leases, active-window timestamps, semantic replay, immutable SQL, metadata privacy, and Windows unsupported behavior are covered. Events and CLI retain no names, IDs, paths, sockets, raw keys, or private lease identities. Success is only `exact_owned_resources_absent`; start, exec, export, backend, execution, and Artifact authority remain false. See ADR 0022.

The v62 release gate passed full ordinary/race Go suites (313.6s/329.6s), vet/staticcheck/module/govulncheck, strict TypeScript, 17 frontend tests, OpenAPI/build/npm audit, repository scans, Linux cross-compilation, isolated schema-v62 binary smoke, and repeated four-layer lifecycle tests. GitHub Actions run `29444398815` passed feature commit `d250d32` with Go/Linux in 2m35s and TypeScript in 20s. No unresolved high/medium issue is known; the unexecuted Linux real-daemon opt-in chain remains the explicit start-gate blocker.

Schema v63 adds immutable `sandbox_docker_start_gate_review.v1` facts after completed v62 cleanup. Exact Manifest resubmission and explicit design-review confirmation are required; Application revalidates the complete v48-v62 chain without recapturing input or contacting Docker. The sixteen v51 checks each receive one fixed evidence class/source, blocker code, and future gate, while all remain unverified and insufficient. An eleven-transition lifecycle blueprint fixes future per-Run generation-fenced ownership, write-ahead state, fixed endpoint, bounded logs, wait, TERM/KILL escalation, cancellation fan-out, uncertain-start handling, and orphan reconciliation, but all transitions remain unimplemented and unauthorized. The review outcome is always `blocked/deny_start`, all process/output/Artifact authority bits are false, migration fabricates no history, and CLI/list/show output is metadata-only. See ADR 0023.

The v63 local release gate passed full ordinary/race Go suites (196.9s/212.3s), vet/staticcheck/module/govulncheck, strict TypeScript, 17 frontend tests, OpenAPI/build/npm audit, repository privacy/capability/encoding/diff scans, Linux sandbox cross-compilation, isolated schema-v63 real-binary Workspace/Skill smoke, and Sandbox/Store/Application/CLI repetitions of 20/15/10/10. The audit fixed missing immediate child-row iteration-error returns and static error-string casing; no unresolved high/medium issue is known. The Linux real-daemon opt-in chain remains the explicit blocker for production start evidence. GitHub Actions run `29503856229` passed commit `e25a2ab` with Go/Linux in 2m32s and TypeScript in 24s.

The read-first console now includes Go-owned projections for the current Run mode and Plan/Delivery state, the bounded Agent graph, operator-gated delegation lifecycle, read-only Fan-out plans/latest execution summaries, Finding/Report facts, and schema v64 execution profiles. Browser DTOs deliberately omit raw Fan-out reports, private decision narratives, Artifact content, operation digests, and lease/fencing identities. Its only mutation is non-authorizing profile selection; it adds no model call or process-execution capability.

P9 now also includes `headless.v1`: a read-only, sequence-resumable NDJSON projection of the same persisted Run events. It can take a bounded snapshot or follow local SQLite until terminal state, emits a final machine-readable exit record, and maps completed/failed/cancelled/bound/deadline outcomes to stable exits 0/4/7/8/9. It adds no schema, execution loop, Provider call, tool, network, or file mutation.

The Bubble Tea entry surface is now Run-first. Its picker uses bounded Store pages for the latest 50 Runs and Sessions, validates Run/Mission/Session bindings, and supports exact `tui --run` opens with a reverse projection check. A Run adds an Edits activity view and full-screen read-only detail for at most 20 exact-Session/Workspace FileEdit previews. The dedicated SQL projection excludes original and replacement file bodies; displayed diff lines are terminal-safe and capped at 128 KiB/4096 lines. Approval keys still mutate only the Tools view, and the diff screen has no review or write authority.

Current product priority: migrate the working v0.1 scaffold into the V2 run-centric, resumable agent runtime described in ADR 0002 and `docs/TASK_BOOK.md`. CTF-specific solving logic is intentionally deferred until the generic runtime is stable.

Canonical remote: `https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench`. Group work into three focused slices, then run an integrated functional gate, review the combined diff, update project memory, commit, push, and verify CI. Every second batch, after six slices, adds the complete race/static/vulnerability/dependency/privacy robustness gate. This repository currently develops directly on `main`; do not create a branch or pull request unless the user explicitly requests one.

Use these files first when resuming:

- `README.md`
- `LICENSE`
- `docs/PROGRESS_BOOK.md`
- `docs/TASK_BOOK.md`
- `docs/adr/0001-go-control-plane.md`
- `docs/adr/0002-run-centric-runtime.md`
- `docs/adr/0003-run-execution-modes.md`
- `docs/adr/0004-plan-delivery-workflow.md`
- `docs/adr/0005-operator-steering-queue.md`
- `docs/adr/0006-operator-steering-controls.md`
- `docs/adr/0007-specialist-skill-context.md`
- `docs/adr/0008-sandbox-manifest-boundary.md`
- `docs/adr/0009-sandbox-approval-candidate.md`
- `docs/adr/0010-disabled-sandbox-lifecycle.md`
- `docs/adr/0011-disabled-sandbox-preflight.md`
- `docs/adr/0024-strict-inert-skill-package.md`
- `docs/adr/0031-content-addressed-inert-skill-registry.md`
- `docs/SKILL_PACKAGE_PLAN.md`
- `docs/architecture.md`
- `docs/usage.md`
- `docs/http-api.md`
- `docs/openapi.json`
- `internal/app/app.go`
- `internal/app/headless_command.go`
- `internal/headless/export.go`
- `internal/application/run_supervisor.go`
- `internal/application/specialist_runner.go`
- `internal/application/skill_packages.go`
- `internal/skills/specialist.go`
- `internal/skills/package.go`
- `internal/skills/installation.go`
- `internal/skills/object_store.go`
- `internal/store/specialist_skill_context.go`
- `internal/sandbox/manifest.go`
- `internal/sandbox/intent.go`
- `internal/sandbox/preflight.go`
- `internal/application/sandbox_manifest.go`
- `internal/application/sandbox_preflight.go`
- `internal/store/sandbox_manifests.go`
- `internal/store/sandbox_preflight.go`
- `internal/app/sandbox_command.go`
- `internal/application/specialist_scheduler.go`
- `internal/application/specialist_model_cancellation_watch.go`
- `internal/application/specialist_context.go`
- `internal/application/execution_lease.go`
- `internal/coordinator/coordinator.go`
- `internal/coordinator/runtime.go`
- `internal/domain/agent_node.go`
- `internal/domain/agent_attempt.go`
- `internal/domain/agent_context.go`
- `internal/domain/specialist_action.go`
- `internal/domain/specialist_repair.go`
- `internal/domain/specialist_schedule.go`
- `internal/domain/specialist_model_cancellation.go`
- `internal/store/coordinator.go`
- `internal/store/coordinator_attempts.go`
- `internal/store/coordinator_completion.go`
- `internal/store/coordinator_snapshots.go`
- `internal/store/root_inbox_context.go`
- `internal/store/specialist_models.go`
- `internal/store/specialist_repairs.go`
- `internal/store/specialist_schedules.go`
- `internal/store/specialist_model_cancellations.go`
- `internal/store/specialist_context.go`
- `internal/domain/execution_lease.go`
- `internal/store/execution_leases.go`
- `internal/domain/root_action.go`
- `internal/domain/run_mode.go`
- `internal/domain/plan_delivery.go`
- `internal/domain/work_item.go`
- `internal/domain/note.go`
- `internal/application/work_item_service.go`
- `internal/application/run_mode.go`
- `internal/application/plan_delivery_tool.go`
- `internal/application/plan_delivery_selection.go`
- `internal/application/note_service.go`
- `internal/contextmgr/context.go`
- `internal/contextmgr/selector.go`
- `internal/session/session.go`
- `internal/toolgateway/model.go`
- `internal/toolgateway/gateway.go`
- `internal/toolgateway/structured_memory.go`
- `internal/application/supervisor_tools.go`
- `internal/domain/supervisor_tool.go`
- `internal/store/supervisor_tools.go`
- `internal/llm/anthropic.go`
- `internal/application/structured_memory_tool.go`
- `internal/runmutation/operation.go`
- `internal/store/structured_tool_operations.go`
- `internal/toolgateway/script_process.go`
- `internal/toolgateway/artifact_capture.go`
- `internal/artifact/artifact.go`
- `internal/store/run_artifacts.go`
- `internal/scriptprocess/scriptprocess.go`
- `internal/application/script_process_service.go`
- `internal/store/script_processes.go`
- `internal/approval/grant.go`
- `internal/store/approval_grants.go`
- `internal/store/tool_budget.go`
- `internal/toolrun/toolrun.go`
- `internal/tui/model.go`
- `internal/tui/picker.go`
- `internal/tui/edit_activity.go`
- `internal/store/sqlite.go`
- `internal/store/migration_v69.go`
- `internal/store/skill_package_installations.go`
- `internal/store/run_modes.go`
- `internal/store/work_items.go`
- `internal/store/notes.go`
- `internal/httpapi/server.go`
- `internal/httpapi/handlers.go`
- `internal/httpapi/projection_handlers.go`
- `internal/httpapi/projection_views.go`
- `internal/httpapi/openapi.go`
- `internal/httpapi/event_stream.go`
- `internal/httpapi/control.go`
- `internal/webui/bundle.go`
- `internal/domain/model_cancellation.go`
- `internal/store/model_cancellations.go`
- `internal/application/model_cancellation_watch.go`
- `internal/app/api_command.go`
- `web/README.md`
- `web/src/api/client.ts`
- `web/src/api/sse.ts`
- `web/src/components/run-workspace.tsx`
- `web/src/components/run-projections.tsx`
- `web/src/components/session-workspace.tsx`
- `internal/domain/finding_report.go`
- `internal/application/finding_report.go`
- `internal/store/finding_reports.go`
- `internal/store/finding_validation.go`
- `internal/store/finding_remediation.go`
- `internal/report/render.go`
- `internal/report/sarif.go`
- `internal/report/gate.go`
- `internal/app/report_command.go`

## Progress Review

- Architecture completion: about 99%; the V2 run-centric control plane is about 99% complete.
- Product usability: about 93-95% for the complete Code + Cyber product.
- Generic coding-agent workflow usability: about 93%.
- Cyber autonomous-workflow usability: about 20%.
- These values are engineering estimates derived from tested roadmap slices, not performance benchmarks. The retired single-axis "overall product vision" percentage must not be used for current status.

Latest implemented batch: D1-G3/V2/F3 and schema v80. It adds bounded exact-root local repository history, immutable operator-authored verification plans separate from outcomes, and digest/high-water-bound Markdown/JSON Code Handoff export. Its ordinary Go/frontend/build/Chrome gate is green with no unresolved high/medium issue known on an enabled path. ADR 0050 is authoritative.

Completed:

- H1/H2/H3 runtime blocking guards: default 15-second/five-minute-maximum Tool deadlines, caller cancellation, panic recovery, context-aware bounded regular-file reads, a 4,096-node/8,192-edge reference-counted synchronous wait graph, permanent lower-layer-to-Agent callback rejection, root/Specialist/Tool integration, and schema v79's atomic three-repeat/six-stagnant Run pause with exactly-once replay and explicit-resume recovery. These controls grant no process, Shell, network, Local, Docker, or child authority.
- Schemas v48-v55 Go-owned Sandbox Manifest preparation, shared approval request/review, exact Manifest resubmission, symlink-safe mount-source resolution, transactionally rechecked budgets/lease, immutable disabled candidates, Artifact-bound disabled lifecycle, independent generation fencing, cancellation/cleanup recovery, fixed required-but-unverified backend checks, metadata-only output preflight, simulation-only backend evidence, atomic fake output transactions, fixed-endpoint read-only Docker observations, deterministic container plans, fake write rollback, default-disabled create-inspect-remove rehearsals, digest-keyed replay, and the corresponding CLI. Local and container-process execution remain disabled even after approval, preflight, simulation, observation, plan compilation, or non-started daemon rehearsal.
- Schema v41 Go-owned Run modes with immutable `code|cyber` surface, append-only `plan|deliver` phase revisions, digest-keyed replay, quiescent transition checks, Plan completion denial, legacy `code/deliver` backfill, and consistent CLI/TUI/HTTP/OpenAPI/React projection. Mode remains independent from Policy, Scope, approvals, tools, network, Sandbox, and child admission.
- Go CLI entrypoint and command dispatch.
- Schema v19-v38 Agent Coordinator with stable root identity, idempotent inbox operations, strict wake/dependency semantics, explicit internal-only Specialist admission, validated same-Run Agent ownership for WorkItems/Notes, exact-attempt CompletionReports, a default-disabled Specialist Attempt Runtime, two-phase exactly-once root and Specialist instruction context, internal no-tool Specialist model turns, one isolated child lifecycle repair, durable schedule start/stop summaries, exact cross-process child-call cancellation, review-gated root delegation proposals, immutable operator review facts, recoverable operator application, and explicit operator schedule requests. A policy permits at most two depth-one children with parent-Skill subsets, dedicated Sessions, reserved budgets, lease-fenced turns, cumulative exactly-once usage accounting, redacted crash notifications, takeover recovery, lifecycle interruption, SHA-256-backed recovery snapshots, and atomic Supervisor/Run integration. The scheduler runs at most two ready children per round under one lease, fans cancellation out to siblings, reconciles root plus child token/model-time usage from SQLite before and after every round, and converges orphaned schedules to `abandoned/worker_lost` on takeover. Schema v26 atomically records child model terminal state, usage, Policy, and allowed redacted Session messages. Schema v27 selects strict direct-parent instructions plus active child-owned WorkItems/Notes and preserves pending instructions across crash, interruption, and takeover. Schema v28 separates global model sequence from primary/repair transport counters, charges both valid usage reports cumulatively, excludes raw invalid output from prompts/history/events, and aborts unresolved repair before Attempt termination. Schema v29 keeps schedule/cancellation events free of model text and fencing identities. Schema v30 verifies the active root, lease, scope, parent-Skill subset, remaining child capacity, and suggested budget before persisting an immutable proposal. Schema v31 records one redacted approved/rejected decision with digest-only replay. Schema v32 rechecks Policy and live invariants, then correlates each existing admission/message operation with a recoverable assignment transition; it creates ready children but no Attempt or schedule. Schema v38 requires a same-operator immutable request before those children may execute. Public/model approval, application, spawn, and autonomous scheduling remain unavailable; ordinary tools and HTTP cannot schedule.
- Authenticated loopback-only `api.v1` read plane with stable envelopes, typed errors, bounded cursor pagination, graceful shutdown, and Run/Session/Event/WorkItem/Note/Artifact/ToolRound plus token-free execution-lease inspection.
- Go DTO/OpenAPI-first Workspace explore/search, repository state/Diff/history, model availability/diagnostics/routes/generation, runtime capabilities/worker health, Run creation/lifecycle/bounded execution/wake intent/foreground consume, controlled Session queue/cancellation/evidence attachment/inventory, operator action center, Plan/Deliver, approvals, verification evidence/plans, Code Handoff/export, FileEdit proposal/read-only recovery/review/apply, inert Skill install, terminal receipt history, Agent graph, delegation, read-only Fan-out, Finding/Report, execution-profile, external-Skill provenance, SSE, and high-water event-poll projections with bounded Store queries and generated React/Vite views. The current contract has 65 paths, 71 operations, and 155 schemas. Ordinary DTOs expose no Workspace root, Provider key/Base URL/environment name, submitted Session body, model/tool output, approval command/path/content, private lifecycle narrative, private identity, operation key, lease owner, or fencing identity; FileEdit recovery is the narrow read-only exception that returns integrity-checked stored bodies for one exact pending proposal, while the sole archive-bearing install request remains strict, bounded, and pathless.
- Schema v18 root and schema v29 Specialist cross-process active-call cancellation with a distinct optional control token, exact Run/Agent/attempt/model preconditions, one-to-one hashed idempotency, audit-first request/observation, worker-owned context signalling, atomic terminal resolution, and stale-attempt/worker-loss cleanup. Read and control capabilities are not interchangeable, and clients never receive or submit fencing tokens.
- Deterministic OpenAPI 3.1 generation from Go DTOs and an explicit route catalog, with `api openapi` stdout/file export, a protected raw `/api/v1/openapi.json` endpoint, a committed golden document, live-handler contract tests, capability separation, and forbidden-internal-field checks.
- Bounded read-only `/api/v1/runs/{run_id}/events/stream` SSE backed by durable SQLite sequences, with Run-bound opaque cursors, `Last-Event-ID` resume, heartbeats, cross-connection polling, per-frame write deadlines, event/time/batch bounds, process-wide connection slots, and server-shutdown cancellation. Go/OpenAPI/TypeScript share the literal envelope version `v1`; the client cancels the response body before reconnect after any parse/transport failure so malformed streams cannot exhaust browser connection slots.
- Bounded read-only `/api/v1/runs/{run_id}/events/poll` for non-streaming embedded renderers, returning real SSE-compatible frames, the same Run-bound high-water cursor, strict contiguous sequences, `limit+1` continuation detection, and no synthetic renderer event source.
- Mock plus environment-backed Anthropic-compatible Providers, one Go-owned model Registry, persisted routes, and redacted no-probe availability.
- CGO-backed SQLite store using `github.com/mattn/go-sqlite3`.
- Workspace layout under `~/.cyberagent-workbench`.
- Script, CTF, learn, provider, model, and context commands.
- Persisted sessions with `session create/list/send/history`.
- Slash commands: `/help`, `/compact`, `/model`, `/workspace`, `/ls`, `/read`, `/run`.
- Workspace-scoped file context tools: `list_workspace` and `read_file` reject absolute paths and traversal outside the workspace root.
- CLI workspace file commands: `workspace tree <name> [path]` and `workspace read <name> <path>`.
- Added `internal/toolgateway` with normalized ToolCall, Decision, Proposal, Execution, Result, Outcome, approval modes, action classes, hard bounds, and cross-object lifecycle invariants.
- Workspace reads, shell proposals, and whole-file replacements now share one Go-owned schema/scope/policy/approval/result pipeline. CLI, Session, and TUI production paths use Gateway adapters rather than constructing legacy managers directly.
- Production file calls resolve a trusted root from the persisted workspace ID and reject a mismatched supplied directory before filesystem access.
- Gateway results carry validated MIME, UTF-8 output, explicit truncation, secret redaction, and hard stdout/stderr/preview limits. Shell approval remains dry-run and file edits remain proposal-first.
- `script run` now requires a persisted workspace and relative existing file, then atomically creates a Script Profile Mission/Run/Session, initial budget charge, typed `script_process.v1` proposal, durable Approval, and full Policy/Tool event projection.
- Schema v13 adds first-class `script_process_proposals`, strict Run/Session/Workspace binding, multiple processes per Run, redacted executable/argv fields, and recoverable `--idempotency-key` replay without persisting the raw key.
- Generic `tool list/show/approve/deny` resolves Shell and ScriptProcess proposals through the durable approval ledger. Script approval advances a recoverable typed state machine and only returns `execution_mode=disabled` dry-run output.
- Schema v14 adds first-class `run_artifacts` for terminal tool output and automatic Run-bound workspace reads. Capture occurs before Result truncation, stores up to 4 MiB of redacted UTF-8 text with MIME/SHA-256/size/source metadata, and appends one content-free `artifact.created` event in the same transaction.
- The Gateway Result remains bounded to 128 KiB stdout and 32 KiB stderr and carries Artifact IDs/hashes/sizes. `artifact list/show/read/verify` inspects descriptors, reads bounded redacted content, and verifies stored hashes; `tool show/approve` links the source proposal back to its Artifacts.
- Artifact source validation binds Shell, ScriptProcess, failed FileEdit, and automatic read/list evidence to the exact persisted proposal or tool-budget invocation. Duplicate terminal review reuses the same Artifact, while changed content, cross-Run scope, event failure, and stored-content tampering are rejected or recoverable without repeating Tool/Approval events.
- Schema v15 adds create-only `work_item_create` and `note_create` tools under the `run_memory` class. Strict JSON and identity validation precede budget charging; Policy and authoritative persisted Run/Session/Workspace binding apply before mutation.
- `tool schema [work_item_create|note_create]` exports provider-ready definitions. `tool invoke` accepts one bounded JSON payload or UTF-8 payload file, requires a stable operation key, derives trusted scope from the Run, and fixes the audit requester to `cli`.
- `structured_tool_operations` stores only domain-separated operation-key digests and normalized redacted-intent fingerprints. Same-intent replay returns the original entity, changed intent conflicts, independent SQLite connections converge concurrently, and successful entity/Policy/domain/tool events commit atomically.
- Schema v16 adds durable Supervisor tool rounds/calls and connects only `work_item_create`/`note_create` to the Provider loop. Each response is limited to four calls and each turn to four rounds; the successful model event and pending batch commit atomically, and restart recovery resumes only pending calls.
- Schema v17 adds one durable execution lease per Run, explicit replay tokens, heartbeat renewal, generation takeover, and checkpoint fencing. `Step` holds one turn lease; `Execute` holds one lease across its bounded loop.
- Every Supervisor checkpoint/model/tool mutation validates the active fencing token transactionally. Structured-memory budget charging and entity persistence both validate it, so a stale worker consumes neither budget nor state after takeover.
- Lease acquisition/takeover/release events contain owner, generation, and timestamps but never `lease_id`. `run lease` and Run detail expose the same token-free metadata.
- Provider call IDs are validated but never persisted. Stable local call IDs and Gateway operation keys derive from Run, turn, tool name, and redacted canonical arguments, so changed Provider IDs and repeated semantic intent converge without duplicate entities or success events.
- Anthropic-compatible non-streaming and SSE paths now send tool definitions, encode `tool_use`/`tool_result` transcripts, parse streamed argument deltas, and return final typed ToolCalls. Protocol repair removes all advertised tools.
- Policy denials and tool-budget exhaustion become bounded metadata-only error results. Storage/cancellation/internal failures leave the call pending; Shell, file, process, network, update, delete, completion, and archive tools remain unavailable to the model.
- `--local` is retained only as requested-backend metadata. Schema v48 uses `NoopRunner` only for deterministic manifest validation; production paths never invoke Local/Docker `Runner.Run`, and tool approval remains dry-run without host side effects.
- JSON payload redaction is structure-aware: Store code parses JSON with exact numbers, recursively redacts string values, and re-encodes before validation/persistence, preserving nested escaped JSON.
- Schema v11 adds Run/Session-bound `tool_approvals` and immutable `approval_operations`; proposal creation appends `approval.requested` transactionally, review commits `approval.decided` before compatibility-state execution, and an identical key resumes safely after restart.
- A proposal created by an unbound legacy Session is transactionally adopted with `approval.bound` if that Session later becomes attached to a Run.
- SQLite rejects ghost approvals, changed proposal fingerprints, conflicting idempotency-key intent, and privileged ToolRun/FileEdit states without a matching durable approval. `approval list/show` exposes the ledger without storing raw command/file content there.
- Schema v12 adds revocable `approval_session_grants` bound to one Run, active Session, Workspace, Tool, and ActionClass. Grant create/revoke operations persist domain-separated key digests and project durable events; `approval grant create/list/show/revoke` exposes the lifecycle.
- Matching active grants may authorize allowed Shell/FileEdit proposals automatically. Revocation takes effect for future proposals, while terminal Runs, archived Sessions, scope mismatch, and permanent Policy denial fail closed. Shell still completes as dry-run only.
- Schema v12 atomically accounts every valid Run-bound Gateway call through `run_tool_usage` and ordered `run_tool_calls`. The first over-budget attempt appends one `tool.budget_exhausted`; subsequent attempts return typed resource exhaustion without duplicating that event. `run usage` exposes counters.
- Secret redaction layer for common API keys, bearer tokens, GitHub tokens, AWS access keys, JWTs, assignment-style secrets, and private-key blocks.
- Redaction is applied at file reads, session message creation, SQLite session/context/tool-run storage, context prompt construction, and final LLM router dispatch.
- Automatic context compaction after long active session histories.
- Optional Anthropic-compatible `mimo` and `deepseek` providers registered from separate environment-variable namespaces only.
- `provider test` and session `/model` both support direct `provider/model` references.
- Tool proposal and approval flow: `/run` creates `tool_runs`; `tool approve` completes dry-run; `tool deny` records denial.
- Persisted file edit lifecycle: `edit propose/list/show/review-approve/review-deny/apply/deny` stores redacted whole-file replacements, unified-style Diffs, separate review intent, and idempotent apply results in `file_edits` plus schema-v76 ledgers. Legacy `edit approve` remains only for non-Run compatibility proposals.
- Session `/write <path> <content>` normally creates a reviewable file edit proposal; a matching active Session grant may apply it immediately, and Session/TUI text reflects the persisted outcome.
- File edit apply re-resolves Workspace paths, rejects traversal/symlink escape, rechecks current Policy and original/current/proposed SHA-256 hashes, refuses stale proposals, verifies the write, and supports idempotent restart recovery without a second write.
- Existing text files and new text files under existing workspace directories are supported; non-UTF-8 content, missing parents, directories, and files over 256 KiB are rejected.
- Run-first Bubble Tea TUI with bounded Run/Session selection, exact `--run` opening, session messages, Run identity/status, snapshot mode, message scrollback, and eight activity views for Shell ToolRuns, WorkItems, Notes, durable Supervisor ToolRounds, Events, Agents, Findings, and bounded read-only FileEdit diffs.
- TUI Tool controls expose `a` for one durable approval and `g` for an exact revocable Session Grant. Session approval verifies the stored proposal fingerprint/scope and current Policy before grant creation; future allowed Shell calls remain dry-run and permanent denial still wins.
- TUI rendering uses terminal-cell-aware grapheme wrapping/truncation for CJK and other wide Unicode text, while bounded Store queries cap each Run memory view.
- TUI Run-first picker/start screen: open one of the latest bounded Runs, switch to recent Sessions, use exact `--run`/`--session`, or press `n` to create a new Session.
- TUI async action loop: sends, refreshes, and tool approve/deny actions enter a `busy` state and complete through Bubble Tea commands without freezing the UI event path.
- TUI workspace context side panel: attached sessions show workspace ID/name/root and lightweight local directory counts without reading file contents.
- Safety policy skeleton for high-risk cyber text/tool calls.
- Redacting Noop runner, explicitly fail-closed LocalRunner, and detection-only placeholder Docker runner; no production code can start a host process.
- Manual context compaction with persisted summaries.
- Accepted ADR 0001: Go is the sole control plane; future TypeScript calls Go over HTTP/WebSocket, while Rust remains a deterministic JSON analyzer behind Go.
- Accepted ADR 0002: Mission/Run aggregates, RunSupervisor, a single AgentCoordinator, structured WorkItems/Notes/Findings, lifecycle actions, and a unified event stream.
- Reworked `docs/architecture.md` around run-scoped budget, event, sandbox, report, approval, and recovery ownership without copying the reference implementation.
- Replaced the obsolete README migration/scaffold copy with a bilingual Chinese/English product overview, current capabilities, architecture boundary, and development-scope notice.
- Added `docs/TASK_BOOK.md` with phased migration tasks, acceptance criteria, compatibility rules, and CTF deferred to the final phase.
- Versioned SQLite migrations through schema v55: legacy baseline, run-centric foundation, Run/Session projection constraints, legacy Task mapping, Supervisor checkpoints, cumulative model budgets, durable pending input, restart-safe root repair, Work Board, Notes, durable per-call approvals, revocable Session grants, atomic tool budgets, typed script processes, Run tool-output Artifacts, idempotent structured-memory operations, durable Supervisor tool rounds/calls, Run execution leases with checkpoint fencing, cross-process root cancellation, the bounded Coordinator, hashed inbox idempotency, hashed Specialist admission, same-Run Agent-owned memory, attempt-bound Specialist CompletionReports, lease-fenced Specialist Attempt history, root inbox context delivery, the Specialist model-call/context/repair ledgers, durable schedule summaries, cross-process Specialist cancellation, immutable digest-keyed delegation proposals/reviews/applications/schedule requests, immutable read-only Fan-out plans and lease-fenced executions, deterministic Finding/Evidence/Report lifecycles, immutable Run Skill selections, root and minimal Specialist Skill-context provenance, Go-owned Run modes, strict Plan/Delivery selection, immutable context provenance, operator-owned Delivery checkpoints/completion gates, ordered exactly-once operator steering, immutable steering cancellation controls, metadata-only Sandbox Manifest preparation/validation, approval-bound disabled candidates, Artifact-bound disabled lifecycle/fencing/cancellation/cleanup facts, disabled backend/output preflight facts, simulation-only backend-evidence/output-transaction facts, fixed-endpoint read-only Docker-observation facts, deterministic Docker-container-plan/fake-write facts, and bounded Docker create-inspect-remove rehearsal facts; each version is checksummed and transactional.
- Read-only report export includes OASIS SARIF 2.1.0 with stable severity rules, workspace-relative escaped URIs, and v35 Finding fingerprints. Only unresolved operator-confirmed `validated/accepted` Findings enter `results`; draft, fixed, and rejected claims remain persisted and summarized but cannot become upload alerts.
- `report check` provides a typed CI gate with a conservative `validated/high` default that includes accepted unresolved Findings, explicit `active` draft admission, a disabled `none` mode, fixed/rejected exclusion, text/JSON output, and stable failed-precondition exit code 4.
- Migration tests cover idempotence, legacy data preservation, checksum history, and failed-migration rollback.
- Unified `internal/idgen` now backs agent tasks, sessions, tool runs, file edits, Mission/Run, and event IDs.
- Added pure Go Mission, Scope, Budget, RunConfig, Run status machine, and legal transition checks.
- Added transactional `missions`, `runs`, and append-only `run_events` persistence; event sequence is assigned only by the Store.
- Added `run create/list/show/events/start/pause/resume/cancel` CLI and end-to-end lifecycle tests.
- Run status updates and corresponding events commit atomically; Store independently rejects illegal or stale transitions.
- Added schema v3 with a unique Run/Session association and triggers that reject references to missing sessions.
- Every new Run now creates a dedicated active Session by default; an existing active Session can be attached once after workspace validation.
- Run creation, optional Session creation/update, and initial `run.created` plus `session.attached` events commit in one transaction.
- Session messages, assistant-output policy decisions, ToolRun policy/status changes, and FileEdit status changes project into the append-only Run timeline.
- Activity records and projected events commit atomically, repeated saves do not duplicate events, and Store rejects cross-workspace projection.
- Added stable `apperror` codes with compatible Go wrapping, CLI exit codes, and future HTTP status mappings while preserving current error text.
- Added `cyberagent headless events` with bounded 100-row reads, a 10,000-event hard cap, strict sequence/record validation, resumable `--after-sequence`, optional `--follow`/poll/timeout, NDJSON-only stdout, a final `stream.end`, and stable terminal/bound/deadline exits. It reads the existing Store and cannot execute or mutate a Run.
- Added schema v4 `legacy_task_runs` with unique Task, Mission, and Run identities.
- Added `run adapt-task <task-id>` and a transactional TaskAdapter that creates one Session/Mission/Run plus `legacy.task_adapted`, or returns the existing mapping.
- Concurrent and repeated adaptation converges on one Run; historical Task status is audit data and never starts execution implicitly.
- Legacy Task goals and legacy Event messages/payloads are now redacted at the SQLite Store boundary.
- Added schema v5 `run_supervisor_checkpoints` with bounded phase, next-turn, attempt, and redacted last-error state.
- Added `RunSupervisor`, `RunHandle`, and `LifecycleResult`, plus `run step` and `run checkpoint` CLI commands.
- A supervised turn checkpoints before the model call and atomically commits Session messages, policy decision, model usage, completion event, and the next checkpoint.
- Started turns recover across Store/process restart; repeated completion is idempotent and committed turns are not duplicated.
- The initial schema v5 Supervisor slice enforced MaxTurns/preflight cancellation and rejected all ToolCalls; schema v16 later opened only the two create-only structured-memory tools without creating legacy ToolRuns.
- Immediate Supervisor responses and persisted responses share the same secret-redaction boundary.
- Added schema v6 cumulative input/output/total token counters and model-execution milliseconds to durable Supervisor checkpoints.
- MaxTokens and TimeoutSeconds are checked before each call; remaining tokens and model-call time are passed to the Provider boundary.
- Added a bounded `run execute` loop with an explicit step ceiling and structured stop reason.
- Added atomic, idempotent operator-controlled `run finish` and `run fail` transitions across Run status, Supervisor checkpoint, and event stream.
- Provider nil responses and negative token usage are rejected and checkpointed instead of reaching persistence.
- Counter accumulation rejects integer overflow, and bounded execution no longer preallocates memory from an untrusted `--max-steps` value.
- Added the strict `root_lifecycle.v1` domain contract and decoder with UTF-8, 64 KiB, unknown-field, trailing-data, and action-specific validation.
- RunSupervisor requests JSON lifecycle output and is the only layer that interprets `continue`, `finish`, or `wait`.
- Model `finish` atomically commits the turn and completed Run; `wait` atomically commits the turn and paused Run, then resumes at the next turn.
- Raw lifecycle JSON is excluded from Session history; redacted message, summary/reason, and normalized Supervisor events are persisted.
- Lifecycle completion replay is idempotent, and bounded execution stops cleanly on root finish, root wait, or an already paused Run.
- Added a cycle-free `session.RunChatExecutor` boundary and an application adapter that routes ordinary Run-bound Session input through RunSupervisor.
- Ordinary CLI/TUI chat automatically starts a created Run or resumes a paused Run, returns normalized action/status metadata, and rejects terminal or approval-waiting Runs.
- Schema v7 checkpoints redacted, 64 KiB-bounded pending input before the Provider call; restart recovery reuses the authoritative input and commits one exact user/assistant pair atomically with lifecycle state and events.
- Unbound legacy Sessions retain an explicit direct Router compatibility path, while slash commands remain existing command adapters.
- RunSupervisor now feeds the latest compacted Session summary back into later model context.
- Added typed Provider outcomes for retryable transport errors, rate limits, invalid responses, cancellation, and permanent failures; Router preserves those types across provider boundaries.
- Anthropic-compatible HTTP failures classify 429, 408/425, 5xx/529, permanent 4xx, malformed JSON, empty responses, and bounded `Retry-After` without exposing API keys or raw unredacted error text.
- RunSupervisor now performs at most three side-effect-free model attempts by default with cancellation-aware exponential backoff; long server retry delays are returned rather than shortened.
- Added durable sequential `model.started`, `model.completed`, and `model.failed` events. Attempt numbering resumes across Store restart and Store rejects stale, duplicate-terminal, or out-of-order writes.
- Model terminal events, token usage, and execution-millisecond accounting commit atomically; replay is idempotent and cannot double-charge the budget.
- Parent-context cancellation uses a bounded audit-only context to persist the cancelled model event and elapsed time while leaving the Supervisor turn recoverable.
- A failed custom pending input survives rate-limit exhaustion and can be resumed by `run step` without being replaced by the Mission goal.
- Added one explicit `root_lifecycle.v1` repair phase. It has its own bounded transport retry counter while global model attempt numbers remain continuous.
- Schema v8 persists pending/exhausted repair state and a redacted diagnostic. Restart recovery resumes pending repair once and fails exhausted repair without another Provider call.
- Invalid protocol output is never copied into the repair prompt, Session history, or events. Only the bounded parser diagnostic is retained.
- Initial invalid-response usage is charged before the repair budget check; repaired success commits one legal message pair, while a second invalid response records failure and stops.
- Added `supervisor.protocol_repair_requested/started/completed/failed` events plus CLI `protocol_repairs`, `repair_phase`, and `repair_reason` observability.
- Router streaming now shares model resolution, request redaction, and typed startup failures with ordinary Chat calls; RunSupervisor uses the stream path for every model attempt.
- The Anthropic-compatible provider now parses real SSE message/content/usage/error events and requires a final usage-bearing completion marker.
- The Supervisor stream aggregator accepts UTF-8 code points split across transport chunks, rejects invalid final UTF-8 or output above 64 KiB, preserves cancellation semantics, and feeds the existing retry, repair, budget, and terminal transactions.
- Each model attempt persists at most 32 ordered `model.delta` records containing only sequence/chunk/byte/done counters. Store validation makes replay idempotent and requires terminal stream counters to match the durable delta ledger.
- `run step` and `run execute` expose `stream_events` and `stream_bytes` without persisting model text in incremental events.
- Added an application-owned, concurrency-safe ActiveCallRegistry keyed by Run and attempt identity. Reservations prevent duplicate Provider calls, while public visibility begins only after durable `model.started` persistence.
- Added in-process active-call lookup/list, idempotent audited cancellation, and a versioned metadata-only subscription envelope for snapshot/progress/cancel/completed/failed states.
- Each subscriber has a 32-event buffer and is disconnected when slow; Provider execution never waits for a live consumer and persisted `model.delta` remains the only restart-safe progress ledger.
- Explicit cancellation persists one redacted `model.cancel_requested` before signalling the Go-owned context. All Provider terminal paths remove the active entry, and cancellation races report whether a signal actually reached the call.
- The CLI entrypoint now propagates Ctrl+C/SIGTERM through `ExecuteContext`, allowing cancelled model usage/events and the recoverable Supervisor checkpoint to be committed before process exit.
- ActiveCallInfo now carries the Store-bound Session identity, allowing Bubble Tea to discover the correct Run call before the Session send returns.
- Bubble Tea runs submit and active-call discovery concurrently, renders provider/model, attempt, chunk/byte, cancellation, disconnect, and terminal metadata, and never receives raw stream text.
- `Ctrl+X` invokes the application audit-first cancellation API. Legacy or pre-activation calls fall back to cancelling only the current application request context; the UI never owns a Provider context.
- Busy chat actions reject `Esc/Ctrl+C` keyboard exit until they complete or receive explicit cancellation. Direct, picker-selected, and picker-created models share the same App-owned registry/controller.
- Responsive TUI help now includes cancellation without overflowing supported 80/100/120/145-column layouts, and the three previous staticcheck findings were removed.
- Added a pure-Go WorkItem aggregate with normalized title/description/owner/acceptance/dependencies, legal transitions, blocked/completed invariants, terminal immutability, and optimistic versions.
- Schema v9 persists `work_items` and same-Run `work_item_dependencies`; composite foreign keys and Store checks reject cross-Run, missing, self, cyclic, and incomplete prerequisite relationships.
- WorkItem record changes and `work_item.created/changed` Run events commit atomically. Duplicate event failures roll back the record, and stale concurrent writers converge on one version winner.
- Added `todo create/list/show/update/start/block/reopen/complete/cancel` with repeated acceptance/dependency flags, clear operations, filters, and optional explicit `--version` locking.
- RunSupervisor loads at most 20 active WorkItems into a redacted `work_board.v1` JSON user/evidence message capped at 16 KiB; terminal items are excluded and WorkBoard text cannot acquire system authority.
- A model root `finish` conflicts with active work and uses the existing single protocol-repair path. Explicit `run finish` remains the operator override.
- `CompleteSupervisorTurn` repeats the active-item check under its SQLite write transaction, so a WorkItem created by another process during the model call rolls back a stale finish and leaves the turn recoverable.
- Added a pure-Go Note aggregate with five categories, run/root/owner visibility, normalized tags and source/Evidence references, pinning, archive/restore, strict size limits, and optimistic versions.
- Schema v10 persists Notes and normalized relation tables. Composite foreign keys prevent cross-Run relation injection, while Note record changes and `note.created/changed` events commit atomically.
- Added `note create/list/show/update/archive/restore` with bounded UTF-8 content-file input, exact filters, replace/clear relation operations, and optional explicit `--version` locking.
- Root Supervisor memory includes only active run-visible, root-visible, and `owner=root` Notes; archived Notes and another owner's Notes are excluded.
- A generic Context Section selector ranks compacted summary, Work Board, pinned Notes, and category-weighted Notes under an 8,192-token estimate.
- Every `model.started` event records selected and omitted context source IDs/token estimates without persisting Note bodies, preserving restart-safe context provenance.

Not done yet:

- Define the Go-owned Sandbox Manifest and immutable execution-intent envelope while keeping Local/Docker process execution disabled. Schema v47 Specialist Skill minimization, schema v46 steering controls, schema v44 Delivery gates, and schema v43 context provenance are complete.
- OpenAI-compatible/Ollama providers.
- Dedicated TUI file-edit diff pane; existing Tool approval/denial remains available from the Tools view.
- User-visible safe model-text streaming; durable metadata SSE and exact cross-process cancellation are complete.
- Script generate-run-fix loop with real model calls.
- CTF-specific solving workflows beyond placeholder commands.
- General Web control mutations and Rust analyzer processes. The generated React/Vite Run/Agent/delegation/Fan-out/Finding read-first console, bounded local read API, durable metadata SSE, narrowly scoped cancellation/profile controls, and same-process production Web asset serving are complete.
- Provider monetary cost budgets; token/time budgets, child-Agent scheduling/completion, Findings/Evidence/Report, bounded admission, Agent-owned WorkItems/Notes, create-only Provider dispatch, and bounded TUI summary views are complete.
- Real Local/container-process execution and Sandbox Artifact export from an actual process; current terminal Shell/ScriptProcess completion remains dry-run only, and v55 never starts its rehearsal container.

## Code Audit Notes

No high-severity issue was found in the latest slice.

The schema v41 Run-mode audit found no unresolved high- or medium-severity issue. Review hardened eight boundaries before release: legacy backfill IDs no longer derive from potentially oversized Run identifiers; active-lease checks use Store/SQLite time rather than caller timestamps; phase transitions are blocked while any execution lease remains active; model and operator completion both recheck Plan mode transactionally and SQLite independently guards the Run status; unredacted mode metadata is rejected at the Store boundary; the Store recomputes operation fingerprints instead of trusting application input; script-process creation fingerprints bind the complete immutable mode tuple; and phase timestamps tolerate system-clock rollback without moving history backwards. Domain, Store, application, CLI, TUI, HTTP/OpenAPI, and React tests cover defaults, invalid values, replay/conflict, cross-Store concurrency, SQL immutability, active-lease denial, Plan finish repair, both completion paths, migration from v40, and cross-surface projection. Full ordinary/race tests, vet/staticcheck, module and vulnerability checks, deterministic TypeScript regeneration, 15 frontend tests, production build, npm audit, credential scan, and isolated real-binary mode smoke all pass.

The first React/Vite console audit found no unresolved high- or medium-severity issue. `docs/openapi.json` is the DTO source, the API base is pinned to same-origin `/api/v1`, and the Vite proxy accepts only HTTP(S) loopback targets. The read bearer lives only in the Zustand memory store, is sent only through `Authorization`, and is cleared with the entire React Query cache on disconnect; neither browser storage nor URL parameters are used. Native `EventSource` is not used. The bounded fetch-stream parser validates UTF-8, frame size, event id/cursor, Run identity, and sequence agreement before projection and resumes with `Last-Event-ID`. Five low-risk robustness or repository-hygiene gaps found during review were fixed before release: aborted reconnect delays no longer reject in detached effects, non-retryable 4xx streams stop instead of polling forever, collection envelopes require an array at runtime, normalized paths cannot escape `/api/v1`, and TypeScript incremental-build caches are excluded from Git. Vitest covers these boundaries, the OpenAPI-generated types pass strict TypeScript, and desktop/mobile browser checks found no horizontal overflow or console error. The console remains read-only and exposes descriptors rather than Artifact content; no control token, Go Policy decision, Shell, Docker, persistence, or model route moved into TypeScript.

The same-origin production Web hosting audit found no unresolved high- or medium-severity issue. `api serve --ui-dir` validates the real bundle before opening the Store/listener, rejects root/assets symlinks, non-regular or unsupported files, unhashed asset names, excessive counts, and per-file/aggregate size overflow, then serves only an immutable in-memory snapshot. `/api` remains a reserved authenticated namespace; anonymous UI requests still pass canonical path, loopback Host/client, target-size, body, method, query, and authorization-header checks before static dispatch. Route-aware CSP has no `unsafe-inline` or `unsafe-eval`; HTML is `no-store`, exact hashed assets are immutable, and bounded HTML-accepting SPA fallback cannot mask missing assets or extension paths. Tests cover panic containment, API authorization preservation, disabled-UI compatibility, cache/ETag/HEAD behavior, malformed trees, and real CLI startup/shutdown. A real Vite bundle was exercised over one Go process in desktop and 375px browser viewports with authenticated API/SSE reads and no document overflow.

The first Linux GitHub Actions run exposed one pre-existing low-risk lease-heartbeat resilience issue rather than a frontend regression. A custom short renewal interval also became the SQLite renewal operation's entire timeout, discarding the larger `TTL - RenewInterval` safety window; under a loaded two-core runner, one delayed renewal could cancel the Run before lease expiry. Renewal now uses the available expiry slack with the existing two-second cap. The long-call exclusivity test still waits beyond the original expiry, but uses scheduler-tolerant timing, and an internal table test pins default, short-interval, and near-expiry timeout behavior. Ten ordinary and three race-enabled targeted repetitions pass locally; the Linux CI rerun is recorded with the follow-up commit.

The schema v19 Coordinator audit found no unresolved high- or medium-severity issue. Root registration, Supervisor begin/continue/wait/finish/failure, operator Run transitions, inbox mutation, and graph snapshots share their existing SQLite write transaction. Database checks cap the graph at three nodes and depth one, while the current root is created with `child_limit=0`, so the new schema cannot accidentally enable recursive execution. Inbox payload values are recursively redacted, secret-shaped or non-protocol JSON keys are rejected, payloads cap at 16 KiB, and snapshots store SHA-256 plus metadata rather than duplicated content. Per-Agent pending messages cap at 128, total message history at 4,096, consume batches at 32, and retained snapshots at 32. Registration no longer repairs an existing root before inspection, so `run graph` reports lifecycle/snapshot drift instead of blessing it with a new snapshot. Tests cover restart recovery, concurrent idempotent registration, exactly-once consume, cancellation cascade, blocked child insertion, key/value secret handling, v18 migration, snapshot tamper detection, and Run/Supervisor identity continuity. Full-race review also removed three pre-existing timing assumptions from cancellation accounting, mid-stream cancellation setup, and concurrent multi-line API startup-output tests.

The schema v20 inbox-protocol audit found no unresolved high- or medium-severity issue. Send intent is normalized and redacted before a domain-separated key digest and request fingerprint are computed; random message identity and timestamps are excluded. The raw key is never persisted or included in events, snapshots, or errors. Replay lookup occurs under the SQLite writer reservation before recipient lifecycle checks, so a successful wake can be retried after the Specialist is already ready without duplicating a message, status transition, event, or snapshot. Changed intent under the same key conflicts. Strict decoders reject unknown semantic fields, invalid dependency states, mismatched kinds, senderless dependencies, and any attempt to wake root or a Specialist outside a running Run. Ordinary v19 message rows receive the `message` default, and their snapshot JSON remains byte-compatible because that semantic is omitted from the projection.

The schema v21 Specialist-admission audit found no unresolved high- or medium-severity issue. Admission is absent from the default Coordinator and requires an explicit in-process policy; no CLI, HTTP route, model tool, or Provider message can invoke it. The Store independently revalidates running/idle-root state, root parentage, capacity, depth, parent-Skill subset, per-child positive budgets, aggregate reservations, and root coordination headroom. Replay lookup precedes current lifecycle checks and stores only digests. Root version fencing and SQLite's writer reservation serialize different-key and same-key callers. Session creation, root capacity/effective budget, child insertion, operation fact, audit events, and snapshot are one transaction; an injected event failure leaves none of them behind. Root Supervisor turns receive the reduced effective budget. Cause-specific pause/resume avoids waking dependency waits, while terminal Run projection terminates every nonterminal child and archives its Session.

The schema v22 Agent-owned-memory audit found no unresolved high- or medium-severity issue. WorkItem and Note ownership remains optional for legacy compatibility, but every nonempty `owner_agent_id` is normalized, resolved to an actual same-Run Agent, and protected again by SQLite foreign keys plus insert/update triggers. New assignment to a terminal Agent fails closed. Owner visibility uses the viewer's persisted Agent role and identity; a root cannot read a Specialist's owner-only Note, while both can read Run-visible memory. Agent ownership survives a visibility change. Existing v21 rows retain their label and receive a null Agent reference during migration. The v10 Note CHECK still requires a label for owner-only rows, so Agent-only private Notes deterministically mirror their Agent ID into that compatibility field instead of rebuilding the user table. Go injects the root identity into Supervisor and CLI structured-memory calls; model-facing schemas reject `owner_agent_id`, and policy/tool events identify the responsible Agent.

The schema v23 CompletionReport audit found no unresolved high- or medium-severity issue. The strict `agent_completion.v1` contract requires an explicit version, `succeeded`/`partial` outcome, bounded UTF-8 summary, and normalized bounded references. The Store validates the raw summary before redaction, then revalidates the redacted result; successful completion cannot strand active child WorkItems, partial completion must name all active child WorkItems, and only active parent-visible child Notes may be referenced. Completion is bound to the running Specialist's exact attempt and direct root parent. Report, digest-only operation, parent result message, child terminal state, Session archive, three metadata-only events, and graph snapshot are one transaction. A SQLite trigger makes committed reports immutable. Event-failure injection proves rollback, two SQLite connections converge to one report/message/operation, stale attempts fail, same-key changed intent conflicts, direct update is rejected, and restore rejects a completed child after report deletion.

The schema v24 Specialist Attempt Runtime audit found no unresolved high- or medium-severity issue. Scheduling is an explicit internal capability separate from admission and disabled on the default Coordinator. `agent_attempt.v1` binds each turn to the active Run execution lease and generation, charges the turn before work begins, accepts one immutable usage record, and accumulates actual tokens on the child. Start, usage, continuation, and crash mutations store only digest/fingerprint idempotency facts. Completion additionally requires the current lease and recorded usage. Crashes redact bounded reasons before persistence, notify the direct root parent, and fail/archive the child when no retry budget remains. Lease takeover recovers each stale running attempt once and fences every new write from the former worker. Run lifecycle transitions interrupt attempts before moving children. Graph restore verifies contiguous turns, token sums, the active attempt, CompletionReport linkage, and the latest snapshot. SQLite triggers independently require the matching active, unexpired lease for Attempt creation, first usage, and CompletionReport insertion, so direct writes cannot forge or reuse the fenced lease.

The schema v25 root-inbox-context audit found no unresolved high- or medium-severity issue. Go selects at most four sequence-ordered messages from direct Specialist children and accepts only strict dependency payloads, results linked byte-for-semantics to an immutable CompletionReport, or failure notifications linked to a crashed AgentAttempt. A prepared batch is fenced to one active Supervisor attempt and turn. Successful lifecycle commit changes delivery state and consumes messages in the same transaction as Session messages and checkpoint advancement; turn failure supersedes delivery rows while keeping messages pending. Cancellation, process restart, and lease takeover reuse the same prepared batch. Prompt construction redacts and truncates typed payload fields and excludes message IDs, sequence values, cursors, and consumption control; sender identity comes from the durable route rather than model input. Manual root consumption cannot race a running Supervisor, and graph recovery validates prepared deliveries while old v24 snapshots remain readable.

The post-v25 whole-project audit found no remaining high- or medium-severity issue after remediation. It removed the package-level Windows advisory inherited through Bubble Tea by upgrading `golang.org/x/sys` from v0.38.0 to v0.44.0. LocalRunner no longer contains a host execution path and always fails closed; Noop redacts display text, and all runners honor pre-cancelled contexts. Anthropic-compatible providers now accept only HTTPS or exact-loopback HTTP base URLs, reject URL credentials/query/fragment and malformed API keys, clone their HTTP client, and refuse every redirect so `x-api-key` cannot cross origins. Newly created Unix runtime directories and SQLite files use `0700`/`0600`; Windows remains ACL-controlled. Production source contains no `exec.Command`/`CommandContext`, while Docker availability uses only `exec.LookPath`.

Residual robustness limits are explicit rather than hidden: Policy and prompt-injection detection remain heuristic rule sets; approved whole-file replacement re-resolves and hashes immediately before writing but cannot eliminate a same-host external process racing the final filesystem operation; monetary model pricing, real container isolation, autonomous child scheduling, Web control mutations, Rust analyzers, and CTF-specific automation remain future work. These limits do not currently create an unapproved host command or network-tool execution path.

The OpenAPI audit found no unresolved high- or medium-severity issue. The contract is generated without opening SQLite or reading credentials. At that checkpoint it published 43 paths: 30 bodyless authenticated `GET` operations and sixteen separately authorized control `POST` operations. Live-route tests exercised each path against real SQLite state. Golden comparison prevented DTO/document drift. Security tests rejected unauthorized and queried contract requests and asserted that Workspace roots, Artifact/Skill/Session/file bodies, model output, tool arguments, approval commands/paths/content, private narratives, operation/lease-owner/fencing identities, digests, API keys, Provider Base URLs, and environment-variable names were absent. The runtime document was precomputed once at API construction and remained under the existing request-size, response-size, loopback, Host, client-address, and bearer-token boundary.

Independent Redocly validation accepts the OpenAPI 3.1 document with no warnings. The repository owner selected Apache License 2.0, and the generated contract publishes `info.license.identifier: Apache-2.0` from Go alongside the repository `LICENSE`.

The Run-event SSE audit found no unresolved high- or medium-severity issue. The stream reuses Store-redacted `EventView` data and never reads Artifact content, checkpoint pending input, active-call channels, or fencing tokens. Cursors contain only a version, durable sequence, and a Run-scope digest and cannot be reused across Runs. Fresh and resumed batches require contiguous append-only sequences. Defaults bound each batch to 32 events, each frame to 2 MiB, each connection to 10,000 events/five minutes, each write to two seconds, and the process to 16 concurrent streams. Invalid cursors, missing Runs, and exhausted connection slots fail as ordinary typed JSON before SSE headers are committed; failures after commit close the connection for cursor recovery without writing an audit event for a read operation.

The server now cancels its BaseContext before graceful shutdown, preventing long SSE handlers from extending shutdown to their configured lifetime. Integration tests cover exact replay, `Last-Event-ID`, query resume, malformed/repeated/cross-Run cursors, heartbeat-only streams, zero timeline mutation, another SQLite connection appending a visible event, process-slot exhaustion/release, deadline-bounded slow writers, and cancellation of a minute-long stream during server shutdown.

The read-API audit fixed three low-risk robustness defects before release: a pre-cancelled Windows listen context could still bind, access-token validation returned an untyped internal CLI error and silently trimmed environment values, and a cursor at the 100,000-row window could advertise an unusable next cursor. The listener now checks cancellation before binding, token errors are stable `INVALID_ARGUMENT` values and environment tokens are neither normalized nor echoed, and bounded pagination reports `truncated` instead of returning an invalid cursor. Empty collection DTOs use stable JSON arrays. Tests cover loopback Host/client enforcement, exact bearer authorization, method/body rejection, error non-disclosure, no CORS, response headers, token non-persistence, concurrent SQLite reads, graceful shutdown, cursor scope, and metadata-only Artifacts.

The Tool Gateway audit found and fixed four correctness/security issues: invalid UTF-8 before a truncation boundary could be deleted and misreported as valid text; a tiny output limit could overflow its truncation marker; a persisted shell denial could be mapped inconsistently when a later Store operation also returned an error; and production file calls trusted a caller-supplied workspace root. Regression tests cover each case, and production roots are now bound to the workspace Store record.

The script slice removed the direct LocalSandbox execution path. Its audit also found that applying regex redaction to an already serialized Run-event payload could corrupt escaping around a nested JSON command. Store redaction now parses JSON with exact numbers, recursively redacts string values, re-encodes, and enforces 1 MiB, 64-level, and 100,000-node limits. Regression tests cover nested JSON, token-shaped argv, invalid JSON, resource exhaustion, policy denial, and zero host side effects.

The durable-approval audit found and fixed two medium-risk integrity gaps before release: the public adoption path could otherwise create an approval for a nonexistent proposal, and an idempotently re-saved policy denial could drift from `never` to `per_call`. `EnsureApproval` now verifies the persisted ToolRun/FileEdit identity and fingerprint, while Store synchronization preserves the original denial mode. A later robustness pass also stopped persisting raw client review keys; `approval_operations` stores a domain-separated SHA-256 digest instead. Tests prove that a crash after `approval.decided` but before proposal completion can be recovered by replaying the same immutable operation key.

The schema v12 audit found and fixed one low-risk observability gap: a rejected over-budget call originally returned a stable typed error but did not record the first exhaustion boundary. `run_tool_usage.exhausted_at` and a single `tool.budget_exhausted` event now preserve that fact without allowing repeated rejected calls to flood the event stream. Grant and budget tests cover restart-style CLI use, idempotency-key conflict, revocation, scope mismatch, terminal Run/archived Session denial, concurrent budget saturation, and Policy precedence.

The schema v13 audit found and fixed three correctness/boundary defects before release. ScriptProcess review dispatch existed but request normalization still rejected the tool; Session stores were accidentally coupled to an unrelated atomic Script Run method; and the initial table made `run_id` unique while failing to enforce the Process Run/Session composite binding. Script capabilities are now optional Gateway Store interfaces, one Run may own multiple processes, and a forged cross-Run binding rolls back. Tests cover 12-way concurrent replay, changed-intent conflict, full event-failure rollback, approval-ledger bypass, v12 Run/Grant migration, redaction, permanent Policy denial, and zero host side effects.

The schema v14 audit found no high-severity issue and fixed two low-risk robustness defects before release: one Gateway error string violated Go error conventions, and custom Store terminal output could remain invalid UTF-8 until Artifact capture even though the bounded Result repaired it. Terminal output is now normalized to valid UTF-8 before redaction, hashing, capture, and Result truncation. Tests additionally lock cross-Run source rejection, content-free Artifact events, no Artifact on Policy denial, idempotent replay, event-failure recovery, and hash tamper detection.

The schema v15 audit found no high-severity issue and fixed three low-risk robustness/privacy defects. WorkItem dependency validation was initially broad enough for a secret-shaped value to reach a missing-dependency error; strict JSON/enum decoder errors could echo a secret-shaped field or value; and independent SQLite deferred transactions could race during read-to-write promotion with `database is locked`. Structured dependencies now accept only the real generated WorkItem ID shape, parser diagnostics pass through redaction, and SQLite uses immediate write transactions plus the existing busy timeout. The read-only Session-grant lookup no longer starts a transaction that would unnecessarily take a writer reservation. Tests cover zero-charge malformed input, content-free errors/events, exact scope/invocation binding, rollback, migration, and repeated cross-Store concurrency.

The schema v16 audit found no high-severity issue and fixed four robustness defects before release. Application and Store originally canonicalized typed JSON in different field orders; repeated semantic intent in a later round originally reused a local call ID; concurrent recovery could produce different durable results because `replayed` is timing-dependent; and protocol repair still advertised tools despite forbidding them in text. Canonical JSON is now shared across boundaries, local IDs include the round while operation keys remain semantic, Provider results omit timing-dependent replay metadata, and repair requests carry no tools. The Store independently revalidates strict typed payloads, and concurrent result recording across two SQLite connections converges on one result and one round-completion event.

The schema v17 audit found no unresolved high- or medium-severity issue and fixed six concurrency/security defects before release. Structured-memory writes were fenced but their earlier budget charge was not; durable idempotency records incorrectly required a transient lease; implicit same-owner acquisition replay allowed concurrent calls to share a lease; acquisition/takeover updates did not verify affected rows; the fencing token could have escaped through lease events or Gateway outcomes; and the required-lease check was briefly placed in generic `ToolCall.Validate`, which also validates deliberately token-free safe outcomes. Budget and entity transactions now independently verify the same token, stored intent uses a separate validation mode, replay requires the explicit current `lease_id`, every conditional update checks one affected row, Gateway ingress enforces the lease, and token values remain confined to the lease/checkpoint tables and process memory. Tests cover independent SQLite connections, expiry takeover, heartbeat beyond the original TTL, legacy checkpoint migration, stale-write rejection, and token-free CLI/API/event projections.

Residual risks to address soon:

- `staticcheck ./...` is clean; the prior TUI `S1008`, `S1011`, and unused-helper `U1000` findings were removed in this slice.
- `script run --local` no longer executes commands. It creates a workspace-scoped, Run-bound, policy-checked proposal and records `execution_mode=disabled`; LocalSandbox remains disconnected from production.
- Schema v13 removes the former Script Run/ToolRun two-transaction window. Mission, Session, Run, budget, Process, Approval, and initial events now roll back together on any failure.
- Schema v14 commits each Artifact row and `artifact.created` together. If capture fails after a terminal proposal was committed, replay resumes capture without repeating execution or approval; ordinary events contain metadata only, while hashes cover redacted content rather than inaccessible raw secrets.
- Schema v16 exposes only create-only WorkItem/Note calls. Model-driven update, completion, cancellation, archive, restore, file, Shell, process, and network actions stay disabled until their version, approval, Sandbox, and evidence semantics are separately reviewed.
- Schema v17 provides cross-process execution exclusion and stale-write fencing for one local SQLite database. Schema v18 adds cancellation through that same database, but neither feature is a multi-host consensus protocol; live subscriptions and the actual Provider cancel function remain process-local.
- Cross-process cancellation polling defaults to 100 ms and is available only while the worker and API share the same SQLite database. A crash before observation may leave the request pending until a later model attempt resolves it as `superseded`; cancellation is best-effort control, not a guarantee that an already-completed remote request can be recalled.
- Structured-memory replays, changed-intent conflicts, authoritative scope failures, and Policy denials consume tool-call budget because each is a well-formed invocation attempt. Malformed payloads and missing identities do not consume budget.
- The current Policy checker conservatively rejects Notes containing dangerous scanner command text even when used descriptively. Future intent-aware classification may refine that behavior, but permanent cyber-action denial must remain authoritative.
- A workspace read Artifact contains exactly the bounded content returned by that invocation. It does not reconstruct bytes intentionally excluded by the read tool's own requested maximum.
- The Gateway still persists Shell and file proposals in legacy `tool_runs` and `file_edits`; typed ScriptProcess persistence is now independent. Future compatibility removal should migrate those older proposal types without changing the approval ledger contract.
- Automatic workspace read outcomes are normalized but are not independently persisted when invoked by standalone CLI commands; Session slash-command text is still audited through Session messages.
- Secret redaction is heuristic, not a full secrets manager; add opt-in raw local inspection later only with clear warnings.
- Binary or non-UTF-8 files are refused by `read_file`; richer file viewers should stay workspace-scoped and type-aware.
- File edit writes re-resolve and re-hash immediately before `os.WriteFile`, but portable Go cannot fully eliminate filesystem TOCTOU races without OS-specific no-follow/open-handle code. Keep workspace permissions as the primary local boundary.
- The symlink-escape unit test is skipped on this Windows account because creating symlinks requires an unavailable privilege; traversal, path resolution, and stale-file tests pass, and the runtime still resolves links before accepting a path.
- Docker runner intentionally returns a clear placeholder error and is not a real isolation boundary yet.
- Session `/run` now creates a persisted tool proposal; approval still dry-runs by design. Real execution must flow through stricter workspace scoping, sandbox, and event logging.
- Mimo and DeepSeek API keys must remain env-only for tests; do not persist user keys until a real secrets backend exists.
- DeepSeek model availability is an external contract. `deepseek-v4-flash` was live-verified on 2026-07-10, while `DEEPSEEK_MODEL` remains the explicit override when the service changes its model catalog.
- Future Rust and TypeScript modules must not bypass Go for LLM, secrets, policy, workspace permissions, Docker, shell, network scope, or persistence.
- `run start` advances lifecycle only; `run step` performs one model turn and `run execute` performs only the operator-selected number of durable steps.
- A crash after the pre-call checkpoint can repeat a model request, but committed messages and completed turns are never duplicated. Schema v16 makes repeated create-tool intent idempotent even when Provider call IDs change; each real Gateway retry still consumes tool budget.
- Structured memory now has an 8,192-token estimate, but recent Session history is still bounded by 20 messages rather than sharing that token budget.
- MaxCostUSD is not enforced until provider pricing metadata exists. Tool-call budgets are enforced by the Gateway; zero remains unlimited for older/API-created Runs unless a caller supplies a limit.
- ExecutionMillis measures Provider model-call time, not total wall-clock orchestration time.
- One Provider response can exceed the remaining token allowance; actual usage is committed conservatively and the next call is blocked.
- Budget exhaustion leaves the Run in `running` until an operator explicitly finishes, fails, or cancels it. Only a validated structured action can change lifecycle state; free-form model text cannot.
- Strict lifecycle JSON has exactly one automatic repair. A Provider that returns two invalid protocol responses fails the turn without an unbounded correction loop.
- Provider retry is enabled only inside RunSupervisor; legacy unbound Sessions receive typed errors but still use the direct, non-retrying Router compatibility path.
- Retry backoff is deterministic and intentionally capped at three attempts/2 seconds for the local single-user runtime. Add jitter before enabling concurrent remote workers.
- A server `Retry-After` above the local ceiling is not auto-retried; the Run remains running with a failed Supervisor turn and preserved input until a later operator retry.
- If the process dies after a final zero-tool `model.completed` but before the turn-completion transaction, recovery may repeat that final model request under the next durable attempt number. Prior usage remains charged. Tool-producing responses do not have this window because their model event and pending batch commit atomically, and semantic operation keys prevent duplicate entities.
- Persisted `model.delta` events intentionally contain counters rather than model text. Historical SQLite replay can reconstruct progress and accounting, not token-by-token content; the current live envelope is also metadata-only until a safe lifecycle/text projection exists.
- Active-call subscriptions are process-local and non-replayable. A full 32-event buffer closes that subscriber; consumers must inspect `Dropped()` and recover from durable Run events.
- Application cancellation is audit-first: if SQLite cannot append the request, the registry does not silently signal an unaudited cancellation. Parent process-context cancellation remains the emergency path and still records `model.failed(cancelled)` when possible.
- The Go API can inspect durable state, token-free lease activity, and resumable persisted Run events from another process. Its separately gated control path persists an exact cancellation request; only the fenced worker observes it and signals its own in-memory active-call registry. A read token cannot mutate, a control token cannot read, and neither API surface exposes the lease id.
- TUI live state is transient metadata, not a durable transcript. Disconnect or process exit must recover from SQLite Run events, and user-visible text streaming remains disabled.
- When no active registry item exists, `Ctrl+X` cancels the current application request context after a bounded lookup. This covers legacy/pre-activation calls without fabricating an audited Run cancellation event.
- Root `wait` currently maps to `paused` plus a textual reason; structured dependencies and approvals are future Coordinator/Work Board work.
- Unbound Sessions still use the direct Router compatibility path. New product flows should create a Run instead of expanding this legacy path.
- Slash commands remain separate command adapters and do not consume a Supervisor turn, but `/ls`, `/read`, `/write`, and `/run` now share the Tool Gateway approval/event behavior. Future model-authored calls must use that same boundary without silently enabling execution.
- Pending input is redacted but otherwise stored as Session/model content; this is not a secrets vault. The CLI checkpoint view intentionally omits it.
- Applied migration statements are immutable once released because their checksums are verified. Schema changes must always add a new migration version.
- Schema v3 intentionally rejects duplicate non-empty Run/Session associations. A legacy database containing duplicates must be audited before upgrade instead of silently discarding an association.
- `apperror.Normalize` includes a transitional text classifier for legacy plain errors. New services must return typed errors directly so future localization cannot affect classification.
- Models can create WorkItems and Notes only through the bounded schema v16 Tool Gateway loop. Update, status transition, archive, restore, and delete remain operator/application operations until their version and approval semantics are separately implemented.
- The Supervisor queries at most 20 active WorkItems and 100 visible active Notes before token selection. SQLite retains overflow, but later relevance search or explicit loading must make those records discoverable.
- Explicit `run finish` can close a Run with unfinished WorkItems as an intentional operator override. Future report projections should surface those unfinished records.
- WorkItem/Note retain bounded legacy Owner labels for compatibility, while optional `owner_agent_id` is the authoritative same-Run Agent binding for new Coordinator-aware flows.
- Root and Specialist Note visibility is AgentNode-backed. The schema v27 child turn receives its Mission, assigned Skills, budget counters, bounded child Session history, up to four strict direct-parent instructions, and token/byte-selected active child-owned WorkItems plus child-visible Notes. Message IDs remain audit-only and do not enter the child prompt.
- Note Evidence IDs are structured references rather than foreign keys because the Evidence entity is deferred to the report phase.
- Context token counts are deterministic estimates for selection. Provider-reported usage remains the authoritative budget and billing value.

## Feature Verification

Latest verified commands:

```powershell
go test ./...
go run ./cmd/cyberagent api openapi
go run ./cmd/cyberagent api openapi --output docs/openapi.json
curl.exe -N -H "Authorization: Bearer $env:CYBERAGENT_API_TOKEN" http://127.0.0.1:8765/api/v1/runs/<run-id>/events/stream
go run ./cmd/cyberagent run create "review this workspace" --workspace demo --profile review --max-turns 15
go run ./cmd/cyberagent skill select <run-id> review --operation-key <stable-key> --token-budget 4096
go run ./cmd/cyberagent skill selection <run-id>
go run ./cmd/cyberagent run start <run-id>
go run ./cmd/cyberagent run step <run-id>
go run ./cmd/cyberagent run execute <run-id> --max-steps 2
go run ./cmd/cyberagent run execute <run-id> --max-steps 2 --finish --summary "planning complete"
go run ./cmd/cyberagent run finish <run-id> --summary "review complete"
go run ./cmd/cyberagent run fail <run-id> --reason "blocked by provider"
go run ./cmd/cyberagent run checkpoint <run-id>
go run ./cmd/cyberagent run lease <run-id>
go run ./cmd/cyberagent run usage <run-id>
go run ./cmd/cyberagent tool schema
go run ./cmd/cyberagent tool schema work_item_create
go run ./cmd/cyberagent tool invoke work_item_create --run <run-id> --operation-key <stable-key> --payload-file C:\temp\work-item.json
go run ./cmd/cyberagent tool invoke note_create --run <run-id> --operation-key <stable-key> --payload-file C:\temp\note.json
go run ./cmd/cyberagent run pause <run-id>
go run ./cmd/cyberagent run resume <run-id>
go run ./cmd/cyberagent run cancel <run-id>
go run ./cmd/cyberagent run show <run-id>
go run ./cmd/cyberagent run events <run-id>
go run ./cmd/cyberagent todo create <run-id> "inspect parser" --priority high --acceptance "tests pass"
go run ./cmd/cyberagent todo create <run-id> "write tests" --depends-on <work-id>
go run ./cmd/cyberagent todo list <run-id> --status pending,blocked
go run ./cmd/cyberagent todo show <work-id>
go run ./cmd/cyberagent todo block <work-id> --reason "waiting for fixture"
go run ./cmd/cyberagent todo reopen <work-id>
go run ./cmd/cyberagent todo complete <work-id>
go run ./cmd/cyberagent note create <run-id> "parser decision" --content "Use strict JSON" --category decision --pin
go run ./cmd/cyberagent note create <run-id> "fixture evidence" --content-file C:\temp\note.txt --tag parser --source docs/spec.md --evidence evidence-1
go run ./cmd/cyberagent note list <run-id> --status active --category decision,summary --tag parser
go run ./cmd/cyberagent note show <note-id>
go run ./cmd/cyberagent note update <note-id> --content "Revised decision" --version 1
go run ./cmd/cyberagent note archive <note-id>
go run ./cmd/cyberagent note restore <note-id>
go run ./cmd/cyberagent workspace init demo
go run ./cmd/cyberagent workspace tree demo --depth 2
go run ./cmd/cyberagent workspace read demo README.md
go run ./cmd/cyberagent workspace read demo env.txt
go run ./cmd/cyberagent edit propose --workspace demo --path README.md --content "# Demo updated"
go run ./cmd/cyberagent edit list --workspace demo --status proposed
go run ./cmd/cyberagent edit show <edit-id>
go run ./cmd/cyberagent edit approve <edit-id>
go run ./cmd/cyberagent context compact --workspace demo --task task-demo --message "user: imported a Flask session CTF" --message "assistant: classified likely cookie signing" --message "tool: read app.py and config.py" --message "user: asked for next exploit step" --message "assistant: keep actions scoped and generate verifier"
go run ./cmd/cyberagent context show --task task-demo
go run ./cmd/cyberagent session create --workspace demo --title "Agent basics" --route learn
go run ./cmd/cyberagent session send <session-id> "hello, summarize your current capabilities"
go run ./cmd/cyberagent session send <session-id> "/model script"
go run ./cmd/cyberagent session send <session-id> "/ls ."
go run ./cmd/cyberagent session send <session-id> "/read README.md"
go run ./cmd/cyberagent session send <session-id> "/read env.txt"
go run ./cmd/cyberagent session send <session-id> "/write README.md # Session proposal"
go run ./cmd/cyberagent session send <session-id> "/run echo hello"
go run ./cmd/cyberagent session history <session-id> --all
go run ./cmd/cyberagent provider test mimo/mimo-v2.5-pro
go run ./cmd/cyberagent provider test deepseek/deepseek-v4-flash
go run ./cmd/cyberagent session send <session-id> "/model mimo/mimo-v2.5-pro"
go run ./cmd/cyberagent session send <session-id> "/model deepseek/deepseek-v4-flash"
go run ./cmd/cyberagent session send <session-id> "/run echo hello"
go run ./cmd/cyberagent tool list --session <session-id>
go run ./cmd/cyberagent tool show <tool-run-id>
go run ./cmd/cyberagent approval list --run <run-id> --status pending
go run ./cmd/cyberagent approval show <approval-id>
go run ./cmd/cyberagent approval grant create --session <session-id> --tool shell --reason "trusted build commands"
go run ./cmd/cyberagent approval grant list --run <run-id> --status active
go run ./cmd/cyberagent approval grant revoke <grant-id> --reason "phase complete"
go run ./cmd/cyberagent tool approve <tool-run-id>
go run ./cmd/cyberagent artifact list --run <run-id> --stream stdout
go run ./cmd/cyberagent artifact show <artifact-id>
go run ./cmd/cyberagent artifact read <artifact-id> --max-bytes 65536
go run ./cmd/cyberagent artifact verify <artifact-id>
```

Expected context behavior:

- `context compact` writes one row to `context_summaries`.
- Recent messages are preserved outside the summary according to `contextmgr.DefaultConfig`.
- Explicit `context compact` always moves at least one message into the summary when messages exist.
- `context show --task <id>` prints the latest summary for that task.
- `session send` on a Run-bound Session auto-starts/resumes the Run, applies Supervisor policy/budgets/actions, and persists one user/assistant pair; unbound Sessions retain legacy behavior.
- Slash commands persist the operator command plus a non-authoritative `tool` result; workspace/file/diff/tool output never becomes assistant history.
- Long session histories automatically compact older active messages into `context_summaries`.
- MiMo live smoke passed with env-only key and `mimo-v2.5-pro`; no key is stored by the application.
- DeepSeek live smoke passed with an env-only key and `deepseek-v4-flash` through both non-streaming provider health and RunSupervisor SSE paths; durable events contained model metadata/counters without the key.
- Tool proposal smoke passed: proposed shell command, dry-run approval completion, policy-denied risky command.
- Durable approval smoke passed in an isolated `CYBERAGENT_HOME`: pending lookup, approval detail, dry-run completion, approved lookup, and `approval.requested/decided` Run events all matched one proposal. Restart integration tests recover the same immutable review key without duplicate decision events.
- Session Grant/tool-budget smoke passed across separate CLI processes: active Shell authorization completed as dry-run, revocation restored per-call proposals, a new grant did not override dangerous-command Policy denial, and `run usage` reached the configured limit. Store tests prove exact scope, terminal/archived denial, grant-key conflict, v11-to-v12 preservation, atomic concurrent saturation, and one exhaustion event.
- TUI snapshot smoke passed with existing session history, selected proposed tool run, status line, and keyboard help rendered from SQLite.
- TUI picker smoke passed for empty state, existing session list, and direct session snapshot.
- TUI async submit unit test passed: Enter on `/run echo async` enters busy state, returns an async command, and refreshes the proposed tool run after `actionDoneMsg`.
- TUI workspace context unit tests passed: chat models render attached workspace metadata and picker-created sessions preserve workspace lookup.
- Workspace-scoped file tool tests passed: normal read/list works, absolute paths are rejected, `../` escape is rejected, and long reads are truncated.
- Session `/ls` and `/read` smoke passed with attached workspace; `../outside.txt` is denied and persisted as a safe Go command result, while successful workspace content is persisted as non-authoritative tool evidence.
- Secret-redaction tests passed across `redact`, `tools`, `session`, `contextmgr`, `toolrun`, `store`, and `llm`.
- Redaction smoke passed: runtime-created token-shaped content is redacted from `workspace read`, session `/read`, and session history.
- File edit unit tests passed for existing-file replacement, new-file creation, traversal rejection, stale proposal rejection, secret redaction, safe redacted diff fallback, and approval integrity checks.
- SQLite file edit persistence and filtering tests passed; store-boundary redaction recomputes the proposed-content hash.
- CLI file edit smoke passed in an isolated `CYBERAGENT_HOME`: propose/show/list/approve changed the file only after approval; session `/write` produced a persisted diff and denial left the file unchanged.
- `go vet ./...` and targeted `go test -race` for `fileedit`, `store`, `session`, and `tools` passed.
- Repository token-prefix scan returned `NO_TOKEN_PATTERN_IN_REPO`.
- Mission/Run CLI smoke passed in an isolated home: create, ordered events, start, pause, resume, cancel, show, filtered list, legacy provider command, and cleanup all succeeded.
- Final run-centric race tests passed for `domain`, `events`, `application`, `store`, and `app`.
- Run activity projection tests passed for automatic/existing Session binding, one-to-one reuse rejection, contiguous event order, idempotent saves, invalid-state rollback, and cross-workspace rejection.
- Isolated CLI smoke produced 14 contiguous events spanning Run, Session, Policy, ToolRun, and FileEdit across separate process invocations.
- TaskAdapter tests passed for repeated and eight-way concurrent adaptation, event order, unsupported legacy kinds, and a single persisted Run.
- Isolated adapter CLI smoke passed across separate processes with one four-event timeline including root registration and stable exit codes `2` (invalid argument) and `3` (not found).
- Legacy Task/Event Store-boundary redaction tests passed with runtime-generated token-shaped fixtures.
- RunSupervisor tests passed for normal completion, strict lifecycle parsing, JSON request metadata, root finish/wait, wait-resume, paused execution, lifecycle replay idempotence, schema v8 checkpoint persistence, cumulative tokens, persisted execution timeout, remaining call deadline, bounded execution, MaxTurns rejection, cancellation before begin, nil response/negative usage rejection, tool-call rejection, and immediate/persisted redaction.
- Restart recovery test persisted `turn_started`, closed and reopened SQLite, resumed the same attempt, and observed one `agent.turn_started` plus one `agent.turn_completed` event.
- Isolated Supervisor CLI smoke passed across separate processes with two bounded turns, completed/failed finalization, cumulative token exhaustion, and stable exit codes `4` (precondition) and `8` (budget exhausted).
- Isolated root lifecycle CLI smoke passed with visible `action: continue`, two `supervisor.action_committed` events, one terminal completion event, and token-budget exit code `8`.
- Final gate passed with `go test -count=1 ./...`, `go vet ./...`, and targeted `go test -race` across error, domain, event, application, store, session, LLM, and app packages.
- Session/Run integration tests passed for automatic start, wait/resume across Store restart, terminal rejection, legacy unbound compatibility, pending-input conflict/size/redaction boundaries, compacted-summary reuse, and exactly-once messages/events.
- Isolated CLI smoke passed across separate processes with Run-bound `session send`, visible action/status metadata, `idle/next_turn=2`, one message pair, one started/completed event pair, and an unbound legacy Session fallback.
- Provider tests passed for HTTP 429/503/529/401 classification, malformed/empty responses, numeric/date/overflow `Retry-After`, network/cancellation normalization, and redacted error bodies.
- RunSupervisor retry tests passed for two transient failures then one commit, permanent no-retry, rate-limit exhaustion plus pending-input recovery, long `Retry-After` refusal, cancellation during call/backoff, cross-Store attempt continuation, and idempotent execution-time accounting.
- Protocol repair tests passed for repair success, second-invalid failure, raw-output isolation, token-budget blocking, atomic terminal replay, pending/exhausted restart recovery, and cancellation after a returned response.
- Repair transport tests passed with global model attempts `1/2/3` and phase-local transport attempts `1/1/2`; Store rejects terminal metadata that differs from its durable start event.
- Isolated Provider CLI smoke showed `model_attempts: 1`, `protocol_repairs: 0`, `model_outcome: success`, one `model.started`, one `model.completed`, no `model.failed`, and an idle next-turn checkpoint with empty repair state.
- Streaming tests passed for Anthropic-compatible SSE, Router redaction, split UTF-8, the 64 KiB output ceiling, the 32-event coalescing cap, malformed/missing final metadata, mid-stream retry, cancellation and restart recovery, delta idempotence, and terminal-ledger consistency.
- Isolated streaming CLI smoke showed one text-free `model.delta`, positive `stream_bytes`, matching terminal counters, and no failed model event.
- Active-call tests passed for durable-start visibility, duplicate-Run exclusion, ordered snapshot/progress/cancel/terminal envelopes, bounded slow-consumer disconnection, idempotent cancellation, redacted audit reasons, terminal cleanup, and cancellation races.
- Signal-aware CLI tests repeatedly cancelled a blocking SSE Provider, returned exit code 7, persisted `model.failed` with outcome `cancelled`, and retained a recoverable `turn_started` checkpoint.
- TUI tests passed for parallel submit/discovery commands, live snapshot/progress/terminal rendering, slow-consumer disconnect, busy-exit protection, `Ctrl+X` audited cancellation, request-context fallback, picker propagation, and responsive help widths.
- A real Run-bound TUI integration test streamed partial output from a blocking Provider, rendered byte progress, cancelled through the shared Supervisor, and observed one durable `model.cancel_requested` plus one `model.failed`.
- The local Go toolchain was upgraded from 1.26.1 to 1.26.5 after `govulncheck` identified reachable standard-library advisories; the repeated scan reports zero reachable vulnerabilities.
- Final Go 1.26.5 gates passed: `go test -count=1 ./...`, `go vet ./...`, targeted race tests, isolated CLI/TUI smoke, credential-prefix scanning, and clean `staticcheck ./...`.
- Work Board gates passed for domain invariants, migration v9 and legacy preservation, dependency/cycle/FK enforcement, transactional rollback, stale/concurrent versions, service and CLI lifecycle, Supervisor context bounds/redaction, and premature model-finish repair.
- The commit-time completion-race test created a WorkItem during the Provider response, then verified the stale model finish wrote no Session messages or completion events and retained a recoverable started checkpoint.
- An isolated Work Board CLI smoke completed a two-item dependency chain and observed exactly two `work_item.created` plus five `work_item.changed` events.
- Final Notes/Context gates passed with uncached full tests, vet, full-repository race tests, clean staticcheck, zero reachable govulncheck findings, `NO_CREDENTIAL_PATTERN_IN_REPO`, and `NO_RUNTIME_OR_SECRET_ARTIFACTS_IN_REPO`.
- Note gates passed for domain invariants, invalid UTF-8 rejection, migration v9-to-v10 preservation, relation foreign keys, visibility/tag/limit filters, transactional rollback, exact changed-field audit, stale/concurrent versions, service/CLI lifecycle, bounded content-file input, and terminal-Run rejection.
- Context selection tests passed for deterministic priority, exact estimate limits, redaction, root visibility, pinned/category priority, overflow provenance, and Note-body isolation from durable events.
- An isolated Note CLI smoke created, updated, archived, and restored one Note, ending at version 4 with exactly one `note.created` and three `note.changed` events.
- DeepSeek adapter tests passed for env-only registration, no-key exclusion, default model selection, Anthropic request path/header shape, and CLI key non-disclosure. Live `deepseek-v4-flash` health and Supervisor SSE smoke both succeeded with positive stream bytes and durable started/delta/completed events.
- Tool Gateway tests passed for exact schemas, lifecycle invariants, approval modes, workspace-root binding, secret redaction, hard output bounds, MIME/UTF-8 validation, valid multibyte truncation, invalid bytes at and before the boundary, policy denial, dry-run shell review, file approval, and legacy adapter compatibility.
- Production-path regression tests passed for CLI, Run-bound and legacy Session slash commands, SQLite ToolRun/FileEdit Run-event projection, and Bubble Tea tool review after direct manager construction was centralized behind the Gateway.
- The final Tool Gateway gate passed with uncached full tests, vet, full-repository race tests, clean staticcheck, zero reachable govulncheck findings, isolated CLI approval/denial smoke, `NO_CREDENTIAL_PATTERN_IN_REPO`, and `NO_RUNTIME_OR_SECRET_ARTIFACTS_IN_REPO`.
- Script Gateway tests passed for required workspace scope, relative-file resolution, absolute/traversal rejection before Run creation, deterministic `script_process.v1` encoding, backend/argv/size constraints, token redaction, policy-denied persistence, Run-event projection, and no local side effects before or after approval.
- Structure-aware Store redaction tests passed for nested JSON strings, exact 64-bit numbers, invalid payloads, 1 MiB size, 64-level depth, and event rollback on failure.
- The final script slice gate passed with uncached full tests, vet, full-repository race tests, clean staticcheck, zero reachable govulncheck findings, and an isolated real-binary smoke that observed risky exit code 5 with no marker file. Scans returned `NO_PRODUCTION_SANDBOX_RUNNER_CALLS`, `NO_CREDENTIAL_PATTERN_IN_REPO`, and `NO_RUNTIME_OR_SECRET_ARTIFACTS_IN_REPO`.
- The final schema v12 gate passed with uncached full tests, full-repository race tests, vet, clean staticcheck, and zero reachable govulncheck findings. Repository scans found zero credential-pattern files, zero tracked runtime artifacts, and zero production Sandbox references.
- The final schema v13 gate passed with uncached full tests, full-repository race tests, vet, clean staticcheck, and zero reachable govulncheck findings. Twelve-way idempotency, rollback, migration, approval-gate, multi-Process, cross-Run binding, CLI conflict/policy, redaction, and no-side-effect tests passed. Isolated real-binary smoke returned conflict exit code 4 and Policy exit code 5, consumed one tool call across replay, completed only as dry-run, and created no marker file. Repository scans returned `NO_USER_TEST_KEYS_IN_REPO`, `NO_CREDENTIAL_PATTERN_IN_REPO`, `NO_TRACKED_RUNTIME_OR_SECRET_ARTIFACTS`, and `NO_PRODUCTION_SANDBOX_RUNNER_CALLS`.
- The final schema v14 gate passed with uncached targeted/full tests, full-repository race tests, vet, clean staticcheck, and zero reachable govulncheck findings. Domain, migration, source-binding, redaction, truncation, rollback/recovery, replay, tamper, CLI, and Policy-denial tests passed. Isolated real-binary smoke created one stable Artifact and one `artifact.created`, verified its hash and redacted content, and retained `tool_calls: 1` after approval replay.
- The final schema v15 gate passed with `go test -count=1 ./...`, full-repository `go test -race -count=1 ./...`, `go vet ./...`, clean `staticcheck ./...`, and zero reachable `govulncheck` findings. Cross-Store budget and structured replay tests passed ten consecutive runs. An isolated real-binary smoke verified WorkItem create/replay, changed-intent exit code 4, redacted Note creation, Policy exit code 5, five charged attempts, one domain/completion event per successful entity, and no raw operation key or secret in the timeline; the temporary runtime was removed.
- The schema v16 gate passed uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. Tests cover Anthropic request/response/SSE tool blocks, strict Store revalidation, model/tool transactional persistence, restart after entity creation but before result recording, semantic replay across attempts and rounds, Policy denial, budget exhaustion, four-round bounds, and cross-Store result convergence. An isolated real-binary mock smoke exported both schemas and completed one Run turn with `tool_rounds: 0`/`tool_calls: 0`; its runtime was removed. Credential scanning found only the intentional redaction-test fixture and no user test keys.
- The local read-API gate passed uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. Tests cover real SQLite state, every published resource family, endpoint-scoped pagination, historical Supervisor tool rounds, secret redaction, omitted Artifact content/checkpoint input, loopback and bearer boundaries, internal error hiding, 32 concurrent readers, CLI token non-persistence, and graceful server cancellation. An isolated real-binary smoke verified `v0.1.0`, `api.v1`, schema v16, authenticated 200, bad-token 401, POST 405, no CORS, no environment-token echo, and no token in the closed runtime database; its process and runtime were removed.
- The Run-aware TUI gate passed uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. Real SQLite tests cover all four activity views, pending tool rounds, exact Grant linkage, `g` key async completion, later safe Shell auto-dry-run, grant-authorized crash recovery, current-Policy recheck before grant creation, cross-Session approve/grant/deny rejection with no state change, permanent dangerous-command denial, and terminal-cell-safe Chinese rendering. An isolated real-binary smoke created a Run, WorkItem, Note, active Shell Grant, and auto-dry-run proposal, rendered their shared TUI snapshot, and removed its runtime.
- The schema v17 execution-lease gate passed uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. Tests cover eight-way cross-connection acquisition, explicit replay, expiry takeover, stale checkpoint/renew/release rejection, v16 pending-checkpoint migration, long-call heartbeat, one lease across two Execute turns, atomic stale tool-budget rejection, zero stale entity/event writes, and token-free Outcome/CLI/API/event projections.
- The OpenAPI contract gate passed uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. It covers deterministic generation, 16 initial live authenticated routes, 23 initial DTO-derived schemas, golden-file drift detection, raw media type, query/auth rejection, read-only operations, and explicit exclusion of Artifact content, checkpoint pending input, `lease_id`, fencing tokens, and API-key fields. The former license advisory is now resolved by the owner-selected Apache-2.0 metadata; an isolated real binary exported the contract without creating runtime state.
- The durable Run-event SSE gate passed uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. It covers the then-current 17-path/24-schema contract, exact bounded replay and resume, heartbeat comments, cross-SQLite-connection visibility, process concurrency exhaustion/release, write deadlines, and graceful server cancellation. An isolated real binary streamed two durable frames over authenticated loopback SSE, exposed no internal fields or token, persisted no API token, and left no temporary runtime.
- The schema v18 cross-process cancellation gate passed full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. It covers two-connection HTTP-to-worker cancellation of a blocking Provider, distinct read/control tokens, default-disabled control, 202 exact replay, changed-key/intent conflict, latest-attempt checks, stale lease rejection, crash-orphan `superseded` resolution, secret redaction, raw-key non-persistence, strict 4 KiB JSON, and token/fencing-field exclusion. The generated contract now has 18 paths, 26 schemas, two security schemes, and Apache-2.0 license metadata; Redocly validates it without warnings. An isolated real binary reported schema v18, accepted read health, returned 401 for read-token POST and control-token GET, returned 404 for an authorized missing-Run cancellation, echoed neither token, persisted neither token after shutdown, and removed its temporary runtime.

- The schema v19 single-root Coordinator gate passed full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. It covers stable root creation and lazy v18 registration, concurrent idempotent registration, atomic ready/running/waiting/terminal projection, wait/restart/resume identity continuity, Run cancellation cascade, redacted bounded inbox send, exactly-once consume, payload-hash tamper detection, and 32-snapshot retention. An isolated CLI smoke created a schema v19 Run and restored one ready root through `run graph`; child creation remains structurally disabled with `child_limit=0`, and inbox delivery is internal rather than model-visible.
- The schema v20 inbox-protocol gate passed uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. It covers exact replay, changed-intent conflict, operation-key digest-only persistence, waiting-Specialist wake exactly once, root/pause safeguards, strict dependency payloads, zero duplicate events/snapshots, and v19 row/snapshot compatibility. Specialist admission, child model execution, model-visible inbox delivery, and new Shell/network authority remain disabled.
- The schema v21 Specialist-admission gate passed uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. It covers default denial, invalid policy, Skill escalation, per-child and aggregate budgets, two-child capacity, dedicated Session inheritance, exact replay/changed-intent conflict, cross-Store convergence, event-failure rollback, digest-only key storage, reduced root Supervisor budget, pause/resume cause tracking, terminal cascade/Session archival, restart recovery, and v20 migration. Child model execution and public/model-driven spawn remain disabled.
- The schema v22 Agent-owned-memory gate passed uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. It covers same-Run root/Specialist ownership and Note visibility, cross-Run service and SQLite-trigger rejection, Agent reassignment, visibility changes that preserve ownership, deterministic v21 migration, CLI/HTTP filters and DTOs, OpenAPI drift protection, and automatic Supervisor binding without a model-controlled identity field. An isolated binary smoke created a v22 Run/root, filtered an Agent-owned WorkItem and owner-only Note, verified the generated OpenAPI field, and removed all temporary runtime data.
- The schema v23 CompletionReport gate passed uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. It covers explicit protocol versions, raw and redacted size limits, child-owned WorkItem and parent-visible Note references, success/partial handoff rules, exact active-attempt binding, stale-attempt rejection, event-failure rollback, digest-only idempotency, cross-Store convergence, Session archival, parent result delivery, graph recovery, tamper detection, and deterministic v22 migration. Credential-pattern and tracked runtime-artifact scans are clean. An isolated CLI smoke created a schema v23 Run and removed its temporary runtime.
- The schema v24 Specialist Attempt Runtime gate passed uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. It covers explicit runtime denial, turn/token charging, exactly-once usage, continuation/crash replay, terminal immutability, event-failure rollback, cross-Store scheduling convergence, budget exhaustion, Session archival, redacted parent notification, expired-lease takeover, stale-worker fencing, exactly-once recovery, pause interruption/resume, graph consistency, and deterministic v23 migration. Production credential patterns and tracked runtime artifacts are absent. An isolated real binary reported v0.1.0, listed only the mock Provider, initialized a workspace, opened the schema v24 runtime, created/listed a review Run, and removed its temporary home.
- The schema v25 root inbox context gate passed uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. It covers strict protocol decoding, bounded ordering, direct-child and durable-report/attempt backing, double-Store convergence, prepare/commit rollback, failure supersession, exact-once consumption, cancellation plus process-restart replay, lease takeover and stale fencing, prompt redaction/cursor exclusion, graph recovery, and deterministic v24 migration. An isolated real binary reported v0.1.0, listed only mock, initialized a workspace, opened schema v25, created/listed a review Run, restored its ready root through `run graph`, and removed its temporary home.
- The schema v26 internal Specialist-turn gate passed full tests, full-repository race tests, `go vet`, clean `staticcheck`, `go mod verify/tidy -diff`, a production credential-pattern scan, and `govulncheck` with zero reachable vulnerabilities. It covers strict `specialist_lifecycle.v1`, no-tool enforcement, continue/CompletionReport finish, budget-exhausted continuation denial, retry with exactly-once usage, invalid-response charging, Policy denial, cancellation before lease release, stale-worker takeover, atomic child Session history, immutable model terminal rows, intent-fingerprint conflicts, SQLite sequence/lease/usage triggers, and deterministic v25 migration. An isolated real binary reported v0.1.0, listed only mock, initialized a workspace, opened schema v26, created/listed a review Run, and removed its temporary runtime. No CLI/HTTP/OpenAPI route, public/model spawn, Shell/network capability, or child tool was added.
- The schema v27 recoverable Specialist-context gate passed full tests, full-repository race tests, `go vet`, clean `staticcheck`, `go mod verify/tidy -diff`, a production credential-pattern scan, and `govulncheck` with zero reachable vulnerabilities. It covers strict `specialist_instruction.v1`, direct-root routing, four-message ordering, child-owned/visible memory filtering, token and byte omission, content-free provenance, prepare replay, manual-consume denial, atomic continue/finish commit, injected event-failure rollback, crash preservation, active-supersede rejection, expired-lease takeover redelivery, graph restore, direct-SQL malformed-payload rejection, and deterministic v26 migration. The audit found no unresolved high/medium issue and fixed one pre-existing low-risk boundary by reserving every running Specialist inbox for AgentAttempt context before generic consumption. An isolated real binary reported v0.1.0, listed only mock, initialized a workspace, created/listed a review Run in a schema v27 runtime, and deleted all temporary data. No CLI/HTTP/OpenAPI route, public/model spawn, Shell/network capability, or child tool was added.
- The Go-internal bounded Specialist-scheduler gate passed full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero reachable vulnerabilities. A schedule owns one Run lease, runs at most two explicitly selected ready children per round, caps execution at 32 rounds, fans parent/heartbeat/first-error cancellation out to active siblings, and waits for every child Attempt to reach a durable terminal state before release. Its goroutine boundary converts a custom Provider/runtime panic into a payload-free `INTERNAL` failure, durably crashes an already-started Attempt, and cancels the sibling instead of crashing the process. Tests prove real two-call overlap, one shared lease generation, parent cancellation, first-failure and panic sibling cancellation, all-terminal and round-limit stops, root-plus-child token accounting, deterministic execution-time slices, summed child model time, zero calls after a spent root timeout, stale-attempt takeover recovery, and hard conflict on projection drift. Aggregate usage comes from the root checkpoint, Agent and Attempt token ledgers, and every Specialist model-call duration. No schema migration, CLI/HTTP/OpenAPI route, public/model spawn, child tool, Shell, or network authority was added. The remaining low-risk budget caveat is that Provider `MaxTokens` constrains output only; input-inclusive final usage may cross the line once, is fully persisted, and blocks every later round.
- The schema v28 Specialist lifecycle-repair gate passed full tests, full-repository race tests, `go vet`, clean `staticcheck`, `go mod verify/tidy -diff`, production credential/runtime-artifact scans, and `govulncheck` with zero reachable vulnerabilities. It covers successful repair, second-invalid exhaustion, cancellation and budget abort, primary/repair independent transport retries, contiguous global numbering, phase-local numbering, cumulative token/model-time usage, exact start/terminal replay, raw-invalid-output isolation from repair prompt/Session/events, direct-SQL phase and terminal guards, and deterministic v27 model-ledger migration. The audit found no unresolved high/medium issue and fixed four low-risk defects: terminal-start replay ordering, rune-buffer reset during bounded reason truncation, wall-clock rollback at repair resolution, and consecutive user roles in Anthropic-compatible repair requests. An isolated real binary reported v0.1.0, listed only mock, initialized a workspace, created/listed a review Run in a schema v28 runtime, and removed all temporary data. No CLI/HTTP/OpenAPI route, public/model spawn, child tool, Shell, or network authority was added.
- The schema v29 Specialist schedule-control gate adds lease-fenced `specialist_schedules`/target rows, immutable start/stop summaries, takeover convergence to `abandoned/worker_lost`, a separate digest-only and immutable child-cancellation operation ledger, and `/api/v1/runs/{run_id}/agents/{agent_id}/active-call/cancel`. Tests cover exact schedule finish replay and immutability, metadata-only lifecycle events, stale schedule plus Attempt recovery/counts, coordinator-panic failure summaries, cancellation replay/conflict/observation/terminal resolution, monotonic timestamps under wall-clock rollback, Attempt-terminal cleanup, eight-Store same-intent convergence (ten consecutive runs), and an end-to-end blocking Specialist Provider cancelled through a second SQLite connection and the control API. The audit fixed four low-risk defects: SQLite root `parent_id` is `NULL`; the original OpenAPI fixture root was already running; child cancellation observation/resolution could predate its request after wall-clock rollback; and a coordinator panic after schedule start could be misclassified as completed. Schedule persistence remains outside the ordinary Runner's minimal Store interface, watcher/context cleanup is panic-safe, and scheduler panic payloads never enter events. Final `go test ./...`, full-repository `go test -race ./...`, `go vet ./...`, clean `staticcheck ./...`, `go mod verify`, `go mod tidy -diff`, credential/runtime-artifact scans, and `govulncheck ./...` passed with zero reachable vulnerabilities. An isolated CLI smoke registered only mock, initialized a workspace, created/listed a review Run in a schema v29 runtime, and removed all temporary data. Root and Specialist cancellation retain distinct tables and exact targets; no raw operation key, model text, lease id, generation, API token, public/model spawn, child tool, Shell, or network authority is exposed.
- The schema v30 delegation-proposal gate adds strict `specialist_delegation.v1`, the Supervisor-only `specialist_delegation_propose` tool, a separate `agent_proposal` action class, immutable proposal/assignment tables, a digest-only one-to-one operation ledger, metadata-only `agent.delegation_proposed`, and read-only `run delegations/delegation` CLI inspection. Tests cover unknown/trailing/version/size drift, duplicate goals, unfenced and non-Supervisor calls, Policy denial, parent-Skill escalation, non-delegable proposal authority, legacy-root capability repair, child and budget headroom, SQL immutability, secret redaction, stable semantic replay across changed Provider IDs, and eight concurrent invocations through two independent SQLite connections. Exactly one proposal/event/tool completion is created, raw operation keys and assignment text stay out of events, and the Agent graph remains root-only. Model-attributable semantic rejection is returned as a bounded `INVALID_ARGUMENT` tool result; lease, Store, and internal failures still fail the turn. The final audit removed accidentally persisted `lease_id/generation` columns from the draft operation ledger; its trigger now correlates the current active lease and checkpoint directly, and a schema assertion prevents lease identity from returning to that table. Final full tests, full-repository race tests, `go vet`, clean `staticcheck`, module verification/tidy diff, credential/runtime-artifact scans, and `govulncheck` all pass with zero reachable vulnerabilities. An isolated CLI smoke verified version/provider, workspace and Run creation, the delegation schema, empty read-only inspection, and rejection of the ordinary `tool invoke` bypass, then removed its runtime. Review/application is intentionally absent, so every proposal remains immutable `proposed` with `admission_authorized=false`; existing Go-internal admission is not called by this path.
- The schema v31 delegation-review gate adds immutable `specialist_delegation_reviews`, a separate digest-only operation ledger, metadata-only `agent.delegation_reviewed`, and explicit `run delegation approve/reject/show` plus list status. Approval requires a running Run; rejection requires a redacted reason and may close a terminal Run proposal. Same-key intent converges across eight calls and two SQLite connections, changed intent and a second decision conflict, update/delete triggers preserve evidence, v30 proposals survive migration, and CLI replay remains explicit. Review reasons and raw operation keys stay out of events; the operation table has no reason or lease columns. Every result reports `admission_authorized=false` and `application_required=true`, and tests confirm the Agent graph remains root-only. The audit found no unresolved high/medium issue and fixed three low-risk consistency defects: an omitted JSON false field could satisfy the event check, wall-clock rollback could predate the proposal, and a second operation key could return a lifecycle error instead of stable conflict after Run termination. Required pointer fields, application clamping plus Store/trigger timestamp checks, and existing-review-first ordering close those paths. The concurrent gate passed ten consecutive runs. Final full tests, full-repository race tests, `go vet`, clean `staticcheck`, module verification/tidy diff, credential/runtime-artifact scans, and `govulncheck` pass with zero reachable vulnerabilities.
- The schema v32 delegation-application gate adds `applying/applied/aborted` application state, ordered `pending/admitted/instructed` assignment transitions, digest-only user and deterministic Coordinator operations, metadata-only lifecycle events, and `run delegation apply`. Begin revalidates current Policy, operation-backed approval, running Run, ready root, active Session, idle child runtime, parent Skills, existing child policy, capacity, and aggregate budget. Agent/Message commit gaps recover through exact Coordinator replay; eight callers across two Stores converge on one application, at most two children, and one strict instruction per child. Applying blocks root turns, unrelated admission/messages, schedules, and direct Attempts; terminal Run transitions atomically abort and count partial progress. Applied children remain ready with zero Attempts and schedules. Policy denial creates only an audit event. v31 rows survive migration, raw keys and assignment content stay out of events, and update/delete/timestamp triggers defend direct mutation. The independent audit found no unresolved high/medium issue and fixed two low-risk integrity gaps by binding initial assignment/operation timestamps in both Go and SQLite and adding a direct durable-schedule reservation regression. Final full tests, full-repository race tests, `go vet`, clean `staticcheck`, module verification/tidy diff, credential/runtime-artifact scans, and `govulncheck` pass with zero reachable vulnerabilities.
- The schema v33 read-only Fan-out planning gate adds independent `readonly_fanout.v1` tiers `auto/1/2/4/6`, a compile-time and SQLite-pinned workspace-list/read capability fingerprint, bounded canonical workspace scanning, immutable file/hash manifests and deterministic shards, a digest-only operation ledger, metadata-only `readonly_fanout.planned`, and explicit `run fanout plan/fanouts/fanout show`. Core Agent/delegation/scheduler limits remain two. Planning requires a running network-disabled Run and active same-workspace Session; it follows no symlink, excludes VCS/dependency/build/binary/secret-like material, and is capped at 20,000 entries, 256 files, 128 KiB per file, and 768 KiB total. Eight calls across two Stores converge to one plan/event, ten repeated concurrency runs pass, changed intent conflicts, denial creates no state, v32 upgrades in place, and raw keys/goals/paths/roots stay out of events. Agent/Attempt/schedule counts remain unchanged and every v33 CLI surface states `execution_authorized=false`. The audit found no unresolved high/medium issue and fixed two low-risk boundaries: an accidental dependency on initialized Tool Budget was replaced by a non-charging Run row lock, and symlink scope roots are now rejected while bounded reads use Go `os.Root`. Full tests, full-repository race tests, vet, clean staticcheck, module verification/tidy diff, zero-reachable-vulnerability scanning, credential/runtime-artifact scans, and an isolated tier-six CLI smoke pass. That planning-only boundary is now followed by the separate schema v34 hash-revalidation, budget, cancellation, and result-ledger gate.
- The schema v34 read-only Fan-out execution gate adds strict `readonly_fanout_report.v1`, `readonly_fanout_executions`, execution shards, conservative model-call reservations, shard-scoped findings, a digest-only operation ledger, and explicit `run fanout execute/execution`. Under one Run lease it rebuilds the complete v33 manifest, then reopens every file through Go `os.Root` and rechecks regular-file identity, byte count, and SHA-256 before dispatch. Verified source input is held only in memory and credential-redacted before a tool-free JSON-mode request. Go runs exactly the planned 1/2/4/6 workers, cancels siblings on first error, waits for durable terminal shards, and exposes no Shell, file-write, process, network, external-tool, or recursive-spawn capability. New lease generations abandon uncertain calls, preserve their token/time reservations, reset only running shards, and fence old workers; completed shards are never repeated. `RunAgentUsage` now reconciles root, Specialist, and Fan-out usage, and Specialist schedule snapshots persist the Fan-out component. Relative manifest paths and strict report/finding results remain in local SQLite for recovery and audit, while raw source, absolute workspace roots, operation keys, and report/finding content stay out of Run events. Tests cover six-call overlap, success/replay, zero-call budget denial, one-failure/five-cancellation cleanup, stale-lease fencing, conservative crash charges, same-key convergence, v33 migration, the CLI lifecycle, metadata-only events, and an unchanged root-only Agent graph. The audit found no unresolved high/medium issue and fixed three low-risk code defects: random execution IDs were removed from replay intent, Specialist schedule snapshots gained compatible Fan-out usage columns, and non-completed terminal executions can no longer claim every shard completed. A final documentation audit also corrected one overbroad persistence claim. Full tests, full-repository race tests, vet, clean staticcheck, module verification/tidy diff, credential scanning, and `govulncheck` pass with zero reachable vulnerabilities. An isolated real-binary tier-six CLI smoke also completes six shards, replays with zero duplicate calls, reports unified usage, and preserves a root-only Agent graph.
- The schema v35 Finding/Evidence/Report slice adds generic bounded domain types, exact-fact fingerprints, conservative duplicate-confidence selection, deterministic IDs/projection digests, `finding_reports/findings/finding_evidence`, metadata-only `report.generated`, and `run fanout report` plus `report show`. A writer transaction creates a private `building` aggregate and can generate it only after SQLite rechecks the completed v34 source, grouped/source counts, severity totals, and contiguous Finding/Evidence ordinals; generated rows are immutable. Each source remains `model_assertion` Evidence bound to its source fingerprint and shard report digest, and every Finding remains `draft`. Markdown neutralizes model markup and hostile backticks; JSON and Markdown are byte-stable across replay and make no Provider call. Tests cover exact deduplication, severity disagreement preservation, minimum confidence, rendering injection safety, v34 migration, zero-finding reports, ten rounds of eight callers across two SQLite Stores, one event/projection, direct mutation denial, content-free events, and CLI generate/replay/show. The audit found no unresolved high/medium issue and fixed three low-risk issues: write-lock acquisition now precedes transactional reads to avoid SQLite lock upgrades, ordering compares path/line before the digest, and Markdown code spans plus Evidence ordinal checks handle hostile names and direct SQL drift. An isolated real-binary smoke completes plan/execute/report, byte-identical JSON replay, Markdown reload, one report event, and a root-only Agent graph before cleaning its runtime. Full tests, full-repository race tests, vet, clean staticcheck, module verification/tidy diff, formatting, credential/documentation scans, and `govulncheck` pass with zero reachable vulnerabilities.
- The schema v36 Artifact-backed validation slice adds immutable `finding_artifact_evidence` and `finding_validation_decisions`, separate digest-only operation ledgers, metadata-only `finding.evidence_attached`/`finding.validation_decided`, and `report finding attach/validate/reject/verify`. The Store rereads and hash-validates the full same-Run Artifact, then snapshots size, MIME, stream, tool, source, SHA-256, and redaction metadata. SQLite freezes all Run Artifacts plus evidence/decision/operation rows. Validation requires at least one Artifact Evidence record; rejection may have zero. A decision freezes the ordered Evidence count/digest, blocks later attachment, and leaves the v35 source Finding and projection digest untouched. Eight calls across two Stores converge to one evidence and one decision; changed intent, cross-Run evidence, evidence-free validation, post-decision attachment, and second decisions fail with stable typed errors. Tests cover zero-evidence rejection, legacy-tamper detection, v35 migration, raw-key and narrative non-persistence in events, Markdown injection safety, and the complete CLI workflow. The audit found no unresolved high/medium issue and fixed one low-risk semantic omission by freezing and displaying the Artifact `redacted` flag. Full tests, full-repository race tests, `go vet`, clean `staticcheck`, module verification/tidy diff, and `govulncheck` pass with zero reachable vulnerabilities.
- The trust-aware SARIF/CI slice initially added no migration or mutable state. `report show --format sarif` produces deterministic OASIS SARIF 2.1.0 using five stable severity rules, workspace-relative escaped paths, and the v35 Finding fingerprint. Schema v37 now projects only confirmed unresolved `validated/accepted` Findings into `results`; all status counts remain run metadata, while Artifact bodies, Evidence notes, operator reasons, and Evidence-set digests are excluded. `cyberagentValidationStatus` preserves the validation decision and `cyberagentFindingStatus` carries the current lifecycle. `report check` defaults to validated/high, includes accepted unresolved items, admits drafts only under explicit `active`, never matches fixed/rejected, writes text or JSON before returning stable exit code 4, and supports `none` for non-blocking inspection. The original audit fixed one medium-risk interoperability issue: draft/rejected OASIS `review/pass` results could still appear as GitHub alerts because Code Scanning ignores unsupported `result.kind`. Three low-risk robustness fixes reject non-canonical public `GatePolicy` values instead of failing open, propagate text-output errors, and remove the unnecessary Evidence-set digest from public output.
- The schema v37 Finding-remediation gate adds immutable acceptance, remediation Evidence, and fix decisions plus separate digest-only operation ledgers and metadata-only `finding.accepted`, `finding.remediation_evidence_attached`, and `finding.fixed` events. Acceptance requires and freezes an exact validated decision. Remediation Evidence must use a different same-Run Artifact whose durable `artifact.created` sequence is strictly after the acceptance event; neither validation Evidence nor a pre-acceptance Artifact can be reused. Fix requires at least one remediation record and freezes its ordered count and domain-separated digest. The source v35 Finding and projection digest remain unchanged. Two SQLite connections with eight simultaneous callers converge to one acceptance, Evidence, fix, operation row, and event at each phase; the convergence test passed ten consecutive runs. Tests also cover changed intent, missing Evidence, post-fix append, v36 preservation, raw-key/narrative isolation, direct SQL immutability, full Markdown/JSON/SARIF/gate semantics, and an end-to-end CLI lifecycle. The audit found no unresolved high/medium issue and fixed three low-risk consistency defects: SQLite triggers now pin decision/Evidence timestamp order, shared mutation errors no longer mislabel every lifecycle failure as validation, and SARIF separates the actual validation status from the current Finding status. Uncached full tests, full-repository race detection, `go vet`, clean `staticcheck`, module verification/tidy diff, credential/runtime-artifact scans, and `govulncheck` all pass with zero reachable vulnerabilities. An isolated real binary created a schema v37 runtime, completed report validation/acceptance/fresh remediation/fix, verified fixed status, emitted zero SARIF results, passed the CI gate, observed exactly one event of each new type, and removed its temporary runtime.
- The schema v38 operator-scheduling gate adds immutable request, selected-target, digest-only operation, and schedule-attempt mapping ledgers plus metadata-only `agent.operator_schedule_requested`. Only the operator recorded by an applied v32 application may request one or two of its instructed ready children. The service rechecks current Policy before each not-yet-terminal execution and then reuses the existing lease-fenced scheduler, aggregate root/Specialist/Fan-out budgets, sibling cancellation, model-call accounting, and maximum 32 rounds. Pending requests reserve their targets from ordinary internal scheduling. Same-key concurrent calls across two SQLite Stores converge to one request and one schedule; changed intent conflicts, a new key creates an explicit continuation, and an expired running attempt is abandoned and recovered under the same request with the next ordinal. Targeted tests cover eight callers, exact model/Attempt counts, unauthorized operators, Policy denial with zero request state, raw-key/event isolation, SQL immutability, v37 migration, real CLI schedule/replay/continue/show, and ordinary-tool bypass rejection; the eight-caller convergence test passed ten consecutive runs. The code audit found no unresolved high/medium issue. Uncached full tests, full-repository race detection, `go vet`, clean `staticcheck`, module verification/tidy diff, credential/runtime-artifact scans, and `govulncheck` all pass with zero reachable vulnerabilities. An isolated real binary registered only mock, initialized a workspace, created and listed a schema v38 Run, and removed its temporary runtime.
- The GitHub Actions annotation slice adds no migration or Store write. `GateResult` now retains an in-memory, JSON-hidden snapshot of the exact Findings selected by the existing status/severity policy; consistency validation rejects mismatched counts, nonmatching lifecycle states, duplicate Finding IDs, invalid paths/lines, and more than the report-wide 192-Finding bound. `report check --format github` renders those snapshots as official `notice/warning/error` workflow commands with deterministic file/line/title/status/category/ID/fingerprint metadata, while preserving the existing pass/fail exit. Properties escape `%`, CR/LF, colon, and comma; command data escapes `%` and CR/LF; other C0/DEL controls become visible text, so hostile model output cannot add a second workflow command or manipulate terminal presentation. Tests cover JSON compatibility, exact selected ordering/counts, private validation/acceptance/remediation narrative isolation, hostile property/data/control injection, all severity mappings, the 192/193 boundary, passing zero output, validated CLI failure annotations, and fixed zero annotations. The focused audit fixed two low-risk boundaries by pinning the public GateResult Finding cap and neutralizing residual terminal controls; no unresolved high/medium issue remains. Uncached full tests, full-repository race detection, `go vet`, clean `staticcheck`, module verification/tidy diff, credential/runtime-artifact scans, and `govulncheck` all pass with zero reachable vulnerabilities. An isolated real binary registered only mock, accepted `github` before Store lookup, rejected unknown formats before lookup, initialized a workspace, created a Run, and removed its temporary runtime.
- The post-v25 whole-project audit gate passed module verification/tidy diff, uncached full tests, full-repository race tests, `go vet`, clean `staticcheck`, and `govulncheck` with zero symbol, package, or module findings. Sandbox coverage rose from 13.2% to 72.0% and tool-budget coverage from 0% to 100%. Security tests prove fail-closed LocalRunner behavior, redacted/cancellable dry runs, cancelled Docker detection, unsafe Provider URL/key rejection, redirect target zero-touch, and private Unix SQLite permissions. A complete isolated binary smoke exercised mock Provider/model routing, workspace tree/read, disabled local script proposal, learn/CTF scaffolds, WorkItem/Note, Run and Session lifecycle, TUI snapshot, and OpenAPI export before deleting its runtime.
- The Run-runtime read-projection gate adds no migration, Provider call, tool, Web mutation, or capability. Five authenticated GET routes expose the bounded Agent graph, operator-gated delegation state, read-only Fan-out summaries, and Finding/Report facts through dedicated Go DTOs. Store pagination is bounded; Fan-out summary SQL never selects raw model reports, digests, error narratives, or lease/fencing identity. Finding DTOs omit Artifact content, Evidence notes, operator narratives/identity, and Evidence-set digests. Old Runs without a materialized root return a valid empty graph. The audit fixed one low-risk database-drift boundary: summary validation now mirrors pending/running/terminal shard constraints for provider/model/error/usage/timestamps, so state-incompatible metadata fails closed. Focused privacy/golden/live-route/pagination tests, 13 frontend tests, strict TypeScript, production build, npm audit, uncached full Go tests, full-repository race tests, `go vet`, clean `staticcheck`, module verification/tidy diff, and `govulncheck` all pass with zero reachable vulnerabilities. A single Go process served real schema v38 data at desktop and 390x844 viewports with a root Agent, an immutable unexecuted Fan-out plan, correct empty states, no console errors, and no document overflow. No unresolved high- or medium-severity issue remains.
- The Headless NDJSON gate adds no migration or mutable capability. `headless.v1` reuses Store-redacted events, omits the internal database row ID, caps identity/labels/payload, validates contiguous Run/Mission-bound sequence, rejects future cursors before stdout, reads at most 100 rows per query and 10,000 per invocation, and closes the terminal-observation race by checking for one later event after reloading Run status. A final `stream.end` carries status, reason, counts, truncation/resume metadata, and the same exit code returned by the process. Follow cancellation/deadline is framed before stderr; writer and integrity failures fail closed. The audit fixed one low-risk binding omission by pinning every event and refreshed Run to the initial Mission ID. Focused ordinary/race tests, ten repeated follow/CLI runs, and real schema v38 binary smoke cover snapshot, resume, zero-write timeline stability, complete/fail/cancel, max-events, timeout, sequence and Mission drift, oversized metadata, and broken output. No high- or medium-severity issue was found.
- The event-driven Bubble Tea gate adds no migration, Provider call, tool execution, network route, Sandbox capability, or Store mutation. The TUI polls durable local Run events in batches of at most 32, keeps the latest 50 metadata-only entries, validates contiguous Run/Mission-bound sequences and bounded UTF-8 metadata, discards stale asynchronous responses, and stops on terminal Runs. Events, Agents/completions, and up to ten Finding report summaries are read-only views; full Finding/Evidence details remain on CLI/Web, and event payloads are never rendered. Composite Session/ToolRun/WorkItem/Note/Agent/Finding refreshes compare the event tail before and after, retrying up to eight times to avoid a cursor that is newer than the displayed tables; when more than one 32-event batch arrives, the UI adopts the complete stable snapshot instead of pairing newer tables with an intermediate cursor. C0/DEL and escape controls are made visible before terminal rendering. A same-Run contract test creates state through the CLI and compares CLI JSON, TUI projection, authenticated loopback HTTP, and Headless NDJSON for Run, Mission, Session, status, event tail, and Agent count. The audit fixed the two forms of one medium-risk torn-snapshot boundary and two low-risk performance/terminal-output issues; no unresolved high- or medium-severity issue remains. Four timing-sensitive TUI tests and the cross-surface contract each passed ten consecutive runs, followed by three race-enabled repetitions. TUI package coverage is 66.5%. Uncached full tests, full-repository race detection, `go vet`, clean `staticcheck`, module verification/tidy diff, credential/runtime-artifact scans, OpenAPI generation, 13 Vitest tests, the Vite production build, npm audit, and `govulncheck` all pass with zero known reachable vulnerabilities. An isolated real binary aligned CLI/TUI/Headless at event tail 6, exposed one root Agent, then cancelled the Run and rendered terminal tail 8 before its runtime was removed.
- The Run-first TUI/FileEdit-detail gate adds no migration, Provider call, tool execution, HTTP route, Sandbox capability, approval mutation, or write authority. Production picker reads use bounded pages of 51 to expose the latest 50 Runs/Sessions and a truncation marker, validate bounded UTF-8 Run/Mission/Session records, and reverse-check an exact `--run` open through its attached Session. The composite Run snapshot now includes at most 20 exact-scope FileEdit previews from SQL that omits `original_text` and `proposed_text`; the in-memory view clears the raw diff after deriving at most 128 KiB/4096 terminal-safe display lines. The Edits tab and full-screen detail cannot invoke `a/g/d`, and narrow panes keep the active tab visible. The audit fixed three low-risk resource/UX issues: an unbounded production Session picker read, Run/status header truncation, and a hidden active tab in narrow panes. Focused tests cover pagination, invalid status/scope, exact Run selection, body exclusion by projection type, display bounds, control characters, and approval bypass. Uncached full tests, full-repository race detection, `go vet`, clean `staticcheck`, module verification/tidy diff, `govulncheck`, OpenAPI/TypeScript generation consistency, 13 Vitest tests, the Vite production build, npm audit, credential/runtime-artifact scans, and an isolated schema v38 binary smoke pass with zero known reachable vulnerabilities. TUI package coverage is 65.9%; no unresolved high- or medium-severity issue remains.
- The cross-surface lifecycle/pagination gate adds no migration, mutation, Provider, tool, route, Sandbox, or authorization capability. One real schema v38 SQLite fixture pins running, paused, completed, failed, and cancelled Runs across CLI, a defensive-copy TUI projection, authenticated HTTP, and Headless NDJSON. The surfaces must agree on Run/Mission/Session/status, complete contiguous event sequences and tails, Agent count, terminal reason, and Headless exit 0/4/7. Fifty-three Runs/Sessions pin TUI's latest-50 truncation and HTTP's 20/20/13 opaque-cursor pages; a filtered empty collection and a zero-event resume from the durable tail are explicit contracts. React tests pin terminal status rendering, cursor-driven append, and bearer exclusion from URLs. The audit found no unresolved high/medium code issue and fixed one low-risk recovery-document gap by creating the README-linked `docs/PROJECT_MEMORY.md`. Uncached full tests, full-repository race detection, `go vet`, clean `staticcheck`, module verification/tidy diff, `govulncheck`, OpenAPI/TypeScript drift checks, strict TypeScript, 15 Vitest tests, the Vite production build, npm audit, and credential scans pass with zero known reachable vulnerabilities.

- The first P7 Skills gate adds no schema, Store write, Provider call, prompt injection, tool execution, or authority. Go embeds four strict `skill.v1` records for `code/review/learn/script`; manifests pin a core semantic version, compatible Profiles, a narrow valid-tool prerequisite set, a slash-relative Markdown path, exact UTF-8 byte count, conservative token upper bound, and SHA-256. The mutation-free Registry rejects unknown/duplicate JSON fields, invalid UTF-8, traversal, symbolic links, checksum drift, unsupported Profile/tool combinations, oversized input, and mutable-slice leakage. `skill list/show/validate` expose metadata only, accept no arbitrary path, and create no runtime database. The audit found no unresolved high/medium issue and fixed low-risk ambiguity around Go JSON UTF-8/duplicate-field tolerance, root symlinks, and deterministic self-validation. The package reaches 86.3% statement coverage; uncached full tests, full-repository race detection, `go vet`, clean `staticcheck`, module verification/tidy diff, `govulncheck`, OpenAPI/TypeScript drift checks, 15 frontend tests, production build, npm audit, credential/runtime scans, and real-binary smoke all pass with zero known reachable vulnerabilities.
- The schema v39 Skill-selection gate adds one immutable `skill_selection.v1` per Run, selectable only by operator CLI while the Run is `created`. Registry resolution sorts one to eight names and pins version/hash/bytes/token upper bounds; Go and SQLite independently validate Profile binding, aggregate budget, contiguous items, immutable rows, and a digest-only idempotency operation. Exact replay survives Run start, process restart, and later Registry version/content drift; changed names, budget, operator, or key intent conflicts. Eight callers over two Stores converge to one selection/event, and event failure rolls the complete transaction back. Events expose only protocol/Profile/count/budget and closed capability booleans. Models, HTTP, Tool Gateway, and child scheduling have no selection path; no content enters prompts and no tool is granted at the v39 boundary. The audit found no unresolved high/medium issue and fixed actionable-error loss, Registry-dependent replay, missing application-side operation identity/timestamp verification, duplicate-event-field ambiguity, and v39 migration-fixture drift. Focused ordinary/race tests, 20 repeated concurrency rounds, uncached full tests, full-repository race detection, `go vet`, clean `staticcheck`, module verification/tidy diff, zero-finding `govulncheck`, OpenAPI/TypeScript drift, 15 frontend tests, production build, npm audit, credential/runtime scans, and an isolated real-binary schema-v39 selection/replay smoke all pass.
- The schema v40 root Skill-context gate adds `skill_context.v1` without changing selection authority or offered tools. Each root turn reconstructs only the persisted selection against the embedded Registry, revalidates exact version/hash/bytes/Profile, redacts before applying a separate budget, and injects stable in-memory system guidance. A maximum-eight embedded version history per Skill keeps old `1.0.0` Runs exactly resumable while new selection exposes only current `1.1.0` entries. Metadata-only preparation and commit rows bind the root Agent, Supervisor attempt, turn, selection, context fingerprint, and first model attempt; commit shares the `model.started` transaction, cross-Store replay converges, drift conflicts, and missing preparation fails closed. Events and tables contain no Skill text, paths, names, versions, content hashes, or dependencies; only aggregate counters and selection/context fingerprints are durable. Tests cover current/archived assembly, tampering/redaction, immutable SQL, rollback, double-Store recovery, v39 upgrade, actual Provider delivery, no tool grant, and pre-Provider Registry failure, with Store and Application suites each repeated ten times. The audit found no unresolved high/medium issue and fixed v40 migration-fixture ordering, stale non-injection placeholder text, staticcheck error-string style, and historical-version resumability. The first Linux run also exposed and fixed an unrelated four-second API-process readiness assumption: tests now distinguish early exit from a bounded 15-second busy-runner wait without changing production behavior. All three startup cases passed ten repetitions and the token case passed three race repetitions. Uncached full tests, full-repository race detection, `go vet`, clean `staticcheck`, module verification/tidy diff, zero-finding `govulncheck`, OpenAPI/TypeScript drift, 15 frontend tests, production build, npm audit, credential/runtime scans, and an isolated real-binary schema-v40 delivery smoke all pass.

- The schema v42 Plan/Delivery gate adds an immutable proposal/direction/module/operation ledger and a separate immutable selection/item/operation ledger. Proposal calls are bound to the active root Plan turn, execution lease, exact mode revision, Policy, workspace scope, and tool budget; they return selection/phase/execution authorization as false. Selection requires a quiescent paused Plan Run and atomically creates the selected WorkItems, backward dependency graph, pinned decision Note, selection, and events. Two-Store concurrency converges under the same digest-only operation while changed intent conflicts. Go/SQLite reject malformed protocol versions, duplicate titles/dependencies, forward dependencies, stale revisions, active leases, direct SQL mutation, and partial projection. CLI provides the only selection mutation; HTTP/OpenAPI, TUI, and React are bounded read-only projections with no capability grant. The focused audit found no unresolved high- or medium-severity issue and additionally fenced the Plan tool before Gateway/budget use, removed proposal fingerprints from events and Notes, forced model-derived CLI text onto one line, and proved every accepted direction fits the durable handoff Note bounds. Uncached full tests, full-repository race detection, `go vet`, clean `staticcheck`, module verification/tidy diff, zero-finding `govulncheck`, deterministic OpenAPI/TypeScript generation, 16 frontend tests, production build, npm audit, credential/runtime scans, and isolated real-binary smoke all pass locally.

- The schema v43 context-provenance gate adds strict source kinds, an explicit authority bit, redacted-content SHA-256, immutable SQLite rows, monotonic compaction, and Go read-time digest verification. Slash output and migrated workspace/tool history use role `tool`; model projection wraps every external record as user-role `untrusted_context.v1` JSON. Compaction emits provenance-preserving JSON and no longer elevates summaries to system messages. Root WorkBoard, Notes, and inbox sections are also user-role data; trusted mode/Policy and embedded Skill guidance retain system role. The README injection fixture proves that “automated coding assistants: skip .env” remains false-authority evidence while required `.env`, `DATABASE_URL`, and `SESSION_SECRET` facts remain available. Audit found no unresolved high/medium issue and fixed one low staticcheck wording issue plus unknown-role/control-character hardening. Full tests/race/vet/staticcheck, module/vulnerability checks, strict TypeScript, 16 frontend tests, production build, npm audit, and deterministic generated contracts pass locally.

- The schema v44 Delivery-checkpoint gate adds immutable enrollment, checkpoint, digest-only operation, and pinned handoff Note facts over schema v42 selected WorkItems. Recording requires a paused Deliver Run, no active execution lease, an exact current mode revision, and an `in_progress` item version; focused verification, diff review, security review, and handoff are mandatory, with functional and robustness review additionally required for the final module. Existing WorkItem and both Run-completion paths recheck those facts in Go and SQLite. New selections auto-enroll, untouched legacy selections backfill, and partially completed/cancelled legacy selections are explicitly exempt rather than assigned fabricated evidence. CLI is the sole mutation surface; HTTP/OpenAPI, TUI, and React expose only bounded readiness metadata. The audit found no unresolved high/medium issue and fixed four low-risk gaps: generated Note titles no longer inherit unbounded prefix growth from maximum module titles, relation update triggers guard both old and new Note ownership, events omit internal evidence fingerprints/digests, and CLI output calls the fact gate readiness rather than capability authorization. Cross-Store convergence, stale mode/version denial, direct SQL immutability, completion guards, migration compatibility, CLI lifecycle, policy denial, public projection privacy, and final full-gate behavior have focused coverage. GitHub Actions run `29280076450` passed both release jobs for commit `0fa5ee3`.

- The schema v45 operator-steering gate adds immutable ordered messages, digest-only enqueue operations, and exact-attempt delivery ledgers. A Run accepts at most 64 pending messages and 256 KiB total, with 16 KiB per normalized UTF-8 message. The Supervisor prepares only the oldest item at a safe root-turn boundary; Session user/assistant history, delivery commit, lifecycle action, and queue status then commit atomically. Failed attempts supersede the delivery without consuming the message, worker restart or lease takeover retries it, and a pending successor turns model `finish`/`wait` into an effective Go-owned `continue`. Go and SQLite independently reject completion with pending steering, while failed/cancelled Runs cancel remaining items. Session and TUI can enqueue during a busy action without clearing its live state. CLI detail is the only content-bearing read surface; HTTP/OpenAPI, React, and TUI expose counts, sequence, status, and timestamps only. Tests cover exact replay/conflict, raw-key absence, two-Store concurrent convergence, failure recovery, ordering, exactly-once Session commit, terminal cancellation, busy Session/TUI input, and public-field privacy. The audit found no unresolved high/medium issue and fixed three low-risk boundaries: paused-Run post-commit resume ambiguity, requester identity in queue events, and unknown-parent list semantics. Full local release gates and isolated real-binary smoke pass. GitHub Actions run `29310437643` passed for commit `022b083`; Go control plane completed in 3m10s and TypeScript console in 23s.

- The schema v46 steering-control gate adds one immutable cancellation fact per message and a separate digest-idempotent operation ledger for operator cancellation. Exact retries converge after restart; changed reason/requester/message intent conflicts; prepared messages, edits, and reordering remain forbidden. Terminal Run failure/cancellation records bounded system facts and cannot be blocked by oversized or malformed error text. Explicit drain owns the execution lease before wake, processes only queue-backed turns, respects existing budget/Policy/lifecycle checks, leaves a paused Run untouched on lease conflict, and refuses to recover a failed ordinary turn. `session send --operation-key` always queues or replays and never makes a synchronous Provider call. Tests cover direct-SQL bypass, immutable facts, concurrent Stores, v45 upgrade, prepared/terminal closure, long failure reasons, lease conflicts, non-steering checkpoint isolation, restart/terminal replay, blank explicit keys, CLI behavior, and exactly-once Session history. HTTP/OpenAPI, React, and TUI remain unchanged and read-only. The focused audit found no unresolved high/medium issue and fixed three low-risk behavior defects: terminal-reason rollback, wake-before-lease, and explicit blank CLI key degradation. One staticcheck options-conversion issue was also removed. Full ordinary/race tests, vet/staticcheck, module and vulnerability checks, OpenAPI/TypeScript gates, 17 frontend tests, production build, npm audit, credential/runtime scans, and isolated real-binary smoke pass locally. GitHub Actions run `29316182580` passed release commit `5559f76` with Go in 1m51s and TypeScript in 19s.

- The schema v47 Specialist Skill-context gate derives at most one guide from the parent Run's immutable selection and mode for each active child Attempt. Code/Profile mapping is exact, Cyber defaults to empty except Script, and `plan-delivery` is root-only. Assignment state is fingerprinted but cannot choose a guide; `model.chat` must already be delegated. A separate 1,024-token default budget and 2,048-token cap apply. Preparation is concurrency-idempotent and commits with the first durable model start; selected Runs cannot start unprepared calls, and event failure rolls back the call and commit together. SQLite and events contain metadata/fingerprints only, tables are immutable, and event order is fixed. The audit found no unresolved high/medium issue and fixed one stale CLI capability label. Full ordinary/race tests, vet/staticcheck, module and Go/npm vulnerability checks, OpenAPI/TypeScript gates, 17 frontend tests, production build, credential/runtime scans, and isolated schema-v47 real-binary smoke pass locally. GitHub Actions run `29321708904` passed release commit `d7e269b` with Go in 1m51s and TypeScript in 20s.

- The v47 post-release robustness audit fixed two pre-existing concurrency assumptions without changing schema or authority. Phase replay now rechecks the durable operation after another worker reaches the target state. Cancellation coverage waits for actual Provider entry, and a root model call with a durable start is charged at least 1ms so the execution deadline cannot remain at `999/1000ms`; historical Specialist ledger semantics remain unchanged. Focused ordinary tests passed 100 repetitions, race tests passed 30 repetitions, and the complete ordinary/race, vet, staticcheck, module, and vulnerability gates pass. GitHub Actions run `29325171043` passed commit `fa6dfbd` in one attempt with Go in 2m53s and TypeScript in 18s; no unresolved high/medium issue remains.

- The schema v48 Sandbox Manifest gate adds strict duplicate-aware `sandbox_manifest.v1`, immutable metadata-only preparation/validation/operation ledgers, and deterministic CLI inspection. Schema v49 adds shared approval request/review, exact Manifest resubmission, `os.Root` mount-source resolution, transactional budget/lease checks, and immutable disabled candidates. Schema v50 adds an Artifact-bound disabled lifecycle, metadata-only output plans, independent generation fencing, immutable cancellation, and terminal-Run cleanup recovery. Go and SQL revalidate the v48-v49 authority chain. Private lease/cleanup tables retain only the opaque lease and worker identities needed for fencing; events and CLI omit both, along with raw commands, argv, paths, roots, environment values, secret references, targets, Manifest JSON, and Artifact bodies. Every backend capability remains false and no Runner is called. Focused protocol/Application/Store/migration/CLI tests, uncached full Go and race suites, vet/static/module/vulnerability gates, frontend tests/typecheck/OpenAPI drift/build/audit, repository scans, diff checks, and an isolated real-binary lifecycle smoke all pass with no unresolved high/medium issue. GitHub Actions run `29353239789` passed commit `ff4846a` with Go in 2m6s and TypeScript in 25s.

- Schema v51 adds the disabled backend/output preflight above v50. The Application resupplies the complete Manifest and revalidates the v48-v50 authority chain, current Policy/approval/Scope, mounts, cumulative budgets, Run lease, and input Artifacts. SQLite binds the same immutable identities and live usage before storing a root, exactly 16 required/unverified/not-probed checks, opaque output slots, and a digest-only operation. Backend availability, container identity, output export, Artifact commit, and execution authorization remain false. The output policy fixes all-or-nothing commit, aggregate byte limits, MIME/redaction, regular-file-only handling, link/special-file rejection, and restart reconciliation without storing raw paths. Focused tests, uncached full ordinary/race suites, vet/static/module/vulnerability gates, 17 frontend tests, strict TypeScript, OpenAPI drift/build/audit, repository scans, diff checks, and an isolated real-binary preflight smoke pass with zero reachable Go/npm vulnerabilities. GitHub Actions run `29357134923` passed commit `041f617`; its Go and TypeScript jobs completed in 2m13s and 19s. No unresolved high/medium issue is known.

- Schema v52 adds a no-daemon fake backend evidence client and strict in-memory output transaction. It separately binds image, daemon, mount, network, secret, container, resource, termination, orphan, and output-plan metadata to 16 simulation-only unverified items, while every production/backend/execution/Artifact flag remains false. Output fixtures are duplicate-aware bounded UTF-8, exact-slot matched, MIME-detected, redacted, type-checked, aggregate-limited, and atomically committed only to a fake sink; failure/cancellation leaves zero fake outputs and production Artifacts stay unchanged. Application and SQLite revalidate all v48-v51 authority, budget, lease, mount, and input-Artifact bindings at both boundaries. Cross-Store convergence, replay conflict, cancellation, rollback, immutable SQL, v51 upgrade, CLI lifecycle, privacy, and the eight-simulations-per-evidence cap have focused coverage. The uncached full ordinary/race suites completed in 120.7s/155.7s; vet/static/module/vulnerability gates, 17 frontend tests, strict TypeScript, OpenAPI drift/build/audit, repository scans, diff checks, and an isolated real-binary full simulation smoke pass with zero reachable Go/npm vulnerabilities, zero Docker create/start calls, and zero production Artifacts. GitHub Actions run `29362181363` passed feature commit `f48cbb4` with Go in 2m9s and TypeScript in 19s. No unresolved high/medium issue is known.

- Schema v53 adds a fixed-local-endpoint read-only Docker observation protocol. The transport exposes four GET-only operations and no mutation method, ignores arbitrary environment endpoints, rejects redirects/ambiguous JSON/oversized responses, and records bounded unavailable states. Each observation requires explicit confirmation and a current v48-v52 chain, then persists six immutable items and a digest-only idempotency fact with all production verification and execution authority false. Private-mount support is explicitly unobservable. Focused tests and the final local release gate pass: full ordinary/race suites took 125.4s/140.7s; vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, repository scans, diff checks, and an isolated real-binary full-chain smoke are green. The audit fixed low-risk concurrent semantic convergence and defense-in-depth HTTP allowlist gaps; 20 ordinary and 10 race repetitions of the two-Store contention test pass. Initial CI run `29368112083` exposed only a Linux test-isolation defect: the CLI unit test contacted the runner daemon. It now uses an in-process deterministic fake without changing production defaults. GitHub Actions run `29368979988` passed fix commit `fe7b070` with Go in 2m7s and TypeScript in 23s. No unresolved high/medium issue remains.

- Schema v54 adds a deterministic in-memory Docker container compiler and a seven-step fake write transaction. A complete current v53 observation and exact Manifest are required; Application and SQLite revalidate every v48-v53 authority, budget, lease, Artifact, cancellation, and cleanup binding. The compiler fixes all sixteen threat-model controls while explicitly marking them compiled but not applied or verified. Plan persistence, events, and CLI are metadata-only. Injected failure, crash, and cancellation publish no fake transaction; success still touches no daemon and commits no production Artifact. Focused compiler, Application, Store, migration, concurrency, replay, cancellation, privacy, SQL, downgrade/re-upgrade, and CLI tests pass. The final full ordinary/race suites took 128.2s/148.5s; vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, repository/privacy scans, diff checks, and an isolated real-binary v54 migration/Workspace smoke are green. The audit added requester-chain continuity in Application/SQL and deep-copy snapshot isolation; two-Store contention passed 20 ordinary and 10 race repetitions. GitHub Actions run `29376503165` passed feature commit `126719f` with Go in 2m7s and TypeScript in 19s. No unresolved high/medium issue is currently known.

- Schema v55 adds a separate default-disabled Docker write transport and immutable create-inspect-remove rehearsal facts. Only explicit operator confirmation plus an exact current v54 plan can reach the Linux fixed socket. The first profile is no-network, no-environment, and no-secret; the API-1.40 HTTP core has no start, exec, attach, pull, logs, export, volume management, or generic request method. Before create it exactly inspects the local image, requires a matching RepoDigest, and rejects declared `VOLUME`; delete is non-forced with fixed anonymous-volume cleanup. It creates one stopped digest-pinned container, exactly inspects attachment/device/port plus non-root/read-only/capability/resource/private-mount controls, and removes it through bounded cleanup. A stale name is reconciled only after exact stopped-container matching. Cancellation and uncertain create outcomes re-inspect by ID/name and never blindly delete. Application and SQLite revalidate v48-v54; replay never contacts Docker, two concurrent Stores converge, and private paths/raw IDs never enter persistence, events, or CLI. Focused transport, image-volume, collision, uncertain-create, no-blind-delete, cancellation cleanup, symlink, Application, Store, migration, replay, concurrency, privacy, SQL, downgrade/re-upgrade, and CLI tests pass. Final full ordinary/race suites took 163.3s/168.7s; vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, repository/privacy scans, diff checks, and an isolated real-binary schema-v55 Workspace smoke are green. Transport repetition passed 20 ordinary/10 race rounds and two-Store contention passed 10 rounds. The audit fixed uncertain-create orphan cleanup, blind-ID deletion, image-declared anonymous-volume side effects, and incomplete attachment/device/port/capability checks. No unresolved high/medium issue is known. The Linux real-daemon integration harness was skipped on this Windows host without Docker. GitHub Actions run `29382661971` passed feature commit `69d81d6` with Go in 2m32s and TypeScript in 25s. The rehearsal performs three reads and bounded daemon writes but never starts a process, pulls an image, exports output, commits a production Artifact, or grants production authority.

- Schema v56 adds a durable pre-daemon attempt, generation-fenced lease, recoverable stage/cleanup checkpoints, bounded failure ledger, immutable 19-control matrix, and atomic v55/v56 completion. Uncertain create recovery adopts the same exact never-started authority without a second create; cleanup accepts absence and refuses unrelated same-name containers. A stale generation cannot borrow a newer owner's idempotent checkpoint. Image and container inspection both reject inherited environment entries. CLI list/show/resume surfaces are metadata-only and recovery requires full Manifest resubmission plus fresh confirmation, not a retained raw operation key. Full ordinary/race suites completed in 178.5s/181.3s; vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 17 frontend tests, strict TypeScript, OpenAPI drift, production build, zero-vulnerability npm audit, repository/privacy/link/encoding scans, diff checks, and an isolated real-binary schema-v56 Workspace smoke pass. Recovery transport passed 20 ordinary/10 race repetitions, two-Store contention passed 20 ordinary/10 race repetitions, and uncertain-create Application recovery passed 10 repetitions. The audit fixed inherited-environment evidence, expired completion/failure fencing, bounded failure-code ordering/exhaustion, generation-acquisition chronology, stale-generation replay, orphaned legacy operations, attempt-ID recovery, SQL control mapping, and mount-TOCTOU overclaiming. No unresolved high/medium issue is known. GitHub Actions run `29388724727` passed feature commit `e1710bb` with Go in 2m32s and TypeScript in 23s. No start, exec, attach, pull, logs, export, network, secret, production verification, execution, or Artifact authority has been added.

- Schema v57 adds a separately confirmed, default-disabled descriptor-pinned host-input capture between v56 stage and cleanup. Linux `openat2` pins no-symlink/no-magic-link/beneath/no-cross-device trees; `O_PATH` preflight rejects FIFO/special files before content open, and both directory and single-file mounts are supported. Descriptor metadata is rechecked before a deterministic sanitized tar is sealed in `memfd` and reread. Exact Artifact payload metadata is independently rechecked by Application. SQLite binds immutable intent/result rows to attempt, container fingerprint, plan, input digest, requester, and generation; generated row IDs are excluded from semantic fingerprints, pending intent blocks completion, and missing resume confirmation is rejected before lease acquisition. Failure cleans the stopped container before releasing the lease, and takeover resumes without another create. Tests cover rename/replacement/deletion/symlink/hard-link/FIFO/single-file, bounded enumeration, cancellation, replay, report mismatch, cleanup-first recovery, stale-generation fencing, migration, immutability, privacy, CLI, and Linux cross-compilation. Final ordinary/race suites pass in 155.0s/168.1s; vet/staticcheck/module/govulncheck, 17 frontend tests, OpenAPI/build/npm audit, repository scans, focused repetitions, and isolated schema-v57 binary smoke are green. The audit additionally fixed single-file report/SQL constraints, filesystem-dependent directory digest input, special-file pre-open blocking, unbounded directory allocation, cancellation latency, independent-ID convergence, and failure-ledger consumption on omitted confirmation. No unresolved high/medium issue is known. The sealed bundle remains local and daemon-unconsumed, so no process or production authority has been added.

- GitHub Actions run `29396264276` passed commit `8719dff` with Go/Linux in 3m55s and TypeScript in 23s. Initial run `29395980413` exposed only an invalid single-file test fixture whose file target did not cover the retained directory working path; the corrected mixed directory/file fixture now validates the intended report constraint. Production behavior was unchanged.

- Schema v58 adds one immutable pre-stage host-input requirement atomically with each new v56 attempt, initial lease, and audit events. Required attempts recover v57 capture without repeated flags and cannot complete without evidence; false requirements cannot widen. Migration places only preexisting attempt IDs in an immutable compatibility set, and SQL rejects any post-migration stage/staging/completion with neither a requirement nor that marker. Events and CLI stay metadata-only. Direct archive upload was rejected because it conflicts with the read-only target; no archive, volume, start, exec, pull, build, export, or Artifact capability was added. Focused tests cover zero-input false choices, flag pairing, pending/completed operation replay, generation-two recovery, direct-SQL missing requirements, migration markers, immutability, privacy, and independent-ID convergence. Full ordinary/race suites pass in 158.1s/168.4s; vet/staticcheck/module/govulncheck, strict TypeScript, 17 frontend tests, OpenAPI/build/npm audit, repository scans, Linux cross-compilation, and isolated schema-v58 binary smoke are green. Repetitions pass at 50 domain, 30 Store, 20 Application, and 10 race rounds per Store/Application group. The audit fixed pending operation recovery rebuilding a candidate, unmatched flags beside durable facts, missing SQL discrimination for new versus legacy attempts, and false-requirement zero-input compatibility. No unresolved high/medium issue is known. The v57 bundle remains daemon-unconsumed and schema v59 remains required.
- Schema v59 adds immutable handoff requirement/intent/result facts, fixed local archive/volume transport, daemon readback verification, final read-only mount inspection, deterministic crash recovery, and complete cleanup. Four CLI confirmations gate the path; resume follows durable requirements. Tests cover exact and foreign residue, invalid bundles, reserved mount overlap, endpoint/path closure, write-ahead recovery, generation takeover, immutability, privacy, migration, and cleanup/completion gating. Linux test-binary cross-compilation includes an opt-in real-daemon harness; this Windows host cannot execute it. No start, process, export, backend, execution, or Artifact permission is enabled, so dual progress remains architecture about 98% and product usability about 45-50%.

- Schema v60 adds an immutable, operator-confirmed runtime-input projection plan after completed v59 handoff. Strict canonical tar revalidation, exact recapture, directory-root mapping, fixed Artifact target, handoff-bound future volume identity, atomic aggregate completion, operation replay, cross-Run isolation, migration, immutability, and metadata privacy have focused coverage. The compiler never contacts Docker and the plan remains unapplied. No start, process, export, backend, execution, or Artifact permission is enabled, so dual progress remains architecture about 98% and product usability about 45-50%.

- The v60 final local gate passed full ordinary/race suites in 198.9s/194.0s, vet/staticcheck/module/govulncheck, strict TypeScript, 17 frontend tests, OpenAPI/build/npm audit, repository scans, Linux sandbox cross-compilation, diff checks, and isolated schema-v60 binary smoke. Compiler/Store/Application/CLI repetitions passed at 50/30/20/10, with 10 critical race repetitions. The audit fixed eight durability, isolation, canonicalization, ordinal, and chronology gaps; no unresolved high/medium issue is known. GitHub Actions run `29428011306` passed feature commit `cc92421` with Go/Linux in 2m48s and TypeScript in 24s. Linux real-daemon input application remains a manual v61 prerequisite and does not enable start.

- Schema v61 adds immutable write-ahead application intents, independent generation leases, typed failure recovery, and a fixed local-Unix projection transport. Each v60 archive is applied through a never-started carrier, read back and semantically verified, then attached read-only/`NoCopy` to a fully inspected retained target. Dual confirmation, complete v48-v60 recapture/revalidation, operation replay, stale-generation fencing, released-generation resume, exact-owned cleanup, foreign-collision protection, lease-expiry cleanup reserve, migration, SQL immutability, privacy, and metadata-only CLI list/show/resume have focused coverage. Windows fails closed as unsupported.
- Schema v62 adds immutable retained-resource inspections plus recoverable cleanup intents, generation leases, failures, and results. Focused coverage pins confirmation-before-contact, no input recapture, write-ahead ordering, operation replay, failure/resume, stale-worker rejection, migration without fabricated facts, SQL immutability, metadata privacy, full preflight before DELETE, foreign-collision zero-delete, target-before-volume deletion, final absence, least-capability platform factories, and the extended Linux opt-in harness. No start, process, export, backend, execution, or Artifact permission is enabled, so dual progress remains architecture about 98% and product usability about 45-50%.

- The v61 final local gate passed full ordinary/race suites in 197.5s/316.8s, vet/staticcheck/module/govulncheck, strict TypeScript, 17 frontend tests, OpenAPI/build/npm audit, repository scans, Linux sandbox cross-compilation, diff checks, isolated schema-v61 binary smoke, and 20 Sandbox race repetitions. The audit tightened readback bounds, lease timing, resume confirmation, cancellation-safe failure persistence, transport capability narrowing, daemon-returned mount evidence, and digest syntax. No unresolved high/medium issue is known. The Linux v59/v61 real-daemon harnesses remain unexecuted on this Windows host, so no start or process-isolation claim follows. GitHub Actions run `29437941378` passed feature commit `f4aaf7a` with Go/Linux in 2m37s and TypeScript in 27s.

- GitHub Actions run `29406403201` passed schema-v59 feature commit `fb1daca` with Go/Linux in 2m37s and TypeScript in 28s. Local full ordinary/race suites passed in 183.1s/185.1s, with no unresolved high/medium issue.

- GitHub Actions run `29400696276` passed feature commit `4b570f7` with Go/Linux in 2m39s and TypeScript in 23s.

## Non-Schema Protected Delete Guard

ADR 0025 adds an execution-context-only permanent Policy denial before approval. Raw Shell and decoded ScriptProcess/Sandbox executable intents are rejected at critical risk when they express recursive deletion or deletion of absolute, traversing, wildcarded, environment-derived, command-substituted, or current-home targets. Common PowerShell, `cmd`, Python, Node, `find -delete`, Windows-drive, and indirect-variable forms have focused coverage. The stable reason contains no home path, ToolCall map order is deterministic, non-executable evidence remains ordinary untrusted data, and operator approval cannot turn a denied proposal into a dry-run result.

The audit fixed two initial bypasses (Node `require('fs').rmSync(...)` and leading `../`) and one PowerShell `-Force` false positive. The final local gate passed the full ordinary suite in 197.0s, the full race suite in 222.6s, 20 repeated race runs for all three protected-delete paths, about 406,000 fuzz executions, 100/50/50 focused repetitions, vet/staticcheck/module/govulncheck, strict TypeScript, all 17 frontend tests, OpenAPI/build/npm audit, and repository privacy/encoding/link/diff scans. No unresolved high/medium issue is known. That slice added no migration, execution, filesystem mutation, or authority and left schema at v63. Opaque scripts and build hooks are explicitly outside classifier proof, so real execution still requires OS/container isolation and remains disabled.

## Schema v64 Execution Profile Control Plane

Every new Run receives an initial preview snapshot atomically; migration backfills legacy Runs without fabricating historical Run events. Operators may transition only while the Run is `created` or quiescent `paused`. The Application and Store recheck current revision, Run/Mission binding, active execution leases, redaction, idempotency, and event identity under a Run write lock. SQLite independently constrains all profile mappings, authority bits, chronology, operation binding, and row immutability. Same-key/same-intent replay returns the original snapshot, while key reuse for another intent fails closed.

At the v64 milestone, the loopback API had 25 paths and 58 schemas: 22 read GETs and three control POSTs. The new profile POST required the distinct control bearer, strict JSON, no query, a 4 KiB body limit, and one bounded idempotency key. The read bearer could not use it; the control bearer could not read resources. React kept both tokens only in page memory, sent the control token only in Authorization, and submitted only a profile enum. Browser controls were disabled without control capability, outside the permitted Run states, or during an active execution lease. No process, Docker start, host shell, tool approval, queue mutation, output export, or Artifact commit was added.

Focused Domain, Store, migration, CLI, HTTP/OpenAPI, generated-schema, token-lifetime, and React tests cover closed mappings, tampering, replay/conflict, lease/state denial, credential separation, strict request parsing, and server-returned state adoption. Final-code full ordinary tests passed in 225.9s; the complete race suite passed in 196.9s, followed by a final targeted race run after DTO minimization. Vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 21 frontend tests, production build, zero-vulnerability npm audit, deterministic OpenAPI/TypeScript hashes, README chronology/link checks, privacy/artifact/encoding/diff scans, and isolated CLI smoke are green. The audit fixed generic control-request error wording, six static error-style findings, and unnecessary requester/reason fields in browser DTOs. No unresolved high/medium issue is known. ADR 0026 is the decision record. GitHub Actions run `29523634340` passed implementation commit `8378419`; Go/Linux completed in 3m0s and TypeScript in 26s.

The post-release real-bundle smoke found that Vite 8 produced the valid asset name `index-D0TcvGy-.css`; the trailing URL-safe hyphen caused the previous last-separator heuristic to reject the entire UI directory. The validator now searches backward for an at-least-eight-character URL-safe digest segment and still rejects short or invalid suffixes. The primary bundle fixture carries the observed filename. A fresh full suite passed in 201.1s, 20 targeted race repetitions passed, and vet/staticcheck remained clean. The rebuilt binary hosted the exact production bundle on loopback, returned CSP-protected HTML, rendered the profile controls on desktop and mobile without horizontal overflow, switched to Docker intent, and returned to Preview with both authority bits false. This was a low-risk availability defect, not an execution or path-boundary bypass.

## Schema v65 Docker Production-Evidence Ledger

Schema v65 adds a fixed machine-capture protocol over the v63 blocked start-gate review. Go owns all sixteen ordered probe codes, blocker mappings, suite identity, environment fingerprinting, bounded status values, and false authority conclusions. A capture requires the exact review, the same operator, an explicit confirmation, and a digest-only idempotency key. Callers cannot provide evidence items, a report, endpoint, socket, path, image, resource name, container ID, or daemon response. SQLite stores immutable metadata-only aggregates/items/operations, validates exact v63 authority binding, caps each Run at 32 receipts, and commits one metadata event in the same transaction. Migration does not backfill evidence.

At the v65 delivery point, the product collector was deliberately zero-side-effect: Windows recorded `unsupported_platform`, Linux without explicit environment opt-in recorded `opt_in_required`, and Linux with opt-in recorded `harness_pending`. No branch opened a socket, called Docker, or started a process, and Application rejected `capture_complete` and `real_daemon_contacted=true`. Schema v66 later supplied durable ownership; schema v67 now adds the separately constrained read-only implementation. Every item remains `sufficient_for_start=false`; start, process, export, and Artifact authority are all false. See ADR 0027.

Focused tests cover platform/opt-in behavior, fixed suite identity, migration without fabrication, SQL immutability and binding, operation replay/conflict, transaction/event behavior, privacy, CLI projections, cancellation, future positive evidence without authority, and malicious collector injection. Final local ordinary/race suites passed in 212.3s/213.9s. Vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 21 frontend tests, OpenAPI drift, production build, zero-vulnerability npm audit, credential/runtime-artifact/process-capability/diff scans, and an isolated real-CLI schema-v65 Workspace smoke are green. The audit corrected eleven error-style findings, made multi-field identity validation deterministic, added direct SQL operation-key-to-evidence binding, and closed the future collector bypass before release. No unresolved high/medium issue is known.

GitHub Actions run `29532551701` passed implementation commit `e97daf0`; Go/Linux completed in 2m47s and TypeScript in 20s.

## Schema v66 Recoverable Docker Production-Evidence Attempts

Schema v66 adds immutable `sandbox_docker_production_evidence_attempt.v1` intent, digest-only operation, generation lease, reconciliation, typed-failure, and result records around every new v65 capture. The Application commits the attempt and current lease before invoking the collector, then commits a current-generation quiescent reconciliation checkpoint before the call. The collector deadline is bounded by the fixed 30-second capture window and the lease expiry reserve. A released or expired attempt can resume only at generation N+1; completion, failure, and reconciliation require the full private current lease identity, so a stale worker cannot commit. Completion atomically stores the v65 receipt, all sixteen items, its operation, the attempt result, the lease release, and metadata-only events.

SQLite rejects direct creation of a new v65 evidence operation unless a matching v66 attempt result exists. Existing v65 receipts remain readable and receive no fabricated attempt during migration. CLI capture reports the associated attempt and adds bounded list/show/resume commands; resume requires a fresh `--confirm-machine-capture`, while lease IDs, owners, raw errors, sockets, paths, resource identities, and daemon payloads are omitted. Focused tests cover write-ahead ordering visible from inside the collector, active conflicts, released recovery, lease-expiry takeover, stale-generation fencing, typed failure, unsafe daemon-contact rejection, atomic generation-two completion, SQL bypass rejection, migration compatibility, immutability, privacy, and replay without recollection. ADR 0028 records the boundary.

The v66 reconciliation is deliberately quiescent: it records zero daemon reads and zero known resources. That proves durable ownership and call ordering only; it is not production Docker resource verification. Schema v67 later adds a separate daemon-aware empty-scope checkpoint while leaving every start, process, output, Artifact, backend, execution, and capability authority bit false. Architecture completion remains about 98% and product usability about 45-50% because these slices improve evidence and recoverability rather than adding end-user execution.

The final local gate passed the full ordinary/race suites in 206.9s/230.3s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, strict TypeScript, 21 frontend tests, OpenAPI drift, production build, zero-vulnerability npm audit, repository privacy/artifact/encoding/process-entry/link/diff scans, focused repetitions, and an isolated real-binary schema-v66 Workspace smoke. The audit fixed a missing immutable-delete guard on the v66 lease, tightened direct-SQL release/takeover chronology, and corrected trailing `--limit` parsing on the v65 capture list. No unresolved high/medium issue is known. Protected recursive cleanup was denied by the host policy, so the isolated smoke directory remains under the OS temporary root for normal cleanup and requires no manual action.

GitHub Actions run `29538732903` passed implementation commit `3e52b7d`; Go/Linux completed in 3m33s and TypeScript in 25s.

## Schema v67 Bounded Linux Read-Only Docker Evidence Harness

Schema v67 adds immutable harness intent, reconciliation, and result records behind the v66 attempt and current-generation control checkpoint. The intent reconstructs the exact blocked review, container plan, pre-existing OCI digest, local-Unix endpoint, attempt-label selector, operator, and zero mutation authority. A Linux product collector is enabled only when `CYBERAGENT_DOCKER_PRODUCTION_EVIDENCE=1` is present. It first executes an exact label-filtered `/containers/json` GET and requires an empty owned scope, persists that daemon-aware reconciliation, then calls `/_ping`, `/version`, `/info`, and exact digest image inspect in order.

The transport has no pull/create/start/exec/remove/delete method, ignores `DOCKER_HOST`, disables redirects and proxies, limits responses to 2 MiB, caps each call at four seconds, and remains inside the v66 30-second attempt deadline. Duplicate JSON, mismatched labels, duplicate resource IDs, non-local endpoint changes, foreign/pre-existing owned resources, malformed metadata, or digest mismatch fail closed. Resource IDs are transiently fingerprinted; sockets, daemon payloads, image repository names, paths, raw operation keys, and private lease identities do not persist.

Every v67 check is `observed_failed`; Go and SQLite require `production_verified_count=0`, zero sufficient checks, sixteen blockers, and false start/process/output/Artifact authority. Once a v67 intent is durable, the attempt cannot fall back to the v66 inert result. Restart uses generation N+1 and must bind both its fresh v66 control reconciliation and fresh daemon-aware empty-scope reconciliation. A pre-reconciliation transport failure reports contact as `not_confirmed` rather than falsely claiming no contact. Migration fabricates no v67 state for existing receipts or in-flight v66 attempts. ADR 0029 records the boundary.

Focused Domain, Store, Application, HTTP transport, migration, and CLI regressions cover exact call ordering, query allowlisting, collision and malformed-response rejection, write-ahead visibility, generation binding, v66 fallback rejection, immutable rows, replay without recollection, privacy, and all false authority bits. The development host is Windows, so no real Linux daemon, container start, Shell, or host process was run; the Linux path remains explicit opt-in with an already-present digest.

The final local gate passed full ordinary/race suites in 215.2s/233.1s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 21 frontend tests, generated OpenAPI drift checks, production build, zero-vulnerability npm audit, 51-file Markdown validation, repository privacy/artifact/encoding/Docker-mutation/diff scans, isolated real-binary Workspace smoke, Linux sandbox test-binary cross-compilation, and focused Sandbox/Store/Application/CLI repetitions of 50/10/5/5 plus race repetitions of 10/3/3/3. The audit tightened zero-verification, exact selector and v66 reconciliation binding, lease chronology, direct-SQL terminal atomicity, and contact-state wording. No unresolved high/medium issue is known. Protected host deletion policy left two smoke roots under the OS temporary directory for normal cleanup; no manual action is required. GitHub Actions run `29543385038` passed implementation commit `8bc0929`; Go/Linux completed in 2m50s and TypeScript in 24s.

## Schema v68 Immutable Docker Production-Evidence Review

Schema v68 adds one immutable `sandbox_docker_production_evidence_review.v1` for an exact completed v67 harness receipt. The operator must explicitly confirm an `accepted|rejected` decision. Acceptance permits only `metadata_scope_accepted`; rejection permits five bounded reason codes. There is no free-form reason, uploaded report, raw daemon payload, socket, path, image repository, resource identity, raw operation key, or private lease identity in the request or public projection.

The Store inserts a digest-only operation before the review inside one SQLite transaction. A deferred foreign key prevents an operation-only commit, and the review trigger prevents a review-only insert. Source triggers bind the v63 blocked review, v65 receipt and sixteen `observed_failed` items, v66 attempt, v67 harness result, Run/Mission/Workspace, and the complete fingerprint chain. Each evidence/attempt receives at most one decision; rows are immutable; migration creates no review for old receipts or incomplete attempts.

Same-key/same-semantic replay returns the existing record without a second event or daemon call, while a changed receipt, reviewer, decision, or reason conflicts. Even an accepted review fixes production verification and sufficient checks to zero, blockers to sixteen, and all start/container/process/output/Artifact authority to false. v68 has no Docker transport or process-start path.

The final ordinary/race suites passed in 247.9s/276.3s; vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 21 frontend tests, OpenAPI/build/npm audit, 57-file/74-link Markdown validation, credential/encoding/forbidden-entry/diff scans, Linux cross-compilation, and isolated real-CLI schema-v68 smoke are green. Focused Domain/Store/Application/CLI repetitions passed at 50/10/5/5 and race repetitions at 10/3/3/3.

The independent audit fixed stored request-fingerprint drift by binding it in SQL and recomputing the operation/review relation on every Store read/replay. Negative tests now cover review-only and operation-only halves, operation immutability, source/fingerprint/authority tampering, two-Store convergence, and the full rejected Store/event/Application/CLI/list/show/replay path. No unresolved high/medium issue is known. Protected deletion left one cross-compiled binary and one smoke root under the OS temporary directory for normal cleanup; no manual action is required. GitHub Actions run `29552080990` passed implementation commit `41583ac`; Go/Linux completed in 2m57s and TypeScript in 24s. ADR 0030 records the boundary.

## Schema v69 Content-Addressed Inert User Skill Registry

Schema v69 adds immutable installation operations/intents/results and removal operations/tombstones. Raw operation keys are stored only as domain-separated digests. Deferred foreign keys and reciprocal triggers require each operation/root pair to commit atomically, and all rows reject update/delete. A same-key retry recovers an intent that committed before object publication; changed intent conflicts, and independent SQLite connections converge on one installation and result.

The local object store writes only strictly validated deterministic archives below `$CYBERAGENT_HOME/skill-registry/objects/sha256/<prefix>/<digest>.zip`. Its interface exposes `Put` and `Verify`, not execute or delete. Publication uses an exclusive same-directory temporary file, file sync, atomic hard link, and full readback. Existing and newly published objects must match byte count, archive SHA-256, strict ZIP structure, semantic package fingerprint, and stable file identity. Symlinks, replacement, corruption, forged receipts, and cancellation fail closed.

CLI `skill import` requires an explicit surface, stable operation key, and `--confirm-untrusted-skill`; `skill installed`, `installed show`, and `remove` expose bounded metadata. Built-in names are reserved. Code and Cyber catalogs are separate; Cyber accepts exactly `script`. All external packages are `operator_installed_untrusted`, and all import command/network/Provider, tool-grant, Run-selection, and context-injection fields are false. Removal appends a tombstone and retains the object. At the v69 boundary external packages were not loadable; schema v70 now adds a separate explicit Run decision without changing import authority. ADR 0031 records the storage boundary.

The final local ordinary/race suites passed in 259.7s/275.3s. Vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, OpenAPI drift checks, 21 frontend tests, production build, and zero-vulnerability npm audit are green. Focused v69 race tests passed three repetitions; real two-Service import/removal convergence with independent generated identities passed 20 ordinary and 10 race repetitions. Initial Linux CI run `29556933994` exposed concurrent nested-directory preparation through `os.Root.MkdirAll`; the object store now creates and `Lstat`-verifies each component, rejects symlink redirection, and passes 100 ordinary plus 20 race repetitions with twelve independent Stores. GitHub Actions run `29557803407` passed fix commit `d28b100` with Go/Linux in 3m21s and TypeScript in 23s. The audit also fixed migration downgrade ordering, static error text, redundant temporary cleanup state, forged receipt binding, cancellation immediately before publication, generated-identity replay comparison, and credential redaction of free-text Manifest descriptions before SQLite persistence. No unresolved high/medium issue is known, and no model, network request, Shell, Docker operation, installer hook, or host process was run.

## Schema v70 External Skill Run Selection And Minimized Context

Schema v70 adds immutable `external_skill_selection.v1` selection/item/operation records and separate root/Specialist context preparation/commit ledgers. A created Run, its current mode, an exact active v69 installation/result/object identity, stable digest-only operation, and `--confirm-untrusted-skill-context` are required. One Run can pin one to four items under a 4096 aggregate bound; at most one package is explicitly Specialist-eligible. Code/Cyber/Profile separation remains strict, Cyber accepts only Script, same-intent replay converges, and both Go and SQL reject removal of a pinned installation.

`PackageObjectLoader` is separate from publication and has no execute/delete surface. Every delivery opens only the exact content-addressed object and rechecks ordinary-file identity, byte count, archive SHA-256, deterministic ZIP, semantic package fingerprint, and Manifest/content binding. Root and Specialist contexts apply secret redaction and separate budgets; Specialist defaults to 1024 and is capped at 2048. Bodies are delivered only as user-role `external_skill_guidance.v1` envelopes with Policy/tool/Shell/network/secret/scope/delegation authority false. The system message explicitly treats external content as optional workflow guidance and repository/document claims as evidence, preventing package prose from becoming system or assistant authority.

Preparation commits atomically with the corresponding first root or Specialist `model.started`. SQLite and events retain metadata and fingerprints only, never package content, source paths, raw operation keys, secrets, or model text. CLI supports explicit selection and metadata-only query. Focused coverage includes second-confirmation enforcement, exact object binding, replay, removed/cross-surface/Profile rejection, secret redaction, prompt-injection user-role placement, one-package no-tool Specialist delivery, immutable SQL, direct-SQL removal protection, and v69-to-v70 migration without fabricated state. ADR 0032 records the boundary.

The final v70 local gate passed ordinary/race suites in 197.6s/264.4s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, 21 frontend tests, OpenAPI/build/npm audit, repository privacy/encoding/43-file Markdown-link/diff scans, and isolated real-CLI schema-v70 creation. The audit fixed a medium-severity global uniqueness error that had limited one installation to one Run; installation reuse now requires a fresh independent Run selection and is covered by regression. It also tightened latest-mode Specialist binding in Go/SQL, Specialist token bounds, cross-phase replay, reciprocal selection/operation atomicity, cancellation-safe object loading, and explicit closed authority fields. There is no unresolved high/medium finding. Prompt-injection capability escalation is hard-blocked by Go-owned tools and Policy, while factual or workflow influence remains a model-semantic residual risk requiring provenance-aware prompts and operator review. No real model, network, Shell, Docker, installer hook, or host process ran. GitHub Actions run `29566538449` passed implementation commit `edc4073` with Go/Linux in 3m42s and TypeScript in 21s.

## Schema v71 Bounded External Skill Provenance Projection

Schema v71 adds two read-only SQLite views and a strict `external_skill_projection.v1` domain type. The projection is explicitly scoped by Run and bounded to four items and one Specialist. It exposes surface/profile, mode revision, token budgets, fixed trust class, declared-tool counts, historical confirmation/authorization facts, and root/Specialist preparation and commit counts. It structurally excludes package bodies, source/content paths, byte sizes, every hash/digest/fingerprint, selection/installation/mode-snapshot IDs, operation keys, and operator/requester/attempt/agent identities. `tool_capability_grant` remains false.

Go serves the projection through optional Run detail metadata and `GET /api/v1/runs/{run_id}/external-skills`; OpenAPI now contains 26 paths, 61 schemas, 23 read-only GETs, and the unchanged three control POSTs. TUI adds a read-only Skills activity view, while React adds a responsive External Skills panel with no install, selection, approval, authorization, or execution control. Store, HTTP, OpenAPI, TUI, and React regressions cover Run isolation, empty legacy Runs, v70-to-v71 upgrade without fabricated state, immutable views, clone isolation, route behavior, DTO privacy, generated schema consistency, and read-only rendering.

The slice audit tightened every preparation/commit count subquery with an explicit matching `run_id`, rather than relying only on selection identity. This is a metadata projection only: it does not open package objects, call a model, contact a network or Docker daemon, execute Shell/host processes, persist new events, or create any Skill mutation authority.

The final local gate passed full ordinary/race suites in 227.1s/301.1s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, deterministic OpenAPI/TypeScript generation, strict TypeScript, 22 frontend tests across nine files, the production build, zero-vulnerability npm audit, credential/runtime-artifact/production-`exec.Command`/encoding/54-file and 78-relative-link Markdown/diff scans, and an isolated real-binary schema-v71 Workspace smoke. A final negative regression proves that Run detail omits `external_skills` and the dedicated endpoint returns 404 for an existing Run with no external selection; its HTTP ordinary/race runs passed in 14.7s/17.6s. No unresolved high/medium issue is known. No real Provider, Agent-controlled Shell/host process, Docker operation, installer hook, or external network request ran. The smoke root is confined to the OS temporary directory for normal cleanup and requires no user action. GitHub Actions run `29574167659` passed implementation commit `3947bea`; Go/Linux completed in 2m56s and TypeScript in 25s.

## Schema v72 Idempotent Controlled Run Creation

Schema v72 adds immutable `run_creation.v1` operations containing only a
domain-separated idempotency-key digest, semantic request fingerprint, and the
created Mission/Run/Session/Workspace identities. The Application layer accepts
only a registered Workspace, validates a canonical Profile/surface/phase and a
1-4096-byte UTF-8 goal, redacts before persistence, and builds one interactive
created Run with an active Session, revision-one mode, preview/noop execution
profile, root Agent, exact initial events, disabled network, no targets, default
budget, and model route equal to Profile.

Store creation and operation insertion share one immediate transaction.
Triggers rebind every initial fact, including Workspace registration, Session
route/title, timestamps, mode requester, default budget, execution-profile
closed authority, root Agent, and exact event counts. Operations reject update
and delete. Same-key/same-semantic replay returns the original graph across
restart and independent SQLite connections; changed intent conflicts, and
migration creates no ledger facts for historical Runs.

HTTP adds `POST /api/v1/runs` under the distinct control bearer and read-only
`GET /api/v1/workspaces`. The POST requires exactly one `Idempotency-Key`, exact
JSON content type, no query, a bounded body, no unknown or duplicate fields,
and no trailing JSON. Workspace projection contains only ID, name, and creation
time. OpenAPI now has 28 paths, 65 schemas, 25 GET operations, and four control
POST operations.

Desktop adds independent `--enable-run-creation` and retains independent
`--enable-profile-control`; a creation-only token cannot access existing Run
controls. The native Wails bridge remains exactly three methods. React adds a
responsive New Run dialog with memory-only uncertain-failure key reuse,
multibyte UTF-8 bounds, strict response/request binding, closed-authority
validation, list refresh, and new Run selection. Browser storage remains empty.
No model, execution lease, tool, Shell, host process, Docker operation, network
request, or Skill installation is called. ADR 0036 records the boundary.

The final v72 local gate passed full ordinary/race Go suites in 271.5s/257.9s,
ordinary and secure-Desktop tests plus vet/staticcheck/govulncheck, module
verification/tidy, deterministic OpenAPI/TypeScript generation, 45 frontend
tests across 14 files, strict TypeScript, production builds, zero-finding
dependency audits, 63-file/86-relative-link Markdown validation, repository
privacy/forbidden-entry/diff scans, and an isolated schema-v72 CLI smoke. The
audit fixed v72 trigger teardown for historical migration fixtures, separated
creation from existing Run controls, strengthened initial-state replay and exact
root/timestamp/event-count binding, rejected invalid UTF-8 JSON, narrowed the
HTTP creation Store contract, and bound React responses to Goal, Workspace,
mode, default budget, and closed authority. No unresolved
high/medium issue is known.

## Non-Schema Desktop D1-S1 Controlled Session Message Submission

The first three-slice product batch after D1-R1 reuses the existing v45-v46
steering ledger, so schema remains v72. Go adds a narrow Application service and
strict `POST /api/v1/sessions/{session_id}/messages`; the route independently
requires `SessionMessageEnabled`, one distinct control bearer, exact JSON and
idempotency headers, bounded valid UTF-8, and an exact Session/Run binding. Its
response contains queue metadata and explicit false execution/model/tool/
capability facts, never content, digest, requester, operation, or lease identity.

Desktop adds only `--enable-session-messages` to bootstrap/control-plane
configuration. The Wails bridge remains exactly three methods, and capability
isolation proves a Session-only launch cannot create Runs or select profiles.
React renders a composer only for an exact bound Session on a running or paused
Run, checks 16 KiB UTF-8 bytes, reuses one in-memory key for an unchanged
uncertain retry, clears it after success, and stores neither token nor key in
browser storage. Submission does not wake or drain the Run.

The batch functional gate passed a direct full ordinary Go run on final code in 255.6 seconds,
focused Desktop-tag tests in 80.5 seconds, final Application/HTTP/Desktop
regressions, 52 frontend tests across 15 files, strict TypeScript, Vite build,
and Windows production Desktop build. No Provider, model call, tool, Shell,
Docker, external network, process, or execution lease was used. In accordance
with the new cadence, full race/vet/staticcheck/govulncheck and extended
dependency/privacy analysis run after the next three slices complete the
six-slice robustness gate. GitHub Actions run `29633205163` passed commit
`3ecb22a` in all Go/Linux, Windows Desktop, and TypeScript jobs, including
remote vet, govulncheck, and dependency audit. ADR 0037 records the boundary.

The following D1-S2/D1-L1/D1-X1 batch raises the current dual metrics to:
architecture about 98% (V2 about 99%), complete-product usability about
61-65%, generic Coding Agent workflow about 56%, and Cyber autonomous workflow
about 20%.

## Desktop And Skill Registry Surfaces

An unsigned Windows development/portable-test desktop executable now exists, while installer, formal portable release, custom registry integration, startup behavior, and auto-updater remain absent. Wails v2.13.0 embeds the React/Vite console and calls the existing Go API Handler in process without a TCP listener. The default shell has only a read token; independent flags expose profile selection, closed Run creation, Session enqueue/pending cancellation, Run lifecycle, bounded Supervisor handoff, Plan/Deliver, and constrained approval decisions through one distinct control token. Redacted model availability stays under the read token. D0-B lifecycle/concurrency, high-water resumption, secure WebView2 preflight, exact renderer origin, and Windows 11 real-process recovery are complete; Windows 10 real-machine coverage remains pending. Portable ZIP/signed MSIX remain D2 and MSI/protocol/startup/services/update remain separate. CLI stays first-class, TypeScript gains no path/Shell/Docker/key/private-lease authority, and business data does not move into the registry.

The project has a strict `skill.v1` manifest, internal immutable `fs.FS` loader, strict `skill_package.v1` boundary, schema-v69 local user Registry, schema-v70 external Run context, schema-v71 safe read-only provenance projection, and a pathless native-preview bridge now visible through the Desktop `.zip` picker. The pure-memory parser accepts only an exact two-entry deterministic ZIP, checks archive structure before bounded decompression, applies existing manifest/content validation, and separates raw archive SHA-256 from a canonical semantic fingerprint. Validation and import execute nothing. A separate Run confirmation pins an exact active object and delivers only redacted user-role guidance; no declared tool or package prose gains authority. HTTP/TUI/React receive no body, path, digest, private identity, or installation capability. The Desktop renderer receives only a short-lived one-time metadata handle after Go validation. ADR 0024 and ADR 0031 through ADR 0036 fix the import/storage/context/preview/shell/lifecycle/Run-creation boundaries.

The final slice gate passed full ordinary/race suites in 239.4s/226.8s, vet, zero-warning staticcheck, module verification/tidy diff, zero-finding govulncheck, roughly 26.45 million fuzz executions, 78.5% `internal/skills` statement coverage, repeated parser/CLI regressions, strict TypeScript, all 17 frontend tests, OpenAPI/build/npm audit, and repository privacy/encoding/link/diff scans. The audit fixed deterministic creator-version enforcement, exact Deflate-stream exhaustion against hidden post-stream payloads, a deprecated fixture API, and source-path disclosure in filesystem errors. No unresolved high/medium issue is known. GitHub Actions run `29512332025` passed commit `55b3fae` with Go/Linux in 3m4s and TypeScript in 20s.

The CLI can import, inspect, and explicitly pin inert external packages to a Run. The embedded and external catalogs remain separate, and selected external bodies enter only the current model request as untrusted guidance. D1-A provides the Go-only one-time-handle validation boundary; D0-A now exposes it through a native Desktop dialog without adding any Desktop/HTTP installation endpoint. Import and preview never execute content, call a model/network/tool, or grant capability.

## Non-Schema Desktop D1-A Pathless Skill Preview

`skills.ReadPackageFile` is now shared by CLI validation/import and the future native path. It accepts only a bounded non-symlink ordinary file, rejects whitespace path rewriting, and rechecks identity, size, and modification time before and after the read. `skills.ValidatePackageFile` feeds the existing strict parser and returns only the immutable metadata preview. Errors are stable and do not include the selected path.

`desktop.NewSkillPackagePreviewBoundary` returns a Go-native selector closure and a separate renderer bridge. The selector validates the file immediately and stores no path or body. It issues a 256-bit URL-safe random handle that expires after five minutes, is capped with at most sixteen pending projections, and can be consumed exactly once. The renderer DTO excludes file name/path, package body, Manifest description/content path/content digest, and all private identities. Every command, hook, network, Provider, tool, install, and capability-authority field remains false.

At the D1-A delivery point the bridge was not registered in CLI startup, HTTP/OpenAPI, React, or any production command and had no Store, model, Tool Gateway, Sandbox, Docker, process, or network call. The final full ordinary/race suites passed in 255.8s/314.0s; Desktop passed 100 ordinary and 25 race repetitions, the Skill file boundary 100, and the CLI package chain 10. Vet/staticcheck, module verify/tidy, govulncheck, OpenAPI drift, 22 frontend tests, production build, zero-vulnerability npm audit, 60-file/79-relative-link Markdown validation, and repository privacy scans were green. Privacy and robustness tests covered exact JSON key allowlists, source replacement after selection, invalid/symlink/oversized inputs, expiry/capacity, cancellation, entropy failure, stable CLI errors, replay, and 32-way concurrent consumption. GitHub Actions run `29578985787` passed implementation commit `45c047c` with Go/Linux in 3m13s and TypeScript in 27s. Schema remained v71; no unresolved high/medium issue was known.

## Non-Schema Desktop D0-A Embedded Read-First Shell

Wails v2.13.0 is pinned as a direct dependency and `cmd/cyberagent-desktop` is a Windows-only `desktop` build. A production Vite bundle is compiled into the binary and validated by the new `webui.LoadEmbeddedFS` snapshot boundary. The Wails AssetServer calls the existing Go `httpapi.API` Handler directly, so the desktop opens no listening port and does not duplicate API, Policy, SQLite, or redaction logic. The adapter clones requests, pins loopback Host/RemoteAddr, and narrowly normalizes the empty-root and no-body GET forms observed in Wails v2.13.

The entire renderer binding allowlist contains exactly three methods: memory-only connection bootstrap, native `.zip` selection, and one-time package preview. It accepts no path, bytes, URL, command, Scope, Policy decision, or authority field. Tokens are generated per process, validated, distinct, and absent from storage/logs/registry. The default launch is read-only; `--enable-profile-control` adds only the existing v64 control route and `--enable-run-creation` independently adds only the v72 closed creation route. Process, Shell, Docker, package installation, renderer path input, and capability authority remain false.

React auto-connects to `/api/v1` and stores tokens only in Zustand process memory. It keeps ordinary Web SSE unchanged and uses bounded cursor polling for Windows Desktop event activity because Wails v2 does not stream AssetServer responses there. A desktop-only top-bar command opens the bounded preview modal; the UI has no install control and no visible path. Single-instance restore, WebView file/drop denial, disabled default context menu, renderer code integrity, CSP, and a path-free native startup error dialog are active. The unsigned binary is not an installer or release artifact.

Real-window validation against an isolated schema-v71 home covered automatic connection, Run/Session empty states, the Skill preview modal, and the native `.zip` dialog at 1440x900 with no blank renderer or overlap. The final local gate passed ordinary/race suites in 205.1s/293.9s, ordinary and desktop-tag vet/staticcheck/govulncheck, module verification/tidy, deterministic OpenAPI, strict TypeScript, 31 frontend tests, production builds, zero-vulnerability npm audit, 61-file/82-link Markdown validation, and repository privacy/forbidden-entry scans. Desktop packages passed 50 ordinary and 10 race repetitions. The final ignored unsigned GUI is 21,022,208 bytes with SHA-256 `6b355cfa72b41d225e62ed58ac24cb9493bbf2a71f4d45120e6f0dbf5308ad0c`. GitHub Actions run `29602281365` passed implementation commit `2c0b81c`: Go/Linux completed in 4m57s, TypeScript in 26s, and the new Windows Desktop build/test job in 4m27s. No unresolved high/medium issue is known. ADR 0034 records the decision.

## Non-Schema Desktop D0-B Lifecycle And Event Resumption

`desktop.ControlPlane` owns the existing Store and `httpapi.API` in process without a listener, making same-database CLI/Desktop writes, idempotent close, concurrent open, and exact close/reopen cursor recovery directly testable. `desktop.Lifecycle` coalesces pre-start second-instance signals, ignores their arguments and working directory, restores only the existing window, serializes native restore against shutdown, cancels its context, and remains permanently inert after stop.

The new read-only `run-event-poll.v1` endpoint shares the SSE Run-bound high-water cursor and returns actual `run-events.v1` frames. It rejects unknown or duplicate query fields, invalid limits, `Last-Event-ID`, and cross-Run cursors, then validates a contiguous Store batch. React uses it only under the native Desktop bridge, retains at most 16 Runs and 500 frames per Run in module memory, resumes across remount, performs one stale-cursor reset per mount, and never synthesizes cursors or writes browser storage. Browser clients still use SSE.

Secure production builds require `desktop,production,wv2runtime.error`. A read-only WebView2 `94.0.992.31` prerequisite probe runs before bundle/database initialization and returns bounded path-free guidance without a URL, download, or installer. The in-process adapter accepts exact `http://wails.localhost`, clones and canonicalizes the request, pins loopback identity, and rejects alternate origins. A Desktop-only guard blocks external links, forms, and popup calls; Wails start-origin validation remains the native binding authority.

Final ordinary/race suites passed in 256.6s/273.5s, and the post-audit lifecycle race passed 20 rounds. Ordinary and secure-Desktop vet/staticcheck/govulncheck, deterministic OpenAPI-to-TypeScript generation, strict TypeScript, 37 frontend tests across 13 files, production build, and dependency audits are green. The first Desktop-tag scan found five reachable `x/net/html@v0.54.0` advisories; upgrading to fixed `x/net@v0.55.0` and resolved `x/sys@v0.45.0` returned both final scans to zero. Windows 11 Pro x64 10.0.26200 real-process smoke of the final 19,572,224-byte binary (SHA-256 `f26ea87f42701a7eba8efa789900ea6953ef3c1533ff95106ec4b8e6b02b1160`) proved second-instance handoff, forced termination, database reopen, and no residual Desktop process. GitHub Actions run `29609621468` passed implementation commit `c9b1c66` with Go/Linux in 5m00s, Windows Desktop in 4m21s, and TypeScript in 23s. The audit fixed repeated stale-cursor reset, non-canonical source `RequestURI`, and native restore/shutdown serialization. No unresolved high/medium issue is known. Windows 10 real-machine validation remains pending; no Agent-controlled process, Provider, Docker, Shell, external network call, or Skill installation ran. ADR 0035 records the decision.

## Non-Schema Model, Plan, And Approval Controls

D1-M1/P1/A1 completes the next three product slices without a migration, so schema remains v73. A new Go-owned Provider Registry is now the single initialization path for CLI, ordinary API, and Desktop. It registers mock plus valid environment-backed Mimo, DeepSeek, and Anthropic-compatible Providers and then loads persisted routes. `GET /api/v1/models` returns deterministic bounded status only. It contains no key, Base URL, environment-variable name, HTTP client, or raw configuration error and never probes a Provider. Secret-like or malformed model/route identifiers fail closed or are redacted.

`plan_delivery_control.v1` has separate direction and Deliver operations. The former exact-binds Run/proposal/direction and reuses the existing atomic selection/WorkItem/Note path without changing phase. The latter requires the immutable selection and reuses the Run-mode operation ledger. Neither starts or resumes the Run, obtains a lease, calls a model/tool, or grants capability.

`approval_queue.v1` returns at most 100 pending metadata records without command, arguments, path, content, fingerprint, reason, operation, or private authority. `approval_control.v1` reloads exact Run/approval/source and permits only approve-once or deny. Approve-once rechecks current Policy and is limited to dry-run Shell or process-disabled ScriptProcess; replace-file can only be denied, permanent denial cannot be overridden, and no Session Grant, file write, Shell/Local/Docker process, or capability is created. A committed decision remains same-key replayable after terminal Run transition; pending terminal mutation is rejected.

Desktop adds independent `--enable-plan-delivery` and `--enable-approvals`; model availability stays under the read bearer. The Wails bridge remains exactly three native methods. React adds a redacted Models dialog, explicit Plan/Deliver controls, and an Approval tab with memory-only intent keys and exact authority validation. OpenAPI now contains 36 paths, 84 schemas, 27 GET operations, and 11 control POST operations.

The final ordinary integrated gate passes the full Go suite in 310.1 seconds, focused Windows Desktop tags, 73 frontend tests across 18 files, strict TypeScript, Vite and Windows production builds, and zero-vulnerability npm audit. Audit fixes cover secret-like model projection, terminal replay for an already committed approval, frontend Session/Workspace approval binding, and a malformed new-dialog label. No Provider network call, real process, file write, Session Grant, installer, or external Skill execution occurred. No unresolved high/medium issue is known. ADR 0039 records the boundary.

Current metrics are architecture about 98% (V2 about 99%), complete-product usability about 67-71%, generic Coding Agent workflow about 63%, and Cyber autonomous workflow about 20%.

## Provider Diagnostics, Diff Review, And Durable Wake Intent

D1-M2 adds `provider_diagnostic.v1` and `model_route_control.v1`. Route changes are
validated and persisted before the concurrency-safe in-memory Router changes. A
diagnostic is an explicit, 15-second, content-free, tool-disabled single model request.
Its result contains bounded status only and no response text, key, endpoint,
environment-variable name, client, or raw error. Availability reads remain no-probe.

D1-D1 adds exact Run/Mission/Session/Workspace FileEdit list/detail/review. Public DTOs
select metadata plus a bounded redacted Diff, never original/proposed file bodies.
Approve-intent and deny synchronize the existing durable approval with FileEdit state
without writing the workspace. The audit found and fixed a two-transaction crash
window: a retry now finishes edit state only when the already committed approval has
the same exact binding and outcome; an opposite decision conflicts.

Schema v74 D1-Q1 adds digest-idempotent schedule/cancel operations and a durable wake
intent with bounded attempts, backoff, deadline, takeover, and generation fencing.
Public state omits lease owner/identity and fixes background loop, model, tool, and
execution authority false. There is no goroutine, service, automatic Run transition,
or Run execution lease. OpenAPI now has 43 paths, 96 schemas, 30 GET operations, and
16 control POST operations. ADR 0040 records the authority boundary.

The final cumulative six-slice gate passes on the audited code: ordinary/race suites
took 278.6s/296.1s; ordinary and secure-Desktop vet/staticcheck/govulncheck, module
verification/tidy, deterministic OpenAPI, 80 React tests across 20 files, strict
TypeScript, Vite/Windows production builds, zero-vulnerability npm audit, isolated CLI
smoke, and repository privacy/encoding/link/artifact/process-entry scans are green. The
audit fixed exact Diff body loading, the approval/edit crash window, expired-final-wake
event ordering, invalid wake timelines, and concurrent route ordering. No unresolved
high/medium issue is known, and no live Provider, process, Docker, or file apply ran.

## Foreground Wake, File Apply, And Inert Skill Installation

Schema v75 D1-Q2 adds `run_wake_consumer.v1`. It claims one due wake only after an
explicit foreground action and invokes at most eight steps through the existing
`run_execution_handoff.v1`, RunSupervisor, Policy, cumulative budgets, cancellation,
checkpoints, model/tool ledgers, and execution lease. Exact generation/handoff binding
and restart replay prevent duplicate completion. An uncertain in-flight handoff remains
prepared across lease expiry, cannot be cancelled, and cannot record model/tool facts
without a durable handoff result. No background goroutine or service exists.

Schema v76 D1-D2 adds `file_edit_apply.v1`. Apply exact-binds Run, Mission, Session,
Workspace, proposal, and durable approval, then rechecks the running Run, active Session,
current Policy, path resolution, and original/current hash at the final write boundary.
One Edit admits one operation; same-directory staging and atomic replacement precede
the written-target hash check. HTTP/React contains no path or body, and replay reports
no second write.
Run-bound proposals can no longer use the legacy approve path to bypass review/apply.

Non-schema D1-B1 exposes a fourth narrow Wails method and one strict HTTP control for
confirmed import into the existing content-addressed inert Skill Registry. Desktop uses
a short-lived one-time handle; HTTP uses bounded canonical base64. Both require the
untrusted-package confirmation and execute no content, script, hook, command, tool,
Provider, network request, Run selection, or context delivery. All three new mutation
capabilities default off and are independently gated. ADR 0041 records the boundary.

The final ordinary Go suite passes in 333.1s; focused race, Windows Desktop tags, 85
React tests, strict TypeScript, deterministic contracts, Vite/Windows production builds,
vet, module verification/tidy, npm audit, isolated CLI smoke, and repository hygiene are
green. The audit fixed prepared-wake reclaim/cancel, durable failed-call facts, stale
FileEdit recovery authority, duplicate per-Edit operations, and direct-truncation writes.
No high/medium issue remains known. Forced termination may leave one redacted staging
file before atomic replacement; the following D1-U1 slice now owns this recovery path. Historical estimates at that checkpoint were
architecture about 98% (V2 about 99%), complete-product usability about 70-74%, generic
Coding Agent workflow about 66%, and Cyber autonomous workflow about 20%.

## Durable Receipts, Workspace Evidence, And Portable Diagnostics

Non-schema D1-U1 adds `operation_receipt.v1` to FileEdit apply, foreground wake
consumption, and inert Skill installation. The Go-owned receipt exposes only a closed
outcome/replay/retry/recovery/cleanup tuple. It omits operation keys and digests,
paths/bodies, requester identity, model text, and private leases. FileEdit terminal
replay may remove only an old ordinary reserved staging file in the exact target
directory whose complete bytes match the approved proposal digest. Cleanup uncertainty
is reported but cannot rewrite the durable mutation result.

D1-E1 adds authenticated read-only `workspace_explorer.v1`. Go resolves the registered
Workspace root and accepts only canonical slash-separated relative paths. Traversal,
absolute/volume paths, links, redirects, controls, surrounding whitespace, normalized
aliases, internal staging, and ambiguous names fail closed or are omitted. Directory
scan/return bounds are 400/200; file input is 64 KiB valid UTF-8 and redacted output is
capped at 128 KiB. The DTO never includes the host root and marks every projection as
`context_provenance.v1` with `instruction_authorized=false`. React's Files tab accepts
only the exact current-parent/name child path and renders file text without Markdown.

D1-W1 adds reproducible linker metadata, `cyberagent doctor portable`, double-build
SHA-256 verification, and a PowerShell 5.1-compatible Windows checklist for PE
signature/machine/executable flags, zero COFF timestamp, metadata/hash binding,
`-trimpath`, Go module identity, and the no-installer/no-registry/no-startup/no-update
boundary. Build output must remain under the repository without traversing a child
reparse point. Automated checks do not sign off the manual Windows 10/WebView2/display/
launch/recovery matrix, so `release_ready=false` remains mandatory.

The cumulative six-slice gate passed: ordinary/race 294.0s/338.3s, ordinary and
secure-Desktop test/vet, zero-warning staticcheck, zero-finding govulncheck, module
verify/tidy, deterministic OpenAPI/TypeScript, 88 React tests across 22 files, strict
TypeScript, Vite build, zero-vulnerability npm audit, isolated mock-only CLI smoke,
privacy/artifact scans, and a real reproducible Windows double build. OpenAPI is 47
paths/106 schemas/31 GET/19 control POST. The binary SHA-256 is
`33fb9ca3064df98191ac50b2a3ef9431e1b5c81abe8c610d4be15db113cdf1ef`.
GitHub Actions run `29658783000` passed implementation commit `5f0f397`: Go control
plane 5m49s, TypeScript console 32s, and Windows Desktop shell 2m11s. No unresolved
high/medium issue is known. Current estimates are architecture about 98%
(V2 about 99%), complete-product usability about 74-78%, generic Coding Agent workflow
about 70%, and Cyber autonomous workflow about 20%. ADR 0042 records the decision.

## Completed Workspace Search, Evidence Attachment, And Receipt History

Schema v77 D1-E2/C1/U2 completes the next three-slice batch. `workspace_search.v1`
performs one deterministic scan over redacted Explorer projections with hard query,
directory, entry, file, result, snippet, and read ceilings. It follows no links, creates
no indexer, searches no raw bytes, and returns canonical relative references with
`instruction_authorized=false`.

`session_evidence_attachment.v1` is a separate default-off capability. Go exact-binds
the Run, Mission, active Session, registered Workspace, canonical source, and current
projected SHA-256. One transaction stores the tool-role evidence message, metadata
event, and immutable attachment. Schema v77 independently verifies that binding and
false instruction authority. Repository text addressed to an automated assistant is
therefore still evidence, never an operator instruction, approval, or capability.

`operation_receipt_history.v1` returns at most 100 newest terminal apply/wake/install
receipts with an optional exact Run filter and opaque public IDs. It omits raw operation
identity, paths/content hashes, requesters, archive metadata, and private leases.
FileEdit staging inspection is read-only and uncertainty stays `pending_review`.

The ordinary gate passed the uncached Go suite in 297.9s, Windows Desktop tags, `go
vet`, module verification/tidy, 92 React tests across 23 files, strict TypeScript,
deterministic OpenAPI/TypeScript, Vite build, zero-vulnerability npm audit, and a
isolated mock-only CLI smoke plus a reproducible Windows double build. OpenAPI is 50
paths/53 operations/112 schemas. The
unsigned executable SHA-256 is
`d187601e9e9d8cb0d4ee644e3c9aa1c7617905580b001ef7955dbc35b8c47af3`;
automated compatibility passed and `release_ready=false` remains correct. The audit
fixed Unicode case-mapping offsets, the true UTF-8 look-ahead search read ceiling, and
schema-level canonical evidence-reference enforcement. No unresolved high/medium issue
is known. Current estimates are architecture about 98% (V2 about 99%), complete-product
usability about 78-82%, generic Coding Agent workflow about 74%, and Cyber automation
about 20%. ADR 0043 records the decision.

GitHub Actions run `29661764283` passed implementation commit `ffbdc72`: TypeScript
console 34s, Windows Desktop shell 2m21s, and Go control plane including govulncheck
3m48s.

## Completed Operator Action Center, Evidence Inventory, And Command Palette

Non-schema D1-O1/C2/K1 completes the second three-slice batch while SQLite remains
v77. `operator_action_center.v1` exact-binds one Run/Mission/Session/Workspace and
returns at most 100 closed metadata items for pending steering, approvals, FileEdit
review/apply readiness, and due wake intent. Public IDs are opaque domain-separated
hashes. Source rows, operations, requesters, messages, commands, paths, Diffs, private
leases, and authority fields stay inside Go/Store; listing performs no mutation.

`session_evidence_inventory.v1` lists only immutable attachments for the exact active
Run-bound Session and Workspace. It exposes closed source kind, canonical reference,
SHA-256, attachment time, and fixed `instruction_authorized=false`; message ID/body,
attaching identity, operation, event sequence, and capability state remain absent.
Opening a source reuses the Go Explorer and its independent path checks.

The static `Ctrl+K` palette can navigate existing Run tabs or invalidate current Run
queries. It cannot submit a path, content, approval, operation, capability, secret, or
process request and writes no browser storage.

The cumulative six-slice gate passed full ordinary/race Go in 319.6s/299.8s,
ordinary/secure-Desktop tests and vet, zero-warning staticcheck, zero-finding
govulncheck, module verification/tidy, deterministic OpenAPI/TypeScript, 97 React tests
across 26 files, strict TypeScript, Vite build, zero-vulnerability npm audit, isolated
mock-only CLI smoke, repository hygiene scans, and a reproducible Windows double build.
OpenAPI is 51 paths/55 operations/116 schemas with SHA-256
`B9CD79254D9AE09A2DB4BCC6268F04CA8F4ADD6C638E6BAA4DA42FC223A10181`.
The unsigned Desktop binary SHA-256 is
`a89b2357a5f1e7376ea8a533356028ccd5ea5eaec388b14d7623343fd041f520`;
automated checks pass and `release_ready=false` remains correct.

Real-browser desktop/mobile auditing verified the new views and command interactions,
then found and fixed an event-envelope mismatch (`event.v1` versus canonical `v1`) and
response-body leakage on failed reconnect. The generated TypeScript version is now a
literal imported from Go's OpenAPI source and failure cancels the reader before retry.
No unresolved high/medium issue is known. Current estimates are architecture about 98%
(V2 about 99%), complete-product usability about 80-84%, generic Coding Agent workflow
about 77%, and Cyber automation about 20%. ADR 0044 records the decision.

GitHub Actions run `29665187925` passed implementation commit `1151aaf`: TypeScript
console 36s, Windows Desktop shell 2m23s, and Go control plane 3m35s.

## Completed Editor, System Credentials, And Bounded Wake Batch

Non-schema D1-I1/M3/J1 leaves SQLite at v77. `file_edit_proposal.v1` gives a running
Run's active Session a five-minute, one-intent source handle over complete,
untruncated, unredacted UTF-8. Locally bundled Monaco receives no host path and may
only create a pending proposal; review/apply stay separate. `provider_credential.v1`
stores exact supported keys in Windows Credential Manager and returns status only;
non-Windows has no plaintext fallback and changes currently require restart.
`run_wake_worker.v1` starts only from an explicit flag plus control token and consumes
at most one due intent/one Supervisor step per tick without a Tool Runner or
Shell/Local/Docker dependency.

The ordinary integrated gate passed full Go in 327.6s, vet, secure Desktop tags,
deterministic OpenAPI/TypeScript, strict TypeScript, 102 React tests across 28 files,
Vite production build, zero-vulnerability npm audit, and a reproducible Windows double
build. The unsigned binary SHA-256 is
`a0e6aa0a3d15ccc39712f8a0a64d7de06e4a6af426e060b6378b1011c93a1cf6` and remains
`release_ready=false`. The audit fixed credential bounds/provider normalization,
single-Provider failure isolation, post-close worker restart, FileEdit ID/intent drift
after uncertain persistence, post-review replay error typing, unconfigured Provider
empty-model serialization, secret-test snapshotting, and Monaco CDN/dependency
exposure. Desktop and 390x844 mobile UI smoke pass. No unresolved high/medium issue is
known, and no real key, Provider network, Shell, LocalRunner, or Docker operation was
used.

GitHub Actions run `29671519260` passed implementation commit `ee36405`: TypeScript
42s, Windows Desktop 2m31s, and Go control plane 3m54s.

## Completed Safe Recovery, Provider Generations, And Worker Health

Non-schema D1-I2/M4/J2 leaves SQLite at v77. D1-I2 rotates an expired source handle
only while the current Workspace file still matches the previously issued SHA-256;
renderer draft text is not part of reissue. A durable pending proposal is recovered
only as an integrity-checked, handle-free, non-editable Diff. Stale and missing targets
cannot be silently rebased, approved, applied, or reopened by recovery.

D1-M4 builds a complete candidate Provider Registry, validates persisted routes, and
completes credential reads before atomically advancing a generation. Any failure keeps
the old generation active. Router calls capture route and immutable Provider together,
so active calls finish on the old Provider and later calls use the new one. Responses
remain status-only; a successful reload sets restart false.

D1-J2 exposes authenticated read-only `runtime_capabilities.v1` and bounded
`run_wake_worker_health.v1` to ordinary Web and Desktop. Worker state is a closed
`disabled|ready|running|draining|stopped` set with concurrency/max steps fixed at one.
No token, owner, lease, Run, operation, or private error is exposed, and runtime enable,
persistent service, process, Shell, and Docker authority remain false. Public
`RunOnce` callers serialize, reject nil context, and drain before stop.

The cumulative six-slice gate passed final ordinary/race Go in 322.9s/352.8s, vet,
zero-warning staticcheck, zero-finding govulncheck, module verification/tidy, secure
Desktop tags, strict TypeScript, 108 React tests across 29 files, deterministic
OpenAPI/TypeScript, Vite build, zero-vulnerability npm audit, and a reproducible
Windows double build. OpenAPI has 57 paths, 61 operations, and 125 schemas. The
unsigned GUI SHA-256 is
`30a3d9d19e02f32f8ea976fc071bc6942ed06fba3e7cad937310a78e46e74dfc` and remains
`release_ready=false`. Desktop/mobile browser smoke found no horizontal overflow or
console error.

The combined audit fixed mixed Registry generations, candidate credential-read
failure replacing the serving Registry, concurrent reload false failure, inconsistent
list generations, worker construction without a control token, concurrent `RunOnce`,
nil context, missing-file proposal recovery, and recovery-dialog failure behavior. No
unresolved high/medium issue is known. No real key, Provider request, Shell,
LocalRunner, Docker, attack traffic, or external network was used. Current estimates
are architecture about 99% (V2 about 99%), complete-product usability about 87-90%,
generic Coding Agent workflow about 84%, and Cyber automation about 20%. ADR 0046 is
authoritative.

GitHub Actions run `29674460349` passed implementation commit `7d5736e`: TypeScript
console 38s, Windows Desktop shell 2m49s, and Go control plane 3m43s.

## Non-Schema D1-G1/I3/F1 Repository And Code Delivery Batch

D1-G1 adds pure-Go `repository_state.v1` for one exact registered Workspace root.
Parent discovery, redirected `.git`, nested Git-metadata links, subprocesses, network,
remotes, and hooks are rejected or unused. The cancellable metadata walk is capped at
50,000 entries, status at 10,000 entries, and output at 200 canonical relative paths.
Secret-looking path/reference data, host roots, file bodies, and remote configuration
do not enter the DTO.

D1-I3 adds `file_edit_change_set.v1` over at most 100 exact-bound previews. It returns
status and Diff-size metadata while preserving independent per-file review/apply,
explicitly denying batch/atomic mutation, and exposing mixed partial state. D1-F1 adds
a Code-only five-stage Journey that navigates existing Repository, Overview, Actions,
Diffs, and Findings surfaces. It owns no API client or composite mutation and does not
change Cyber mode.

The cumulative six-slice gate passed ordinary Go in 321.7s before audit hardening and
the final-code full race suite in 490.4s, followed by ten repository repetitions.
Vet, zero-warning staticcheck, module verification, zero reachable/imported-package
govulncheck findings, secure Desktop tags, 114 React tests across 31 files, strict
TypeScript, deterministic OpenAPI/TypeScript, Vite, zero-vulnerability npm audit,
isolated mock CLI smoke, and a reproducible Windows double build passed. OpenAPI has
59 paths, 63 operations, and 129 schemas. The unsigned GUI SHA-256 is
`145757cb1a8bbafc9080fdc29f4ada69d34b850ca64f702310ea44578ca677a9` and remains
`release_ready=false`.

The module graph retains no-fix, uncalled `GO-2026-5932` for the transitive openpgp
package; the application does not import or call it. The audit added nested Git
metadata-link rejection and fixed strict-client handling of terminal proposed edits
with zero allowed actions. Desktop/mobile browser checks found no page overflow or
console error. No unresolved high/medium issue is known; no real key, Provider,
Shell, LocalRunner, Docker, hook, attack traffic, or external network was used. Current
estimates are architecture about 99% (V2 about 99%), complete-product usability about
89-92%, generic Coding Agent workflow about 88%, and Cyber automation about 20%.
ADR 0047 records the boundary.

GitHub Actions run `29678257802` passed implementation commit `d69a812`: TypeScript
console 43s, Go control plane 5m32s, and Windows Desktop shell 5m29s.

## Schema v78 D1-G2/V1/F2 Repository Diff, Verification, And Handoff Batch

D1-G2 adds `repository_diff.v1`, a pure-Go exact-root read projection. It returns at
most 50 secret-redacted patches, 64 KiB per item and 512 KiB aggregate, with closed
states for binary, oversized, linked, or unavailable content. Parent discovery,
redirected/nested metadata links, subprocesses, network, remotes, hooks, host roots,
and raw unredacted bodies remain absent.

Schema v78 D1-V1 adds a separate verification capability and immutable
`operator_verification_evidence.v1`. Go redacts and exact-binds one normalized
`pass|fail|unknown` operator observation to the Run/Mission/active Session/registered
Workspace. Transactional Store and SQLite checks bind one metadata event and keep
command/model/approval/authority facts false. Listing is capped at 100 and v77 upgrade
creates no synthetic evidence.

D1-F2 adds read-only Code-only `code_handoff.v1`. It compacts durable Plan, WorkItem,
steering queue, FileEdit change-set, verification, operator-action, and Finding-report
references without private bodies or composite authority. A before/after event
sequence check retries four times and returns conflict rather than a torn snapshot.

The ordinary gate passed full uncached Go in 308.1s, Desktop-tag tests, vet, targeted
zero-warning staticcheck, module verification/tidy, deterministic OpenAPI/TypeScript,
strict TypeScript, 120 React tests across 35 files, Vite production build,
zero-vulnerability npm audit, repository hygiene scans, and a reproducible Windows
double build. OpenAPI is 62 paths/67 operations/143 schemas with SHA-256
`652707A6D9CA72EBBD86B6FD407A382DFBE85B094927C82AFC3765D2648332B3`.
The unsigned GUI SHA-256 is
`2ab74a47794287bac71877172136f02631b5cc9a44febd930e8ee7b1913ba93f` and remains
`release_ready=false`.

Production browser verification exercised record/list/Handoff refresh and Repository
empty state at desktop/mobile widths with no page overflow or console error. The audit
fixed active-Session transactional revalidation, Handoff snapshot consistency, Diff
truncation propagation, strict-client reference/action/truncation checks, CR parity,
and React capability wiring. No unresolved high/medium issue is known. No real key,
Provider, Shell, LocalRunner, Docker, hook, attack traffic, installer, registry,
startup task, or external network was used. Current estimates are architecture about
99% (V2 about 99%), complete-product usability about 92-94%, generic Coding Agent
about 92%, and Cyber automation about 20%. ADR 0048 records the boundary.

GitHub Actions run `29682547524` passed implementation commit `cff7489`: TypeScript
console 42s, Windows Desktop shell 2m34s, and Go control plane including vet and
govulncheck 3m33s.

## H1/H2/H3 And Schema v79 Runtime Blocking Guards

H1 places every enabled in-process Tool behind a default 15-second hard deadline,
with a five-minute construction maximum, stable cancellation/timeout exits, and panic
recovery. Read/list built-ins are context-aware and bounded. Platform-specific opens
admit regular files only and reject FIFO/device/socket inputs before they can block.

H2 introduces one bounded, reference-counted synchronous wait graph. Direct and
indirect cycles fail before edge installation, and Tool/Retriever/Store/Runner nodes
cannot synchronously wait on an Agent. Root Supervisor context, Specialist
parent/child rounds, and Tool Gateway invocations are wired now. Future RAG, Store
callback, Model adapter, and Runner boundaries must enter the same graph.

H3/schema v79 records metadata-only progress fingerprints and fixed counters. Three
identical `continue` actions or six turns without selected structured-state change
atomically produce `supervisor.livelock_detected`, preserve the completed Session turn,
and move the Run to `paused/waiting`. Completion replay is exactly once; reset requires
a later durable operator `paused -> running` transition. Upgrading v78 creates no
synthetic guard or event.

The cumulative six-slice gate passed final uncached Go in 312s and final-code race in
358s, plus 20 Tool/wait-graph and ten progress-guard Store repetitions. Ordinary and
secure-Desktop tests/vet, zero-warning staticcheck, module verification/tidy, no
reachable govulncheck finding, 120 React tests in 35 files, strict TypeScript,
deterministic API generation, Vite, zero npm vulnerabilities, isolated mock-only CLI,
credential/artifact scans, reproducible Windows build, and desktop/mobile browser
checks are green. The unsigned GUI SHA-256 is
`31e0df63d3fbbccac6728ad2322196bee55d57e775a15cc34f752c0632bdc699` and remains
`release_ready=false`.

The audit hardened read-size integer/OOM limits, Go/SQL threshold constraints,
explicit resume evidence, corrupt-row fail-closed handling, and counter saturation.
The module graph retains uncalled `GO-2026-5932`. No unresolved high/medium issue is
known on an enabled path. A non-cooperative third-party goroutine cannot be forcibly
killed by an in-process timeout; real process-tree deadlocks remain for the separate
Runner start/wait/TERM/KILL/orphan gate while Local/Docker execution stays disabled.
No real key, Provider, Shell, LocalRunner, Docker, attack traffic, or external network
was used. ADR 0049 records the boundary. Architecture remains about 99%, complete
product usability about 92-94%, generic Coding Agent usability about 92%, and Cyber
automation about 20%.

GitHub Actions run `29688544340` passed implementation commit `2012bfa`: TypeScript
console 42s, Windows Desktop shell 3m13s, and Go control plane including vet and
govulncheck 3m54s.

中文复核：当前启用链路已经分别防住 Tool 永久阻塞、同步依赖成环和 Run 无进展空转；
三者都失败关闭并可审计。真实宿主机/容器进程仍未开放，所以端口、句柄和进程树级死锁
必须等独立 Runner 生命周期门禁，而不能把 v79 当作进程隔离证明。

## Schema v80 D1-G3/V2/F3 Repository History, Verification Plan, And Export Batch

D1-G3 adds pure-Go `repository_history.v1` for the exact registered Workspace root.
It returns at most 50 first-parent commits and 64 local branches after bounded scanning.
Commit subjects are normalized, bounded, and secret-redacted. Author/email/body/remote/
root data, subprocesses, network, and hooks remain absent; redirected or linked Git
metadata fails closed.

D1-V2/schema v80 adds immutable `operator_verification_plan.v1` with up to 32 ordered
operator checks. Plans are exact-bound to a Code Run, active Session, Workspace,
metadata event, and digests. They remain guidance-only and separate from v78 outcomes;
command/model/result inference/approval/authority fields are fixed false by Go and SQL.

D1-F3 exports one stable Code Handoff as at most 256 KiB of Markdown or JSON. The
wrapper carries a source event high-water mark, byte count, MIME type, safe filename,
and SHA-256. The strict TypeScript client recomputes the digest and source binding.
Export cannot resume, accept, apply, mutate, or execute.

The ordinary gate passed uncached full Go in 334.6s, post-audit focused Repository/
Application/Store/HTTP tests, vet, 124 React tests across 37 files, strict TypeScript,
deterministic API generation, and Vite production build. OpenAPI is 65 paths, 71
operations, and 155 schemas with SHA-256
`99887F651B563C56C87D19C5624EDD776AFC29AA6095EAB8C685E6767C165E7F`.
Chrome-extension checks of the final production bundle found no root/email leak,
inferred verification result, page overflow, console warning, or console error.

The audit fixed exact-limit plan inventory parsing, hostile Git counter saturation,
stale idempotency-key reuse after editing a failed plan, and premature object-URL
revocation. No unresolved high/medium issue is known. No real
key, Provider, Shell, LocalRunner, Docker, hook, attack traffic, or external network
was used. Architecture is about 99%, complete-product usability about 93-95%, generic
Coding Agent usability about 93%, and Cyber automation about 20%. ADR 0050 records the
boundary.

GitHub Actions run `29695882120` passed implementation commit `d70d96c`: TypeScript
console 43s, Windows Desktop shell 2m39s, and Go control plane including vet and
govulncheck 3m56s.

## Schema v81 D1-G4/V3/R1 Exact Commit, Association, And Runner Lifecycle Batch

D1-G4 adds pure-Go `repository_commit_detail.v1` for one exact lowercase SHA-1 object
at the exact registered Workspace root. It compares the commit tree with its first
parent and returns at most 200 canonical path entries with added/modified/deleted,
content-change, and regular/executable/symlink/submodule mode metadata. Author/email/
body, blob content, remote/root, checkout/ref mutation, subprocess, network, and hooks
remain absent. Redirected metadata, links, malformed trees, and missing objects fail
closed.

D1-V3/schema v81 adds immutable plan-item/evidence associations. One evidence may
answer exactly one earlier plan item, while an item may retain multiple contradictory
observations. Exact Code Run/active Session/Workspace/plan/item/evidence/event/
operation/digest binding is repeated in Go, the transactional Store, and SQLite.
Coverage is a bounded per-item pass/fail/unknown count plus unobserved state, never an
inferred aggregate result.

R1 adds `runner_lifecycle_contract.v1` for simulation-only backends. It covers
start/wait, pre-cancellation, timeout, shared wait-graph admission, TERM/KILL grace,
final inspect/reap, partial-start cleanup, invalid-identity cleanup, and orphan cleanup.
There is no CLI, HTTP, Desktop, Agent, LocalRunner, Docker, `os/exec`, or product
capability wiring.

The cumulative six-slice robustness gate passed final uncached Go in 509s and full
race in 341s, ordinary/secure-Desktop tests and vet, zero-warning staticcheck, module
verify/tidy, zero reachable govulncheck findings, 127 React tests across 37 files,
strict TypeScript, deterministic OpenAPI/TypeScript, Vite, zero-vulnerability npm
audit, isolated mock-only CLI, privacy/artifact scans, reproducible Windows build, and
desktop/mobile Chrome checks. OpenAPI is 68/74/163 with SHA-256
`CFAD160A85306B2602F95A62298828DB86BDFAAF6D55F47BA468860079C42E8D`; generated
TypeScript schema SHA-256 is
`CCA5EF8B86E7F0D494E7B2BAF4FCA92FBE3FCB9C3A54E58D4A3C3B77028D5B73`. The unsigned
GUI SHA-256 is `77fb4d6fede1c1e3a0c3f3e9d39581e28f7a6880e0e25b222dcf0d3c701d1213` and remains
`release_ready=false`.

Chrome recorded and recovered one plan, pass observation, and explicit association as
`1/1 observed` plus `1 linked`, with no page-level overflow or console warning/error.
The audit replaced a Git tree walker that could silently skip missing subtree objects,
fixed v81 downgrade-trigger cleanup ordering, saturated redaction counters, constrained
the OpenAPI control whitelist, and cleaned partial/invalid Runner starts. No unresolved
high/medium issue is known on an enabled path. The module graph retains only unimported/
uncalled transitive `GO-2026-5932`. No real key, Provider, Shell, LocalRunner, Docker,
hook, attack traffic, or external network was used. Architecture remains about 99%,
complete-product usability about 94-96%, generic Coding Agent usability about 94%, and
Cyber automation about 20%. ADR 0051 records the boundary.

## Schema v82 Conservative Context And Cumulative Handoff Memory Batch

C1 makes token planning conservative for multilingual content. ASCII uses the larger
word/four-character estimate, non-ASCII counts UTF-8 bytes, and every addition
saturates. C2 adds `model_context_window.v1` with a 32,768-token fallback, 1,024 safety,
1,024 default output, and 4,096 output cap. Complete Root/Specialist requests include
message/tool/schema cost; only oldest ordinary history can be removed. Mandatory
overflow fails before Provider activity. Exact per-model overrides swap with Router
generations but have no user-facing configuration surface yet.

C3/schema v82 makes context handoffs cumulative and append-only. Active history still
compacts above eight messages and keeps four. The 4,000-character handoff retains at
most 12 prioritized provenance records with an exact predecessor ID, row/record
digests, cumulative compacted and omitted counts, monotonic ordinal, and Session-message
ID high-water. SQLite rejects
mutation, deletion, and stale forks; Go verifies on read. V0 rows are preserved and
folded once as non-authoritative history. Only provenance-confirmed operator or Go
control records can retain instruction authority; arbitrary files and model/tool text
remain untrusted evidence and are not automatically loaded from `AGENTS.md` or README.

The uncached full Go suite passed in 348.5 seconds. Changed-package `go vet`, strict
TypeScript, and 127 Vitest tests in 37 files passed. The audit fixed the original loss
of earlier summaries across repeated compaction, separated handoff retention into a
dedicated 12-record cap, initialized zero-value Router maps, corrected v81 downgrade-fixture ordering, bounded/redacted source references, clamped clock rollback, and added message-high-water crash recovery.
No unresolved high/medium issue is known. No real Provider, key, Shell, LocalRunner,
Docker, hook, attack traffic, or external network was used. ADR 0052 records the
boundary.

## D1-G5/V4/R2 Exact Preview, Handoff Coverage, And Process Conformance

D1-G5 adds pure-Go `repository_commit_file_preview.v1`. It exact-binds the registered
Workspace root, lowercase commit object, and canonical path; accepts only regular or
executable UTF-8 text; caps input at 64 KiB and redacted projection at 128 KiB; and
returns projected-content SHA-256 with non-authorizing provenance. Links, binary and
oversized files, unavailable objects, raw blobs, roots, checkout/ref mutation,
subprocesses, network, remotes, and hooks remain absent.

D1-V4 adds `operator_verification_plan_coverage.v1` to `code_handoff.v1` and both export
formats. At most 100 flat plan-item references carry only digests, explicit outcome
counts, and the latest association sequence. Contradictory observations remain visible;
private plan/evidence bodies and aggregate result inference are absent. Go and the
strict TypeScript client independently verify bindings, bounds, duplicate identities,
counts, digests, sequences, and closed authority fields.

R2 renames the lifecycle backend marker to `NonProductOnly` and adds real OS
conformance only in `_test.go`. Windows uses a private kill-on-close Job Object; Unix
uses a private process group. Tests start only the current Go test binary and prove
cooperative termination, forced-kill escalation, and child cleanup after parent exit.
No product package can construct these adapters, and no Local/Docker start authority
is added.

The six-slice robustness gate passed uncached ordinary Go in 380 seconds and full race
in 411.2 seconds. Post-audit focused ordinary/race tests, ordinary and secure-Desktop
tests/vet/staticcheck/govulncheck, module verify/tidy, 127 Web tests across 37 files,
strict TypeScript, deterministic OpenAPI/TypeScript, Vite, zero npm vulnerabilities,
isolated mock-only CLI, privacy/artifact/process-entry scans, Linux runner-test binary
cross-compilation, and a reproducible Windows build passed. OpenAPI is 69 paths, 75
operations, and 167 schemas with SHA-256
`C548ADCBFB4BF271009348A36352E987FBF4CA10681F4B9C7CC694543487FDF6`.
The TypeScript schema SHA-256 is
`6093BB23C1A413154027FF3283AD2485DC3F44C8763949C1A45B7D066B7BB914`.
The unsigned GUI SHA-256 is
`44d54bf9d50b7cd99b89f5089833823ce0337bb0e0158ec16ef6aa9a5b415614` and remains
`release_ready=false`, without installer or registry writes.

The audit fixed platform-width coverage-count addition, negative/empty aggregate event
facts, and duplicate plan/count acceptance at the narrow Store boundary. No unresolved
high/medium issue is known on an enabled path. The dependency graph retains one
required-module advisory that is not imported or called. User test keys are absent;
no real Provider, Shell, LocalRunner, Docker, hook, attack traffic, or external network
was used. Architecture is about 99%, complete-product usability about 95-97%, generic
Coding Agent usability about 95%, and Cyber automation about 20%. ADR 0053 is
authoritative.

## Recommended Next Batch

Complete D1-G6 bounded exact-file history, D1-V5 read-only verification coverage
drill-down, and R3 bounded output/exit-evidence contract. The repository slice must
remain exact-root and process-free; the verification slice must keep plans, evidence,
and results separate; R3 must remain non-product and must not map profiles, approvals,
Local, Docker, CLI, HTTP, Desktop, or Agent paths to process start. This is the first
half of a new six-slice cycle and receives the ordinary integrated gate. The manual
Windows 10 matrix, signed ZIP/MSIX distribution, real Sandbox release gate, Rust
analyzers, xterm input, network grants, and CTF solving remain separately gated.

Keep the Local profile disabled until a real OS sandbox makes protected host roots unavailable or read-only; never map it to unrestricted `os/exec`. Product Docker start/wait/TERM/KILL/orphan behavior still requires a later independent release gate; R2 test-binary conformance is not production evidence. Broader HTTP/Desktop mutations, Rust analyzers, network/secret support, end-user process execution, and CTF solving remain deferred.

Real Local/container-process execution remains disabled until every v51 check has independently verified and independently accepted production evidence and Sandbox retained-resource cleanup, resource/network, cancellation, running-orphan, and atomic Artifact-export paths pass separate audits. Schema v52 simulation, v53 metadata observation, v54 compilation/fake writes, v55-v56 non-started daemon rehearsals, v57 sealing, v58 durable capture requirements, v59 never-started handoff evidence, v60 projection plans, v61 never-started volume application, v62 cleanup, v63 design review, v64 profile selection, v65 non-authorizing capture receipts, v66 recoverable capture ownership, v67 read-only daemon metadata, and v68 receipt acceptance do not satisfy that requirement. TypeScript, future Rust analyzers, and model providers remain unable to bypass the Go Tool Gateway or Policy boundary.
