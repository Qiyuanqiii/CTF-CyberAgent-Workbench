# Usage

## Missions and Runs

```powershell
cyberagent run create "review this workspace" --workspace demo --profile review --surface code --phase plan
cyberagent run create "explain this code" --profile learn --max-turns 40 --max-tokens 20000 --timeout 20m
cyberagent run adapt-task <legacy-task-id>
cyberagent run list
cyberagent run list --status paused
cyberagent run show <run-id>
cyberagent run mode <run-id>
cyberagent run plans <run-id>
cyberagent run plan show <proposal-id>
cyberagent run plan choose <proposal-id> 2 --operation-key <stable-key>
cyberagent run plan selection <run-id>
cyberagent run phase <run-id> deliver --operation-key <stable-key> --reason "plan accepted"
cyberagent run delivery checkpoint <work-id> --operation-key <stable-key> --focused "focused tests passed" --diff-audit "diff reviewed" --security-audit "security reviewed" --handoff "slice handoff"
cyberagent run delivery checkpoint <final-work-id> --operation-key <stable-key> --focused "focused tests passed" --diff-audit "diff reviewed" --security-audit "security reviewed" --handoff "module handoff" --functional "full suite passed" --robustness "race and failure paths passed"
cyberagent run delivery list <run-id>
cyberagent run delivery show <checkpoint-id>
cyberagent run steer enqueue <run-id> "review the current diff" --operation-key <stable-key>
cyberagent run steer cancel <steering-id> --operation-key <stable-key> --reason "requirement withdrawn"
cyberagent run steer drain <run-id> --max-steps 1
cyberagent run steer list <run-id> --limit 100
cyberagent run steer show <steering-id>
cyberagent run events <run-id>
cyberagent headless events <run-id> --max-events 1000
cyberagent headless events <run-id> --after-sequence <n> --follow --timeout 30m
cyberagent run start <run-id>
cyberagent run step <run-id>
cyberagent run execute <run-id> --max-steps 3
cyberagent run execute <run-id> --max-steps 3 --finish --summary "planning complete"
cyberagent run finish <run-id> --summary "review complete"
cyberagent run fail <run-id> --reason "blocked by provider"
cyberagent run checkpoint <run-id>
cyberagent run graph <run-id>
cyberagent run lease <run-id>
cyberagent run pause <run-id>
cyberagent run resume <run-id>
cyberagent run cancel <run-id>
```

A Mission is the stable goal and authorization scope. A Run is one resumable execution attempt. Each new Run creates a dedicated active Session unless `--session <id>` selects an unattached active Session. Session creation or attachment, Run creation, and their initial events commit together in SQLite.

Schema v41 gives each Run one immutable `run_mode.v1` snapshot with two independent axes. `--surface code|cyber` selects the work domain and cannot change inside that Run. `--phase plan|deliver` selects whether the Supervisor is preparing a bounded plan or delivering it. Omitting both preserves the compatibility default `code/deliver`. Neither axis grants tools, network, Shell, file mutation, approval, or child-Agent authority; those remain separate Go-owned Policy and Scope decisions.

`run mode` prints the current snapshot and revision. `run phase` is an explicit operator transition that requires a stable 16-256-byte operation key. It is accepted only for `created` or `paused` Runs with no active execution lease; the Store rechecks these conditions transactionally. Exact replay returns the existing revision, changed intent conflicts, and the raw key is never persisted. Surface, Profile, Scope, protocol, and policy version remain fixed for every revision. To move from Code to Cyber or vice versa, create a new Run under the intended authorization scope.

Session messages, assistant policy decisions, ToolRun changes, and FileEdit changes are projected into `run events` transactionally. Activity carrying a workspace different from the Run scope is rejected. `run start` advances the lifecycle from `created` through `preparing` to `running`; it does not invoke a model by itself.

`run step` asks the RunSupervisor to execute exactly one root Agent turn. It writes a `turn_started` checkpoint before the model call, loads the persisted execution-mode snapshot, permits only the bounded create-only WorkItem/Note tool loop, and ultimately requires one strict `root_lifecycle.v1` JSON action. The Supervisor validates and interprets the action, then atomically stores the user-facing message, policy decision, model usage, lifecycle events, cumulative token/model-time counters, and next checkpoint. Raw protocol JSON is not written to Session history. `run checkpoint` displays the durable Supervisor phase, protocol-repair phase/reason, next turn, token counters, and execution milliseconds. A process restart resumes an unfinished started turn or pending tool batch; a committed turn is never appended twice. Turn or token budget exhaustion returns exit code 8, while the persisted execution-time boundary returns a deadline error.

Root actions use `continue`, `finish`, or `wait`. `continue` advances to another idle turn. `finish` requires a summary and atomically completes the Run. `wait` requires a reason, atomically pauses the Run, and resumes at the next turn after `run resume`. Unknown fields, trailing data, Markdown fences, invalid combinations, and responses over 64 KiB fail the current turn without writing user/assistant messages. Assistant prose by itself cannot mutate Run state.

In `plan` phase, `finish` is deliberately invalid: the model receives one bounded lifecycle repair and must return `continue` or `wait`, while explicit operator completion is also rejected. After the plan is accepted, pause the Run if needed and use `run phase <id> deliver`; the following turn receives the new durable mode revision. The current root tools remain proposal/create-only, so Plan mode cannot silently execute Shell, files, processes, network calls, or Specialist schedules.

Schema v42 adds the strict `plan_delivery.v1` proposal tool for root Plan turns. A valid proposal has exactly three directions. Each direction contains 1-8 ordered delivery modules with a title, summary, acceptance criteria, bounded tradeoffs, and dependencies that may reference earlier modules only. Unknown fields, duplicate titles or dependencies, stale mode revisions, inactive root turns, invalid leases, Policy denial, and exhausted tool budgets fail closed. Proposal creation records no selection and authorizes no phase change, work execution, tool, Shell, network, file mutation, or child Agent.

Use `run plans` and `run plan show` to inspect proposals after the Run pauses. `run plan choose` is the only selection path and accepts direction `1`, `2`, or `3`, an optional bounded operator identity, and an exact normalized 16-256-byte operation key. It requires a paused Plan Run with no active execution lease, then atomically creates the selected WorkItems and backward dependency graph, a pinned decision Note, the immutable selection, and metadata-only events. Exact cross-process replay returns the same objects; a changed direction or identity under the key conflicts. Selection still leaves the Run in Plan, so `run phase <id> deliver` remains a separate explicit action. HTTP, TUI, and Web only read this state.

Schema v44 enrolls new and untouched legacy selections in `delivery_checkpoint.v1`. Before a selected WorkItem can complete, move it to `in_progress`, pause the Run in Deliver phase, and record one checkpoint for its exact current WorkItem version and mode revision. `--focused`, `--diff-audit`, `--security-audit`, and `--handoff` are mandatory, redacted, normalized operator attestations. The last selected module is the deterministic larger-module boundary and also requires `--functional` plus `--robustness`; non-final modules reject those flags. Recording atomically creates an immutable pinned handoff Note, a digest-keyed idempotency operation, and metadata-only events. Exact retries converge across processes; changed evidence under the same key conflicts. Afterward, `todo complete <work-id>` uses the existing WorkItem transition path and rechecks the gate in Go and SQLite.

The model has no checkpoint tool. HTTP, TUI, and Web expose only enforcement, required/ready counts, and bounded checkpoint metadata; they omit evidence, internal digests, operation keys, and requester identity. Policy also denies obvious Agent attempts to execute `cyberagent run delivery checkpoint` through Shell, process, script, or Sandbox tools. This is defense in depth rather than a claim that command-text regexes are a complete OS security boundary; real process execution remains disabled. A pre-v44 selection that already contained a completed or cancelled WorkItem is left explicitly compatibility-exempt (`delivery_gate_enforced=false`) rather than receiving invented evidence.

Schema v45 adds ordered operator steering for running or paused Runs. `run steer enqueue` requires a normalized 16-256-byte operation key and accepts one normalized UTF-8 message up to 16 KiB. A Run may hold at most 64 pending messages and 256 KiB of pending text. Exact replay returns the existing message; changed content, Run, or operator under the same key conflicts, and SQLite stores only a domain-separated key digest. `list` shows counts and ordered metadata, while trusted local `show` also displays the redacted content and its digest.

The Supervisor consumes only the oldest message at the next safe root-turn boundary. A failed model/tool turn leaves it pending and supersedes only that attempt's delivery; restart or lease takeover prepares it again. Session history receives the operator message and assistant response only in the same successful lifecycle transaction. If another message remains, model `finish` or `wait` is deferred to an effective `continue`, and Run completion is rejected until the queue drains. Failing or cancelling the Run cancels all outstanding steering. The queue never interrupts an active tool/model commit and grants no tool, Shell, network, write, approval, or child-Agent capability.

For a Run-bound Session, an ordinary `session send` automatically uses this queue when an execution lease is active, a recoverable attempt already owns PendingInput, or the Run already has queued steering. The command reports `queued`, steering ID, and sequence instead of pretending a model reply was produced. During a busy TUI action, plain text follows the same path without clearing live progress; slash commands remain blocked. HTTP, React, and the TUI queue view expose metadata only and cannot enqueue. A paused Run remains paused after enqueue and must be resumed explicitly.

Schema v46 adds local operator controls without changing queue authority. `run steer cancel` requires a stable 16-256-byte operation key and a non-empty reason of at most 2 KiB. It creates an immutable cancellation fact only while the message is pending and has no prepared delivery. Exact retry returns the same fact; changed intent conflicts. Prepared, committed, already-cancelled, or terminal-Run messages cannot receive a new operator cancellation. Editing and reordering remain unsupported. Run failure/cancellation closes remaining messages with bounded terminal facts in the lifecycle transaction.

`run steer drain` processes one queued turn by default and at most 64 per invocation. It acquires the Run execution lease before explicitly resuming a paused Run. A conflicting lease leaves the Run paused. The steering-only begin path refuses to generate a Mission-goal turn or recover an unrelated failed ordinary input, and every real turn still consumes the existing token/turn/time budgets and passes Policy. Empty queues do not wake paused Runs. This is an explicit local operation, not a background worker or new execution capability.

