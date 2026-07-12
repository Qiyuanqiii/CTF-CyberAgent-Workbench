# Usage

## Missions and Runs

```powershell
cyberagent run create "review this workspace" --workspace demo --profile review
cyberagent run create "explain this code" --profile learn --max-turns 40 --max-tokens 20000 --timeout 20m
cyberagent run adapt-task <legacy-task-id>
cyberagent run list
cyberagent run list --status paused
cyberagent run show <run-id>
cyberagent run events <run-id>
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

Session messages, assistant policy decisions, ToolRun changes, and FileEdit changes are projected into `run events` transactionally. Activity carrying a workspace different from the Run scope is rejected. `run start` advances the lifecycle from `created` through `preparing` to `running`; it does not invoke a model by itself.

`run step` asks the RunSupervisor to execute exactly one root Agent planning turn. It writes a `turn_started` checkpoint before the model call, permits only the bounded create-only WorkItem/Note tool loop, and ultimately requires one strict `root_lifecycle.v1` JSON action. The Supervisor validates and interprets the action, then atomically stores the user-facing message, policy decision, model usage, lifecycle events, cumulative token/model-time counters, and next checkpoint. Raw protocol JSON is not written to Session history. `run checkpoint` displays the durable phase, protocol-repair phase/reason, next turn, token counters, and execution milliseconds. A process restart resumes an unfinished started turn or pending tool batch; a committed turn is never appended twice. Turn or token budget exhaustion returns exit code 8, while the persisted execution-time boundary returns a deadline error.

Root actions use `continue`, `finish`, or `wait`. `continue` advances to another idle turn. `finish` requires a summary and atomically completes the Run. `wait` requires a reason, atomically pauses the Run, and resumes at the next turn after `run resume`. Unknown fields, trailing data, Markdown fences, invalid combinations, and responses over 64 KiB fail the current turn without writing user/assistant messages. Assistant prose by itself cannot mutate Run state.

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

The Go-internal `SpecialistScheduler` may run at most two explicitly selected ready children per round under one Run execution lease and stops within 32 rounds. Parent cancellation, heartbeat loss, or the first child error fans cancellation out to the active sibling, and the scheduler waits for durable Attempt terminal state before releasing the lease. Root and child token/model-time totals are rebuilt from SQLite before and after every round. There is still no CLI, HTTP, or model path that admits or starts a schedule.

Schema v29 persists schedule boundaries and exact child cancellation. A schedule writes metadata-only `agent.schedule_started/stopped` events plus an immutable terminal summary; a later lease generation marks an orphaned running schedule `abandoned/worker_lost`. The optional control API can cancel one already-started child call at `/api/v1/runs/{run_id}/agents/{agent_id}/active-call/cancel`, with strict AgentAttempt/model identity and a digest-only idempotency ledger. Only the worker holding the Attempt lease observes the request and cancels its own Provider context. Responses and events expose no raw key, model text, lease id, or fencing generation. In a concurrent scheduler round, the selected child's cancellation error may activate the scheduler's existing local sibling fan-out, but no sibling control request is fabricated.

Schema v30 lets the root model call `specialist_delegation_propose` with one or two strict `specialist_delegation.v1` assignments. Proposal creation is additive and review-gated: Go validates the active root turn and lease, trusted Run/Session/Workspace scope, parent-Skill subsets, child capacity, and suggested turn/token headroom, then stores only a redacted immutable `proposed` record. The result always reports `admission_authorized=false`; no child Agent, Session, budget reservation, or schedule is created. Operators inspect proposals with `run delegations <run-id>` and `run delegation <proposal-id>`.

Schema v31 adds explicit operator review without adding execution authority. `run delegation approve` records one immutable approval only while the Run is still running; `run delegation reject` requires a reason and may close a proposal even after the Run is terminal. Both commands require a 16-256-byte stable operation key and default the reviewer identity to `cli_operator`. The Store redacts reasons, keeps them out of Run events, hashes operation keys, rejects changed-intent replay and second decisions, and emits one metadata-only `agent.delegation_reviewed`. Every result still reports `admission_authorized=false` and `application_required=true`; review does not create or schedule a child.

Schema v32 adds `run delegation apply` as the only current operator application entry point. The operator must match the approved review identity and provide a stable 16-256-byte operation key. Before creating state, Go reruns Policy and verifies the immutable review operation, running Run, ready root, active Session, idle child runtime, parent Skills, default limits of two children/eight turns/16,384 tokens per child, current capacity, and aggregate root headroom. It then admits each child and sends its strict parent instruction through deterministic internal keys. A restart after either write safely replays the existing Agent/Message. Applying blocks root turns, unrelated admission/messages, and child scheduling; terminal Run transitions abort the application. Applied children remain `ready`, with `scheduling_started=false`.

Schema v33 adds planning-only read-only Fan-out. `run fanout plan` requires a running Run whose Mission and active Session bind the same local workspace and whose network mode is disabled. Tiers `auto/1/2/4/6` are concurrency caps for independent analysis shards, not Agent admission limits. The scanner walks one workspace-relative directory without following symlinks, excludes VCS/dependency/build directories, binary and secret-like files, and stores only a bounded immutable path/hash manifest. Policy and `readonly_fanout.planned` events contain metadata but no goal, file path, or local root. `run fanouts` and `run fanout show` inspect the immutable plan; planning itself performs no Provider call.

Schema v34 adds `run fanout execute <plan-id>`. Execution requires the same plan operator and a stable operation key. Go acquires the Run lease, rebuilds the manifest, then verifies every file identity, size, and hash before making any Provider call. Source content is redacted and held only in memory. Each shard gets a tool-free JSON-mode request and must return strict `readonly_fanout_report.v1`; findings outside the shard are rejected. The planned 1/2/4/6 calls run concurrently, first failure cancels siblings, and every terminal state is persisted before the lease is released. `run fanout execution <execution-id>` displays the durable result. A repeated operation key and intent returns the same execution without another model call. Root, Specialist, and Fan-out token/model-time usage share the Run budget. Crash-uncertain calls retain their reserved charge and a newer lease retries only incomplete shards. This path still creates no Agent, Attempt, schedule, tool, file edit, process, network request, or automatic source change.

Schema v35 adds `run fanout report <execution-id> --format markdown|json`. It accepts only a completed execution and performs no Provider call. Go groups only source assertions with identical severity/category/title/detail/path/line facts, retains every source row as immutable `model_assertion` Evidence, and uses the minimum claimed confidence for an exact duplicate group. Every projected Finding is `draft`; report generation does not validate a vulnerability. The `building -> generated` SQLite transaction checks source bindings, contiguous ordinals, counts, and severity totals before making the projection immutable. Repeating the command or using `report show <report-id>` renders the same persisted projection byte for byte.

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
cyberagent run fanout plan <run-id> "audit source modules" --operation-key <stable-key> [--tier auto|1|2|4|6] [--path <dir>] [--operator cli_operator]
cyberagent run fanouts <run-id> [--limit 20]
cyberagent run fanout show <plan-id>
cyberagent run fanout execute <plan-id> --operation-key <stable-key> [--operator cli_operator] [--max-output-tokens 1024]
cyberagent run fanout execution <execution-id>
cyberagent run fanout report <execution-id> [--format markdown|json]
cyberagent report show <report-id> [--format markdown|json]
```

