# Architecture

CyberAgent Workbench is evolving from a CLI-first agent scaffold into a run-centric, resumable AI workbench. The redesign keeps the existing Go implementation and safety boundaries while organizing them around explicit execution ownership.

## Design Goals

- Go remains the sole control plane.
- One user objective is a `Mission`; one execution attempt is a `Run`.
- Every state change is auditable and recoverable from SQLite.
- Agent concurrency is coordinated by one owner, not by agents calling each other directly.
- Privileged actions always cross policy, approval, scope, and sandbox boundaries.
- CLI, TUI, the React/Vite read UI, and CI use the same Go-owned application and HTTP services.
- Rust analyzers remain deterministic tools behind Go.
- CTF-specific behavior remains a late profile, not the runtime foundation.

## Control Plane

```text
CLI / Bubble Tea TUI / Headless CI / React-Vite read UI
                              |
                    Go Application Services
                              |
        +---------------------+---------------------+
        |                     |                     |
  Mission Service       Approval Service       Report Service
        |                     |                     |
        +-------------- Run Supervisor -------------+
                              |
                     Agent Coordinator
                              |
          +-------------------+-------------------+
          |                   |                   |
      Root Agent        Specialist Agent     Specialist Agent
          |                   |                   |
          +------------ Tool Gateway -------------+
                              |
              Policy -> Scope -> Approval -> Runtime
                              |
        +---------------------+---------------------+
        |                     |                     |
  Workspace/File       Sandbox Backend       Rust Analyzer Bridge
        |                     |                     |
        +---------------------+---------------------+
                              |
                  SQLite Event and State Store
```

Allowed external directions remain:

```text
TypeScript -> Go -> Rust
TypeScript -> Go -> Docker
TypeScript -> Go -> LLM
```

TypeScript and Rust never bypass Go for secrets, persistence, policy, workspace permissions, shell execution, network scope, or Docker. See [ADR 0001](adr/0001-go-control-plane.md).

The production React bundle is built by Vite but hosted only when Go receives an explicit `--ui-dir`. Go validates and snapshots the static tree before serving it from the same loopback origin as `api.v1`; `/api` remains a reserved authenticated namespace. Vite's loopback proxy is a development adapter, not a second control plane.

## Core Domain

`Run` is a domain aggregate, not a programming language, operating-system process, or replacement for Go/TypeScript/Rust. Go creates and owns Run lifecycle state; TypeScript may display and control it through Go APIs; Rust may execute a bounded analyzer request carrying a `run_id` but never owns the Run.

Budget, events, sandbox sessions, and reports are Run-scoped rather than embedded into one large object. Their modules remain independent and persist references by `run_id`; `RunSupervisor` coordinates their lifecycle.

| Aggregate | Purpose | Durable owner |
|---|---|---|
| `Mission` | Stable user goal, workspace, profile, and authorization scope | Mission Store |
| `Run` | One resumable execution attempt with config snapshot, budget, and status | Run Store |
| `AgentNode` | Addressable root or specialist worker inside a run | Coordinator Store |
| `WorkItem` | Structured unit of planned work with dependency and completion state | Work Board Store |
| `Note` | Durable observation or reasoning aid, scoped to run and optional agent | Memory Store |
| `Finding` | Structured, evidence-backed result awaiting validation/acceptance | Finding Store |
| `Evidence` | Immutable reference to logs, commands, files, diffs, or test output | Artifact Store |
| `Approval` | Decision record for a privileged proposed action | Approval Store |
| `Event` | Append-only lifecycle and activity record | Event Store |

Existing `agent.Task` and `session.Session` are not discarded. During migration, `Task` becomes a compatibility view of `Mission`/`WorkItem`, while `Session` remains conversation history attached to a `Run` and optionally an `AgentNode`.

## State Machines

Run lifecycle:

```text
created -> preparing -> running -> completed
                       |   |  |
                       |   |  +-> failed
                       |   +----> waiting_approval -> running
                       +--------> paused -> running
                                   |
                                   +-> cancelled
```

Agent lifecycle:

```text
pending -> running -> waiting -> running
             |          |
             |          +-> cancelled
             +-> completed / failed / crashed / cancelled
```

Work item lifecycle:

```text
pending -> ready -> in_progress -> done
                         |
                         +-> blocked / failed / cancelled
```

Finding lifecycle:

```text
draft -> validating -> validated -> accepted -> fixed
             |             |
             +-> rejected  +-> duplicate
```

Transitions are checked in Go domain services and written with an event. UI code and model output cannot directly mutate status fields.

## Run Supervisor

`RunSupervisor` is the application-level owner of a run. It is responsible for:

1. loading the mission and immutable run configuration snapshot;
2. validating authorization scope and budget;
3. provisioning or reconnecting the sandbox;
4. restoring the coordinator, agent sessions, work board, notes, and pending approvals;
5. starting the root agent or resuming parked agents;
6. forwarding normalized events to CLI/TUI/WebSocket subscribers;
7. stopping all workers on cancellation, budget exhaustion, or fatal runtime errors;
8. finalizing findings, artifacts, reports, and cleanup state.

The supervisor owns process handles and channels only in memory. Durable IDs, statuses, inbox messages, checkpoints, and events live in SQLite so a process restart can reconstruct the run.

Schema v17 adds one durable execution lease row per Run. A Supervisor must acquire the lease before entering a turn or operator finalization; `Step` holds it for one turn and `Execute` holds it across the complete bounded loop. The default 30-second lease is renewed every 10 seconds while a Provider call or Store operation is in flight. An expired or released lease may be replaced only by a higher generation. Same-owner acquisition is not implicitly reentrant: only a retry that presents the current `lease_id` can replay the acquisition, preventing concurrent calls from one worker identity from sharing a lease.

The pair `(lease_id, generation)` is a fencing token. It is copied into the durable Supervisor checkpoint and verified inside every model/tool/checkpoint mutation transaction. RunSupervisor structured-memory calls also carry it through Tool Gateway; the budget charge checks it before incrementing, and the entity transaction checks it again before replay or creation. A takeover can rebind an unfinished checkpoint to the new generation while preserving its attempt and pending input, but the old worker can no longer append events, consume budget, or write entities. Heartbeat loss cancels the Go operation context, and release uses a short context independent of caller cancellation. Lease lifecycle events expose owner/generation/timestamps only; the token is absent from events, CLI output, HTTP DTOs, and Gateway outcomes.

The current single-Agent slice persists cumulative input/output/total tokens, model-call execution milliseconds, and the redacted pending user input in the Supervisor checkpoint. A bounded executor performs only an operator-selected number of durable steps. Root model output uses strict `root_lifecycle.v1` JSON: `continue` returns to idle, `finish` completes the Run, and `wait` pauses it for explicit resume. Turn data, status, checkpoint, Session messages, and events commit in one transaction; arbitrary assistant text cannot mutate lifecycle state.

Provider calls use typed outcomes: `retryable`, `rate_limited`, `invalid_response`, `cancelled`, or `permanent`. RunSupervisor retries only the first two, at most three transport attempts per protocol phase by default, with cancellation-aware exponential backoff. Server `Retry-After` is honored only when it is within the local maximum wait; a longer delay returns a stable rate-limit result instead of retrying early. Each call receives a durable global sequence number plus a phase-local transport number and emits `model.started` plus exactly one terminal model event. Terminal event persistence, token usage, and model execution time share one transaction, so restart recovery neither loses nor double-charges completed calls.

Every Supervisor model attempt uses `StreamChat`. The stream aggregator reconstructs UTF-8 across transport chunk boundaries, caps model output at 64 KiB, requires one final completion chunk with valid usage, rejects ToolCalls on non-final chunks, and forwards normalized final ToolCalls to the bounded structured-memory loop before lifecycle parsing. Mid-stream transport failures use the same typed retry policy, lifecycle-protocol repair, budget accounting, and terminal transactions as a non-stream response.

Incremental persistence is deliberately metadata-only. One attempt may append at most 32 ordered `model.delta` events carrying sequence, chunk count, byte count, cumulative bytes, and completion state. The Store validates idempotent replay, strict ordering, size limits, and exact agreement between the durable delta ledger and the terminal model event. Model text remains in bounded process memory until the validated turn transaction writes the final redacted assistant message.

The Go application layer owns an in-process `ActiveCallRegistry`. A call is reserved before `model.started` to reject concurrent Provider calls for the same Run, but it becomes queryable and cancellable only after that durable start succeeds. Registry entries are keyed by Run plus Supervisor/model-attempt identity, own the Provider cancellation function, and are removed on every Provider terminal path. Explicit cancellation writes one idempotent, redacted `model.cancel_requested` event before signalling the context.