Use `session send <id> "message" --operation-key <stable-key>` when a Run-bound client needs durable retry identity. With this flag, the command always enqueues or replays steering, even when the running Run is otherwise idle, and never performs a synchronous model call. The same key and intent converge across process restart and after committed delivery; changed intent conflicts. The flag is rejected for slash commands and unbound Sessions. HTTP/OpenAPI, React, TUI, models, and child Agents have no cancel/drain or idempotent enqueue mutation.

## Skills

```powershell
cyberagent skill list
cyberagent skill list --profile review
cyberagent skill show review
cyberagent skill validate
cyberagent skill select <run-id> review --operation-key <stable-key> --token-budget 4096
cyberagent skill selection <run-id>
```

The embedded read-only `skill.v1` Registry exposes bounded version `1.1.0` workflow guidance for `code`, `review`, `learn`, `script`, and the cross-Profile `plan-delivery` workflow. Schema v39 `skill select` is operator-only and must create the Run's single immutable selection before `run start`. It accepts one to eight names compatible with the Mission Profile, deterministically pins each version/content hash/byte count/token upper bound, and rejects an aggregate above `--token-budget` (maximum 8192). Operation keys must be stable normalized 16-256-byte values; SQLite stores only a domain-separated digest. Exact selection replay returns the original tuples after Run start, while changed intent conflicts. `skill selection` reads those pinned tuples.

Schema v40 loads the complete selected set for root Supervisor turns. Before every Provider call, Go reconstructs `skill_context.v1` from the persisted tuples and embedded Registry, rechecks exact version/hash/bytes/Profile, redacts it, and enforces a separate deterministic token budget. New selection sees only the current `1.1.0` manifests; a hard-bounded embedded history resolves existing `1.0.0` selections exactly and is not a user-controlled load path. A metadata-only preparation is committed with the first model-start event and safely replays after restart; neither SQLite nor Run events contain Skill text, paths, names, or hashes. A selected Skill never authorizes its declared tool dependencies.

Schema v47 derives `specialist_skill_context.v1` for each active child Attempt. Go reloads the child after Attempt start, binds the current immutable Run mode and parent selection, requires delegated `model.chat`, and selects at most one already-pinned guide. Code uses the guide matching its Profile. Cyber receives no broad Code/Review/Learn guide and receives `script` only for the Script Profile. `plan-delivery` is root-only. The default child budget is 1,024 conservative tokens with a 2,048 hard maximum. Preparation is idempotent across concurrent Store callers and commits atomically with the first Specialist model start; a selected Run cannot start that call without preparation. Child assignment text, model output, HTTP, Tool Gateway, and external directories cannot select Skills. The body remains in the current Go Provider request only, while SQLite and events store aggregate metadata and fingerprints.

## Headless NDJSON

`cyberagent headless events <run-id>` exports the same redacted, append-only SQLite Run events used by CLI, SSE, and Web. The protocol is `headless.v1`; stdout contains only newline-delimited JSON. Each durable event is one `kind: "run.event"` record, followed by exactly one `kind: "stream.end"` record for every normal snapshot, terminal outcome, event bound, cancellation, or deadline. Human-readable diagnostics remain on stderr.

`--after-sequence <n>` resumes strictly after a previously emitted durable sequence. A cursor beyond the current tail is rejected before stdout is written. `--max-events` defaults to 1,000 and is bounded to 10,000; a truncated export returns exit 8 and reports `suggested_resume_after` in the end record. Reads occur in batches of at most 100 and validate contiguous sequence, Run/Mission identity, UTF-8 metadata, and a 1 MiB payload ceiling. The command never writes a Run event.

Without `--follow`, a nonterminal Run ends with `reason: "snapshot"` and exit 0. `--follow` polls the same local SQLite database every 250 ms by default, accepts only 50 ms through 5 s, drains any final event before returning, and may be bounded with `--timeout` up to 24 hours. Terminal completion returns 0, terminal failure returns 4, terminal cancellation returns 7, the event cap returns 8, and timeout returns 9. Caller cancellation returns 7. These codes reuse the stable `apperror` contract.

Headless mode is a read adapter, not another execution engine: it does not call `RunSupervisor`, a Provider, Tool Gateway, Sandbox, Shell, network, or file-write path. Run execution remains in the existing CLI/Session/operator services, so closing the Headless consumer cannot stop or mutate a background Run.

Schema v19 assigns every new Run one stable root Agent identity. The root is `ready` before a turn, `running` only while bound to the persisted Supervisor attempt, `waiting` when the Run pauses, and terminal with the Run. These projections, coordinator events, and a bounded recovery snapshot commit in the same transaction as the existing Run/Supervisor change. `run graph <run-id>` lazily registers older Runs when needed, validates the current node and pending-inbox metadata against the latest `agent_graph.v1` snapshot, and prints only bounded metadata. Current roots have `child_limit=0`: no public child-spawn path or concurrent sub-agent execution exists yet. The durable inbox primitive is internal and is not inserted into model prompts in this slice.

Schema v20 makes internal inbox delivery recoverably idempotent. In-process callers must supply a normalized 16-256 byte key; the Store persists only its domain-separated digest and a fingerprint of the redacted canonical intent. Exact retries return the original message with `replayed: true`, while changed intent conflicts. `wake` requires a running Run and a waiting Specialist recipient; it never wakes root or resumes a paused Run. `dependency` requires an Agent sender and a strict `dependency_id`/`state` payload. These operations remain internal: there is no CLI, HTTP, or model tool for arbitrary Agent messaging.

Schema v21 keeps Specialist admission internal and default-disabled. Go code must construct a Coordinator with a valid `SpecialistAdmissionPolicy`; then each request is limited to one of at most two depth-one children, a nonempty parent-Skill subset, a dedicated active Session, positive per-child turn/token reservations, and aggregate root headroom. Admission is idempotent and atomic. Reserved capacity reduces the root budget returned to later Supervisor turns. Pause/resume and terminal Run transitions project to children and graph snapshots, and terminal child Sessions are archived. There is still no CLI/HTTP/model spawn command and no child model execution loop.

Schema v22 binds structured memory to real same-Run Agent identities. `run graph <run-id>` prints the root and any internally admitted Specialist IDs. WorkItem/Note create, list, show, and update support `--owner-agent <agent-id>`; direct Store writes and SQLite triggers reject missing or cross-Run identities, and new assignment to a terminal Agent is refused. Historical label-only ownership remains readable. The root Supervisor automatically loads Notes visible to its root Agent identity and automatically assigns model-created WorkItems/Notes to that identity. Models cannot submit or override `owner_agent_id` because it is absent from their tool JSON schemas.

Schema v23 adds an internal-only `agent.finish` path with strict `agent_completion.v1` reports. It has no CLI, HTTP, or model-facing command. Child workers must submit the exact active attempt, `succeeded` or `partial`, a bounded summary, and only child-owned WorkItem or parent-visible Note references. Completion atomically writes the parent result inbox entry, terminal child state, archived child Session, audit events, and recovery snapshot.

Schema v24 adds the internal Specialist Attempt scheduler, but it remains a Go-only capability with no user command. A turn starts only under the current Run execution lease, consumes one reserved turn immediately, and may record model token/time usage once. `continue`, completion, crash, and Run-lifecycle interruption persist immutable attempt outcomes. Crashes deliver a redacted notification to the root and stop the child when its budget is exhausted. When an expired Run lease is taken over, the new worker recovers stale attempts once and the previous worker cannot commit new usage or terminal state. Users and models still cannot spawn, start, or finish a Specialist directly, and no child Provider loop is enabled yet.

Schema v25 lets the root Supervisor read protocol-backed direct-child inbox updates without adding a public inbox command. Before a root model call, Go prepares at most four sequence-ordered dependency, CompletionReport result, or crashed-Attempt notification messages. A successful turn consumes them atomically with its Session/lifecycle commit; a failed turn leaves them pending, and cancellation, restart, or lease takeover reuses the exact prepared batch. The prompt contains strict redacted typed data and durable sender provenance, but never message IDs, sequence values, cursors, or a model-controlled acknowledgement field. The child Provider loop and public/model spawn remain disabled.

Schema v26 adds one explicitly invoked internal no-tool Specialist model turn. Internal Go code constructs `SpecialistRunner`; no CLI, HTTP, or model-facing command can start it. It uses the Run execution lease, child turn/token budgets, strict `specialist_lifecycle.v1`, Provider retry/cancellation, Policy, durable child Session history, and CompletionReport finish. It offers no tools and cannot grant Shell, network, credential, or spawn authority.

Schema v27 adds recoverable Specialist input context. A direct root parent may send a strict `specialist_instruction.v1` message through the internal Coordinator. One AgentAttempt prepares at most four sequence-ordered instructions and consumes them only when `continue` or `finish` commits; crash, interruption, and lease takeover leave them pending for the next attempt. The request also selects active WorkItems owned by the child and active `run`/`owner` Notes owned by and visible to that child under a 4,096-token estimate and 32 KiB cap. Message IDs remain audit-only, and `model.started` contains source IDs/token estimates rather than instruction or Note bodies. There is still no public/model spawn or autonomous/concurrent child scheduler.

Schema v28 gives each Specialist Attempt one isolated lifecycle-protocol repair. Primary and repair phases have independent transport retry counters but share one contiguous global model sequence and cumulative Attempt usage. Invalid primary output never enters the repair prompt, Session, or events. A second invalid response exhausts repair; cancellation, budget exhaustion, crash, interruption, and takeover abort it before the Attempt terminates.

The Go-internal `SpecialistScheduler` may run at most two explicitly selected ready children per round under one Run execution lease and stops within 32 rounds. Parent cancellation, heartbeat loss, or the first child error fans cancellation out to the active sibling, and the scheduler waits for durable Attempt terminal state before releasing the lease. Root and child token/model-time totals are rebuilt from SQLite before and after every round. At the schema v29 boundary there was no CLI, HTTP, or model path that admitted or started a schedule; schema v38 later adds only the explicit operator CLI gate described below.