`work_item_create` creates one pending WorkItem; `note_create` creates one active Note. They accept strict JSON with unknown fields and trailing data rejected before budget charging. The Run must already have an attached Session, and the CLI derives Session/Workspace scope from persisted Run state instead of accepting caller-supplied scope. `--payload` is also supported, while `--payload-file` avoids native-shell JSON quoting differences and is bounded to 96 KiB of valid UTF-8.

An operation key is mandatory and should remain stable across retries. The raw key is never persisted: schema v15 stores a domain-separated SHA-256 digest and a fingerprint of the normalized, redacted intent. Repeating the same tool, Run, key, and intent returns the original entity with `replayed: true`; changing intent under the same key returns conflict exit code 4. Replay, conflict, authoritative scope mismatch, and Policy-denied attempts each consume a tool-call budget entry because they are well-formed invocations. Malformed JSON, unknown fields, missing identities, and invalid field values are rejected before charging. Successful creation commits the entity, Policy decision, domain event, `tool.completed`, and operation ledger atomically. A failed event write leaves no entity or operation row.

The WorkItem and Note tools are create-only and return metadata rather than content. RunSupervisor advertises those two definitions plus `specialist_delegation_propose`: a Provider response may request at most four calls and one turn may perform at most four tool rounds. The model response and pending batch are committed together; after restart, unfinished calls are safely replayed through the semantic operation ledger and their terminal metadata is returned to the Provider. Anthropic-compatible transports encode this as `tool_use` and `tool_result`. Policy denial, invalid delegation capability requests, and budget exhaustion are returned as bounded error results, while protocol repair exposes no tools.