The active-call registry still owns the actual Provider cancellation function inside its worker process. Schema v18 bridges processes without exposing that registry: a separately authorized HTTP control request persists an exact Run/Supervisor/model-attempt intent, and the worker polls it using its own schema v17-fenced checkpoint. Observation commits `model.cancel_observed` before the registry signals the Provider context. The client never receives or supplies the lease id; later model attempts atomically mark orphaned older requests `superseded` instead of inheriting them.

Schema v29 applies the same audit-first principle to concurrent Specialist calls without reusing the Run-keyed root registry. A separate cancellation row is bound to `Run + Specialist Agent + AgentAttempt + model attempt`; the child worker polls under the Attempt's private execution lease and owns that call's `context.CancelFunc`. Observation commits before signalling, model terminal state resolves the request atomically, and Attempt crash/interruption/takeover resolves leftovers as `attempt_terminated` or `worker_lost`. Root and child ledgers remain separate so two concurrent children cannot alias one Run-level registry key. API responses and events contain neither lease identity nor model text.

Schema v30 adds a proposal-only bridge from root reasoning to the Go-owned Agent graph. `specialist_delegation_propose` accepts strict `specialist_delegation.v1` JSON with one or two assignments. The Tool Gateway canonicalizes and redacts the payload before deriving its semantic operation key; SQLite then verifies the exact active root, Supervisor checkpoint, Run execution lease, charged `agent_proposal` invocation, trusted scope, parent-Skill subsets, child capacity, and root budget headroom. Proposal, assignments, Policy decision, metadata-only `agent.delegation_proposed`, `tool.completed`, and digest-only operation fact commit together. The row is immutable and remains `proposed`; it is deliberately not a capacity reservation or authorization fact. No model response can move it into admission, create an Agent/Session, or select/start the scheduler.

Schema v31 appends one independent operator review fact without mutating that proposal. `specialist_delegation_reviews` stores a redacted `approved` or `rejected` decision, while a separate digest-only operation table provides exact replay and changed-intent conflict handling. The SQLite writer transaction rebinds the review to the immutable proposal, Run, and root; approval requires a running Run, rejection requires a reason and remains available after termination. Review and operation rows reject update and delete. The strict metadata event excludes the reason and declares `admission_authorized=false` plus `application_required=true`. There is no Provider tool, HTTP endpoint, Agent capability, admission call, instruction send, or scheduler call for review.

Schema v32 adds a recoverable operator application state machine. `specialist_delegation_applications` freezes the approved review, application policy, and one applying/applied/aborted lifecycle; ordered assignment rows move only `pending -> admitted -> instructed`. Before those transitions, each row stores the exact admission and instruction operation digests derived from its application ID and ordinal. The Coordinator remains authoritative for Agent/Session creation and message delivery, while application transitions verify the corresponding Coordinator operation ledger, Agent budget/Skill/session projection, and strict instruction payload. A crash between either Coordinator commit and application-state commit therefore replays the same Agent or Message. One applying application reserves the Run against root turns, unrelated admission/messages, Specialist schedules, and direct Attempts. Terminal Run projection aborts it and records counts; normal completion leaves every child ready and starts no schedule.

Schema v33 deliberately does not widen that graph. `readonly_fanout_plans`, ordered file manifests, deterministic shards, and digest-only operations form a separate planning aggregate with a fixed `readonly_fanout.v1` capability fingerprint. The only true capabilities are workspace list/read; Shell, write, process, network, external tools, and recursive child spawn are all false in both Go validation and the SQLite CHECK. Requested tiers are `auto/1/2/4/6`, while effective shard count is bounded by eligible files. Snapshot hashing uses stable relative paths, sizes, and content hashes; scanner scope is canonicalized below the trusted workspace root, symlinks are skipped, and bounded exclusions are explicit. Planning requires a running network-disabled Run plus its active workspace Session. Rows and metadata events are immutable, but file bytes are not copied, so the future executor must reread and verify every stored hash before any Provider call. v33 has no execution transition and no AgentNode, model, tool, scheduler, HTTP, or write entry point.

Schema v34 implements that reread gate without changing the Agent graph. `ReadOnlyFanoutExecutionService` acquires the existing Run execution lease, rebuilds the full plan, then opens each admitted path through `os.Root` and verifies regular-file identity, byte count, and content digest again. Only a redacted in-memory copy reaches a Provider. Every request has no tools, requires JSON mode, and must decode as strict `readonly_fanout_report.v1`; a finding path must belong to the exact shard. Go starts at most the immutable plan parallelism, shares cancellation across the batch, and waits for every shard to become durable before finalizing. SQLite keeps separate execution, shard, model-call, finding, and digest-only operation ledgers. The private lease generation fences stale commits. Takeover marks uncertain calls `abandoned`, conservatively retains their reserved token/time charge, resets only their running shards, and never replays completed reports. `RunAgentUsage` reconciles root, Specialist, and Fan-out calls, while the existing core scheduler remains capped at two children. Fan-out still creates no AgentNode, AgentAttempt, Session, schedule, tool call, file edit, process, or network operation.

Schema v35 adds the first generic Finding/Evidence/Report projection without another model boundary. A completed read-only Fan-out execution is the immutable source. Go fingerprints severity, category, title, detail, path, and line range exactly; only equal facts deduplicate, severity disagreement remains separate, and duplicate confidence becomes the conservative minimum. Each source assertion remains an immutable `model_assertion` Evidence record bound to its v34 fingerprint and shard report digest. `finding_reports` is inserted as `building`, then can transition once to `generated` only when SQLite verifies source bindings, grouped/source counts, severity totals, and contiguous Finding/Evidence ordinals. Generated rows cannot be updated or deleted. Renderers rebuild byte-stable Markdown/JSON from SQLite. Every Finding starts `draft`: this projection records a model claim and provenance, not validation or acceptance.

Schema v36 adds validation as an additive overlay instead of mutating that source projection. `finding_artifact_evidence` snapshots the identity, SHA-256, size, MIME, stream, tool, source, and redaction flag of one same-Run `run_artifacts` row after Go rereads and validates the full blob. Update/delete triggers freeze all Run Artifacts and every new evidence, decision, and operation row. A Finding may receive one `draft -> validated|rejected` decision; validation requires at least one Artifact Evidence record, while rejection may have none. The decision stores the exact ordered Evidence count and digest. Evidence cannot be appended after a decision. Separate digest-only operation ledgers make both mutations replay-safe across processes, while `finding.evidence_attached` and `finding.validation_decided` omit notes, reasons, and Artifact content. The v35 source projection digest deliberately excludes this overlay.

Schema v37 adds acceptance and remediation as further additive overlays. Acceptance is a separate immutable operator fact, not a side effect of validation, and snapshots the exact validated decision plus its Evidence count and digest. Remediation Evidence must reference a different same-Run Artifact whose durable `artifact.created` sequence is strictly later than the acceptance event; wall-clock ordering alone is not trusted. A fix requires at least one such record and freezes the ordered remediation count and domain-separated digest. Writer-lock serialization and digest-only operation ledgers make all three mutations converge across processes. SQLite binds scope, source snapshots, event order, ordinals, timestamps, and immutable rows; domain validation reconstructs the complete chain on every read. The source Finding remains `draft`, and the v35 projection digest excludes acceptance, remediation, and fix overlays.

Live call subscribers receive a versioned metadata-only envelope for snapshot, progress, cancellation request, completion, and failure. Each subscriber has a 32-event buffer; a slow subscriber is closed instead of blocking the Provider. This transient stream has no replay guarantee and intentionally has no model-text field. Future user-facing text streaming needs a separate Go-owned redaction and lifecycle-projection boundary.

If a response fails strict `root_lifecycle.v1` parsing, the Supervisor persists a redacted diagnostic and requests exactly one protocol repair without replaying the raw output. Repair transport retries use their own bounded counter. Pending repair resumes after restart, exhausted repair never calls the Provider again, and request/start/completion/failure transitions are append-only Run events. Only a validated repaired response can reach Session history.

Ordinary text sent to a Run-bound Session uses this same Supervisor path. A `created` Run starts automatically, a paused Run resumes for follow-up input, and terminal or approval-waiting Runs reject new model turns. The input is checkpointed before the Provider call and is recovered after process restart without duplicating the committed user/assistant pair. Sessions without a Run retain an explicit legacy Router path during migration; slash commands remain command adapters rather than implicit Agent turns.

## Agent Coordinator

One `AgentCoordinator` owns the graph for a run:

- root/parent relationships;
- agent identity, role, profile, and assigned skills;
- status transitions and cancellation;
- per-agent inboxes and pending message counts;
- child creation limits and concurrency limits;
- budget and turn allocation;
- completion reports from child to parent;
- snapshot/recovery metadata.