Schema v29 persists schedule boundaries and exact child cancellation. A schedule writes metadata-only `agent.schedule_started/stopped` events plus an immutable terminal summary; a later lease generation marks an orphaned running schedule `abandoned/worker_lost`. The optional control API can cancel one already-started child call at `/api/v1/runs/{run_id}/agents/{agent_id}/active-call/cancel`, with strict AgentAttempt/model identity and a digest-only idempotency ledger. Only the worker holding the Attempt lease observes the request and cancels its own Provider context. Responses and events expose no raw key, model text, lease id, or fencing generation. In a concurrent scheduler round, the selected child's cancellation error may activate the scheduler's existing local sibling fan-out, but no sibling control request is fabricated.

Schema v30 lets the root model call `specialist_delegation_propose` with one or two strict `specialist_delegation.v1` assignments. Proposal creation is additive and review-gated: Go validates the active root turn and lease, trusted Run/Session/Workspace scope, parent-Skill subsets, child capacity, and suggested turn/token headroom, then stores only a redacted immutable `proposed` record. The result always reports `admission_authorized=false`; no child Agent, Session, budget reservation, or schedule is created. Operators inspect proposals with `run delegations <run-id>` and `run delegation <proposal-id>`.

Schema v31 adds explicit operator review without adding execution authority. `run delegation approve` records one immutable approval only while the Run is still running; `run delegation reject` requires a reason and may close a proposal even after the Run is terminal. Both commands require a 16-256-byte stable operation key and default the reviewer identity to `cli_operator`. The Store redacts reasons, keeps them out of Run events, hashes operation keys, rejects changed-intent replay and second decisions, and emits one metadata-only `agent.delegation_reviewed`. Every result still reports `admission_authorized=false` and `application_required=true`; review does not create or schedule a child.

Schema v32 adds `run delegation apply` as the only current operator application entry point. The operator must match the approved review identity and provide a stable 16-256-byte operation key. Before creating state, Go reruns Policy and verifies the immutable review operation, running Run, ready root, active Session, idle child runtime, parent Skills, default limits of two children/eight turns/16,384 tokens per child, current capacity, and aggregate root headroom. It then admits each child and sends its strict parent instruction through deterministic internal keys. A restart after either write safely replays the existing Agent/Message. Applying blocks root turns, unrelated admission/messages, and child scheduling; terminal Run transitions abort the application. Applied children remain `ready`, with `scheduling_started=false`.

Schema v38 adds the separate execution entry point. `run delegation schedule <proposal-id>` and its `continue` alias require the same application operator plus a stable 16-256-byte operation key. With no `--agent`, all instructed ready assignments are selected; repeat `--agent` to choose one or two exact assignment Agent IDs. `--max-rounds` defaults to one and is bounded by 32, while each child still obeys its own reserved turn/token budget and the shared Run token/model-time budget. Reusing a key and identical intent returns the durable terminal schedule without another model call; changing targets or rounds under that key conflicts. Another continuation requires a new key. A crash after request or schedule start is recovered through the same immutable request and a higher fenced attempt ordinal. These commands grant no tools, Shell, file write, process, network, recursive delegation, or child-count expansion, and there is no HTTP/model/ordinary-tool equivalent.

Schema v33 adds planning-only read-only Fan-out. `run fanout plan` requires a running Run whose Mission and active Session bind the same local workspace and whose network mode is disabled. Tiers `auto/1/2/4/6` are concurrency caps for independent analysis shards, not Agent admission limits. The scanner walks one workspace-relative directory without following symlinks, excludes VCS/dependency/build directories, binary and secret-like files, and stores only a bounded immutable path/hash manifest. Policy and `readonly_fanout.planned` events contain metadata but no goal, file path, or local root. `run fanouts` and `run fanout show` inspect the immutable plan; planning itself performs no Provider call.

Schema v34 adds `run fanout execute <plan-id>`. Execution requires the same plan operator and a stable operation key. Go acquires the Run lease, rebuilds the manifest, then verifies every file identity, size, and hash before making any Provider call. Source content is redacted and held only in memory. Each shard gets a tool-free JSON-mode request and must return strict `readonly_fanout_report.v1`; findings outside the shard are rejected. The planned 1/2/4/6 calls run concurrently, first failure cancels siblings, and every terminal state is persisted before the lease is released. `run fanout execution <execution-id>` displays the durable result. A repeated operation key and intent returns the same execution without another model call. Root, Specialist, and Fan-out token/model-time usage share the Run budget. Crash-uncertain calls retain their reserved charge and a newer lease retries only incomplete shards. This path still creates no Agent, Attempt, schedule, tool, file edit, process, network request, or automatic source change.

Schema v35 adds `run fanout report <execution-id> --format markdown|json`. It accepts only a completed execution and performs no Provider call. Go groups only source assertions with identical severity/category/title/detail/path/line facts, retains every source row as immutable `model_assertion` Evidence, and uses the minimum claimed confidence for an exact duplicate group. Every projected Finding is `draft`; report generation does not validate a vulnerability. The `building -> generated` SQLite transaction checks source bindings, contiguous ordinals, counts, and severity totals before making the projection immutable. Repeating the command or using `report show <report-id>` renders the same persisted projection byte for byte.

Schema v36 adds an explicit operator validation workflow. First create or approve a tool output Artifact in the same Run. `report finding attach` rereads and verifies the complete Artifact before recording immutable Evidence. `report finding validate` requires at least one attached Artifact; `report finding reject` may be used with no Artifact when the model assertion cannot be reproduced. A Finding can receive only one decision, and no Evidence can be attached afterward. Reusing the same operation key and intent returns the original row; changed intent or a second decision conflicts. `report finding verify` revalidates every Artifact blob and the decision's ordered Evidence digest. Notes and reasons are redacted and excluded from Run events. These commands do not mark a Finding accepted or fixed.

Schema v37 adds an independent operator remediation workflow. `report finding accept` requires an existing `validated` decision and freezes its ID, Evidence count, and digest; it does not reuse validation as implicit acceptance. Create or approve a new same-Run tool output only after acceptance, then attach it with `report finding remediation attach`. The Store compares durable Run-event sequence numbers, so an Artifact created before `finding.accepted` is rejected even if the system clock moved backward. A validation Artifact cannot be reused. `report finding fix` requires at least one fresh remediation Evidence record and freezes the ordered remediation set. Acceptance, remediation Evidence, and fix facts are immutable and replay-safe. `report finding verify` now validates both Artifact sets and every frozen snapshot.

`report show <report-id> --format sarif` is a deterministic read-only SARIF 2.1.0 projection: it performs no Store mutation or Provider call, emits stable severity rules, workspace-relative percent-encoded paths, and the v35 Finding fingerprint, and excludes validation, acceptance, remediation, and fix narratives plus Artifact content. Only confirmed unresolved `validated` and `accepted` Findings appear in `results`. `cyberagentValidationStatus` remains `validated` for both, while `cyberagentFindingStatus` distinguishes their lifecycle. Draft, fixed, and rejected counts remain in Run properties but are not emitted as results.

Use `report check <report-id>` as the scriptable CI gate. Its default policy is `--fail-status validated --min-severity high`; a match prints the complete text or JSON result and returns the stable `FAILED_PRECONDITION` CLI exit code 4. This policy includes both validated and accepted unresolved Findings. `--fail-status active` additionally includes draft, while `--fail-status none` disables failure. Fixed and rejected Findings never match. The command reads persisted lifecycle facts only.

Use `report check <report-id> --format github` inside GitHub Actions to emit official [workflow-command annotations](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-commands) before the same gate exit. The in-memory `GateResult` owns the exact matched Finding snapshots, while its JSON representation remains the existing count-only contract. Source severity maps to `notice` for info/low, `warning` for medium, and `error` for high/critical. File, line, endLine, title, status, category, Finding ID, and fingerprint are deterministic; command data and properties follow GitHub Toolkit escaping, while other C0/DEL controls become visible `\u00XX` text, so model output cannot create another command or manipulate terminal presentation. A passing or disabled gate emits no annotation. Artifact bodies, validation/remediation Evidence notes, and operator narratives are excluded. Other CI-platform adapters remain separate future renderers.

`run execute` repeats that same durable step up to `--max-steps`; it stops immediately on root `finish` or `wait`. `--finish` remains an explicit operator fallback after a normal step limit and cannot complete a waiting Run. `run finish` and `run fail` atomically update the Run, Supervisor checkpoint, and event stream. Repeating the same terminal command or replaying the same committed lifecycle action is idempotent, while a conflicting terminal transition is rejected.

Schema v17 serializes execution across processes with a durable Run lease. `run step` acquires one lease for one turn; `run execute` holds one lease across all steps in that invocation. The Supervisor renews the lease during long Provider calls. After expiry, a new worker takes over with a higher generation and the old checkpoint token can no longer append model/tool events, charge tool budget, or mutate WorkItems/Notes. An active competing worker returns `CONFLICT`/CLI exit 4; no manual lock cleanup is required because expired leases are recoverable. `run lease <run-id>` shows owner, generation, status, activity, and timestamps but intentionally omits the internal fencing token.

Before each model call, the Supervisor passes the remaining token allowance as the request limit and applies the remaining persisted model-execution deadline. Its Context Builder considers the prepared root inbox batch, latest compacted summary, at most 20 active WorkItems, and at most 100 active Notes visible to the root Agent. It selects those structured sections under a separate 8,192-token estimate, requires every prepared inbox item to fit, keeps Work Board JSON under 16 KiB, and truncates individual Note/inbox fields. `model.started` persists only included/omitted source IDs and token estimates, never Note or inbox bodies. A model `finish` action is sent through the existing one-repair protocol when active work remains; `run finish` remains an explicit operator override. Provider-reported usage is authoritative: if one call exceeds the remaining token allowance, its full actual usage is committed and subsequent calls are blocked. `MaxToolCalls` is now enforced by the Tool Gateway with an atomic SQLite ledger; `MaxCostUSD` remains configuration-only until provider pricing metadata is available.

Provider failures are normalized as `retryable`, `rate_limited`, `invalid_response`, `cancelled`, or `permanent`. RunSupervisor retries only retryable transport/capacity outcomes, with three attempts per protocol phase by default, 100 ms exponential backoff, and a 2 second local wait ceiling. A server `Retry-After` longer than that ceiling is not shortened: the turn returns a rate-limit error and keeps its pending input for a later `run step`. Invalid lifecycle JSON is not transport-retried; instead, it receives exactly one explicit repair phase with its own transport counter. Authentication/configuration failures, policy denial, and tool calls are not repaired.

