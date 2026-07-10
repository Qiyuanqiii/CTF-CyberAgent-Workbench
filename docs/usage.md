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
cyberagent run pause <run-id>
cyberagent run resume <run-id>
cyberagent run cancel <run-id>
```

A Mission is the stable goal and authorization scope. A Run is one resumable execution attempt. Each new Run creates a dedicated active Session unless `--session <id>` selects an unattached active Session. Session creation or attachment, Run creation, and their initial events commit together in SQLite.

Session messages, assistant policy decisions, ToolRun changes, and FileEdit changes are projected into `run events` transactionally. Activity carrying a workspace different from the Run scope is rejected. `run start` advances the lifecycle from `created` through `preparing` to `running`; it does not invoke a model by itself.

`run step` asks the RunSupervisor to execute exactly one root Agent planning turn. It writes a `turn_started` checkpoint before the model call, rejects tool calls, and requires one strict `root_lifecycle.v1` JSON action. The Supervisor validates and interprets the action, then atomically stores the user-facing message, policy decision, model usage, lifecycle events, cumulative token/model-time counters, and next checkpoint. Raw protocol JSON is not written to Session history. `run checkpoint` displays the durable phase, protocol-repair phase/reason, next turn, token counters, and execution milliseconds. A process restart resumes an unfinished started turn; a committed turn is never appended twice. Turn or token budget exhaustion returns exit code 8, while the persisted execution-time boundary returns a deadline error.

Root actions use `continue`, `finish`, or `wait`. `continue` advances to another idle turn. `finish` requires a summary and atomically completes the Run. `wait` requires a reason, atomically pauses the Run, and resumes at the next turn after `run resume`. Unknown fields, trailing data, Markdown fences, invalid combinations, and responses over 64 KiB fail the current turn without writing user/assistant messages. Assistant prose by itself cannot mutate Run state.

`run execute` repeats that same durable step up to `--max-steps`; it stops immediately on root `finish` or `wait`. `--finish` remains an explicit operator fallback after a normal step limit and cannot complete a waiting Run. `run finish` and `run fail` atomically update the Run, Supervisor checkpoint, and event stream. Repeating the same terminal command or replaying the same committed lifecycle action is idempotent, while a conflicting terminal transition is rejected.

Before each model call, the Supervisor passes the remaining token allowance as the request limit and applies the remaining persisted model-execution deadline. Its Context Builder considers the latest compacted summary, at most 20 active WorkItems, and at most 100 active Notes visible to the root Agent. It selects those structured sections under a separate 8,192-token estimate, keeps Work Board JSON under 16 KiB, and truncates individual Note context fields. `model.started` persists only included/omitted source IDs and token estimates, never Note bodies. A model `finish` action is sent through the existing one-repair protocol when active work remains; `run finish` remains an explicit operator override. Provider-reported usage is authoritative: if one call exceeds the remaining token allowance, its full actual usage is committed and subsequent calls are blocked. `MaxToolCalls` is now enforced by the Tool Gateway with an atomic SQLite ledger; `MaxCostUSD` remains configuration-only until provider pricing metadata is available.

Provider failures are normalized as `retryable`, `rate_limited`, `invalid_response`, `cancelled`, or `permanent`. RunSupervisor retries only retryable transport/capacity outcomes, with three attempts per protocol phase by default, 100 ms exponential backoff, and a 2 second local wait ceiling. A server `Retry-After` longer than that ceiling is not shortened: the turn returns a rate-limit error and keeps its pending input for a later `run step`. Invalid lifecycle JSON is not transport-retried; instead, it receives exactly one explicit repair phase with its own transport counter. Authentication/configuration failures, policy denial, and tool calls are not repaired.

Every call attempt emits `model.started` and then `model.completed` or `model.failed` in `run events`. RunSupervisor always consumes the Provider stream interface. It reconstructs UTF-8 split across chunks, limits the complete response to 64 KiB, requires a final chunk with valid usage, and routes mid-stream failures through the same retry and protocol-repair path. Global attempt numbers continue across protocol phases and process restart; phase-local transport numbers reset for the one repair phase.

During an attempt, `run events` may contain at most 32 ordered `model.delta` records. These events contain only chunk and byte counters, sequence, and completion state; they never store model text. The terminal event must match those counters. Model terminal events, token usage, and `execution_millis` commit together, and replaying the same terminal event cannot double-charge the budget. Repair transitions emit `supervisor.protocol_repair_requested/started/completed/failed`. The raw invalid output is never copied into the repair prompt, Session, or event payload. `run step` prints `model_attempts`, `protocol_repairs`, `model_outcome`, `stream_events`, and `stream_bytes`. Exhausted transient retries return unavailable exit code 6, rate limits return resource-exhausted exit code 8, cancellation returns 7, and deadline expiration returns 9.

The application service exposes in-process active-call query, bounded metadata subscription, and idempotent cancellation operations. An explicit application cancellation first appends a redacted `model.cancel_requested` event and then signals the Go-owned Provider context. Subscribers receive no raw model text and are disconnected if their 32-event buffer fills. Bubble Tea consumes this interface through an adapter; the control plane is not yet reachable from a second CLI process, so cross-process cancellation still waits for the local Go HTTP/WebSocket service.

The `cyberagent` process handles `Ctrl+C` and termination signals through its command context. An interrupted Provider call records `model.failed` with a cancelled outcome and keeps the started Supervisor checkpoint recoverable instead of abandoning an unaccounted request.

Ordinary text sent to a Run-created Session uses the same RunSupervisor path as `run step`. The first message automatically starts a `created` Run, a follow-up to a model `wait` automatically resumes its paused Run, and the CLI prints `[run <id>: action=<action> status=<status>]`. Pending input is redacted, limited to 64 KiB, and stored before the Provider call; after restart the same attempt can recover it, while the committed user/assistant pair and lifecycle events are written exactly once. Completed, failed, cancelled, or approval-waiting Runs reject ordinary input instead of falling back to an unsupervised model call.

`run adapt-task` converts a v0.1 `agent.Task` into a new Mission, Run, and Session. The mapping is transactional and keyed by Task ID, so repeated or concurrent calls return the same Run and append only one `legacy.task_adapted` event. Historical task status is recorded for audit, but the new Run always starts at `created` and never executes implicitly. Legacy CTF tasks map to the safe generic `review` profile until the dedicated CTF phase.

CLI errors keep their existing text and use stable exit codes documented in [errors.md](errors.md).

Supported profiles are `code`, `review`, `learn`, and `script`. New runs start with network access disabled. Budget flags reject negative values and include maximum turns, tokens, model cost, and wall-clock timeout.

## Work Board

```powershell
cyberagent todo create <run-id> "inspect parser" --priority high --owner reviewer
cyberagent todo create <run-id> "write tests" --depends-on <work-id> --acceptance "tests pass"
cyberagent todo list <run-id>
cyberagent todo list <run-id> --status pending,blocked --owner reviewer
cyberagent todo show <work-id>
cyberagent todo update <work-id> --description "cover malformed input" --version 1
cyberagent todo update <work-id> --clear-dependencies
cyberagent todo start <work-id>
cyberagent todo block <work-id> --reason "waiting for fixture"
cyberagent todo reopen <work-id>
cyberagent todo complete <work-id>
cyberagent todo cancel <work-id>
```

WorkItems belong to exactly one Run. Dependencies must already exist in that same Run, cannot form a cycle, and must be completed before a dependent item starts or completes. Statuses are `pending`, `in_progress`, `blocked`, `completed`, and `cancelled`; priorities are `low`, `normal`, `high`, and `critical`. Blocked items require a reason, while completed and cancelled items are terminal.

Every item starts at version 1. Mutation commands accept optional `--version <n>` optimistic locking; omitting it uses the version read immediately before the transaction, while an explicit stale value returns conflict exit code 4. `--acceptance` and `--depends-on` may be repeated. `--clear-acceptance` and `--clear-dependencies` replace those lists with empty values. WorkItem records and `work_item.created/changed` Run events commit atomically, and terminal Runs reject later board mutation.

## Notes

```powershell
cyberagent note create <run-id> "parser decision" --content "Use strict JSON" --category decision --pin
cyberagent note create <run-id> "fixture evidence" --content-file C:\temp\note.txt --tag parser --source docs/spec.md --evidence evidence-1
cyberagent note create <run-id> "root summary" --content "Current root-only state" --visibility root
cyberagent note create <run-id> "specialist memory" --content "Private context" --visibility owner --owner specialist
cyberagent note list <run-id> --status active --category decision,summary --tag parser
cyberagent note list <run-id> --visibility root --pinned true
cyberagent note show <note-id>
cyberagent note update <note-id> --content "Revised decision" --version 1
cyberagent note update <note-id> --clear-tags --unpin
cyberagent note archive <note-id>
cyberagent note restore <note-id>
```

Categories are `observation`, `hypothesis`, `decision`, `summary`, and `reference`. Visibility is `run`, `root`, or `owner`; owner-visible Notes require an owner label. The root Supervisor receives run-visible, root-visible, and `owner=root` Notes, while Notes owned by another future Agent remain excluded. Operators can still inspect all Notes through the CLI.

Each Note has normalized tags, source references, Evidence IDs, pinned state, active/archived lifecycle, and an optimistic version. `--tag`, `--source`, and `--evidence` may be repeated; update commands replace those lists or clear them explicitly. Archived Notes remain durable but cannot be edited or selected until restored. Terminal Runs reject later Note mutation.

`--content-file` reads valid UTF-8 through a bounded reader and rejects content over 64 KiB even if the file changes while being read. Content, titles, tags, references, Evidence IDs, event payloads, and model context pass through the redaction boundary. Note records and `note.created/changed` events commit together. Models receive selected Notes as untrusted `note_context.v1` JSON and cannot create or modify Notes in this phase.

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
```