Agents communicate through coordinator messages. A child never invokes a parent callback and siblings do not share mutable memory. Multi-agent execution is opt-in; the first implementation runs one root agent through the same coordinator API.

Schema v19 implements that first single-root slice. Every new Run receives one stable root `AgentNode`; existing databases register it lazily on the next Coordinator/Supervisor operation. `BeginSupervisorTurn`, lifecycle completion/failure/finalization, ordinary Run transitions, inbox changes, and graph recovery snapshots share their existing SQLite business transaction. Root `continue`, `wait`, `finish`, operator failure, and Run cancellation therefore cannot leave the Run and Agent projection in different committed states. `run graph` validates the current node/inbox projection against its latest `agent_graph.v1` snapshot.

The current hard limits are one root plus at most two depth-one children, at most two assignments per core delegation proposal, one immutable review and application per proposal, eight turns and 16,384 tokens per application-created child, 16 assigned execution capabilities, at most one parent-selected built-in Skill guide per Specialist Attempt, 128 pending and 4,096 historical messages per inbox, 32 messages per manual consume batch, four root-context messages per Supervisor turn, four parent instructions per Specialist attempt, one child protocol repair per Attempt, 32 retained snapshots, and 32 internal scheduling rounds. The separate read-only Fan-out accepts execution caps of 1/2/4/6 without creating Agent nodes and permits at most three crash-recovery attempts per shard. Inbox payloads must be JSON objects with bounded ASCII field names; secret-shaped keys are rejected while string values pass through recursive redaction. Snapshot metadata includes a hash of each pending payload rather than a second payload copy. Schema v20 makes send intent idempotent through digest-only operation facts and gives `wake`/`dependency` strict semantics. Schema v21 keeps child admission absent from the default Coordinator and enables it only through an explicit Go-internal policy with parent-capability subsets, dedicated Sessions, positive budget reservations, and root headroom. Schema v25 admits only protocol-backed direct-child messages into root context. Schema v26 adds one explicitly invoked internal child turn, schema v27 gives that turn recoverable direct-parent instructions plus bounded child-owned memory, schema v28 adds one isolated child lifecycle repair, schema v29 persists schedule boundaries plus exact child cancellation, schema v30 persists review-required root delegation suggestions, schema v31 records a separate non-authorizing operator decision, schema v32 applies it through existing admission/instruction paths without starting execution, schema v33 freezes a read-only Fan-out manifest, schema v34 executes it through a lease-fenced no-tool gate, and schema v47 gives each child Attempt a separately budgeted minimal Skill subset derived from the parent's pinned selection and immutable Run mode. The current Go-internal Specialist scheduler can run two explicit ready children concurrently, but no public/model-driven approval, application, spawn, or autonomous scheduler exists.

The first v35 report projection accepts at most 192 source Evidence rows and 192 draft Findings, matching six shards times the v34 per-shard limit of 32. A report with no model findings is valid and still carries a stable projection digest. Schema v36 permits at most 64 validation Artifact Evidence records per Finding and exactly one validation decision. Schema v37 permits at most 64 separate remediation Artifact Evidence records, one acceptance decision, and one fix decision per Finding.

Schema v22 establishes Agent-owned Run memory without granting the model an identity selector. WorkItems and Notes retain the legacy bounded `owner` label and add nullable `owner_agent_id` references. Application and Store validation require normalized identity; transactional Store checks reject missing, terminal, or cross-Run owners; SQLite foreign keys and same-Run insert/update triggers defend direct writes. A Note may retain Agent ownership under `run` or `root` visibility, while `owner` visibility is evaluated against the viewer Agent. Agent-only private Notes mirror the Agent ID into the legacy label so v10's established CHECK constraint and old readers remain valid. Supervisor and CLI structured-memory calls inject the root identity from Go-owned Run state, and the model-facing tool schema has no `owner_agent_id` property.

Schema v23 establishes the child completion boundary without starting a child model loop. `agent_completion.v1` permits only `succeeded` or `partial`, an 8 KiB/4,096-rune redacted summary, up to 16 child-owned WorkItem references, and up to 16 active parent-visible Note references. A successful report is rejected while the child owns active WorkItems; a partial report must reference every such item. The Store binds completion to the exact running Specialist attempt and direct root parent, then atomically inserts the immutable report and digest-only operation fact, writes one parent result inbox message, completes the child, archives its Session, appends metadata-only events, and snapshots the graph. Same-key concurrent retries return the original report, changed intent conflicts, stale attempts fail, and graph recovery rejects a completed Specialist whose report has been removed.

Schema v24 establishes the internal child scheduling boundary without exposing spawn or starting a model loop. `agent_attempt.v1` records one immutable turn attempt with its Run lease id/generation, turn number, optional exact-once model usage, terminal status, bounded redacted failure, and parent notification identity. Scheduling charges the child turn before external work; usage atomically updates the child token counter. Continuation returns the child to ready only when another turn and token headroom remain. Completion requires the current lease and recorded usage, then terminalizes the Attempt before the child in the same transaction. A crash sends one bounded notification and either schedules retry or fails and archives the child according to persisted budgets. After lease takeover, recovery crashes stale attempts once; all former-worker usage, completion, and crash commits fail the lease fence. Run pause/wait/terminal projection first interrupts a running Attempt, and restart validation recomputes contiguous turns and token totals before accepting the graph snapshot. The runtime capability is separate from admission and disabled by default.

Schema v25 establishes the root inbox-to-context boundary. A writer transaction prepares up to four sequence-ordered messages from direct Specialist children and records immutable attempt/turn/ordinal identity in `root_inbox_deliveries`. Dependency messages must pass their strict protocol; result and failure messages must match an immutable CompletionReport or crashed AgentAttempt. A successful root lifecycle transaction first commits each delivery and then consumes its message before Session/checkpoint commit. Failure or a Run transition away from running supersedes prepared rows without consuming the messages. Cancellation and lease takeover keep the started Supervisor attempt recoverable, so the same batch is replayed rather than rebound. Context construction exposes bounded typed task state and durable sender provenance but excludes message IDs, sequence values, cursors, and consumption controls. Prepared metadata participates in graph snapshots and restore validation.

Schema v26 establishes the first child Provider boundary. `SpecialistRunner` is constructed only by internal Go code and executes one no-tool turn under the same Run execution-lease heartbeat and generation fence as the root. `specialist_lifecycle.v1` accepts only `continue` or `finish` with `agent_completion.v1`; usage, retry, identity, Policy, lease, and lifecycle commits are never model fields. `specialist_model_calls` records each started/completed/failed transport attempt. A successful or invalid usage-bearing response atomically updates the model row, child Attempt usage and token counter, Policy audit, graph snapshot, and, only when allowed, a redacted child Session message pair. Transport failures may retry without charging tokens. Context cancellation records failure and crashes the Attempt before releasing the lease; a hard-lost worker is recovered by the next generation. Child history is queried as the latest 12 messages and capped again at 64 KiB before Provider dispatch. It still provides no tool specifications, public admission/spawn, or autonomous scheduling; schema v29 later adds only exact-call cancellation control.

Schema v27 establishes the parent-to-child context boundary. Only a strict `specialist_instruction.v1` message routed from the direct root parent to the child can enter `specialist_context_deliveries`; SQLite also verifies the active AgentAttempt, Run lease generation, payload shape, and pending status. Up to four sequence-ordered messages are prepared. `continue` and `finish` commit deliveries and consume messages in their existing lifecycle transaction, while crash, interruption, and lease takeover supersede deliveries after terminalizing the Attempt so the messages remain pending for a fresh attempt. Prepared metadata participates in graph snapshots and restore validation. The child context builder adds active WorkItems owned by the child and active `run`/`owner` Notes owned by and visible to that child under a 4,096-token estimate and 32 KiB input cap. Mandatory mission and parent instructions must fit; lower-priority memory is deterministically omitted. Message IDs stay out of the model input, while content-free source IDs and token estimates enter `model.started` provenance.

Schema v28 establishes the child lifecycle-repair boundary. `specialist_model_calls` now carries a durable global model sequence, a phase-local transport sequence, and a bounded `protocol_repair` phase. A usage-bearing invalid primary response atomically records its generic diagnostic, cumulative Attempt/Agent usage, one pending `specialist_protocol_repairs` row, and a metadata-only repair-request event. The one repair request reuses trusted context but never includes the invalid response; its transport retry counter restarts at one. Success resolves the repair and may enter the child Session, a second invalid response exhausts it, and cancellation, budget exhaustion, interruption, crash, or stale-worker takeover aborts a pending repair before the Attempt becomes terminal. SQLite triggers reject skipped phase calls, uncharged repair requests, invalid resolution, terminal mutation, and `continue`/`finish` while repair is unresolved. The Runner rechecks Run-wide total-token and execution-time remainder before dispatch and gives repair only the post-primary remainder.

