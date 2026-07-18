# ADR 0043: Workspace Search, Non-Authorizing Evidence, And Receipt History

- Status: Accepted
- Date: 2026-07-19
- Scope: schema v77 / D1-E2, D1-C1, and D1-U2

## Context

The first Workspace Explorer made registered project text visible as bounded,
redacted evidence, but an operator still had to navigate one path at a time. A file
also could not be deliberately added to an existing Run-bound Session without being
copied into an ordinary operator message, which would lose the distinction between
project facts and instructions written for an automated assistant. Operation receipts
were durable but visible only in the immediate mutation response.

These gaps must close without creating an indexer, granting document text instruction
authority, exposing host paths or operation secrets, or starting a model, tool,
process, network request, or background worker.

## Decision

### D1-E2: bounded search over evidence projections

`workspace_search.v1` performs one deterministic breadth-first scan owned by Go. It
accepts one normalized query of at most 128 Unicode code points and follows no links
or redirected components. A request scans at most 128 directories, 1,000 entries,
64 regular files, 50 results, and the Explorer read budget plus its bounded UTF-8
look-ahead. It searches only the already redacted `workspace_explorer.v1` projection,
never raw file bytes, and returns canonical relative references, bounded plain-text
snippets, and `context_provenance.v1` with `instruction_authorized=false`.

There is no persistent index, watcher, daemon, host-root DTO, Markdown rendering, or
renderer filesystem access. Unicode matching is line-based so offsets from case-folded
text are never used to slice the original UTF-8 bytes.

### D1-C1: explicit attachment does not authorize the document

`session_evidence_attachment.v1` is a separate default-off control capability. The
operator submits only an exact Run, canonical Workspace reference, projected SHA-256,
protocol version, and an in-memory idempotency key. Go reloads the Run, Mission,
active Session, and registered Workspace, reprojects the selected file, and rejects a
changed digest or binding.

One SQLite transaction appends a tool-role evidence message, a metadata event, and an
immutable attachment record. Both Go validation and the schema-v77 trigger require
`context_provenance.v1`, the exact source/hash, and
`instruction_authorized=false`. Context projection presents the message to a model as
untrusted user evidence. Text such as "notes for automated assistants" therefore
cannot become an operator instruction, approve a tool, widen Scope, or grant a
capability. Replaying the same operation returns the original historical snapshot;
using a new operation after the file changed fails the stale hash check.

### D1-U2: refreshable metadata-only receipt history

`operation_receipt_history.v1` derives a bounded newest-first view from terminal
FileEdit apply, foreground wake consumption, and inert Skill installation facts. It
returns at most 100 entries and can be restricted to one exact Run. Public item IDs
are domain-separated opaque hashes. Operation keys and digests, file paths and
content hashes, requester/owner identity, archive metadata, and private leases remain
inside Go/Store.

FileEdit cleanup state is refreshed through read-only staging inspection. Any lookup,
path, age, type, size, identity, or digest uncertainty is reported conservatively as
`pending_review`; listing history never removes or modifies a file. React refreshes
only on an explicit operator action and validates the closed receipt kind/outcome
combinations instead of inferring recovery state.

## 中文结论

本批把 Workspace 文件名/正文搜索限制在一次性、有硬上限、已脱敏的 Go 投影中；把操作者
选中的文件以不可变、非授权证据挂到既有 Session；并让三类终态操作回执可刷新恢复。
README、日志或源码里任何“写给 AI 的话”都只是证据，不能变成用户指令、审批或工具权限。
React 只提交相对引用与摘要，不取得宿主路径、索引器、后台 worker 或执行能力。

## Verification

The ordinary three-slice gate passed the uncached Go suite in 297.9s, `go vet`, module
verification/tidy, Windows Desktop build-tag tests, 92 React tests across 23 files,
strict TypeScript, Vite production build, zero-vulnerability npm audit, deterministic
OpenAPI and generated TypeScript, isolated mock-only CLI smoke, and a reproducible
Windows double build. OpenAPI now
contains 50 paths, 53 operations, and 112 schemas. The unsigned executable SHA-256 is
`d187601e9e9d8cb0d4ee644e3c9aa1c7617905580b001ef7955dbc35b8c47af3`;
automated compatibility passed while release readiness correctly remains false.

The focused audit fixed Unicode case-mapping offsets and accounted for the Explorer's
UTF-8 look-ahead in the real search read ceiling. It also duplicated canonical source
reference validation at the schema boundary. No unresolved high- or medium-severity
issue is known. No real Provider, LocalRunner, Shell, Docker, network request, API key,
installer, registry mutation, startup task, or updater was used.

GitHub Actions run `29661764283` passed implementation commit `ffbdc72`: the
TypeScript console completed in 34s, the Windows Desktop shell in 2m21s, and the Go
control plane including govulncheck in 3m48s.

## Consequences

- SQLite advances to v77 solely for immutable Session evidence attachments.
- Search and receipt history are read-bearer projections; evidence attachment has its
  own default-off control flag and cannot enable any other mutation.
- Search results and attached text remain evidence, not instructions or capability.
- There is still no background indexer/wake worker, general process execution,
  Provider-secret UI, Rust analyzer, or CTF automation.
- This is the first three slices of the next six-slice cadence. The full
  race/staticcheck/govulncheck/dependency/privacy gate runs after the next three slices.
