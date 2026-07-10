# Project Status

Last updated: 2026-07-10

## Resume Context

CyberAgent Workbench is a local-first Go agent runtime for cyber-oriented work. The current implementation is a CLI-first runtime with resumable Runs, streamed model calls, persisted sessions, a transactional SQLite event/message/WorkItem/Note store, a unified Tool Gateway, workspace manager, safety policy, sandbox interfaces, context compaction, and token-aware structured-memory selection.

Current product priority: migrate the working v0.1 scaffold into the V2 run-centric, resumable agent runtime described in ADR 0002 and `docs/TASK_BOOK.md`. CTF-specific solving logic is intentionally deferred until the generic runtime is stable.

Canonical remote: `https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench`. After every completed development slice, run tests and audit checks, update project memory, commit the focused changes, and push them to GitHub. PR creation stays under user control; prepare a concise PR summary and validation notes when a feature branch is used.

Use these files first when resuming:

- `README.md`
- `docs/PROGRESS_BOOK.md`
- `docs/TASK_BOOK.md`
- `docs/adr/0001-go-control-plane.md`
- `docs/adr/0002-run-centric-runtime.md`
- `docs/architecture.md`
- `docs/usage.md`
- `internal/app/app.go`
- `internal/application/run_supervisor.go`
- `internal/domain/root_action.go`
- `internal/domain/work_item.go`
- `internal/domain/note.go`
- `internal/application/work_item_service.go`
- `internal/application/note_service.go`
- `internal/contextmgr/context.go`
- `internal/contextmgr/selector.go`
- `internal/session/session.go`
- `internal/toolgateway/model.go`
- `internal/toolgateway/gateway.go`
- `internal/toolrun/toolrun.go`
- `internal/tui/model.go`
- `internal/tui/picker.go`
- `internal/store/sqlite.go`
- `internal/store/work_items.go`
- `internal/store/notes.go`

## Progress Review

- Overall product vision: about 77%.
- v0.1 generic agent MVP: about 98%.
- V2 run-centric runtime: about 92%.
- Project scaffold/framework: about 99%.

Completed:

- Go CLI entrypoint and command dispatch.
- Mock LLM provider and model router.
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
- Secret redaction layer for common API keys, bearer tokens, GitHub tokens, AWS access keys, JWTs, assignment-style secrets, and private-key blocks.
- Redaction is applied at file reads, session message creation, SQLite session/context/tool-run storage, context prompt construction, and final LLM router dispatch.
- Automatic context compaction after long active session histories.
- Optional Anthropic-compatible `mimo` and `deepseek` providers registered from separate environment-variable namespaces only.
- `provider test` and session `/model` both support direct `provider/model` references.
- Tool proposal and approval flow: `/run` creates `tool_runs`; `tool approve` completes dry-run; `tool deny` records denial.
- Persisted file edit lifecycle: `edit propose/list/show/approve/deny` stores redacted whole-file replacements and unified-style diffs in `file_edits`.
- Session `/write <path> <content>` creates a file edit proposal attached to the current session and renders the diff in session history/TUI messages.
- File edit approval re-resolves workspace paths, rejects traversal/symlink escape, verifies original/proposed SHA-256 hashes, refuses stale proposals, and supports resumable `approved` state.
- Existing text files and new text files under existing workspace directories are supported; non-UTF-8 content, missing parents, directories, and files over 256 KiB are rejected.
- Bubble Tea TUI shell with session messages, input, tool run pane, snapshot mode, `/approve`/`/deny` input commands, message scrollback, tool selection, and key-driven approve/deny.
- TUI session picker/start screen: restore existing sessions or press `n` to create a new one.
- TUI async action loop: sends, refreshes, and tool approve/deny actions enter a `busy` state and complete through Bubble Tea commands without freezing the UI event path.
- TUI workspace context side panel: attached sessions show workspace ID/name/root and lightweight local directory counts without reading file contents.
- Safety policy skeleton for high-risk cyber text/tool calls.
- Noop, local, and placeholder Docker sandbox runners.
- Manual context compaction with persisted summaries.
- Accepted ADR 0001: Go is the sole control plane; future TypeScript calls Go over HTTP/WebSocket, while Rust remains a deterministic JSON analyzer behind Go.
- Accepted ADR 0002: Mission/Run aggregates, RunSupervisor, a single AgentCoordinator, structured WorkItems/Notes/Findings, lifecycle actions, and a unified event stream.
- Reworked `docs/architecture.md` around run-scoped budget, event, sandbox, report, approval, and recovery ownership without copying the reference implementation.
- Replaced the obsolete README migration/scaffold copy with a bilingual Chinese/English product overview, current capabilities, architecture boundary, and development-scope notice.
- Added `docs/TASK_BOOK.md` with phased migration tasks, acceptance criteria, compatibility rules, and CTF deferred to the final phase.
- Versioned SQLite migrations through schema v10: legacy baseline, run-centric foundation, Run/Session projection constraints, legacy Task mapping, Supervisor checkpoints, cumulative budget counters, durable pending user input, restart-safe protocol-repair state, the Run-scoped Work Board, and durable Notes; each version is checksummed and transactional.
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
- Added schema v4 `legacy_task_runs` with unique Task, Mission, and Run identities.
- Added `run adapt-task <task-id>` and a transactional TaskAdapter that creates one Session/Mission/Run plus `legacy.task_adapted`, or returns the existing mapping.
- Concurrent and repeated adaptation converges on one Run; historical Task status is audit data and never starts execution implicitly.
- Legacy Task goals and legacy Event messages/payloads are now redacted at the SQLite Store boundary.
- Added schema v5 `run_supervisor_checkpoints` with bounded phase, next-turn, attempt, and redacted last-error state.
- Added `RunSupervisor`, `RunHandle`, and `LifecycleResult`, plus `run step` and `run checkpoint` CLI commands.
- A supervised turn checkpoints before the model call and atomically commits Session messages, policy decision, model usage, completion event, and the next checkpoint.
- Started turns recover across Store/process restart; repeated completion is idempotent and committed turns are not duplicated.
- MaxTurns and preflight cancellation are enforced before model calls; tool calls are rejected and never create ToolRuns in this slice.
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
- RunSupervisor loads at most 20 active WorkItems into a redacted `work_board.v1` JSON system message capped at 16 KiB; terminal items are excluded.
- A model root `finish` conflicts with active work and uses the existing single protocol-repair path. Explicit `run finish` remains the operator override.
- `CompleteSupervisorTurn` repeats the active-item check under its SQLite write transaction, so a WorkItem created by another process during the model call rolls back a stale finish and leaves the turn recoverable.
- Added a pure-Go Note aggregate with five categories, run/root/owner visibility, normalized tags and source/Evidence references, pinning, archive/restore, strict size limits, and optimistic versions.
- Schema v10 persists Notes and normalized relation tables. Composite foreign keys prevent cross-Run relation injection, while Note record changes and `note.created/changed` events commit atomically.
- Added `note create/list/show/update/archive/restore` with bounded UTF-8 content-file input, exact filters, replace/clear relation operations, and optional explicit `--version` locking.
- Root Supervisor memory includes only active run-visible, root-visible, and `owner=root` Notes; archived Notes and another owner's Notes are excluded.
- A generic Context Section selector ranks compacted summary, Work Board, pinned Notes, and category-weighted Notes under an 8,192-token estimate.
- Every `model.started` event records selected and omitted context source IDs/token estimates without persisting Note bodies, preserving restart-safe context provenance.

Not done yet:

- OpenAI-compatible/Ollama providers.
- Dedicated TUI file-edit pane with key-driven edit approval/denial.
- User-visible safe text streaming and cross-process cancellation routing.
- Script generate-run-fix loop with real model calls.
- CTF-specific solving workflows beyond placeholder commands.
- Go HTTP/WebSocket control-plane API, TypeScript Web UI, and Rust analyzer processes.
- Provider cost/tool-call budgets, AgentCoordinator, and Findings/Evidence/Report; WorkItem/Note model tools and dedicated TUI views remain pending.
- Persisted session approval grants, a unified durable approval/idempotency record, tool-output Artifact capture, and removal of the `script run --local` compatibility bypass.

## Code Audit Notes

No high-severity issue was found in the latest slice.