The internal `SpecialistScheduler` lifts lease ownership above an individual child turn. One schedule holds one Run execution lease and starts at most two ready direct children per round. A shared cancellable context fans parent cancellation, heartbeat loss, or the first child failure out to every active sibling; the scheduler waits for each child to persist its Attempt terminal state before returning and releasing the lease. It stops on all-terminal, no-ready, round-limit, cancellation, child-error, aggregate-token, or aggregate-execution conditions. Aggregate usage is rebuilt transactionally from the root Supervisor checkpoint, Specialist Agent/Attempt token projections, and every Specialist model-call duration before and after each round. Remaining total-token and execution allowance is split deterministically by sorted Agent ID.

Schema v29 wraps that invocation in `specialist_schedules` and ordered `specialist_schedule_agents`. Start validates the exact active Run lease and direct-child targets. Stop records a terminal status, bounded stop reason, rounds, started turns, recovered Attempts, and monotonic before/after `RunAgentUsage`; terminal rows are immutable and events omit the stored lease identity. If a process disappears, the next lease generation marks its running schedule `abandoned/worker_lost` before starting a replacement. At this boundary the scheduler had no public CLI/HTTP/model-spawn path and granted no tools. Schema v38 later adds only an operator CLI request gate for applied/instructed children; the HTTP POST still controls only an already-started exact child model call and cannot create or select a child.

Schema v30 does not call that scheduler. It stores `specialist_delegation_proposals`, ordered assignments, and a one-to-one operation ledger only after every assignment is a normalized parent-Skill subset and the suggested aggregate leaves capacity for the active root turn plus one future root turn. The delegation capability itself is non-delegable. Replays under a fresh Provider call ID converge through the redacted semantic fingerprint, including across independent SQLite connections. The operation ledger stores digests and non-secret scope only, not `lease_id` or fencing generation; CLI commands expose redacted proposal state, while events also omit titles/goals, lease identity, and operation keys. Schema v31 may append one immutable review. Schema v32 then revalidates current Policy, review-operation backing, capacity, Skills, Sessions, idle execution, and budgets before an approved proposal can reach existing admission and instruction paths; it never calls the scheduler.

## Work Board and Notes

Conversation history is not enough for long-running work. Each run therefore has two structured memory surfaces:

- `WorkItem`: actionable work, dependencies, owner, priority, status, and acceptance criteria.
- `Note`: observations, hypotheses, decisions, summaries, and references to evidence.

Work items and notes are stored independently from LLM messages. Context construction selects only relevant summaries, active work, recent messages, and explicitly loaded notes.

The current P3/P4 implementation persists both surfaces. Schema v9 WorkItems use optimistic versions, composite same-Run dependency keys, cycle checks, legal transitions, and transactional `work_item.created/changed` events. Schema v10 Notes add category, visibility, Owner, tags, source references, Evidence IDs, pinning, archive/restore, and transactional `note.created/changed` events. Schema v22 adds authoritative same-Run Agent ownership and Agent-aware Note visibility while preserving label-only rows. Root context includes `run`, `root`, and Notes owned by the root Agent, but excludes owner-only Specialist memory.

Before each root model call, a generic Context Section selector ranks a prepared root inbox batch, the latest compacted summary, bounded active Work Board, pinned Notes, and category-weighted Notes under an 8,192-token estimate. Every prepared inbox message must fit or the turn fails without consuming it. Specialist calls use the same selector under a separate 4,096-token/32 KiB bound for mandatory parent instructions and child-owned active memory. `model.started` records included and omitted `kind/source_id/tokens` metadata so provenance survives restart, while Note and inbox bodies remain outside the event. Model-driven root `finish` is rejected through protocol repair while active work remains and checked again under the final SQLite write transaction. Schema v16 lets RunSupervisor dispatch only the schema v15 create-only WorkItem/Note tools through the same Gateway; all other Provider tools remain denied.

## Lifecycle Protocol

Autonomous/headless execution cannot finish with an arbitrary assistant paragraph. The root protocol now validates one versioned JSON lifecycle result:

- root: `continue`, `finish`, or `wait`; `finish` includes a summary and `wait` includes a reason;
- child: `specialist_lifecycle.v1` with `continue` or `finish`; `finish` carries a structured `agent_completion.v1` report for its parent;
- blocked agent: `agent.wait` with reason and awaited dependency;
- cancellation: coordinator-owned transition, never model-owned.

The current root implementation maps `wait` to a durable paused Run and resumes at the next turn, and it permits one bounded automatic repair for an invalid root response. The child path now executes no-tool Provider turns with strict lifecycle decoding, one isolated repair, lease fencing, cumulative exactly-once usage, Policy, Session history, recoverable parent instructions, child-owned memory, CompletionReport finish, parent inbox delivery, and optional two-child internal scheduling. A model or public client still cannot invoke spawn/finish; structured dependency waiting and autonomous scheduling remain future work.

## Tool Gateway

Every tool invocation uses one pipeline:

```text
Model proposal
  -> schema validation
  -> scope resolution
  -> Run tool-call budget charge
  -> policy decision
  -> approval classification
  -> sandbox/runtime execution
  -> output limits and redaction
  -> evidence/artifact capture
  -> event persistence
  -> result returned to agent
```

The first P5 slice implements this boundary in `internal/toolgateway`. It defines normalized `ToolCall`, `Decision`, `Proposal`, `Execution`, `Result`, and `Outcome` values with bounded UTF-8 fields and legal status combinations. Production CLI, Session, and TUI paths use compatibility adapters over the same Gateway; direct construction of workspace read tools, `toolrun.Manager`, and `fileedit.Manager` is confined to the Gateway.

Workspace IDs are resolved to Store-owned roots before production reads or writes, and a mismatched caller path is rejected before policy or filesystem access. Run-bound calls are atomically charged against `MaxToolCalls`; legacy unbound Sessions remain untracked for compatibility. Scoped low-risk reads use automatic approval. Shell and whole-file replacement normally use per-call approval, while policy rejection maps to permanent denial. Shell completion remains dry-run.

Schema v11 makes per-call review a durable two-phase operation. Creating a Shell or FileEdit proposal inserts one fingerprint-bound `tool_approvals` record in the same SQLite transaction as the compatibility proposal and appends `approval.requested`. Review first commits an immutable domain-separated SHA-256 digest of the client key in `approval_operations` plus `approval.decided`, then advances the ToolRun/FileEdit state. The raw client key is not persisted. If the process exits between those commits, replaying the same key resumes the proposal transition. A legacy approval created before its Session gains a Run is transactionally adopted later with `approval.bound`. Reusing a key with different intent, changing a proposal fingerprint, creating a ghost approval, or writing `approved`/`applied`/`completed` without the matching durable decision is rejected at the Store boundary.

Schema v12 adds `approval_session_grants`, grant-operation idempotency, `run_tool_usage`, and ordered `run_tool_calls`. A grant is exact-scope authorization over Run, active Session, Workspace, Tool, and ActionClass. Gateway proposal creation still runs Policy first; only an allowed proposal may consume a matching active grant, and `tool_approvals.grant_id` records that authorization fact. Revocation is optimistic, durable, and immediately removes the grant from lookup while leaving already completed actions auditable. Tool charging uses a transactionally serialized counter and ordered event. The first rejected call beyond the limit records one `tool.budget_exhausted`; repeated rejection does not duplicate that terminal budget fact.

For schema v17 Supervisor calls, the same tool-budget transaction first validates the active Run lease fencing token. A stale worker therefore cannot consume a call before its later structured-memory write is rejected. Non-Supervisor CLI/Session calls retain their established budget path and do not synthesize execution credentials.

`toolgateway.Store` requires the grant and tool-budget contracts at compile time. Script persistence is an explicit optional Gateway capability (`scriptprocess.Store` plus `toolgateway.ScriptRunStore`), so Session-only backends are not coupled to Run-creation methods they never use. A backend cannot execute the script workflow unless it implements both typed Process persistence and the atomic Run/Process transaction.

Schema v13 promotes scripts out of legacy Shell ToolRuns into `script_process_proposals`. `script run` requires a persisted workspace and a workspace-relative existing file, then submits executable/argv through the Gateway as a validated `script_process.v1` envelope whose execution mode is fixed to `disabled`. Mission, Session, Run, initial budget charge, Policy decision, Process, Approval, and Run events commit in one SQLite transaction. A domain-separated digest of `--idempotency-key` supports recoverable replay without storing the raw key; changed intent under the same key is rejected. Multiple Process proposals may belong to one Run, while Store checks require every Process Run, Session, and Workspace binding to agree.