`script new` prints both the absolute artifact path and `script_relative`; pass the latter to `script run`. `script run` no longer executes a Sandbox or host process. It requires a workspace-relative existing file, rejects absolute paths/traversal/symlink escape, creates a Script Profile Mission/Run/Session, and persists a policy-checked Tool Gateway proposal. The structured `script_process.v1` payload contains executable, argv, workspace-relative working directory, requested backend, and the fixed execution mode `disabled`. `--local` records intent for future Sandbox work; it does not execute locally. Use the printed Tool ID with `tool show` or `tool approve`; approval currently completes as dry-run only.

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

`provider test` accepts either a route name, such as `learn`, or a direct `provider/model` reference. The optional `mimo` provider is registered from `MIMO_API_KEY`, `MIMO_BASE_URL`, and `MIMO_MODEL`. The optional `deepseek` provider uses `DEEPSEEK_API_KEY`, `DEEPSEEK_BASE_URL`, and `DEEPSEEK_MODEL`; its defaults are `https://api.deepseek.com/anthropic` and `deepseek-v4-flash`. Only the API key is required.

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
/deny <tool-run-id> not needed
```

Keyboard controls:

```text
Tab              switch focus between input and tool runs
Enter            send from input or approve selected proposed tool
PgUp / PgDn      scroll messages
j / k            select next/previous tool when tool runs are focused
a                approve selected proposed tool when tool runs are focused
d                deny selected proposed tool when tool runs are focused
Ctrl+R           refresh session/tool state
Ctrl+X           request audited cancellation of the current model call
Esc              quit when idle; a busy action must finish or be cancelled first
```

`--print` renders one snapshot and exits, which is useful for non-interactive verification.

Message sends, live-call discovery, refreshes, cancellation, and tool approval/deny actions run asynchronously. During a Run-bound model call, the status line shows provider/model, attempt, chunk/byte progress, cancellation, slow-consumer disconnect, and terminal state. `Ctrl+X` prefers the application audit-first cancellation API; if a legacy or not-yet-active request has no registry entry, it cancels the current application request context instead. Additional input is held until the current action finishes, and raw model text is never included in the live envelope.

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

The Gateway validates exact argument schemas, binds calls to a Run/Session/Workspace scope, atomically charges the Run tool-call budget, runs policy checks, selects an approval mode, and normalizes execution results. `read_file` and `list_workspace` use automatic approval only when scope and policy allow them. `replace_file` and `shell` normally use per-call approval. A policy denial is terminal and cannot be converted into approval by a grant or later review. Schema v11 persists each per-call decision with a request fingerprint, Run/Session association, and immutable idempotency operation before the compatibility proposal advances. Schema v12 persists revocable Session grants scoped to one Run, Session, Workspace, Tool, and ActionClass; terminal Runs and archived Sessions cannot create or consume them.

New CLI Runs default to 100 tool calls and may set `--max-tool-calls`; zero means unlimited for compatibility. Every valid Run-bound Gateway invocation consumes one call, including Policy-denied attempts. The first attempted call beyond the limit records one `tool.budget_exhausted` event, subsequent attempts return `RESOURCE_EXHAUSTED` without duplicating that event, and `run usage <run-id>` shows consumed, limit, remaining, and exhaustion time.

Text output is valid UTF-8, secret-redacted, MIME-labelled, and bounded to 128 KiB stdout, 32 KiB stderr, and 64 KiB proposal previews. Truncation is explicit. The Gateway does not currently capture oversized output as an Artifact. No production CLI path invokes LocalSandbox; script-process proposals remain deliberately non-executable.

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
cyberagent tool show <tool-run-id>
cyberagent tool approve <tool-run-id>
cyberagent tool deny <tool-run-id> --reason "not needed"
```

`/run` creates a `tool_runs` proposal through the unified Tool Gateway. `tool approve` and `tool deny` use the same review service as file edits. A matching active Shell Session grant can authorize the proposal automatically, but completion remains dry-run. Real command execution stays disabled until Sandbox isolation and Artifact capture are complete.

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