The Tool Gateway audit found and fixed four correctness/security issues: invalid UTF-8 before a truncation boundary could be deleted and misreported as valid text; a tiny output limit could overflow its truncation marker; a persisted shell denial could be mapped inconsistently when a later Store operation also returned an error; and production file calls trusted a caller-supplied workspace root. Regression tests cover each case, and production roots are now bound to the workspace Store record.

Residual risks to address soon:

- `staticcheck ./...` is clean; the prior TUI `S1008`, `S1011`, and unused-helper `U1000` findings were removed in this slice.
- `script run --local` can execute local commands after only lexical policy checks; it needs explicit approval and workspace scoping.
- The Gateway still persists shell and file proposals in the legacy `tool_runs` and `file_edits` tables. Their Run-event projection is transactional, but unified approval idempotency and Session grant records do not exist yet.
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
- A crash after the pre-call checkpoint can repeat a side-effect-free model request, but committed messages and completed turns are never duplicated. Tool calls stay disabled until execution idempotency exists.
- Structured memory now has an 8,192-token estimate, but recent Session history is still bounded by 20 messages rather than sharing that token budget.
- MaxCostUSD and tool-call budgets are not enforced until provider pricing metadata and the unified Tool Gateway exist.
- ExecutionMillis measures Provider model-call time, not total wall-clock orchestration time.
- One Provider response can exceed the remaining token allowance; actual usage is committed conservatively and the next call is blocked.
- Budget exhaustion leaves the Run in `running` until an operator explicitly finishes, fails, or cancels it. Only a validated structured action can change lifecycle state; free-form model text cannot.
- Strict lifecycle JSON has exactly one automatic repair. A Provider that returns two invalid protocol responses fails the turn without an unbounded correction loop.
- Provider retry is enabled only inside RunSupervisor; legacy unbound Sessions receive typed errors but still use the direct, non-retrying Router compatibility path.
- Retry backoff is deterministic and intentionally capped at three attempts/2 seconds for the local single-user runtime. Add jitter before enabling concurrent remote workers.
- A server `Retry-After` above the local ceiling is not auto-retried; the Run remains running with a failed Supervisor turn and preserved input until a later operator retry.
- If the process dies after `model.completed` but before the turn transaction, recovery may repeat the side-effect-free model request under the next durable attempt number. The prior usage is already charged atomically, but tool calls remain disabled for this reason.
- Persisted `model.delta` events intentionally contain counters rather than model text. Historical SQLite replay can reconstruct progress and accounting, not token-by-token content; the current live envelope is also metadata-only until a safe lifecycle/text projection exists.
- Active-call subscriptions are process-local and non-replayable. A full 32-event buffer closes that subscriber; consumers must inspect `Dropped()` and recover from durable Run events.
- Application cancellation is audit-first: if SQLite cannot append the request, the registry does not silently signal an unaudited cancellation. Parent process-context cancellation remains the emergency path and still records `model.failed(cancelled)` when possible.
- Cross-process cancellation is not available until the Go HTTP/WebSocket control plane hosts the shared registry; a second standalone CLI process cannot observe another process's in-memory call.
- TUI live state is transient metadata, not a durable transcript. Disconnect or process exit must recover from SQLite Run events, and user-visible text streaming remains disabled.
- When no active registry item exists, `Ctrl+X` cancels the current application request context after a bounded lookup. This covers legacy/pre-activation calls without fabricating an audited Run cancellation event.
- Root `wait` currently maps to `paused` plus a textual reason; structured dependencies and approvals are future Coordinator/Work Board work.
- Unbound Sessions still use the direct Router compatibility path. New product flows should create a Run instead of expanding this legacy path.
- Slash commands remain separate command adapters and do not consume a Supervisor turn, but `/ls`, `/read`, `/write`, and `/run` now share the Tool Gateway approval/event behavior. Future model-authored calls must use that same boundary without silently enabling execution.
- Pending input is redacted but otherwise stored as Session/model content; this is not a secrets vault. The CLI checkpoint view intentionally omits it.
- Applied migration statements are immutable once released because their checksums are verified. Schema changes must always add a new migration version.
- Schema v3 intentionally rejects duplicate non-empty Run/Session associations. A legacy database containing duplicates must be audited before upgrade instead of silently discarding an association.
- `apperror.Normalize` includes a transitional text classifier for legacy plain errors. New services must return typed errors directly so future localization cannot affect classification.
- WorkItems and Notes are operator/application managed in this slice. Models receive bounded read-only context and cannot mutate either surface until proposal/approval semantics exist in the unified Tool Gateway.
- The Supervisor queries at most 20 active WorkItems and 100 visible active Notes before token selection. SQLite retains overflow, but later relevance search or explicit loading must make those records discoverable.
- Explicit `run finish` can close a Run with unfinished WorkItems as an intentional operator override. Future report projections should surface those unfinished records.
- WorkItem Owner is currently a bounded label, not an AgentNode foreign key. Coordinator identity and per-agent visibility are still future P4 work.
- Note Owner is also a bounded label; `root` is the current Supervisor viewer identity until AgentNode-backed identities exist.
- Note Evidence IDs are structured references rather than foreign keys because the Evidence entity is deferred to the report phase.
- Context token counts are deterministic estimates for selection. Provider-reported usage remains the authoritative budget and billing value.

