# Project Status

Last updated: 2026-07-10

## Resume Context

CyberAgent Workbench is a local-first Go agent runtime for cyber-oriented work. The current implementation is a CLI-first scaffold with a mock model provider, persisted sessions, SQLite event/message store, workspace manager, safety policy, sandbox interfaces, and context compaction.

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
- `internal/contextmgr/context.go`
- `internal/session/session.go`
- `internal/toolrun/toolrun.go`
- `internal/tui/model.go`
- `internal/tui/picker.go`
- `internal/store/sqlite.go`

## Progress Review

- Overall product vision: about 42%.
- v0.1 generic agent MVP: about 97%.
- V2 run-centric runtime: about 34%.
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
- Secret redaction layer for common API keys, bearer tokens, GitHub tokens, AWS access keys, JWTs, assignment-style secrets, and private-key blocks.
- Redaction is applied at file reads, session message creation, SQLite session/context/tool-run storage, context prompt construction, and final LLM router dispatch.
- Automatic context compaction after long active session histories.
- Optional Anthropic-compatible `mimo` provider registered from environment variables only.
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
- Added `docs/TASK_BOOK.md` with phased migration tasks, acceptance criteria, compatibility rules, and CTF deferred to the final phase.
- Versioned SQLite migrations: checksummed `v1` legacy baseline and `v2` run-centric foundation, each applied in its own transaction.
- Migration tests cover idempotence, legacy data preservation, checksum history, and failed-migration rollback.
- Unified `internal/idgen` now backs agent tasks, sessions, tool runs, file edits, Mission/Run, and event IDs.
- Added pure Go Mission, Scope, Budget, RunConfig, Run status machine, and legal transition checks.
- Added transactional `missions`, `runs`, and append-only `run_events` persistence; event sequence is assigned only by the Store.
- Added `run create/list/show/events/start/pause/resume/cancel` CLI and end-to-end lifecycle tests.
- Run status updates and corresponding events commit atomically; Store independently rejects illegal or stale transitions.

Not done yet:

- OpenAI-compatible/Ollama providers.
- Dedicated TUI file-edit pane with key-driven edit approval/denial.
- Streaming token updates and provider cancellation.
- Script generate-run-fix loop with real model calls.
- CTF-specific solving workflows beyond placeholder commands.
- Go HTTP/WebSocket control-plane API, TypeScript Web UI, and Rust analyzer processes.
- Projection of Session, ToolRun, FileEdit, and policy activity into the unified Run event stream.
- RunSupervisor, Work Board, Notes, AgentCoordinator, Findings/Evidence, and resumable execution.

## Code Audit Notes

No high-severity issue was found in the latest slice.

Residual risks to address soon:

- `script run --local` can execute local commands after only lexical policy checks; it needs explicit approval and workspace scoping.
- Secret redaction is heuristic, not a full secrets manager; add opt-in raw local inspection later only with clear warnings.
- Binary or non-UTF-8 files are refused by `read_file`; richer file viewers should stay workspace-scoped and type-aware.
- File edit writes re-resolve and re-hash immediately before `os.WriteFile`, but portable Go cannot fully eliminate filesystem TOCTOU races without OS-specific no-follow/open-handle code. Keep workspace permissions as the primary local boundary.
- The symlink-escape unit test is skipped on this Windows account because creating symlinks requires an unavailable privilege; traversal, path resolution, and stale-file tests pass, and the runtime still resolves links before accepting a path.
- Docker runner intentionally returns a clear placeholder error and is not a real isolation boundary yet.
- Session `/run` now creates a persisted tool proposal; approval still dry-runs by design. Real execution must flow through stricter workspace scoping, sandbox, and event logging.
- Mimo API keys must remain env-only for tests; do not persist user keys until a real secrets backend exists.
- Future Rust and TypeScript modules must not bypass Go for LLM, secrets, policy, workspace permissions, Docker, shell, network scope, or persistence.
- `run start` currently advances lifecycle state only; it intentionally does not call a model or execute tools until RunSupervisor is implemented in P2.
- Applied migration statements are immutable once released because their checksums are verified. Schema changes must always add a new migration version.

## Feature Verification

Latest verified commands:

```powershell
go test ./...
go run ./cmd/cyberagent run create "review this workspace" --workspace demo --profile review --max-turns 15
go run ./cmd/cyberagent run start <run-id>
go run ./cmd/cyberagent run pause <run-id>
go run ./cmd/cyberagent run resume <run-id>
go run ./cmd/cyberagent run cancel <run-id>
go run ./cmd/cyberagent run show <run-id>
go run ./cmd/cyberagent run events <run-id>
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
go run ./cmd/cyberagent session send <session-id> "/model mimo/mimo-v2.5-pro"
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
- `session send` persists user and assistant messages.
- Slash commands are persisted as normal session turns.
- Long session histories automatically compact older active messages into `context_summaries`.
- MiMo live smoke passed with env-only key and `mimo-v2.5-pro`; no key is stored by the application.
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

## Recommended Next Slice

Complete P1 compatibility projection before P2 execution:

- Add stable typed error codes without changing current human-readable CLI errors.
- Attach or create a Session for a Run and record that association.
- Project Session messages, ToolRun proposals/decisions, FileEdit proposals/applications, and policy decisions into `run_events`.
- Add an idempotent compatibility adapter from legacy `agent.Task` to Mission/Run.
- Keep `run start` lifecycle-only and keep multi-agent concurrency disabled until RunSupervisor arrives in P2.