`--local` changes only the recorded requested backend. Approval first commits the durable decision, advances the typed Process through `approved`, then completes it with a dry-run JSON result. The intermediate approved state is recoverable after interruption. No CLI path constructs a Local/Noop runner, and Policy-denied processes can never be promoted by review.

Schema v14 adds `run_artifacts`. A Run-bound terminal Shell or ScriptProcess, a failed FileEdit diagnostic, or an automatic workspace read/list invocation captures each non-empty stdout/stderr stream before the ordinary Result is truncated. The Artifact Store requires exact Run/Session/Workspace and persisted source linkage, normalizes UTF-8, applies redaction again at the Gateway and Store boundaries, stores at most 4 MiB per stream, and computes SHA-256 over the redacted content. The row and metadata-only `artifact.created` event commit atomically. Replaying a completed proposal reuses `(run_id, source_id, stream)`; different content or metadata conflicts. A capture failure after terminal proposal completion is recoverable by replay without repeating the approval or tool lifecycle event. The legacy v1 `artifacts` table remains a generated-file path registry for old Task workflows; it is intentionally separate from the content-bearing, Run-scoped v14 table.

Schema v15 adds automatic, create-only `work_item_create` and `note_create` actions under the new `run_memory` class. Calls carry a typed JSON payload and a non-serializable operation key, while Run, Session, Workspace, and requester identity come from the Go control plane. Strict decoding, normalization, dependency-ID shape validation, required identity checks, and executor availability happen before the budget charge. Policy and the authoritative persisted Run/Session/Workspace binding check happen after charging because a well-formed attempted call consumes budget. Denied calls append a durable decision without mutation; allowed calls atomically commit the redacted entity, allowed decision, domain event, `tool.completed`, and `structured_tool_operations` ledger row. The ledger stores only a domain-separated operation-key digest and normalized request fingerprint. Same-key/same-intent retries return the original target, changed intent conflicts, and concurrent calls from independent SQLite connections converge on one row. SQLite connections use immediate write transactions with the existing busy timeout to avoid deferred read-to-write lock races. Replays, conflicts, scope failures, and denials still count as invocations; malformed input does not.

Structured-memory Results contain metadata only and therefore do not create output Artifacts. Create is automatic because it is additive and reversible through operator lifecycle commands; update, complete, cancel, archive, and restore remain outside the model tool surface.

Schema v16 persists `run_supervisor_tool_rounds` and `run_supervisor_tool_calls`. One successful primary model response and its pending batch commit in the same transaction; each result and round-completion event is also transactional. Restart recovery executes only pending calls, while terminal results are replayed into the Provider transcript as Anthropic-compatible `tool_use`/`tool_result` blocks. A response is limited to four calls and a turn to four rounds. Provider call IDs are validated at ingress but replaced with deterministic local protocol IDs; the idempotency operation key derives from Run, turn, tool name, and redacted canonical arguments, so changed Provider IDs and repeated semantic intent converge. Policy denial and tool-budget exhaustion become bounded error results; storage, cancellation, and internal failures leave the call pending. Protocol repair advertises no tools, and Shell, file, process, network, update, delete, completion, and archive actions remain outside the Provider surface.

Schema v30 rebuilds only the Supervisor-call constraint so `specialist_delegation_propose` can share those same bounded rounds without changing v16 rows. The new `agent_proposal` class has a dedicated executor and operation table rather than masquerading as Run memory. Syntactic protocol failures are rejected before tool-budget charging; a well-formed proposal attempt is charged, then Policy and authoritative root/capability/budget checks run. A Policy denial or semantic rejection is returned as bounded tool-result metadata so the model may correct its suggestion, but no failed attempt leaves a proposal row. Successful output includes a proposal ID, assignment count, and explicit `admission_authorized=false`, never assignment text or execution credentials.

Existing `tool_runs` and `file_edits` remain compatibility proposal records, while typed script processes use their own v13 table. `tool_approvals` is the single authorization fact used to gate every privileged transition, and transactional Run-event projection is preserved. Gateway Results enforce 128 KiB stdout, 32 KiB stderr, valid UTF-8, MIME metadata, truncation flags, secret redaction, and Artifact reference metadata; the larger redacted Artifact remains separately inspectable and hash-verifiable. Store JSON redaction parses payloads with exact numbers, redacts string values recursively, and re-encodes them so escaped nested JSON cannot be corrupted. Payloads are capped at 1 MiB, 64 levels, and 100,000 nodes.

## Sandbox

Schema v48 introduces `sandbox_manifest.v1` as a descriptive protocol before any backend is enabled. Go strictly decodes and normalizes executable/ordered argv, a virtual working directory, workspace-relative mounts, exact network scope, resource limits, environment literal/secret-reference bindings, input Artifact IDs, output capture/paths, timeout, and cancellation grace. Unknown or duplicate fields, invalid UTF-8, trailing data, traversal, overlapping mounts, wildcard or non-canonical network targets, credential-shaped literals, and values outside hard bounds fail closed.

The application binds one normalized fingerprint to an exact non-terminal Run, Mission, persisted Workspace root, normalized Mission Scope, Policy decision, optional exact approval, requester, and generated cancellation identity. A Manifest may only narrow the Mission network allowlist. Docker/Local intent, writable mounts, network, or secret references require approval when Policy allows them; permanent denial remains final.

SQLite stores immutable preparation, validation, and digest-keyed operation metadata plus content-free events. It does not retain executable, argv, paths, environment values, secret references, network targets, or Manifest JSON. Same-key and cross-process replay converge; changed intent conflicts. `NoopRunner` is the only application validator. Go types, SQL checks/triggers, events, and CLI output all fix `backend_enabled=false` and `execution_authorized=false`, including for approved records. `LocalRunner` and `DockerRunner` fail closed and start no process.

The target model remains one isolated environment per Run, shared only by Agents in that Run through the Go coordinator. A future execution step must resupply the Manifest, reproduce its fingerprint, re-resolve mount sources under the Workspace root, and recheck Policy, Scope, approval, budget, and execution lease before it can even become a candidate.

Schema v49 implements that candidate boundary without enabling a backend. Sandbox approval requests use the existing `tool_approvals` table and bind the preparation ID, Run Session, Workspace, `sandbox.manifest/sandbox_execute`, and authorization fingerprint. Operator review is explicit; pending or denied records cannot pass candidate validation. Approval never overrides permanent Policy denial.

Candidate validation receives the full Manifest again and recomputes every v48 binding. Go `os.Root` opens each mount source relative to the resolved Workspace root and rejects escaping symlinks or non-file/non-directory objects. The application then snapshots aggregate root/Specialist/Fan-out usage and tool-call usage. Candidate insertion takes the Run write lock, recomputes those counters, rejects an active execution lease, and commits an immutable digest-keyed fact only when nothing changed. SQL triggers enforce the same approval, budget, lease, and disabled-backend invariants against direct writes. Raw Manifest, executable, argv, paths, Workspace root, environment values, secret references, and network targets remain transient.

`sandbox_execution_candidate.v1` records only that these checks passed at one point. It always has `backend_enabled=false` and `execution_authorized=false`; no Runner is called. See [ADR 0009](adr/0009-sandbox-approval-candidate.md).

Schema v50 adds a disabled lifecycle above that candidate. `sandbox_execution.v1` is immutable and one-to-one with its v49 candidate. Creation resupplies the complete Manifest and rechecks every prior binding, then pins each input Artifact by exact Run/Session/Workspace, ordinal, identity, content digest, size, MIME, stream, source, and redaction state under a 16 MiB aggregate limit. The output plan retains only capture flags, path count, byte limit, and a digest. Neither raw output paths nor Artifact content enter the lifecycle ledger.

Sandbox ownership is deliberately separate from the Run execution lease. A generation-fenced `sandbox_execution_leases` row permits cleanup after the Run becomes terminal and prevents a stale worker from releasing or committing over a successor. The private lease/cleanup rows necessarily retain opaque lease and worker identities for fencing, while events and CLI projections omit both. The initial generation can only prepare the disabled root and is released immediately. Immutable cancellation and cleanup operations converge under digest-only idempotency keys. Cleanup revalidates all inputs while holding the active Sandbox lease; v50 accepts only `backend_disabled`, with no started backend, orphan, or output Artifact. See [ADR 0010](adr/0010-disabled-sandbox-lifecycle.md).

