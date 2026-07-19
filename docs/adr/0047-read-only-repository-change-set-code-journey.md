# ADR 0047: Read-Only Repository State, Independent Change Sets, And Code Journey

- Status: Accepted
- Date: 2026-07-19
- Scope: non-schema D1-G1, D1-I3, and D1-F1

## Context

The workbench already had bounded Workspace reads, recoverable FileEdit proposals,
independent review/apply controls, and the pieces of a Code-mode Run. It still lacked
three ordinary coding-product surfaces: a trustworthy local repository summary, one
place to understand a multi-file proposal without merging file authority, and a clear
journey through the existing Run controls. These surfaces must not turn Git, React, or
one convenience endpoint into a second control plane.

## Decision

### D1-G1: repository state is a bounded read-only projection

`repository_state.v1` inspects only the exact registered Workspace root with
`go-git/v5`. Parent discovery, worktrees represented by a redirected `.git` file,
symbolic links anywhere below `.git`, remotes, hooks, subprocesses, and network access
are rejected or unused. The metadata walk is cancellable and capped at 50,000 entries;
status processing is capped at 10,000 entries and returns at most 200 canonical
relative paths. Secret-looking paths and branch names are omitted, and host roots,
file bodies, remote configuration, and command output never enter the DTO.

The response states its boundary explicitly: read-only is true, while root exposure,
content, remote configuration, process start, network use, and hook execution are
false. React strictly checks those facts before rendering the Repository tab.

### D1-I3: a change set summarizes independent files

`file_edit_change_set.v1` exact-binds one Run, its Session, Mission Workspace, and at
most 100 existing FileEdit previews. It returns per-file identity, canonical path,
status, diff byte count, redaction state, allowed actions, apply readiness, and mixed
status counts. Diff content remains on the existing independently bounded FileEdit
detail path.

The projection fixes `review_independent=true`, `apply_independent=true`,
`atomic_apply=false`, `batch_mutation_supported=false`, and
`partial_apply_visible=true`. There is no Apply All endpoint. Every review and apply
continues through its own operation key, current Policy/hash check, and durable
receipt, so grouping cannot silently claim atomic success.

### D1-F1: Code Journey composes existing capabilities

The Code-only Journey tab presents Scope, Plan, Queue and execute, Review, and Verify
and report stages. Each action navigates to an existing Repository, Overview, Actions,
Diffs, or Findings surface. The component has no API client and creates no mutation;
all actual operations remain separate Go endpoints with their existing capabilities,
budgets, Policy, idempotency, and event records. Cyber mode does not inherit this Code
workflow automatically.

## Consequences

- No SQLite migration is required; schema remains v77.
- Go remains the only control plane, while TypeScript renders strict projections and
  navigation.
- Repository status does not provide commit, branch, checkout, fetch, push, remote,
  hook, Shell, LocalRunner, or Docker authority.
- Multi-file review improves visibility without introducing batch approval or batch
  write authority.
- The next safe product work can add a bounded redacted repository Diff, immutable
  verification evidence, and a resumable handoff summary. Real process execution and
  CTF automation remain separately gated.

## Verification

The final implementation commit is `d69a812`. The ordinary full Go suite passed in
321.7s before the audit hardening; the final-code full race suite then passed in
490.4s, and the repository package passed ten normal repetitions after nested Git
metadata-link rejection was added. `go vet`, zero-warning staticcheck, module
verification, secure Desktop tags, deterministic OpenAPI/TypeScript generation, 114
React tests across 31 files, strict TypeScript, Vite, a zero-vulnerability npm audit,
isolated mock-only CLI smoke, and a reproducible Windows double build are green.
OpenAPI has 59 paths, 63 operations, and 129 schemas. The unsigned Desktop SHA-256 is
`145757cb1a8bbafc9080fdc29f4ada69d34b850ca64f702310ea44578ca677a9`; automated
checks pass while `release_ready=false` remains correct.

Govulncheck reports zero reachable and zero imported-package vulnerabilities. The
dependency graph retains module-level `GO-2026-5932` because
`golang.org/x/crypto/openpgp` is unmaintained and has no fixed version, but the
application does not import or call that package. The combined audit also fixed a
nested `.git` symlink read boundary and allowed a terminal Run to expose a proposed
FileEdit with zero actions without letting the TypeScript parser invent authority.
Desktop and 390x844 browser checks found no page overflow or console error. No real
key, Provider request, Shell, LocalRunner, Docker, hook, attack traffic, or external
network operation was used.

GitHub Actions run `29678257802` passed `d69a812`: TypeScript console 43s, Go control
plane 5m32s, and Windows Desktop shell 5m29s.

## 中文结论

本轮把仓库状态、多文件审阅和 Code 工作流接到既有 Go 控制面，但没有增加 Git
写入或批量授权。仓库页只读取已注册 Workspace 根目录的有界本地元数据，拒绝父仓库、
重定向 `.git` 和内部符号链接，不启动进程、网络或 hook。多文件 change set 只是汇总，
每个文件仍单独 review/apply，部分成功不会伪装成原子成功。Code Journey 只导航既有
能力，不是新的复合执行接口。SQLite 继续保持 v77。
