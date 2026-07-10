# Architecture

CyberAgent Workbench is evolving from a CLI-first agent scaffold into a run-centric, resumable AI workbench. The redesign keeps the existing Go implementation and safety boundaries while organizing them around explicit execution ownership.

## Design Goals

- Go remains the sole control plane.
- One user objective is a `Mission`; one execution attempt is a `Run`.
- Every state change is auditable and recoverable from SQLite.
- Agent concurrency is coordinated by one owner, not by agents calling each other directly.
- Privileged actions always cross policy, approval, scope, and sandbox boundaries.
- CLI, TUI, future Web UI, and CI use the same application services.
- Rust analyzers remain deterministic tools behind Go.
- CTF-specific behavior remains a late profile, not the runtime foundation.

## Control Plane

```text
CLI / Bubble Tea TUI / Headless CI / Future TypeScript UI
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

The current single-Agent slice persists cumulative input/output/total tokens, model-call execution milliseconds, and the redacted pending user input in the Supervisor checkpoint. A bounded executor performs only an operator-selected number of durable steps. Root model output uses strict `root_lifecycle.v1` JSON: `continue` returns to idle, `finish` completes the Run, and `wait` pauses it for explicit resume. Turn data, status, checkpoint, Session messages, and events commit in one transaction; arbitrary assistant text cannot mutate lifecycle state.

Provider calls use typed outcomes: `retryable`, `rate_limited`, `invalid_response`, `cancelled`, or `permanent`. RunSupervisor retries only the first two, at most three transport attempts per protocol phase by default, with cancellation-aware exponential backoff. Server `Retry-After` is honored only when it is within the local maximum wait; a longer delay returns a stable rate-limit result instead of retrying early. Each call receives a durable global sequence number plus a phase-local transport number and emits `model.started` plus exactly one terminal model event. Terminal event persistence, token usage, and model execution time share one transaction, so restart recovery neither loses nor double-charges completed calls.

Every Supervisor model attempt uses `StreamChat`. The stream aggregator reconstructs UTF-8 across transport chunk boundaries, caps model output at 64 KiB, requires one final completion chunk with valid usage, and rejects tool calls before lifecycle parsing. Mid-stream transport failures use the same typed retry policy, lifecycle-protocol repair, budget accounting, and terminal transactions as a non-stream response.

Incremental persistence is deliberately metadata-only. One attempt may append at most 32 ordered `model.delta` events carrying sequence, chunk count, byte count, cumulative bytes, and completion state. The Store validates idempotent replay, strict ordering, size limits, and exact agreement between the durable delta ledger and the terminal model event. Model text remains in bounded process memory until the validated turn transaction writes the final redacted assistant message.

The Go application layer owns an in-process `ActiveCallRegistry`. A call is reserved before `model.started` to reject concurrent Provider calls for the same Run, but it becomes queryable and cancellable only after that durable start succeeds. Registry entries are keyed by Run plus Supervisor/model-attempt identity, own the Provider cancellation function, and are removed on every Provider terminal path. Explicit cancellation writes one idempotent, redacted `model.cancel_requested` event before signalling the context.

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

## Work Board and Notes

Conversation history is not enough for long-running work. Each run therefore has two structured memory surfaces:

- `WorkItem`: actionable work, dependencies, owner, priority, status, and acceptance criteria.
- `Note`: observations, hypotheses, decisions, summaries, and references to evidence.

Work items and notes are stored independently from LLM messages. Context construction selects only relevant summaries, active work, recent messages, and explicitly loaded notes.

The current P3 implementation persists both surfaces. Schema v9 WorkItems use optimistic versions, composite same-Run dependency keys, cycle checks, legal transitions, and transactional `work_item.created/changed` events. Schema v10 Notes add category, visibility, Owner, tags, source references, Evidence IDs, pinning, archive/restore, and transactional `note.created/changed` events. Root context includes `run`, `root`, and `owner=root` Notes but excludes another owner's memory.

Before each model call, a generic Context Section selector ranks the latest compacted summary, bounded active Work Board, pinned Notes, and category-weighted Notes under an 8,192-token estimate. `model.started` records included and omitted `kind/source_id/tokens` metadata so provenance survives restart, while Note bodies remain outside the event. Model-driven root `finish` is rejected through protocol repair while active work remains and checked again under the final SQLite write transaction. Model-authored WorkItem/Note mutation tools are not yet registered in the current Tool Gateway.

## Lifecycle Protocol

Autonomous/headless execution cannot finish with an arbitrary assistant paragraph. The root protocol now validates one versioned JSON lifecycle result:

- root: `continue`, `finish`, or `wait`; `finish` includes a summary and `wait` includes a reason;
- child: `agent.finish` with structured completion report for its parent;
- blocked agent: `agent.wait` with reason and awaited dependency;
- cancellation: coordinator-owned transition, never model-owned.

The current root implementation maps `wait` to a durable paused Run and resumes at the next turn, and it permits one bounded automatic repair for an invalid root response. Child actions and structured dependency records remain future Coordinator work.

## Tool Gateway

Every tool invocation uses one pipeline:

```text
Model proposal
  -> schema validation
  -> scope resolution
  -> policy decision
  -> approval classification
  -> sandbox/runtime execution
  -> output limits and redaction
  -> evidence/artifact capture
  -> event persistence
  -> result returned to agent
