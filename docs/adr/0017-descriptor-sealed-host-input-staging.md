# ADR 0017: Descriptor-Pinned, Kernel-Sealed Host-Input Staging

## Status

Accepted for schema v57.

## Context

Schema v56 can durably create, inspect, recover, and remove one stopped Docker container, but
its bind-mount source checks occur before a future container could consume the files. A path may
be renamed, replaced, deleted, or redirected after validation. Treating the v56 mount control as
proof of immutable input would therefore overstate the evidence and make a future start boundary
unsafe.

Docker Engine API archive upload is not adopted in this slice. The current target has a read-only
root filesystem and read-only input mounts, and the platform must not weaken those controls merely
to make archive upload succeed. Adding daemon volume management, image building/import, or a helper
container would also broaden the closed v55 API surface and requires its own review.

## Decision

1. Add a separate default-disabled `DockerHostInputStager`. It runs only after the v56 stopped-
   container stage checkpoint and before cleanup, and only after a second explicit operator
   confirmation.
2. On Linux, open the absolute workspace root with `openat2` using no-symlink and no-magic-link
   resolution. Resolve each normalized read-only mount beneath that root with `RESOLVE_BENEATH`,
   `RESOLVE_NO_SYMLINKS`, `RESOLVE_NO_MAGICLINKS`, and `RESOLVE_NO_XDEV`.
3. Preflight each entry with `O_PATH`, then reopen only regular files and directories and verify
   that the two descriptors identify the same inode. This rejects FIFOs and other special files
   before a potentially blocking content open. Recursively pin both directory and single-file
   mounts. Directory enumeration is batched and bounded before allocation, and file reads observe
   context cancellation between chunks. Reject symbolic links, magic links, hard links,
   cross-mount traversal, excessive depth, more than 4096 entries, more than 16 MiB of source data,
   or more than a 40 MiB bundle.
4. After every descriptor is pinned, recheck device, inode, mode, link count, size, mtime, and
   ctime. Any overwrite, rename, replacement, or deletion fails as `source_changed`.
5. Revalidate every input Artifact by exact Run, Session, Workspace, digest, size, MIME, stream,
   source, redaction state, and order. Build a deterministic tar with sanitized names, modes,
   ownership, and timestamps.
6. Write the tar only to a sealable `memfd`, apply `F_SEAL_WRITE`, `F_SEAL_GROW`,
   `F_SEAL_SHRINK`, and `F_SEAL_SEAL`, then reread it and verify the digest.
7. Persist an immutable v57 intent before bundle creation. Bind it to the v56 attempt, stopped-
   container fingerprint, v54 plan, mount/input/authority/spec fingerprints, requester, and lease
   generation. Persist only counts, sizes, digests, seal claims, and non-authority flags.
8. SQL blocks v56 completion while a v57 intent has no result. On failure, Application uses an
   independent bounded context to clean the stopped container before appending failure and
   releasing the lease. A later generation may complete a pending intent without another create.
   Semantic fingerprints exclude generated row IDs, so independently generated concurrent
   retries converge. A missing resume confirmation is rejected before acquiring a lease.
9. The bundle is not handed to Docker in v57. Durable facts fix `daemon_consumed=false`,
   `execution_evidence=false`, and all production/backend/execution/Artifact authority to false.

## Consequences

- Replacement, rename, deletion, symlink, hard-link, FIFO, and nested-mount attacks during capture
  fail closed. Directory metadata that varies by filesystem is not part of the content digest.
- Source paths, raw content, descriptors, raw container IDs, and private lease identities never
  enter SQLite events or CLI output.
- Windows fails before container creation with `staging_unsupported`.
- v57 proves that Go captured and kernel-sealed one exact input snapshot. It does not prove that a
  Docker container consumed that snapshot.
- A later schema must design a daemon-owned immutable handoff without weakening read-only inputs or
  broadening Docker control implicitly. Start, exec, logs, export, network, secrets, and production
  Artifact commit remain unavailable.

## 中文说明

schema v56 已能以可恢复方式创建、核验和删除一个从未启动的容器，但路径校验与未来容器
实际读取之间仍存在替换窗口。v57 因此增加独立且默认关闭的宿主输入捕获门禁：Linux 用
`openat2` 固定工作区根和只读目录树，并用 `RESOLVE_NO_XDEV` 拒绝跨挂载点解析。每个条目先
以 `O_PATH` 预检，只有普通文件或目录才会重新打开并核对 inode，因此 FIFO 与其他特殊文件会
在潜在阻塞读取前失败。目录与单文件 mount 均可形成证据；符号链接、magic link、硬链接、
特殊文件、越界与资源超限全部拒绝。全部 descriptor 固定后再次核验 inode、大小、时间戳和
链接数，再把规范化文件与精确复核的 Artifact 写入确定性 tar。tar 只存在于 `memfd`，施加
不可写、不可增长、不可缩小和不可再加 seal 的内核密封，并复读校验摘要。

v57 intent 在封装前绑定当前 v56 attempt、停止容器指纹、计划、输入摘要、操作者和 lease
generation；SQLite 只保存计数、大小、摘要与安全布尔值，并禁止 pending intent 绕过最终
completion。失败时先清理停止容器，新 generation 可恢复且不重复 create。密封包尚未交给
Docker，因此 `daemon_consumed=false`、`execution_evidence=false`，不能据此开放 start。
daemon 侧不可变交接必须作为后续独立 schema 审计，不能通过放宽只读根或输入来绕过。