`specialist_delegation_propose` is Supervisor-only and cannot be invoked through `tool invoke`. Its strict payload is `{"version":"specialist_delegation.v1","assignments":[{"title":"Review parser","goal":"Inspect parser boundaries","skills":["model.chat"],"turn_limit":2,"token_limit":256}]}`. Unknown fields, more than two assignments, duplicate goals, unavailable or non-delegable Skills, stale leases, insufficient child capacity, and proposals that do not leave root budget headroom are rejected. Repeating the same redacted semantic intent returns the original proposal ID; the raw operation key and Provider call ID are never persisted. CLI output may show the redacted goals, independent review, and application state, while Run events contain only proposal identity, counts, suggested aggregate budgets, review/application metadata, and authorization phase flags. Approval/rejection has no Provider tool definition; application is operator-only and calls admission plus strict instruction delivery but never starts the scheduler.

Update, completion, archive, Shell, file, process, network, and other Provider-driven tools remain disabled pending separate lifecycle, approval, and Sandbox audits. Use the ordinary `todo` and `note` commands for operator-controlled updates.

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
cyberagent tui --session <session-id>
cyberagent tui --session <session-id> --print
```

Without `--session`, the TUI opens a session picker. Press `Enter` to open the selected session, `n` to create a new one, `j/k` to move, `r` to refresh, and `q` or `Esc` to quit.

The chat TUI uses the same session and tool approval runtime as the CLI. Normal text sends a session message. Slash commands such as `/run echo hello`, `/model script`, and `/compact` go through the session manager. Tool approvals can be handled in the input box:

```text
/approve <tool-run-id>
/approve-session <tool-run-id>
/deny <tool-run-id> not needed
```

Keyboard controls:

```text
Tab              switch focus between input and the activity pane
Enter            send from input or approve one selected proposed tool
PgUp / PgDn      scroll messages
h / l            switch among Tools, Work, Notes, and Rounds
j / k            select the next/previous item in the active view
a                approve the selected Shell proposal once
g                approve it and grant the exact current Session scope
d                deny the selected Shell proposal
Ctrl+R           refresh Session, Run, memory, and tool state
Ctrl+X           request audited cancellation of the current model call
Esc              quit when idle; a busy action must finish or be cancelled first
```

`--print` renders one snapshot and exits, which is useful for non-interactive verification.

Message sends, live-call discovery, refreshes, cancellation, and tool approval/deny actions run asynchronously. During a Run-bound model call, the status line shows provider/model, attempt, chunk/byte progress, cancellation, slow-consumer disconnect, and terminal state. `Ctrl+X` prefers the application audit-first cancellation API; if a legacy or not-yet-active request has no registry entry, it cancels the current application request context instead. Additional input is held until the current action finishes, and raw model text is never included in the live envelope.

For a Run-bound Session, the activity pane reads WorkItems, Notes, durable Supervisor ToolRounds, active Shell grants, and ToolRuns from the Go Store. Work, Notes, and Rounds are read-only views; approval keys act only in Tools. `a` uses the existing durable per-call decision, while `g` creates or reuses a revocable Grant scoped to the exact Run, Session, Workspace, `shell` tool, and `shell` ActionClass. Keyboard and slash-command approval paths both reject ToolRun IDs outside the currently open Session. The current proposal is matched against its persisted fingerprint and rechecked by Policy before the Grant is created. Later allowed Shell calls may complete automatically as dry runs; Policy denial always wins. TUI text layout uses terminal-cell-aware grapheme wrapping, so wide Unicode text does not break panel boundaries.

When a session has an attached workspace, the TUI side panel shows workspace identity, root path, and lightweight counts for `attachments`, `scripts`, `outputs`, `logs`, and `writeups`. This is metadata only; the panel does not read file contents.

## Agent Sessions

```powershell
cyberagent session create --workspace demo --title "Agent basics" --route learn
cyberagent session list
cyberagent session send <session-id> "summarize your current capabilities"
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