Schema v51 adds a one-to-one disabled preflight above each eligible v50 lifecycle. The Application must resupply and normalize the complete Manifest, then revalidate preparation, candidate, lifecycle, Mission Scope, Policy, exact approval, workspace mounts, aggregate budgets, Run-lease quiescence, and every bound input Artifact before it can append the fact. SQLite independently binds the immutable v48-v50 identities, current usage, released Sandbox lease, non-terminal Run, absence of cancellation/cleanup, and exact input rows. Digest-only operations and a unique execution binding make same-intent retries converge while rejecting a second preflight identity.

The preflight freezes a 16-item backend threat model and a metadata-only output-export design. `DisabledBackendInspector` is the sole production inspector: all checks are required but unverified, backend availability is false, and container identity is explicitly unbound. Output slots retain only domain-separated locator fingerprints and kinds; file slots require regular files and reject symlinks/special files, while every slot requires MIME detection and redaction. The plan is all-or-nothing, aggregate-byte bounded, restart-reconciled, export-disabled, and unable to commit an Artifact. Root/check/slot/operation rows are immutable, and CLI/events omit locators, raw paths, commands, Manifest content, container identity fingerprints, operation digests, and private leases. See [ADR 0011](adr/0011-disabled-sandbox-preflight.md).

Schema v52 exercises the next protocol boundary without introducing production authority. `SimulationBackendClient` derives a metadata-only `sandbox_backend_evidence.v1` report entirely in memory and has no daemon transport. It requires a canonical OCI image digest, separately fingerprints daemon capabilities, mounts, network, secrets, container configuration, resources, termination, orphan recovery, and the v51 output plan, then emits exactly 16 ordered `simulated_pass` items. Those items are satisfied only for harness assertions and remain `verified=false`; `trust_class=simulation_only`, production verification, backend availability, execution authorization, and Artifact authorization are fixed false in Go and SQLite.

The same schema adds strict `sandbox_output_fixture.v1` input and immutable `sandbox_output_simulation.v1` facts. The in-memory harness matches exact output slot order and kind, applies aggregate limits, MIME detection and redaction, accepts only regular files for file slots, rejects links and special files, and stages every redacted output before one fake atomic commit. Injected failure or cancellation leaves zero fake outputs; the Store independently requires zero production Artifacts and never inserts `run_artifacts`. Evidence and output creation each resubmit the complete Manifest and revalidate all v48-v51 authority, budget, lease, mount, and input-Artifact bindings. Persistence and public projections omit fixture bodies, locators, raw paths, commands, Manifest data, secrets, container IDs, operation digests, and lease identities. See [ADR 0012](adr/0012-simulation-only-sandbox-evidence.md).

Schema v53 adds the first real-daemon boundary, but only as a production metadata observation. `DockerReadOnlyTransport` exposes a fixed endpoint plus ping, daemon version/info, and exact-digest image inspection. The Linux implementation disables proxies and redirects, dials only `/var/run/docker.sock`, and allows GET requests only to `/_ping`, `/version`, `/info`, and `/images/<digest>/json`; it ignores `DOCKER_HOST` and accepts no TCP or caller-selected socket. The Windows implementation records a bounded unsupported-transport result until a separate named-pipe audit exists. No Docker CLI or full client is used, and the interface has no create, pull, start, run, exec, stop, or remove operation.

An explicit CLI confirmation is required before probing. The Application first resupplies the complete Manifest and revalidates the matching v52 evidence and output simulation plus every v48-v51 identity, Scope, Policy, approval, mount, budget, Run/Sandbox lease, input Artifact, cancellation, and cleanup condition. SQLite repeats current-state authority checks before committing an immutable root, six ordered items, and digest-only operation. Same-intent replay returns the prior result without a second probe, cross-process races converge, and each output simulation accepts at most eight observations.

The only durable statuses are complete observation, unavailable daemon, and unavailable image. Complete observation means that bounded daemon and image metadata were read; it does not verify the v51 controls. Private mount propagation cannot be established through these read-only APIs and is explicitly `not_observable_read_only`. Production verification, backend availability/enabling, execution authorization, and Artifact-commit authorization remain false in Go and SQLite. Raw daemon ID/name/root, socket, security-option text, image ID/RepoDigests, GraphDriver details, Manifest, command, operation key, and private leases are transient and absent from events and CLI. See [ADR 0013](adr/0013-read-only-docker-observation.md).

Schema v54 freezes the next boundary without adding a write-capable transport. `CompileDockerContainerSpec` accepts only a complete v53 observation and a resubmitted exact Manifest after Application and SQLite independently revalidate every v48-v53 authority binding. It deterministically fixes user `65532:65532`, read-only root and input mounts, exactly one writable output mount, `rprivate` propagation, no-new-privileges, all capabilities dropped, init, network `none` or managed default-deny exact allowlisting, ephemeral secret mounts, CPU/memory/PID/output/time limits, SIGTERM/SIGKILL behavior, and authority-derived orphan labels.

The complete specification and its commands, arguments, paths, targets, environment values, secret references, labels, and container name remain in memory. Immutable plan roots retain only bounded counts, control results, fake transaction steps, and domain-separated fingerprints. Sixteen controls remain `compiled_not_applied` with `applied=false` and `verified=false`. The seven-step fake transaction orders reconcile, create, start, wait, stop, export, and remove; failure, simulated crash, or cancellation publishes nothing. Even its successful result fixes daemon writes, backend contact, production submission, execution/export, and Artifact-commit authority to zero. See [ADR 0014](adr/0014-deterministic-docker-container-plan.md).

Schema v55 introduces a separate, default-disabled `DockerContainerWriteTransport` without extending the v53 observer. The Linux implementation fixes `/var/run/docker.sock`, Docker API `1.40`, no proxy or redirects, and a closed allowlist containing exact image inspect, deterministic-name create, exact container inspect, and returned-ID non-forced delete with fixed `v=1` anonymous-volume cleanup. It rejects `DOCKER_HOST`, TCP, caller-selected sockets, pull, start, exec, attach, logs, archive/export, volume management, image build, and generic requests. Windows remains explicitly unsupported.

Only an explicit operator confirmation and a complete current v54 plan can enter the strict no-network/no-environment/no-secret profile. Before create, the local image RepoDigest must match the plan and its config must declare no `VOLUME`. Host mount sources are resolved component by component beneath the trusted workspace without symlinks. The stopped container must exactly match non-root/read-only/no-new-privileges/drop-all/init/resource/private-mount plus attachment/device/port controls before it is removed. A stale deterministic name is reconciled only when the existing container is an exact never-started authority match. Cancellation, failure, and uncertain create responses use an independent five-second context to re-inspect by ID/name and never delete blindly.

The immutable v55 rehearsal records five steps, three daemon reads, two normal writes or three writes after exact stale reconciliation, and metadata/fingerprints only. Raw IDs, paths, commands, environment values, secrets, socket paths, and complete specifications remain transient. Replay returns before transport access and concurrent Stores converge. Go and SQLite fix container/process start, image pull, output export, production verification, backend enablement, execution authority, and Artifact authority to false. See [ADR 0015](adr/0015-bounded-docker-write-rehearsal.md).

Schema v56 closes the process-crash window between a v55 daemon mutation and its final durable result. Before the first mutation, the Application writes one immutable attempt intent and acquires a bounded generation-fenced lease. The transport then stages one deterministic-name, never-started container, either by creating it once or adopting an exact authority match left by an uncertain response or prior generation. A durable stage checkpoint freezes 19 exact configuration controls, including an actually empty inherited environment, but every item is fixed to `execution_evidence=false`.

Cleanup begins only after the stage checkpoint is durable. It re-inspects the deterministic name and removes only an exact authority, request, configuration, and container-ID-fingerprint match; absence is idempotent and a mismatched same-name container is never removed. Bounded failure codes release the lease, and an operator can resume from the durable attempt ID only by resubmitting the full Manifest and explicit daemon-write confirmation. Stale generations cannot replay checkpoints under a newer owner. The legacy rehearsal, operation, v56 completion, and final lease release commit atomically. Persistence and projections remain metadata-only, while the v55 fixed endpoint and closed no-start/no-exec/no-pull/no-export operation set are unchanged. See [ADR 0016](adr/0016-recoverable-docker-rehearsal-attempt.md).

Schema v57 inserts a separately confirmed host-input capture gate between v56 stage and cleanup. `DockerHostInputStager` is default-disabled. On Linux, the local implementation opens the absolute workspace root and every normalized read-only source with `openat2` no-symlink/no-magic-link/beneath-only/no-cross-device resolution. Each entry is first inspected through `O_PATH`; only a matching regular-file or directory descriptor is then opened for traversal or reading, so FIFOs and other special files fail before a potentially blocking content open. It supports directory and single-file mounts, bounds directory enumeration before allocation, observes cancellation during file reads, rejects hard links, traversal and resource-limit violations, then rechecks device, inode, mode, link count, size, mtime, and ctime after the complete tree has been pinned.