## Feature Verification

Latest verified commands:

```powershell
go test ./...
go run ./cmd/cyberagent run create "review this workspace" --workspace demo --profile review --max-turns 15
go run ./cmd/cyberagent run start <run-id>
go run ./cmd/cyberagent run step <run-id>
go run ./cmd/cyberagent run execute <run-id> --max-steps 2
go run ./cmd/cyberagent run execute <run-id> --max-steps 2 --finish --summary "planning complete"
go run ./cmd/cyberagent run finish <run-id> --summary "review complete"
go run ./cmd/cyberagent run fail <run-id> --reason "blocked by provider"
go run ./cmd/cyberagent run checkpoint <run-id>
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
go run ./cmd/cyberagent tool approve <tool-run-id>
```

Expected context behavior:

- `context compact` writes one row to `context_summaries`.
- Recent messages are preserved outside the summary according to `contextmgr.DefaultConfig`.
- Explicit `context compact` always moves at least one message into the summary when messages exist.
- `context show --task <id>` prints the latest summary for that task.
- `session send` on a Run-bound Session auto-starts/resumes the Run, applies Supervisor policy/budgets/actions, and persists one user/assistant pair; unbound Sessions retain legacy behavior.
- Slash commands are persisted as normal session turns.
- Long session histories automatically compact older active messages into `context_summaries`.
- MiMo live smoke passed with env-only key and `mimo-v2.5-pro`; no key is stored by the application.
- DeepSeek live smoke passed with an env-only key and `deepseek-v4-flash` through both non-streaming provider health and RunSupervisor SSE paths; durable events contained model metadata/counters without the key.
- Tool proposal smoke passed: proposed shell command, dry-run approval completion, policy-denied risky command.
- TUI snapshot smoke passed with existing session history, selected proposed tool run, status line, and keyboard help rendered from SQLite.
- TUI picker smoke passed for empty state, existing session list, and direct session snapshot.
- TUI async submit unit test passed: Enter on `/run echo async` enters busy state, returns an async command, and refreshes the proposed tool run after `actionDoneMsg`.
- TUI workspace context unit tests passed: chat models render attached workspace metadata and picker-created sessions preserve workspace lookup.
- Workspace-scoped file tool tests passed: normal read/list works, absolute paths are rejected, `../` escape is rejected, and long reads are truncated.
- Session `/ls` and `/read` smoke passed with attached workspace; `../outside.txt` is denied and persisted as a safe assistant response.
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
- Isolated adapter CLI smoke passed across separate processes with one three-event timeline and stable exit codes `2` (invalid argument) and `3` (not found).
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

## Recommended Next Slice

Continue P5 after the first unified Tool Gateway slice:

- Route `script run --local` through a proposal-first Gateway path and keep real execution disabled by default.
- Add durable approval idempotency, Run/Session correlation, restart recovery, revocable Session grants, and tool-call budget accounting.
- Add low-risk structured WorkItem/Note mutation proposals with schema validation, scope checks, policy decisions, durable events, and explicit approval where required.
- Capture oversized or durable tool output as hashed, MIME-labelled Artifacts before enabling Local/Docker execution.
- Keep TypeScript, Rust, and model providers unable to bypass the Go Tool Gateway or policy boundary.