Every call attempt emits `model.started` and then `model.completed` or `model.failed` in `run events`. RunSupervisor always consumes the Provider stream interface. It reconstructs UTF-8 split across chunks, limits the complete response to 64 KiB, requires a final chunk with valid usage, and routes mid-stream failures through the same retry and protocol-repair path. Global attempt numbers continue across protocol phases and process restart; phase-local transport numbers reset for the one repair phase.

During an attempt, `run events` may contain at most 32 ordered `model.delta` records. These events contain only chunk and byte counters, sequence, and completion state; they never store model text. The terminal event must match those counters. Model terminal events, token usage, and `execution_millis` commit together, and replaying the same terminal event cannot double-charge the budget. Repair transitions emit `supervisor.protocol_repair_requested/started/completed/failed`. The raw invalid output is never copied into the repair prompt, Session, or event payload. `run step` prints `model_attempts`, `protocol_repairs`, `model_outcome`, `stream_events`, and `stream_bytes`. Exhausted transient retries return unavailable exit code 6, rate limits return resource-exhausted exit code 8, cancellation returns 7, and deadline expiration returns 9.

The application service exposes in-process active-call query, bounded metadata subscription, and idempotent cancellation operations. An explicit application cancellation first appends a redacted `model.cancel_requested` event and then signals the Go-owned Provider context. Subscribers receive no raw model text and are disconnected if their 32-event buffer fills. Bubble Tea consumes this interface through an adapter. Schema v18 extends cancellation across processes through a durable, exact-attempt request: a separately authorized API process records intent, and only the worker holding the private execution lease can observe it and cancel its local Provider context. Request, observation, and terminal resolution are audited without exposing the registry or fencing token.

The `cyberagent` process handles `Ctrl+C` and termination signals through its command context. An interrupted Provider call records `model.failed` with a cancelled outcome and keeps the started Supervisor checkpoint recoverable instead of abandoning an unaccounted request.

Ordinary text sent to a Run-created Session uses the same RunSupervisor path as `run step`. The first message automatically starts a `created` Run, a follow-up to a model `wait` automatically resumes its paused Run, and the CLI prints `[run <id>: action=<action> status=<status>]`. Pending input is redacted, limited to 64 KiB, and stored before the Provider call; after restart the same attempt can recover it, while the committed user/assistant pair and lifecycle events are written exactly once. Completed, failed, cancelled, or approval-waiting Runs reject ordinary input instead of falling back to an unsupervised model call.

`run adapt-task` converts a v0.1 `agent.Task` into a new Mission, Run, and Session. The mapping is transactional and keyed by Task ID, so repeated or concurrent calls return the same Run and append only one `legacy.task_adapted` event. Historical task status is recorded for audit, but the new Run always starts at `created` and never executes implicitly. Legacy CTF tasks map to the safe generic `review` profile until the dedicated CTF phase.

CLI errors keep their existing text and use stable exit codes documented in [errors.md](errors.md).

Supported profiles are `code`, `review`, `learn`, and `script`. New runs start with network access disabled. Budget flags reject negative values and include maximum turns, tokens, model cost, and wall-clock timeout.

## Local HTTP API

```powershell
$env:CYBERAGENT_API_TOKEN = "<a-random-token-of-at-least-32-bytes>"
$env:CYBERAGENT_API_CONTROL_TOKEN = "<a-different-random-token-of-at-least-32-bytes>" # optional
cyberagent api serve --listen 127.0.0.1:8765
cyberagent api openapi --output docs/openapi.json
curl.exe -N -H "Authorization: Bearer $env:CYBERAGENT_API_TOKEN" http://127.0.0.1:8765/api/v1/runs/<run-id>/events/stream
```

`api serve` exposes authenticated, bodyless `GET` routes under `/api/v1` for durable Runs, Sessions, events, a bounded resumable SSE Run-event projection, WorkItems, Notes, Artifact metadata, Supervisor tool rounds, token-free execution-lease status, and the raw OpenAPI 3.1 document. The listener, request Host, and client must all be loopback. The optional schema v18 root and schema v29 Specialist cancellation POST routes are disabled until `CYBERAGENT_API_CONTROL_TOKEN` is set; that token must differ from the read token and cannot authorize GET. Both POST routes require exact attempt identity, strict JSON, and `Idempotency-Key`, and never accept a fencing token. There is no CORS, Artifact-content route, checkpoint pending input, user-visible model stream, tool execution, or general mutation API. `api openapi` deterministically exports the same Go-generated contract without opening SQLite or reading a token. Neither process token is stored. See [http-api.md](http-api.md) for the complete contract.

## Work Board

```powershell
cyberagent todo create <run-id> "inspect parser" --priority high --owner reviewer
cyberagent todo create <run-id> "root-owned plan" --owner-agent <agent-id>
cyberagent todo create <run-id> "write tests" --depends-on <work-id> --acceptance "tests pass"
cyberagent todo list <run-id>
cyberagent todo list <run-id> --status pending,blocked --owner reviewer
cyberagent todo list <run-id> --owner-agent <agent-id>
cyberagent todo show <work-id>
cyberagent todo update <work-id> --description "cover malformed input" --version 1
cyberagent todo update <work-id> --owner-agent <agent-id> --version 1
cyberagent todo update <work-id> --clear-dependencies
cyberagent todo start <work-id>
cyberagent todo block <work-id> --reason "waiting for fixture"
cyberagent todo reopen <work-id>
cyberagent todo complete <work-id>
cyberagent todo cancel <work-id>
```

WorkItems belong to exactly one Run. `--owner` remains a free-form compatibility label; `--owner-agent` is an authoritative reference to a nonterminal Agent in that same Run, and both may coexist. Dependencies must already exist in that same Run, cannot form a cycle, and must be completed before a dependent item starts or completes. Statuses are `pending`, `in_progress`, `blocked`, `completed`, and `cancelled`; priorities are `low`, `normal`, `high`, and `critical`. Blocked items require a reason, while completed and cancelled items are terminal.

Every item starts at version 1. Mutation commands accept optional `--version <n>` optimistic locking; omitting it uses the version read immediately before the transaction, while an explicit stale value returns conflict exit code 4. `--acceptance` and `--depends-on` may be repeated. `--clear-acceptance` and `--clear-dependencies` replace those lists with empty values. WorkItem records and `work_item.created/changed` Run events commit atomically, and terminal Runs reject later board mutation.

## Notes

```powershell
cyberagent note create <run-id> "parser decision" --content "Use strict JSON" --category decision --pin
cyberagent note create <run-id> "fixture evidence" --content-file C:\temp\note.txt --tag parser --source docs/spec.md --evidence evidence-1
cyberagent note create <run-id> "root summary" --content "Current root-only state" --visibility root
cyberagent note create <run-id> "specialist memory" --content "Private context" --visibility owner --owner specialist
cyberagent note create <run-id> "Agent memory" --content "Private context" --visibility owner --owner-agent <agent-id>
cyberagent note list <run-id> --status active --category decision,summary --tag parser
cyberagent note list <run-id> --visibility root --pinned true
cyberagent note list <run-id> --owner-agent <agent-id>
cyberagent note show <note-id>
cyberagent note update <note-id> --content "Revised decision" --version 1
cyberagent note update <note-id> --owner-agent <agent-id> --version 1
cyberagent note update <note-id> --clear-tags --unpin
cyberagent note archive <note-id>
cyberagent note restore <note-id>
```

Categories are `observation`, `hypothesis`, `decision`, `summary`, and `reference`. Visibility is `run`, `root`, or `owner`; owner-visible Notes require either a compatibility owner label or a validated same-Run Agent. When only `--owner-agent` is supplied for an owner-visible Note, its Agent ID is mirrored into the legacy label for old-reader and schema compatibility. The root Supervisor receives run-visible, root-visible, legacy `owner=root`, and root-Agent-owned Notes, while owner-only Specialist Notes remain excluded. Operators can still inspect all Notes through the CLI.

Each Note has normalized tags, source references, Evidence IDs, pinned state, active/archived lifecycle, and an optimistic version. `--tag`, `--source`, and `--evidence` may be repeated; update commands replace those lists or clear them explicitly. Archived Notes remain durable but cannot be edited or selected until restored. Terminal Runs reject later Note mutation.

`--content-file` reads valid UTF-8 through a bounded reader and rejects content over 64 KiB even if the file changes while being read. Content, titles, tags, references, Evidence IDs, event payloads, and model context pass through the redaction boundary. Note records and `note.created/changed` events commit together. Models receive selected Notes as untrusted `note_context.v1` JSON and may create a Note through the bounded RunSupervisor tool loop.

## Structured Memory Tools

`work-item.json`:

```json
{"title":"Inspect parser","description":"Use strict JSON","priority":"high","acceptance_criteria":["tests pass"]}
```

`note.json`:

```json
{"title":"Parser decision","content":"Use strict JSON","category":"decision","visibility":"root","pinned":true}
```