The stager combines those entries with input Artifacts reverified by exact Run/Session/Workspace, digest, size, MIME, stream, source, redaction state, and order. A deterministic tar uses sanitized archive names, modes, ownership, and timestamps. It exists only in a sealable `memfd`, receives write/grow/shrink/seal kernel seals, and is reread to verify its digest. The immutable v57 intent/result binds current attempt, stopped-container fingerprint, plan, input/authority/spec fingerprints, requester, and lease generation. Semantic fingerprints exclude generated row IDs, so independent workers converge on one intent/result. SQL blocks completion while evidence is missing; failure performs bounded stopped-container cleanup before releasing the lease, and takeover can resume without another create. Missing resume confirmation is rejected before lease acquisition. Durable facts retain only counts, sizes, digests, and false authority claims.

The v57 archive is not handed to Docker. `daemon_consumed=false` and `execution_evidence=false` are protocol and SQL invariants, so this closes replacement during descriptor capture but does not prove the bytes a future container would consume. A daemon-owned immutable handoff remains an independent gate. See [ADR 0017](adr/0017-descriptor-sealed-host-input-staging.md).

- local sources are copied or mounted read-only by explicit manifest entries;
- writable outputs use dedicated run directories;
- network access starts disabled or scope-limited;
- CPU, memory, process, output, and wall-clock limits are part of run configuration;
- teardown is idempotent and records cleanup failures;
- the Docker client is introduced only with the real backend.

`LocalSandbox` remains development-only and must use the same approval/event pipeline. It is never treated as an isolation boundary. None of the target-model bullets are execution claims for schemas v48-v57; v51 names required evidence, v52 exercises its shape against fakes, v53 observes only daemon/image metadata, v54 compiles and fake-stages a plan in memory, v55 only creates, inspects, and removes a container that is never started, v56 only adds durable recovery and non-execution control evidence, and v57 captures a kernel-sealed bundle that the daemon does not consume. None verifies process isolation or a production execution control. Real daemon input handoff, mount/network enforcement, secret materialization, timeout/kill, running-container orphan reconciliation, and output Artifact export remain separately gated work. See [ADR 0008](adr/0008-sandbox-manifest-boundary.md), [ADR 0009](adr/0009-sandbox-approval-candidate.md), [ADR 0010](adr/0010-disabled-sandbox-lifecycle.md), [ADR 0011](adr/0011-disabled-sandbox-preflight.md), [ADR 0012](adr/0012-simulation-only-sandbox-evidence.md), [ADR 0013](adr/0013-read-only-docker-observation.md), [ADR 0014](adr/0014-deterministic-docker-container-plan.md), [ADR 0015](adr/0015-bounded-docker-write-rehearsal.md), [ADR 0016](adr/0016-recoverable-docker-rehearsal-attempt.md), and [ADR 0017](adr/0017-descriptor-sealed-host-input-staging.md).

## Scope and Safety

Authorization scope is a system-owned run snapshot. User instructions and model messages may narrow scope but cannot expand it.

Scope includes:

- permitted workspace roots and file access mode;
- approved domains, IP ranges, ports, and protocols;
- whether network access is disabled, allowlisted, or unrestricted by an authorized operator;
- allowed tool classes and required approvals;
- secret-handling and artifact-export rules.

The initial product remains conservative: real public-network attack automation and unapproved shell execution stay disabled.

## LLM, Context, and Skills

The LLM router remains independent from orchestration. Run snapshots record the selected provider/model route without persisting API keys. Providers normalize HTTP, network, protocol, and cancellation errors into typed outcomes; only RunSupervisor decides whether a side-effect-free model request may be retried. Legacy unbound Session chat receives typed errors through Router but does not gain an implicit retry loop.

Environment adapters currently expose `mimo` and `deepseek` as separate names over the shared Anthropic-compatible transport. Each adapter reads only its own API-key/base-URL/model namespace, and the Provider object remains inside the Go control plane; credentials never enter Run configuration or event payloads.

Context is assembled from:

- system safety and run scope;
- agent identity and assigned work;
- compacted conversation summary;
- recent messages;
- active work items;
- selected notes/evidence.

Schema v43 puts a provenance boundary in front of every persisted Session message. `context_provenance.v1` distinguishes operator intent, model output, Go control text, workspace files/listings/diffs, tool results, and Go command results. Current rows carry a SHA-256 of the redacted content plus an explicit `instruction_authorized` bit; SQLite enforces the role/source/authority matrix and immutable content/provenance, while Go recomputes the digest on read. Legacy rows are conservatively backfilled as `context_provenance.v0`; recognizable `/read`, `/ls`, `/write`, and `/run` replies are downgraded from assistant history to tool evidence.

Model projection is separate from persistence. Trusted operator, model, and Go-control records retain their conversational roles. Every file, tool, diff, listing, command, or unclassified legacy record is rendered as a user-role `untrusted_context.v1` JSON envelope containing source kind, bounded reference, digest, `instruction_authorized=false`, and redacted content. Compaction writes provenance-preserving JSON transcript records and replays the transcript as user data, never as a fresh system instruction. Root WorkBoard/Note/inbox memory is likewise user-role untrusted context; embedded Skills and Go mode/policy contracts remain the only additional system guidance. This boundary applies to direct Session chat, RunSupervisor history, and Specialist history. Read-only Fan-out already uses a separate no-tool prompt that labels file bytes as untrusted data.

This design contains authority rather than attempting to classify every malicious sentence. A README can still contain false or contradictory facts, and a model can still reason badly about them, but document text cannot acquire Go capabilities or silently become system/assistant history. Policy, scope, approvals, budgets, leases, and Tool Gateway remain authoritative even if a model follows an indirect injection semantically.

Skills are versioned knowledge packages with metadata, applicability rules, prompt content, and optional tool prerequisites. The embedded `skill.v1` Registry strictly pins metadata and content identity; a bounded version index retains at most eight embedded versions per Skill, exposes only the current version for new selection, and resolves historical content only by an already-persisted exact version. Schema v39 adds one immutable, Profile-compatible, aggregate-budgeted `skill_selection.v1` per Run with digest-only operation replay. Schema v40 adds root-only `skill_context.v1`: each Supervisor turn reloads only the persisted selection, rechecks exact version/hash/bytes/Profile, redacts before independent budgeting, and injects deterministic in-memory system guidance. Metadata-only preparation commits atomically with the first `model.started`; no Skill text, path, name, or content hash is persisted in that ledger or its events. Prerequisites grant no tool capability, and the root policy remains authoritative. Profiles include `code`, `review`, `learn`, and `script`; Specialist minimization and CTF Skills remain later work.

Schema v41 separates product behavior from permission through immutable Run modes: one fixed `code|cyber` surface and an operator-controlled `plan|deliver` phase. Schema v42 then adds strict `plan_delivery.v1` under that boundary. A fenced root Plan turn can persist exactly three bounded directions through the `agent_proposal` tool class, but cannot select or execute one. After the Run pauses and releases its lease, the operator-only selection service atomically projects one direction into existing WorkItems, backward dependency edges, a pinned decision Note, an immutable selection, and metadata-only events. Selection remains in Plan and grants no capability; phase transition is a separate schema v41 operation. HTTP, TUI, and React read the same bounded projection and have no selection route. The embedded cross-Profile `plan-delivery` Skill is subordinate guidance rather than workflow authority.

## Findings and Reports

A finding is not accepted because a model stated it. Schema v35 therefore projects every Fan-out result as `draft` and labels its provenance `model_assertion`. Schema v36 lets an operator attach frozen, same-Run Artifact Evidence and make one immutable `validated` or `rejected` decision. Schema v37 keeps validation distinct from acceptance: an operator explicitly appends `accepted`, attaches fresh post-acceptance remediation Evidence, and only then appends `fixed`. No lifecycle overlay rewrites the model assertion or earlier decisions. Generic finding categories include code defect, security weakness, failed test, policy violation, and improvement opportunity.