```

The first P5 slice implements this boundary in `internal/toolgateway`. It defines normalized `ToolCall`, `Decision`, `Proposal`, `Execution`, `Result`, and `Outcome` values with bounded UTF-8 fields and legal status combinations. Production CLI, Session, and TUI paths use compatibility adapters over the same Gateway; direct construction of workspace read tools, `toolrun.Manager`, and `fileedit.Manager` is confined to the Gateway.

Workspace IDs are resolved to Store-owned roots before production reads or writes, and a mismatched caller path is rejected before policy or filesystem access. Scoped low-risk reads use automatic approval. Shell and whole-file replacement use per-call approval, while policy rejection maps to permanent denial. Shell completion remains dry-run. The session approval mode exists in the domain contract but has no persisted grant implementation yet.

Existing `tool_runs` and `file_edits` remain the durable compatibility records, so their transactional Run-event projection is preserved. Gateway outputs enforce hard stdout/stderr/preview limits, valid UTF-8, MIME metadata, truncation flags, and secret redaction. A unified durable proposal table, Artifact capture rules, model-authored WorkItem/Note tools, and removal of the legacy local-script bypass remain P5 work.

## Sandbox

Sandbox backends implement a Go interface and are selected per run. The target model is one isolated environment per run, shared only by agents in that run through the Go coordinator.

- local sources are copied or mounted read-only by explicit manifest entries;
- writable outputs use dedicated run directories;
- network access starts disabled or scope-limited;
- CPU, memory, process, output, and wall-clock limits are part of run configuration;
- teardown is idempotent and records cleanup failures;
- the Docker client is introduced only with the real backend.

`LocalSandbox` remains development-only and must use the same approval/event pipeline. It is never treated as an isolation boundary.

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
- selected notes/evidence;
- progressively loaded skills.

Skills are versioned knowledge packages with metadata, applicability rules, prompt content, and optional tool requirements. Profiles such as `code`, `review`, `learn`, and `script` select baseline skills. CTF skills are added only after the generic runtime is stable.

## Findings and Reports

A finding is not accepted because a model stated it. It must carry structured evidence and validation state. Generic finding categories include code defect, security weakness, failed test, policy violation, and improvement opportunity.

Reports are projections built from persisted state, not mutable globals. Initial outputs are Markdown and JSON; SARIF and CI annotations follow. Deduplication starts with deterministic fingerprints and may use an optional model-assisted comparison as a secondary signal.

## Events and Interfaces

All user-facing surfaces consume normalized events:

```text
run.created
run.status_changed
agent.created
agent.status_changed
agent.message
model.started / model.completed / model.failed
model.cancel_requested
supervisor.protocol_repair_requested / supervisor.protocol_repair_started
supervisor.protocol_repair_completed / supervisor.protocol_repair_failed
model.delta (bounded, text-free stream progress)
work_item.created / work_item.changed
note.created / note.changed
tool.proposed / tool.approved / tool.completed / tool.failed
file_edit.proposed / file_edit.applied
finding.changed
artifact.created
policy.decided
budget.changed
supervisor.action_committed
supervisor.run_waiting / supervisor.run_completed / supervisor.run_failed
```

CLI and headless mode print persisted events. Bubble Tea consumes the bounded in-memory call envelope through a narrow controller, renders provider/attempt/chunk/byte and terminal state, and requests audited cancellation through the application service. Its request-context cancellation is used only as a fallback for legacy or not-yet-active calls; it never receives a Provider context. A future adapter will expose the metadata envelope over WebSocket and add a separately reviewed user-facing text projection. Persisted `model.delta` remains counter-only.

## Persistence

SQLite remains the local source of truth. Schema migration `v1` records the legacy baseline, `v2` adds the first run-centric tables, `v3` enforces Run/Session projection constraints, `v4` adds the idempotent legacy Task mapping, `v5` adds durable Supervisor checkpoints, `v6` adds cumulative token and model-time budget counters, `v7` adds bounded pending input recovery, `v8` adds protocol-repair phase/reason recovery, `v9` adds the Run-scoped Work Board, and `v10` adds Notes plus normalized tag/source/Evidence relationships. Migrations are ordered, checksummed, transactional, and safe to apply repeatedly; legacy databases are upgraded without deleting their data.

```text
missions
runs
run_events
work_items
work_item_dependencies
notes
note_tags
note_sources
note_evidence
```

Later migrations add:

```text
agent_nodes
agent_inbox
findings
evidence
approvals
```

Existing tables remain available during migration. JSON files may be exported for portability but are not authoritative state.

## Target Package Layout

```text
cmd/cyberagent/             CLI entrypoint
internal/domain/            Mission, Run, WorkItem, Note, future AgentNode/Finding
internal/application/       Supervisors and use-case services
internal/coordinator/       Agent graph, inbox, scheduling, cancellation
internal/events/            Event envelope, subscriptions, projections
internal/memory/            Notes, work board, context selection
internal/approval/          Unified privileged-action decisions
internal/report/            Findings, evidence, report projections
internal/skills/            Skill registry and loading
internal/llm/               Provider interfaces and routing
internal/tools/             Tool definitions and workspace-safe tools
internal/sandbox/           Backend interfaces and Docker/local runners
internal/store/             SQLite stores and migrations
internal/session/           Compatibility conversation service
internal/tui/               Bubble Tea adapter
internal/api/               Future HTTP/WebSocket adapter
internal/analyzer/          Future Rust JSON process bridge
```

This layout is a migration target. Packages move only when a vertical slice uses the new boundary; unrelated working code is not rewritten for naming alone.

## Reference and Independence

The redesign was informed by the public architecture and product behavior of [usestrix/strix](https://github.com/usestrix/strix), especially its resumable run state, addressable agent graph, per-agent work memory, sandbox lifecycle, explicit completion tools, event-driven UI, skills, and structured reports.

CyberAgent Workbench does not copy Strix source code or reproduce its Python architecture. The implementation remains original Go code with stricter approval defaults, SQLite as the authoritative state store, a separate Rust analyzer boundary, and a broader generic-agent scope.
