# ADR 0042: Durable Receipts, Bounded Workspace Evidence, And Portable Builds

- Status: Accepted
- Date: 2026-07-19
- Scope: non-schema D1-U1, D1-E1, and D1-W1

## Context

Schema v75-v76 and D1-B1 made foreground wake consumption, approved FileEdit apply,
and inert Skill installation durable, but their operator surfaces described recovery in
different ways. A forced stop before FileEdit atomic replacement could also retain one
internal staging file. The Desktop Run view still lacked a narrow file browser, and the
Windows build produced an unsigned executable without one reproducible release
diagnostic contract.

These gaps must be closed without moving durable truth, path resolution, release
approval, or filesystem authority into React.

## Decision

### D1-U1: one closed receipt model

`operation_receipt.v1` projects only a settled operation kind, outcome, durable/replay
flags, one closed retry strategy, one closed recovery action, and cleanup state. It is
used by FileEdit apply, foreground wake consumption, and inert Skill installation. It
contains no operation key or digest, file path/body, model content, archive bytes,
requester, owner, or lease identity. Go constructs the receipt and TypeScript validates
it against the enclosing result instead of deriving operation state.

After a durable FileEdit result, Go may inspect the exact target directory for its
reserved staging prefix. It removes only an ordinary file older than fifteen minutes
whose complete bytes match the approved proposal SHA-256 and whose identity remains
stable through inspection. Fresh, non-regular, oversized, unreadable, changed, or otherwise
uncertain candidates are retained. Cleanup uncertainty never changes a successful
durable apply result; the receipt reports `pending_review` and permits only retry with
the same operation key after the grace period.

### D1-E1: Workspace content is evidence

`workspace_explorer.v1` is a read-bearer GET under an already registered Workspace.
Go accepts only canonical slash-separated relative paths. It rejects traversal,
absolute or volume paths, links, redirected components, controls, surrounding
whitespace, aliases that require normalization, and cross-platform ambiguous names.
One directory request scans at most 400 entries and returns at most 200. One file
request reads at most 64 KiB of valid UTF-8 and caps the redacted projection at 128 KiB.
Internal staging and suspicious names are omitted; the host root never enters the DTO.

Every response carries `context_provenance.v1`, a hash of the projected evidence, and
`instruction_authorized=false`. React navigates only an exact child path returned by
Go, cross-checks the parent/name relation, and renders text in `<pre>` without Markdown
or HTML execution. The endpoint is not a model-context injection or file-write route.

### D1-W1: automated diagnostics are not release approval

Release builds set version, revision, source-date epoch, worktree state, and CGO mode as
reproducible linker inputs. `cyberagent doctor portable` reports only bounded build
metadata and a deterministic fingerprint; it neither opens SQLite nor reads Provider
credentials. The Windows build script requires an in-repository output path without a
child reparse point and can compile twice to compare SHA-256. The compatibility script
checks PE signatures and architecture, executable flags, zero COFF timestamp, metadata
binding, `-trimpath`, Go module identity, and the absence of installer, registry,
startup-task, and auto-update authority.

Automated checks and release readiness are separate. The Windows 10/WebView2/display/
launch/recovery matrix remains manual, so `release_ready=false` until that matrix is
signed. The generated executable remains an unsigned portable-test artifact.

## 中文结论

本批把三类已落库 mutation 统一成 Go 生成的无正文恢复回执；把 Workspace 文件明确
作为“证据而非指令”，路径解析、脱敏和上限全部留在 Go；把 Windows 双构建、PE、
哈希和非安装边界变成可重复诊断。React 只呈现结果，不获得文件系统或发布权限。
自动检查通过也不代表正式可发布，Windows 10/WebView2 人工矩阵完成前仍为未就绪。

## Verification

The cumulative six-slice gate passed the final full ordinary and race suites in 294.0s
and 338.3s. Ordinary and secure-Desktop tests/vet, zero-warning staticcheck,
zero-finding govulncheck, module verification/tidy, deterministic OpenAPI generation,
88 React tests across 22 files, strict TypeScript, Vite build, zero-vulnerability npm
audit, isolated CLI smoke, and privacy/artifact scans are green. A real Windows double
build produced identical binaries with SHA-256
`33fb9ca3064df98191ac50b2a3ef9431e1b5c81abe8c610d4be15db113cdf1ef`;
all automated compatibility checks passed and release readiness remained false for the
manual matrix.

Audit fixes covered failed receipts shown as success, non-canonical and cross-directory
Explorer path authority, the OpenAPI live-route fixture, projected-output bounds,
release-readiness overstatement, Windows PowerShell 5.1 compatibility, output reparse
points, and one pre-existing Go error-text convention. No unresolved high- or
medium-severity issue is known.

## Consequences

- SQLite remains at v76; all three protocols are projections or build diagnostics.
- OpenAPI contains 47 paths, 106 schemas, 31 GET operations, and 19 control POSTs.
- Workspace exploration adds no renderer path picker, write route, model instruction
  authority, Shell, LocalRunner, Docker, network, or Provider capability.
- The portable build is reproducible on the tested machine but is not signed, packaged,
  installed, or approved for release until the manual compatibility matrix is complete.