```powershell
cyberagent tool schema
cyberagent tool schema work_item_create
cyberagent tool schema note_create
cyberagent tool schema specialist_delegation_propose
cyberagent tool invoke work_item_create --run <run-id> --operation-key <stable-key> --payload-file .\work-item.json
cyberagent tool invoke note_create --run <run-id> --operation-key <stable-key> --payload-file .\note.json
cyberagent run delegations <run-id>
cyberagent run delegation <proposal-id>
cyberagent run delegation approve <proposal-id> --operation-key <stable-key> [--reviewer cli_operator] [--reason "bounded and in scope"]
cyberagent run delegation reject <proposal-id> --operation-key <stable-key> [--reviewer cli_operator] --reason "outside authorized scope"
cyberagent run delegation apply <proposal-id> --operation-key <stable-key> [--operator cli_operator]
cyberagent run delegation schedule <proposal-id> --operation-key <stable-key> [--operator cli_operator] [--max-rounds 1] [--agent <agent-id>]
cyberagent run delegation continue <proposal-id> --operation-key <new-stable-key> [--operator cli_operator] [--max-rounds 1] [--agent <agent-id>]
cyberagent run fanout plan <run-id> "audit source modules" --operation-key <stable-key> [--tier auto|1|2|4|6] [--path <dir>] [--operator cli_operator]
cyberagent run fanouts <run-id> [--limit 20]
cyberagent run fanout show <plan-id>
cyberagent run fanout execute <plan-id> --operation-key <stable-key> [--operator cli_operator] [--max-output-tokens 1024]
cyberagent run fanout execution <execution-id>
cyberagent run fanout report <execution-id> [--format markdown|json]
cyberagent report show <report-id> [--format markdown|json|sarif]
cyberagent report check <report-id> [--fail-status validated|active|none] [--min-severity info|low|medium|high|critical] [--format text|json|github]
cyberagent report check <report-id> --format github [--fail-status validated|active|none] [--min-severity info|low|medium|high|critical]
cyberagent report finding attach <finding-id> <artifact-id> --operation-key <stable-key> --note <text> [--operator cli_operator]
cyberagent report finding validate <finding-id> --operation-key <stable-key> --reason <text> [--operator cli_operator]
cyberagent report finding reject <finding-id> --operation-key <stable-key> --reason <text> [--operator cli_operator]
cyberagent report finding accept <finding-id> --operation-key <stable-key> --reason <text> [--operator cli_operator]
cyberagent report finding remediation attach <finding-id> <fresh-artifact-id> --operation-key <stable-key> --note <text> [--operator cli_operator]
cyberagent report finding fix <finding-id> --operation-key <stable-key> --reason <text> [--operator cli_operator]
cyberagent report finding verify <finding-id>
```

`work_item_create` creates one pending WorkItem; `note_create` creates one active Note. They accept strict JSON with unknown fields and trailing data rejected before budget charging. The Run must already have an attached Session, and the CLI derives Session/Workspace scope from persisted Run state instead of accepting caller-supplied scope. `--payload` is also supported, while `--payload-file` avoids native-shell JSON quoting differences and is bounded to 96 KiB of valid UTF-8.

An operation key is mandatory and should remain stable across retries. The raw key is never persisted: schema v15 stores a domain-separated SHA-256 digest and a fingerprint of the normalized, redacted intent. Repeating the same tool, Run, key, and intent returns the original entity with `replayed: true`; changing intent under the same key returns conflict exit code 4. Replay, conflict, authoritative scope mismatch, and Policy-denied attempts each consume a tool-call budget entry because they are well-formed invocations. Malformed JSON, unknown fields, missing identities, and invalid field values are rejected before charging. Successful creation commits the entity, Policy decision, domain event, `tool.completed`, and operation ledger atomically. A failed event write leaves no entity or operation row.

The WorkItem and Note tools are create-only and return metadata rather than content. RunSupervisor advertises those two definitions plus `specialist_delegation_propose`: a Provider response may request at most four calls and one turn may perform at most four tool rounds. The model response and pending batch are committed together; after restart, unfinished calls are safely replayed through the semantic operation ledger and their terminal metadata is returned to the Provider. Anthropic-compatible transports encode this as `tool_use` and `tool_result`. Policy denial, invalid delegation capability requests, and budget exhaustion are returned as bounded error results, while protocol repair exposes no tools.

`specialist_delegation_propose` is Supervisor-only and cannot be invoked through `tool invoke`. Its strict payload is `{"version":"specialist_delegation.v1","assignments":[{"title":"Review parser","goal":"Inspect parser boundaries","skills":["model.chat"],"turn_limit":2,"token_limit":256}]}`. Unknown fields, more than two assignments, duplicate goals, unavailable or non-delegable Skills, stale leases, insufficient child capacity, and proposals that do not leave root budget headroom are rejected. Repeating the same redacted semantic intent returns the original proposal ID; the raw operation key and Provider call ID are never persisted. CLI output may show the redacted goals, independent review, and application state, while Run events contain only proposal identity, counts, suggested aggregate budgets, review/application metadata, and authorization phase flags. Approval/rejection has no Provider tool definition; application is operator-only and calls admission plus strict instruction delivery but never starts the scheduler.

Update, completion, archive, Shell, file, process, network, and other Provider-driven tools remain disabled pending separate lifecycle, approval, and Sandbox audits. Use the ordinary `todo` and `note` commands for operator-controlled updates.

## Sandbox Manifest

```powershell
cyberagent sandbox template
cyberagent sandbox validate configs/sandbox-manifest.example.json
cyberagent run sandbox prepare <run-id> --manifest configs/sandbox-manifest.example.json --operation-key sandbox-prepare-001
cyberagent run sandbox list <run-id>
cyberagent run sandbox show <preparation-id>
cyberagent run sandbox request <preparation-id> --operator cli_operator
cyberagent run sandbox review <preparation-id> --decision approve --operation-key sandbox-review-001 --reviewer security_operator
cyberagent run sandbox candidate <preparation-id> --manifest configs/sandbox-manifest.example.json --approval <approval-id> --operation-key sandbox-candidate-001
cyberagent run sandbox candidates <run-id>
cyberagent run sandbox candidate-show <candidate-id>
cyberagent run sandbox begin <candidate-id> --manifest configs/sandbox-manifest.example.json --operation-key sandbox-begin-001
cyberagent run sandbox executions <run-id>
cyberagent run sandbox execution-show <execution-id>
cyberagent run sandbox preflight <execution-id> --manifest configs/sandbox-manifest.example.json --operation-key sandbox-preflight-001
cyberagent run sandbox preflights <run-id>
cyberagent run sandbox preflight-show <preflight-id>
cyberagent run sandbox cancel <execution-id> --operation-key sandbox-cancel-001
cyberagent run sandbox cleanup <execution-id> --operation-key sandbox-cleanup-001
```

`sandbox validate` performs strict duplicate-aware `sandbox_manifest.v1` decoding and deterministic Noop validation without opening the runtime database. `run sandbox prepare` requires a Run whose Mission has a persisted Workspace, then binds the normalized Manifest fingerprint to that exact Run/Mission/Workspace root, Mission Scope, current Policy result, optional exact approval, requester, and a Go-generated cancellation identity. Operation keys are normalized 16-256 byte client identities; SQLite stores only their domain-separated digest.

The preparation and validation ledgers contain counts, limits, fingerprints, status, and binding identities only. Executable, argv, mount/output paths, environment values, secret references, network targets, and Manifest JSON are not stored or emitted in events. Network allowlists may only narrow a Mission allowlist. Docker/Local intent, writable mounts, network, or secret references require approval when Policy allows them, while permanent Policy denial is recorded and cannot be overridden.

Schema v49 uses the shared approval ledger rather than a Sandbox-specific bypass. `request` derives one pending approval from the preparation's exact authorization fingerprint, and `review` records an immutable operator decision. `candidate` must resupply and renormalize the complete Manifest. It rejects fingerprint, Workspace root, Mission Scope, Policy, or approval drift; resolves every mount source through Go `os.Root`; and rechecks aggregate token/model-time usage, tool-call budget, and the absence of an active Run execution lease in the candidate write transaction. Operation keys are digest-only and cross-process retries converge.

Schema v49 is still not an execution API. Candidate rows and events contain only bounded metadata and fix `backend_enabled=false` plus `execution_authorized=false`; Local and Docker remain fail-closed and no host/container process starts. Future execution must revalidate again and pass separate cancellation, cleanup, network, secret-materialization, host-path isolation, and Artifact export audits.

Schema v50 adds a disabled lifecycle, not process execution. `begin` resupplies the complete Manifest and rechecks the candidate, Run/Mission/Workspace/Scope, current Policy and approval, mount binding, aggregate budgets, Run lease, and every input Artifact. Inputs must belong to the exact Run/Session/Workspace, pass content SHA-256 verification, retain their order/source/MIME/stream metadata, and total at most 16 MiB. The output plan stores only stdout/stderr flags, output-path count, maximum bytes, and a fingerprint; raw paths are not retained.

The lifecycle owns a separate generation-fenced lease. Generation one only prepares the disabled record and is released immediately. `cancel` appends an immutable request, while `cleanup` may run after the parent Run is terminal and acquires a successor generation. The current cleanup outcome is always `backend_disabled`: no backend started, no orphan existed, all inputs were reverified, and zero output Artifacts were captured. CLI detail intentionally omits the lease ID and owner as well as Manifest, command, path, and Artifact content. Reusing the same operation key and intent is safe; changed intent conflicts.

Schema v51 adds a separate disabled preflight after `begin` and before any future backend action. `preflight` resupplies the complete Manifest and revalidates the full v48-v50 chain, current Policy/approval/Scope, mounts, cumulative budgets, Run-lease quiescence, and exact input Artifacts. It records a fixed 16-item backend threat model, but every check remains required, unverified, and not probed. The backend handshake reports unavailable, container identity is unbound, and all backend/execution/export/Artifact-commit flags remain false.

The output plan stores only opaque locator fingerprints plus `stdout`, `stderr`, or `file` kinds. File slots require regular files and reject symlinks and special files; every slot requires MIME detection and redaction. Export is all-or-nothing under one aggregate byte ceiling and must reconcile before retry. CLI detail deliberately omits locator fingerprints, raw paths, command/Manifest content, container identity, and private lease data. A successful disabled preflight proves only that the intended checks are frozen and the current authority chain still matches; it does not prove Docker availability and cannot authorize execution.

Schema v52 provides a simulation-only continuation for protocol testing. Start the complete `prepare -> request -> review -> candidate -> begin -> preflight` chain with `configs/sandbox-docker-simulation.example.json`, then use the resulting preflight ID:

```powershell
cyberagent run sandbox evidence <preflight-id> --manifest configs/sandbox-docker-simulation.example.json --image-digest sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee --operation-key sandbox-evidence-001
cyberagent run sandbox evidences <run-id>
cyberagent run sandbox evidence-show <evidence-id>
cyberagent run sandbox output-simulate <evidence-id> --manifest configs/sandbox-docker-simulation.example.json --fixture configs/sandbox-output-fixture.example.json --operation-key sandbox-output-simulation-001
cyberagent run sandbox output-simulations <run-id>
cyberagent run sandbox output-simulation-show <simulation-id>
cyberagent run sandbox observe <evidence-id> --simulation <simulation-id> --manifest configs/sandbox-docker-simulation.example.json --operation-key sandbox-docker-observe-001 --confirm-readonly-probe
cyberagent run sandbox observations <run-id>
cyberagent run sandbox observation-show <observation-id>
cyberagent run sandbox docker-plan <observation-id> --manifest configs/sandbox-docker-simulation.example.json --operation-key sandbox-docker-plan-001 --confirm-fake-write
cyberagent run sandbox docker-plans <run-id>
cyberagent run sandbox docker-plan-show <plan-id>
cyberagent run sandbox docker-rehearse <plan-id> --manifest configs/sandbox-docker-simulation.example.json --operation-key sandbox-docker-rehearsal-001 --confirm-daemon-write --stage-host-inputs --confirm-host-input-staging --handoff-host-inputs --confirm-host-input-handoff
cyberagent run sandbox docker-rehearsals <run-id>
cyberagent run sandbox docker-rehearsal-show <rehearsal-id>
cyberagent run sandbox docker-attempts <run-id>
cyberagent run sandbox docker-attempt-show <attempt-id>
cyberagent run sandbox docker-attempt-resume <attempt-id> --manifest configs/sandbox-docker-simulation.example.json --confirm-daemon-write --stage-host-inputs --confirm-host-input-staging --handoff-host-inputs --confirm-host-input-handoff
cyberagent run sandbox docker-host-inputs <run-id>
cyberagent run sandbox docker-host-input-show <intent-id>
cyberagent run sandbox docker-host-input-handoffs <run-id>
cyberagent run sandbox docker-host-input-handoff-show <handoff-intent-id>
cyberagent run sandbox docker-runtime-input-plan <handoff-intent-id> --manifest configs/sandbox-docker-simulation.example.json --operation-key runtime-input-plan-001 --confirm-runtime-input-plan
cyberagent run sandbox docker-runtime-input-plans <run-id>
cyberagent run sandbox docker-runtime-input-plan-show <projection-id>
cyberagent run sandbox docker-runtime-input-apply <projection-id> --manifest configs/sandbox-docker-simulation.example.json --operation-key runtime-input-apply-001 --confirm-runtime-input-apply --confirm-daemon-write
cyberagent run sandbox docker-runtime-input-apply-resume <application-intent-id> --manifest configs/sandbox-docker-simulation.example.json --confirm-runtime-input-apply --confirm-daemon-write
cyberagent run sandbox docker-runtime-input-applications <run-id>
cyberagent run sandbox docker-runtime-input-application-show <application-intent-id>
```

`evidence` never contacts a daemon. It binds a canonical OCI image digest and separate simulated daemon/mount/network/secret/container/resource/termination/orphan/output fingerprints to the 16 v51 checks, but reports `trust_class=simulation_only`, `production_verified=0`, and `verified_checks=0`. `output-simulate` strictly validates and redacts fixture content, stages all slots, and commits only to an in-memory fake sink. A failure or cancellation rolls the fake transaction back to zero, and no production Artifact is created. The Store and Application revalidate the complete v48-v51 chain at both boundaries. CLI and events omit fixture bodies, locator fingerprints, raw paths, commands, Manifest content, container IDs, operation digests, and private leases. These commands test protocol behavior only; they cannot create or start a Docker container and cannot authorize real execution.

Schema v53 `observe` is the only command in this chain that may contact a real daemon, and it requires the exact `--confirm-readonly-probe` flag. Before the probe, it resupplies the complete Manifest and binds the same v52 evidence and output simulation. Linux connects only to `/var/run/docker.sock`; Windows currently records `transport_unsupported`. `DOCKER_HOST`, arbitrary TCP endpoints, caller-selected sockets, redirects, proxying, the Docker CLI, image pulls, and every container mutation are excluded. The transport can issue only `GET /_ping`, `GET /version`, `GET /info`, and exact-digest image inspection.

An observation records `observation_complete`, `daemon_unavailable`, or `image_unavailable`. A complete observation may report `production_observed=true`, which means only that bounded daemon and image metadata were read. It does not mean `production_verified`, `backend_available`, `backend_enabled`, `execution_authorized`, or `artifact_commit_authorized`; all remain false. Private-mount support is printed as `not_observable_read_only` because these GETs cannot prove it. Raw daemon ID/name/root, socket, RepoDigests, Manifest, commands, operation keys, and private leases are neither persisted nor printed. Repeating the same operation returns the existing row without probing again, and one output simulation accepts at most eight observations.

Schema v54 `docker-plan` requires the exact `--confirm-fake-write` flag and accepts only `observation_complete`. It resupplies the Manifest and revalidates the entire v48-v53 chain before compiling a deterministic in-memory container specification. The compiler fixes non-root execution, read-only root and inputs, one writable output mount, private propagation, network default deny or exact managed allowlisting, ephemeral secrets, resource/time/kill limits, and orphan reconciliation identity. Sixteen plan controls remain `compiled_not_applied`, and the seven reconcile/create/start/wait/stop/export/remove steps run only in an in-memory fake transaction. Failure, simulated crash, or cancellation commits no fake result; success still reports zero daemon writes and no backend contact, execution, export, or production Artifact authority. Durable rows and CLI output omit commands, arguments, paths, network targets, environment values, secret references, labels, and container names. No v54 command mutates Docker.

Schema v55 `docker-rehearse` is the first command that may perform real Docker writes, so it is default-disabled in the Application service and requires exact `--confirm-daemon-write`. It accepts only a current v54 plan whose profile has no network, environment binding, or secret. Linux uses fixed `/var/run/docker.sock` and API `1.40`; Windows returns unsupported. The closed transport permits exact image/container inspection, create, and a non-forced delete with fixed anonymous-volume cleanup. It ignores `DOCKER_HOST` and has no TCP, caller socket, pull, start, exec, attach, logs, export, volume management, image build, or generic request operation.

Before create, the already-present image RepoDigest must match the plan and the image must declare no `VOLUME`. The transport creates one stopped digest-pinned container, verifies attachment/device/port settings plus non-root, read-only root, no-new-privileges, drop-all capabilities, resource limits, network none, and private mounts, then removes it. Cancellation, failure, and uncertain create responses use an independent bounded re-inspection and only delete an exact authority match. Same-key replay returns before transport access. A normal successful fact records three reads and two writes, or three writes after removing one exact stale rehearsal container. It still records process execution, image pull, output export, production verification, backend enablement, execution authority, and Artifact authority as false.

Schema v56 makes that never-started rehearsal recoverable. `docker-rehearse` now persists an attempt and acquires a generation-fenced lease before contacting Docker. After create, it stores a 19-item inspected-configuration matrix before cleanup; every item reports `execution_evidence=false`. A crash or uncertain response can be resumed with the durable attempt ID. `docker-attempt-resume` requires the original complete Manifest and a fresh `--confirm-daemon-write`, recomputes the full v48-v54 authority chain, and refuses any changed intent or requester. It adopts only an exact stopped authority match, accepts already-absent cleanup, and never deletes a mismatched same-name container. The raw operation key is not required for recovery and is not printed or stored.

`docker-attempts` and `docker-attempt-show` expose bounded status, generation, timestamps, failure codes, and non-execution controls. They omit raw container IDs, host paths, commands, environment values, secrets, sockets, operation keys, and private lease owners. The inherited image/container environment must be empty, not merely absent from the Manifest. The fixed local endpoint and no-network/no-secret/no-start/no-exec/no-pull/no-export boundary are unchanged.

Schema v57 optionally adds `--stage-host-inputs`, which always requires a separate `--confirm-host-input-staging`. On Linux, the stager pins the workspace root and read-only mount trees with `openat2` no-symlink/no-magic-link/beneath/no-cross-device resolution. It uses `O_PATH` to reject FIFOs and other special files before a content open, supports both directory and single-file mounts, bounds directory enumeration before allocation, observes cancellation while reading files, rejects hard links and bounded-resource violations, rechecks descriptor metadata after the whole tree is pinned, then writes a deterministic sanitized tar to a sealable `memfd`. It applies write/grow/shrink/seal kernel seals and reads the bundle back to verify its digest. Input Artifacts are reloaded and reverified by exact Run, Session, Workspace, digest, size, MIME, stream, source, redaction state, and order immediately before staging.

The v57 intent is durable before bundle creation and is bound to the current v56 attempt, plan, stopped-container fingerprint, input digest, and lease generation. Generated row IDs are excluded from semantic fingerprints, so independent retries converge on one intent/result. SQL refuses final attempt completion until a matching result exists. A staging error triggers best-effort stopped-container cleanup and leaves a recoverable pending intent. Legacy attempts created on schema v57 retain their conservative compatibility behavior: resume must resubmit both staging flags, missing confirmation is rejected before lease acquisition, and no failure slot is consumed. `docker-host-inputs` and `docker-host-input-show` expose counts, digests, seals, and status only. They never expose source paths, content, descriptors, raw container IDs, or private lease identities.

The bundle is currently discarded after its sealed digest is verified and is not uploaded or mounted into Docker. Accordingly the durable result fixes `daemon_consumed=false`, `execution_evidence=false`, and every production/backend/execution/Artifact authority to false. v57 closes source replacement during descriptor capture, but a later independently audited daemon handoff is still required before any future start boundary. Windows returns the explicit `staging_unsupported` error before a container is created.

Schema v58 makes that staging choice durable for every new attempt. `docker-rehearse --stage-host-inputs --confirm-host-input-staging` stores one immutable host-input requirement in the same transaction as the attempt, initial lease, and audit events, before any daemon stage. The fact is bound to the plan, Run, Mission, Workspace, requester, operation digest, authority fingerprints, and bounded input counts. CLI list/show prints `host_input_required` and whether a durable requirement is present, but never paths, content, descriptors, raw container IDs, operation keys, or private lease identities.