Reports are projections built from persisted state, not mutable globals. Schema v35 provides deterministic Markdown and JSON with a stable source projection digest; schemas v36-v37 render validation, acceptance, remediation, and fix overlays without changing that digest. The read-only SARIF 2.1.0 renderer exports only confirmed unresolved `validated` and `accepted` Findings as `results`, while retaining draft/fixed/rejected counts as metadata. This stricter boundary is intentional because GitHub Code Scanning consumes only a SARIF subset and ignores `result.kind`; unconfirmed or already-fixed claims therefore cannot become alerts by parser behavior. Stable severity rules, workspace-relative escaped URIs, v35 Finding fingerprints, and separate validation/lifecycle properties support portable identity without exposing Artifact content or operator narratives. The adjacent CI gate defaults to validated/high, includes accepted unresolved items, admits drafts only through explicit `active`, and never matches fixed/rejected. Its GitHub Actions renderer consumes the exact JSON-hidden matches already selected into `GateResult`, emits escaped workflow commands, and performs no second lifecycle decision. None of these paths writes Store state or calls a Provider. Deduplication, lifecycle validation, rendering, and gate rules are Go-owned. Optional model-assisted comparison cannot become authoritative.

## Events and Interfaces

All user-facing surfaces consume normalized events:

```text
run.created
run.status_changed
run.execution_lease_acquired / run.execution_lease_taken_over / run.execution_lease_released
agent.created
agent.status_changed
agent.message
model.started / model.completed / model.failed
model.cancel_requested / model.cancel_observed
agent.schedule_started / agent.schedule_stopped
agent.delegation_proposed
skill.selection_created
skill.context_prepared / skill.context_committed
readonly_fanout.planned
readonly_fanout.execution_started / readonly_fanout.execution_recovered
readonly_fanout.shard_started / readonly_fanout.shard_completed
readonly_fanout.shard_failed / readonly_fanout.shard_cancelled
readonly_fanout.execution_completed / readonly_fanout.execution_failed / readonly_fanout.execution_cancelled
report.generated
finding.evidence_attached / finding.validation_decided
supervisor.protocol_repair_requested / supervisor.protocol_repair_started
supervisor.protocol_repair_completed / supervisor.protocol_repair_failed
model.delta (bounded, text-free stream progress)
work_item.created / work_item.changed
note.created / note.changed
tool.proposed / tool.approved / tool.completed / tool.failed
tool.budget_charged / tool.budget_exhausted
file_edit.proposed / file_edit.applied
approval.requested / approval.decided
approval.grant_created / approval.grant_revoked
finding.changed
artifact.created
policy.decided
budget.changed
supervisor.action_committed
supervisor.run_waiting / supervisor.run_completed / supervisor.run_failed
```

CLI and `headless.v1` consume persisted events. The Headless adapter emits one bounded NDJSON `run.event` per durable sequence plus a final `stream.end`, supports numeric sequence resume and optional local SQLite follow, and maps terminal/bound/deadline outcomes to the existing stable CLI exits. It validates continuity and record bounds but never executes a Run or writes Store state. Bubble Tea polls the same local Store sequence in batches of at most 32, retains the latest 50 metadata-only events, validates contiguous Run/Mission-bound sequences, discards stale asynchronous results, and stops after a terminal Run. Each refresh brackets Session messages, ToolRuns, WorkItems, Notes, Agent nodes/completions, and bounded Finding report summaries with the durable event tail; a changing tail retries the composite snapshot up to eight times instead of displaying a torn projection. Event payloads and complete Finding/Evidence bodies are not rendered. The Events, Agents, and Findings tabs are read-only; existing Tool approval and active-call cancellation still cross the Go application service, and Bubble Tea never receives a Provider context. The Go HTTP adapter exposes persisted metadata over bounded resumable SSE. Transient active-call state and any future user-facing text projection still require separate Go-owned lifecycle/redaction design. Persisted `model.delta` remains counter-only.

The same Go adapter owns read projections for the bounded Agent graph, operator-gated delegation lifecycle, read-only Fan-out plans/latest execution summaries, and Finding/Report state. These are intentionally separate DTOs rather than serialized Store records. Fan-out summary queries do not select model report JSON, input/snapshot/report digests, error narratives, or lease/fencing identities; Finding views expose immutable assertion facts and Artifact descriptors but omit Artifact content and private operator narratives. React consumes only the generated OpenAPI shapes and cannot advance any lifecycle. A cross-surface contract test creates one real SQLite Run through the CLI and proves that CLI JSON, the TUI projection, authenticated loopback HTTP, and Headless NDJSON agree on Run, Mission, Session, status, durable event tail, and Agent count.

## Persistence

SQLite remains the local source of truth. Schema migration `v1` records the legacy baseline, `v2` adds the first run-centric tables, `v3` enforces Run/Session projection constraints, `v4` adds the idempotent legacy Task mapping, `v5` adds durable Supervisor checkpoints, `v6` adds cumulative model budgets, `v7`-`v18` add resumable Supervisor, memory, approval, tool, Artifact, lease, and cancellation ledgers, `v19`-`v29` add the bounded Agent graph, inbox, Specialist runtime, context, repair, scheduling, and cancellation ledgers, `v30`-`v32` add review-gated core delegation proposal/review/application, `v33` freezes read-only Fan-out plans, `v34` adds lease-fenced Fan-out execution, `v35` adds immutable generic Finding/Evidence/Report projections, `v36`-`v37` add frozen validation and remediation lifecycle facts, `v38` adds explicit operator Specialist scheduling, and `v39` adds immutable Run Skill selections. Migrations are ordered, checksummed, transactional, and safe to apply repeatedly; legacy databases are upgraded without deleting their data.

```text
missions
runs
run_events
run_execution_leases
work_items
work_item_dependencies
notes
note_tags
note_sources
note_evidence
tool_approvals
approval_session_grants
run_tool_usage
run_tool_calls
script_process_proposals
run_artifacts
structured_tool_operations
agent_nodes
agent_messages
agent_message_operations
agent_admission_operations
agent_attempts
agent_attempt_mutations
agent_completion_reports
agent_completion_operations
root_inbox_deliveries
specialist_model_calls
specialist_context_deliveries
specialist_protocol_repairs
specialist_schedules
specialist_schedule_agents
specialist_model_cancellations
specialist_model_cancellation_operations
specialist_delegation_proposals
specialist_delegation_assignments
specialist_delegation_operations
readonly_fanout_plans
readonly_fanout_files
readonly_fanout_shards
readonly_fanout_executions
readonly_fanout_execution_shards
readonly_fanout_model_calls
readonly_fanout_findings
finding_reports
findings
finding_evidence
finding_artifact_evidence
finding_artifact_evidence_operations
finding_validation_decisions
finding_validation_operations
sandbox_manifest_preparations
sandbox_manifest_validations
sandbox_manifest_operations
sandbox_execution_candidates
sandbox_execution_candidate_operations
sandbox_disabled_executions
sandbox_execution_inputs
sandbox_execution_leases
sandbox_execution_operations
sandbox_execution_cancellations
sandbox_execution_cancellation_operations
sandbox_cleanup_results
sandbox_cleanup_operations
agent_graph_snapshots
```

Schema v37 now stores acceptance/remediation/fix history. GitHub Actions annotations are a later read-only renderer over the same GateResult and require no schema migration; additional platform adapters remain future work.

Existing tables remain available during migration. JSON files may be exported for portability but are not authoritative state.

## Target Package Layout

```text
cmd/cyberagent/             CLI entrypoint
internal/domain/            Mission, Run, AgentNode, WorkItem, Note, Finding, Report
internal/application/       Supervisors and use-case services
internal/coordinator/       Agent graph, inbox, scheduling, cancellation
internal/events/            Event envelope, subscriptions, projections
internal/memory/            Notes, work board, context selection
internal/approval/          Unified privileged-action decisions
internal/report/            Findings, evidence, report projections
internal/skills/            Skill registry and loading
internal/llm/               Provider interfaces and routing
internal/tools/             Tool definitions and workspace-safe tools
internal/toolgateway/       Unified scope, policy, approval, budget, execution, and result boundary
internal/runmutation/       Content-free idempotency identity and fingerprints
internal/sandbox/           Backend interfaces and Docker/local runners
internal/store/             SQLite stores and migrations
internal/session/           Compatibility conversation service
internal/tui/               Bubble Tea adapter
internal/httpapi/           Loopback-only read API, OpenAPI contract, and bounded Run-event SSE
internal/analyzer/          Future Rust JSON process bridge
```

This layout is a migration target. Packages move only when a vertical slice uses the new boundary; unrelated working code is not rewritten for naming alone.

## Reference and Independence

The redesign was informed by the public architecture and product behavior of [usestrix/strix](https://github.com/usestrix/strix), especially its resumable run state, addressable agent graph, per-agent work memory, sandbox lifecycle, explicit completion tools, event-driven UI, skills, and structured reports.

CyberAgent Workbench does not copy Strix source code or reproduce its Python architecture. The implementation remains original Go code with stricter approval defaults, SQLite as the authoritative state store, a separate Rust analyzer boundary, and a broader generic-agent scope.