For a v58 attempt, `docker-attempt-resume` still requires the full Manifest and `--confirm-daemon-write`, but it does not require the staging flags to be repeated. A durable `host_input_required=true` automatically resumes v57 capture before completion; explicitly resubmitting the two matching staging flags is accepted but cannot change the choice, while an unmatched flag pair is rejected before lease acquisition. A durable false choice cannot be widened into staging. Go and SQLite both reject completion without required evidence and reject staging against a false requirement. Existing v57 attempts are not backfilled because migration cannot safely invent historical operator intent. Their IDs are placed in an immutable migration-only compatibility set; inserts are disabled immediately afterward, so a new requirement-free attempt cannot use the legacy path.

Schema v58 does not add a Docker archive, volume, start, exec, pull, build, export, or Artifact endpoint. The sealed bundle remains local and `daemon_consumed=false`. A separately reviewed schema-v59 design must use a daemon-owned carrier, verify exact upload and readback bytes, remove the writable carrier, and recreate the never-started target with the verified carrier mounted read-only; making the target root or input writable is not an acceptable shortcut.

Schema v59 implements that handoff as a separate opt-in boundary. `--handoff-host-inputs` is valid only together with `--stage-host-inputs`, `--confirm-host-input-staging`, `--confirm-host-input-handoff`, and the existing `--confirm-daemon-write`. The immutable handoff requirement is created with the attempt, and a write-ahead intent commits before any archive or volume call. Resume may omit the staging and handoff flag pairs after those required choices are durable; submitting only part of either pair is rejected before lease acquisition, and a durable false choice cannot be widened.

Linux uses only the fixed local Unix socket and Docker API `1.40`. One deterministic, never-started carrier writes the sealed bytes to a daemon-owned local volume at `/cyberagent-input/bundle.tar`. Go reads that file back through Docker, verifies exact length and digest, removes the carrier and original stopped target, creates a never-started target with the volume read-only, verifies its complete configuration, then removes the target and volume. Manifest mounts may not overlap the reserved `/cyberagent-input` tree. Retry reconciles only exact request-owned residue; a foreign same-name container or volume is not modified. The target root and reviewed Manifest mounts never become writable.

`docker-host-input-handoffs` and `docker-host-input-handoff-show` expose status, bounded daemon read/write counts, generation, readback/readonly/cleanup flags, and fingerprints. They omit source paths, raw content, descriptors, raw container IDs, carrier/volume names, socket details, raw operation keys, and private lease identities. A successful record means only `daemon_consumed=true`, `readback_verified=true`, and cleanup completed. Container start, process execution, output export, backend enablement, execution authority, and Artifact commit authority remain false.

Schema v60 `docker-runtime-input-plan` separately confirms and recompiles the exact completed handoff into one canonical relative archive per reviewed directory-root input plus an optional fixed Artifact projection. It never contacts Docker. Schema v61 `docker-runtime-input-apply` then requires both `--confirm-runtime-input-apply` and `--confirm-daemon-write`. Go persists its intent and generation lease before recapture or daemon writes, revalidates v48-v60, writes each transient archive through a never-started carrier, verifies daemon readback, and retains one target with every input volume read-only/`NoCopy`. `apply-resume` reacquires only a released or expired intent and requires the full Manifest plus both confirmations; a completed operation returns metadata without contacting Docker.

Application list/show output includes only status, generation, bounded counts, fingerprints, verification flags, and false authority bits. It excludes targets, host paths, file/resource names, raw IDs, archives, socket details, raw operation keys, and lease identities. `volumes_applied_target_never_started` means the target and input volumes are prepared, not runnable. There is no `start`, process, export, backend, execution, or Artifact authority in v61, and Windows returns `application_unsupported`.

An optional real-daemon test is available only when an exact image is already present; it never pulls or creates anything:

```powershell
$env:CYBERAGENT_DOCKER_READONLY_INTEGRATION = "1"
$env:CYBERAGENT_DOCKER_READONLY_IMAGE_DIGEST = "sha256:<already-present-digest>"
go test ./internal/sandbox -run TestLocalDockerReadOnlyObservationIntegration -count=1 -v
```

The v55 write rehearsal has a separate opt-in Linux test. The supplied digest must already be present, match a RepoDigest, and declare no `VOLUME`; the test never pulls or starts it:

```powershell
$env:CYBERAGENT_DOCKER_WRITE_TEST_IMAGE_DIGEST = "sha256:<already-present-volume-free-digest>"
go test ./internal/sandbox -run TestDockerContainerWriteRealDaemonOptIn -count=1 -v
```

The same opt-in digest can exercise the schema-v59 archive/volume handoff. The image must also expose an empty inherited environment. The harness never pulls or starts a container and asserts that the target, carrier, and volume are all absent afterward:

```powershell
$env:CYBERAGENT_DOCKER_WRITE_TEST_IMAGE_DIGEST = "sha256:<already-present-volume-free-digest>"
go test ./internal/sandbox -run TestDockerHostInputHandoffRealDaemonOptIn -count=1 -v
```

Schema v61 has a separate end-to-end opt-in harness using the same already-present image constraints. It runs the v57 capture, v59 handoff, v60 projection, and v61 application chain, verifies the retained never-started target and read-only volumes, then removes every exact-owned resource. It never pulls or starts the container:

```powershell
$env:CYBERAGENT_DOCKER_WRITE_TEST_IMAGE_DIGEST = "sha256:<already-present-volume-free-digest>"
go test ./internal/sandbox -run TestDockerRuntimeInputApplicationRealDaemonOptIn -count=1 -v
```

## Workspaces

```powershell
cyberagent workspace init demo
cyberagent workspace list
cyberagent workspace show demo
cyberagent workspace tree demo
cyberagent workspace tree demo scripts --depth 2
cyberagent workspace read demo README.md
```

`workspace tree` and `workspace read` only accept paths relative to the selected workspace. Attempts to read outside the workspace, such as `../outside.txt`, are rejected. Text returned by `workspace read` is passed through secret redaction before printing.

## Script Mode

```powershell
cyberagent script new "parse pcap http token" --workspace demo
cyberagent script run scripts/<script-name>.py --workspace demo
cyberagent script run scripts/<script-name>.py --workspace demo --local --flag value
cyberagent script run scripts/<script-name>.py --workspace demo --idempotency-key <stable-key>
```

`script new` prints both the absolute artifact path and `script_relative`; pass the latter to `script run`. `script run` never executes a Sandbox or host process. It requires a workspace-relative existing file, rejects absolute paths/traversal/symlink escape, and atomically persists a Script Profile Mission/Run/Session, initial tool-budget charge, Policy decision, typed Process, Approval, and Run events. The `script_process.v1` payload contains executable, argv, workspace-root working directory, requested backend, and the fixed execution mode `disabled`.

`--idempotency-key` is optional but recommended for retryable clients. Repeating the same key and intent returns the original Mission/Run/Session/Process without a second budget charge or duplicate events. Reusing it with changed path, arguments, backend, scope, budget, or requester returns conflict exit code 4. Only a SHA-256 digest is stored. `--local` records intent for future Sandbox work; it does not execute locally. Use the printed Process ID with `tool show`, `tool approve`, or `tool deny`; approval completes as dry-run only.

## CTF Mode

```powershell
cyberagent ctf init baby-web --category web
cyberagent ctf analyze baby-web
cyberagent ctf writeup baby-web
```

## Model and Provider Commands

```powershell
cyberagent provider list
cyberagent provider test
cyberagent provider test mimo/mimo-v2.5-pro
cyberagent provider test deepseek/deepseek-v4-flash
cyberagent model list
cyberagent model set script mock/mock-code
```

`provider test` accepts either a route name, such as `learn`, or a direct `provider/model` reference. The optional `mimo` provider is registered from `MIMO_API_KEY`, `MIMO_BASE_URL`, and `MIMO_MODEL`. The optional `deepseek` provider uses `DEEPSEEK_API_KEY`, `DEEPSEEK_BASE_URL`, and `DEEPSEEK_MODEL`; its defaults are `https://api.deepseek.com/anthropic` and `deepseek-v4-flash`. Only the API key is required. Base URLs must be absolute HTTPS URLs unless they target an exact loopback host over HTTP; embedded credentials, query strings, fragments, and redirects are rejected. API keys are bounded normalized UTF-8 without whitespace or control characters.

## TUI

```powershell
cyberagent tui
cyberagent tui --workspace demo --title "Agent basics" --route learn
cyberagent tui --run <run-id>
cyberagent tui --run <run-id> --print
cyberagent tui --session <session-id>
cyberagent tui --session <session-id> --print
```

Without `--run` or `--session`, the TUI opens a Run-first picker backed by bounded Store pages. `Tab` or `h/l` switches between the latest 50 Runs and latest 50 Sessions. Press `Enter` to open the selected item, `n` to create and open a new Session, `j/k` to move, `r` to refresh, and `q` or `Esc` to quit. `--run` and `--session` are mutually exclusive; an exact Run open rechecks that its persisted Session resolves back to the same Run projection.

The chat TUI uses the same session and tool approval runtime as the CLI. Normal text sends a session message. Slash commands such as `/run echo hello`, `/model script`, and `/compact` go through the session manager. Tool approvals can be handled in the input box:

```text
/approve <tool-run-id>
/approve-session <tool-run-id>
/deny <tool-run-id> not needed
```

Keyboard controls:

```text
Tab              switch focus between input and the activity pane
Enter            send, approve a selected Tool, or open/close an Edit diff
PgUp / PgDn      scroll messages or an open diff
h / l            switch among Tools, Plan, Work, Notes, Rounds, Events, Agents, Findings, and Edits
j / k            select the next/previous item in the active view
a                approve the selected Shell proposal once
g                approve it and grant the exact current Session scope
d                deny the selected Shell proposal
Ctrl+R           refresh Session, Run, memory, and tool state
Ctrl+X           request audited cancellation of the current model call
Esc              return from diff detail, otherwise quit when idle
```

`--print` renders one snapshot and exits, which is useful for non-interactive verification.

Message sends, live-call discovery, refreshes, cancellation, and tool approval/deny actions run asynchronously. During a Run-bound model call, the status line shows provider/model, attempt, chunk/byte progress, cancellation, slow-consumer disconnect, and terminal state. `Ctrl+X` prefers the application audit-first cancellation API; if a legacy or not-yet-active request has no registry entry, it cancels the current application request context instead. Additional input is held until the current action finishes, and raw model text is never included in the live envelope.

For a Run-bound Session, the activity pane reads Plan/Delivery state, WorkItems, Notes, durable Supervisor ToolRounds, recent Run Events, Agent nodes/completions, bounded Finding-report summaries, active Shell grants, ToolRuns, and FileEdit previews from the Go Store. Plan shows the three directions, any selected direction and projected WorkItem count, and whether an explicit Deliver transition is still needed. Plan, Work, Notes, Rounds, Events, Agents, Findings, and Edits are read-only views; approval keys act only in Tools. Edits loads at most the latest 20 exact-Session/Workspace records through a metadata/diff-only SQL query that excludes `original_text` and `proposed_text`. `Enter` opens a full-screen read-only diff; `j/k` or `PgUp/PgDn` scrolls and `Enter`, `b`, or `Esc` returns. Displayed diff data is valid UTF-8, terminal-control neutralized, and capped at 128 KiB/4096 lines even though the durable proposal remains unchanged. `a` uses the existing durable per-call decision, while `g` creates or reuses a revocable Grant scoped to the exact Run, Session, Workspace, `shell` tool, and `shell` ActionClass. Keyboard and slash-command approval paths both reject ToolRun IDs outside the currently open Session. The current proposal is matched against its persisted fingerprint and rechecked by Policy before the Grant is created. Later allowed Shell calls may complete automatically as dry runs; Policy denial always wins.

The interactive TUI polls only the local Store and never starts a Run by polling. It reads at most 32 new events per batch, keeps the most recent 50 in the panel, validates contiguous sequence plus exact Run/Mission binding, and refreshes the composite Session/tool/Run/FileEdit projection only when durable events arrive. Each refresh compares the event tail before and after all reads and retries up to eight times if a concurrent transaction lands in the middle. A stale asynchronous result cannot overwrite a newer manual/action refresh; a terminal Run stops polling. Event payloads are not rendered, Finding details and Evidence remain on the existing CLI/Web detail surfaces, and all displayed C0/DEL terminal controls are converted to visible text.

TUI text layout uses terminal-cell-aware grapheme wrapping, so wide Unicode text does not break panel boundaries.

When a session has an attached workspace, the TUI side panel shows workspace identity, root path, and lightweight counts for `attachments`, `scripts`, `outputs`, `logs`, and `writeups`. This is metadata only; the panel does not read file contents.

## Agent Sessions

```powershell
cyberagent session create --workspace demo --title "Agent basics" --route learn
cyberagent session list
cyberagent session send <session-id> "summarize your current capabilities"
cyberagent session send <run-session-id> "queue this exactly once" --operation-key <stable-key>
cyberagent session send <session-id> "/help"
cyberagent session send <session-id> "/model script"
cyberagent session send <session-id> "/model mimo/mimo-v2.5-pro"
cyberagent session send <session-id> "/model deepseek/deepseek-v4-flash"
cyberagent session send <session-id> "/compact"
cyberagent session send <session-id> "/ls ."
cyberagent session send <session-id> "/read README.md"
cyberagent session send <session-id> "/write README.md # Proposed replacement"
cyberagent session send <session-id> "/run echo hello"
cyberagent session history <session-id>
cyberagent session history <session-id> --all
```

Session chat is the main path for generic AI agent features. Ordinary text in a Run-bound Session is supervised and consumes that Run's turn, token, and model-time budgets. Older Sessions created directly with `session create` have no Run and temporarily retain the legacy direct Router path for compatibility. Slash commands remain explicit command paths, but `/ls`, `/read`, `/write`, and `/run` now enter the same Go Tool Gateway used by the CLI and TUI and consume the Run tool-call budget. Workspace commands require an attached workspace; `/read` responses are bounded and redacted before persistence or model use. `/write` and `/run` normally create reviewable proposals. A matching active Session grant may apply a file edit immediately or complete Shell as a dry run; Shell is never executed on the host.

## Unified Tool Gateway

The Gateway validates exact argument schemas, binds calls to a Run/Session/Workspace scope, atomically charges the Run tool-call budget, runs policy checks, selects an approval mode, and normalizes execution results. `read_file`, `list_workspace`, `work_item_create`, and `note_create` use automatic approval only when scope and policy allow them. `replace_file`, `shell`, and `script_process` normally use per-call approval. A policy denial is terminal and cannot be converted into approval by a grant or later review. Schema v11 persists each per-call decision with a request fingerprint, Run/Session association, and immutable idempotency operation before the compatibility proposal advances. Schema v12 persists revocable Session grants scoped to one Run, Session, Workspace, Tool, and ActionClass; terminal Runs and archived Sessions cannot create or consume them. Schema v13 stores typed script processes and makes initial Script Run creation atomic and recoverably idempotent. Schema v14 stores source-bound tool-output Artifacts and projects metadata-only creation events. Schema v15 stores idempotent create-only structured-memory mutations without raw operation keys or content-bearing tool events.

New CLI Runs default to 100 tool calls and may set `--max-tool-calls`; zero means unlimited for compatibility. Every valid Run-bound Gateway invocation consumes one call, including Policy-denied attempts. The first attempted call beyond the limit records one `tool.budget_exhausted` event, subsequent attempts return `RESOURCE_EXHAUSTED` without duplicating that event, and `run usage <run-id>` shows consumed, limit, remaining, and exhaustion time.

Text output is valid UTF-8, secret-redacted, MIME-labelled, and bounded to 128 KiB stdout, 32 KiB stderr, and 64 KiB proposal previews. Truncation is explicit. Before this Result projection, each non-empty Run-bound output stream is captured as at most 4 MiB of redacted Artifact content with SHA-256, byte size, MIME, encoding, source proposal/invocation, and Run scope. Hashes cover the redacted content. An explicit `read_file` maximum still bounds what the tool returns and therefore what its Artifact contains. No production CLI path invokes LocalSandbox; script-process proposals remain deliberately non-executable.

## File Edit Proposals

```powershell
cyberagent edit propose --workspace demo --path README.md --content "# Updated"
cyberagent edit propose --workspace demo --path scripts/main.go --content-file C:\temp\main.go
cyberagent edit list --workspace demo
cyberagent edit list --session <session-id> --status proposed
cyberagent edit show <edit-id>
cyberagent edit approve <edit-id>
cyberagent edit deny <edit-id> --reason "not needed"
```

File edits replace the complete text content of one file. Existing files and new files under an existing workspace directory are supported. Absolute paths, `..` traversal, directory targets, symlink escapes, non-UTF-8 content, missing parent directories, and content over 256 KiB are rejected.

Proposals are stored without modifying the workspace. Approval obtains the workspace root from the Store, rejects a mismatched supplied root, compares the current file SHA-256 hash with the proposal's original hash, re-resolves the target immediately before writing, and refuses stale changes. Proposed secrets are replaced with redaction markers before persistence and before any approved write. For exact multiline or whitespace-sensitive content, prefer `--content-file`; session `/write` trims the outer message whitespace.

## Tool Proposals

```powershell
cyberagent tool list
cyberagent tool list --session <session-id>
cyberagent tool list --status proposed
cyberagent tool show <proposal-id>
cyberagent tool approve <proposal-id>
cyberagent tool deny <proposal-id> --reason "not needed"
```

`tool list` combines legacy Shell ToolRuns and typed ScriptProcess proposals and sorts them by update time. `tool show/approve/deny` resolves the proposal type from the durable approval ledger, so callers do not select an implementation-specific manager. `/run` creates a Shell `tool_runs` proposal; `script run` creates a v13 `script_process_proposals` record. A matching active Shell Session grant can authorize Shell automatically, but Shell and ScriptProcess completion remain dry-run. File edits continue to use `edit show/approve/deny`. Terminal tool detail and approval output include associated Artifact IDs. Real command execution stays disabled until Sandbox isolation, network/resource policy, cancellation, and execution-specific evidence export pass a separate audit.

## Run Artifacts

```powershell
cyberagent artifact list --run <run-id>
cyberagent artifact list --source <proposal-or-invocation-id> --stream stdout
cyberagent artifact show <artifact-id>
cyberagent artifact read <artifact-id> --max-bytes 65536
cyberagent artifact verify <artifact-id>
```

`artifact list` and `artifact show` expose metadata without printing content. `artifact read` loads and verifies the stored size and SHA-256 first, then prints at most the requested UTF-8-safe byte limit; its default is 64 KiB and maximum is 4 MiB. `artifact verify` reloads the blob and reports the verified digest and size. Capture is idempotent by Run, source, and stream. Reusing a source with changed content or MIME is a conflict, and a Policy-denied proposal creates no output Artifact.

## Approval Ledger

```powershell
cyberagent approval list --run <run-id> --status pending
cyberagent approval list --session <session-id> --tool shell
cyberagent approval show <approval-id>
cyberagent approval grant create --session <session-id> --tool shell --reason "trusted build commands"
cyberagent approval grant create --session <session-id> --tool replace_file --reason "bounded refactor"
cyberagent approval grant list --run <run-id> --status active
cyberagent approval grant show <grant-id>
cyberagent approval grant revoke <grant-id> --reason "phase complete"
cyberagent run usage <run-id>
```

The approval ledger stores identity, scope, mode, status, reviewer metadata, an optional Session grant ID, and a SHA-256 request fingerprint rather than duplicating command or file content. `approval.requested` is committed with the proposal. `approval.decided` and a domain-separated SHA-256 digest of the immutable review key are committed before ToolRun/FileEdit progression, so rerunning the same CLI approval after a process interruption resumes safely without persisting the raw client key. Grant create/revoke operations use separate domain-separated key digests and append `approval.grant_created` or `approval.grant_revoked`. A key cannot be reused for different intent, a revoked grant cannot authorize a new proposal, and a grant never overrides Policy.

## Context Compaction

```powershell
cyberagent context compact --workspace demo --task task-demo --message "user: imported challenge" --message "assistant: summarized plan"
cyberagent context show --task task-demo
```

`context compact` is the manual v0.1 version of a Codex-style compaction step. It stores a summary in SQLite and reports how many recent messages remain outside the summary.

Run-scoped WorkItems and Notes are independent from Session compaction, so compacting or replacing conversation history does not remove structured plan or memory records. The Supervisor's token-aware memory selector combines the latest summary with those durable sources before each Run model call.
