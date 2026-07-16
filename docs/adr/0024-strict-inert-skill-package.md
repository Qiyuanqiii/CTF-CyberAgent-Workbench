# ADR 0024: Strict Inert Skill Package Validation

## Status

Accepted as a non-schema, validation-only boundary for `skill_package.v1`.

## 中文摘要

`skill_package.v1` 第一阶段只提供纯内存、只读、失败关闭的技能包校验。包必须是一个确定性 ZIP，且根目录按顺序只包含 `manifest.json` 与 `SKILL.md`；校验过程不写磁盘、不访问网络、不调用模型、工具或数据库，也不会安装或授权 Skill。包正文始终是 `operator_installed_untrusted` 指导材料，`tool_dependencies` 只是声明而不是能力凭证。

## Context

The built-in `skill.v1` loader operates on an application-controlled `fs.FS`.
It is suitable for embedded guides and test fixtures, but it is not a user
import boundary. Accepting arbitrary ZIP files through that loader would leave
archive ambiguity, path traversal, decompression amplification, metadata,
installation side effects, and trust classification unspecified.

The first external-package surface must therefore establish container and
semantic identity before any persistent Registry or installation workflow is
introduced. Validation must be useful on its own and must not accidentally
become an execution or capability-grant path.

## Decision

`skill_package.v1` is exactly one ZIP archive with these root entries in this
order:

1. `manifest.json`
2. `SKILL.md`

Both entries use Deflate, zero timestamps, no extra fields, comments, file
attributes, prefix, inter-entry gaps, or trailing data, and the fixed ZIP 2.0
data-descriptor profile. ZIP64, multi-disk archives, links, special files,
directories, alternate names, duplicate/case-colliding entries, and additional
payloads are rejected structurally before content is trusted.

The archive is bounded to 64 KiB. Entry count, compressed and uncompressed
sizes, aggregate size, compression ratio, CRC, local/central header agreement,
data descriptors, exact Deflate-stream exhaustion, UTF-8, strict JSON, manifest
fields, `SKILL.md` content path, declared byte/token bounds, and SHA-256 are all
checked. Bounded decompression prevents the ZIP headers from expanding content
past the declared limits, while exact stream exhaustion rejects hidden bytes
between a valid Deflate terminator and its descriptor.

Validation produces two separate identities:

- `archive_sha256` identifies the exact container bytes.
- `package_fingerprint` identifies the canonical manifest and Skill content
  with protocol/version and length framing, independent of insignificant JSON
  whitespace in a correctly rebuilt archive.

The public preview is metadata-only. It reports the fixed
`operator_installed_untrusted` trust class, bounded risk codes, and explicit
false values for executable assets, hooks, command execution, network access,
Provider calls, tool grants, and installation authorization.

## Security Boundary

`ParsePackage` accepts only an in-memory byte slice. It performs no filesystem,
database, network, Provider, Tool Gateway, Sandbox, or command operation. The
CLI command `cyberagent skill package validate <package.zip>` adds a bounded
regular-file read with symlink rejection and before/after identity checks, then
prints only the metadata preview. It does not persist the source path or Skill
body.

Successful validation means only that the package is structurally and
semantically well formed. It does not install the package, select it for a Run,
raise its trust, authorize declared tools, or execute any text found in it.
Prompt-like instructions in `SKILL.md` remain untrusted content subordinate to
Go-owned Policy, Scope, approval, budget, provenance, and tool controls.

## Recovery And Validation

Unit tests cover deterministic valid packages, canonical semantic identity,
archive-byte identity, truncation, prefix/trailing bytes, comments, wrong order,
extra/case-colliding entries, alternate compression, timestamps, metadata,
symlinks, local/central mismatch, corrupt descriptors, unknown/duplicate JSON
fields, path/hash/UTF-8 drift, compression amplification, and inert command-like
content. It also covers an internally length-consistent archive that hides data
after a valid Deflate stream. Fuzzing requires every accepted input to preserve
all false authority bits and stable fingerprints.

The CLI tests additionally cover valid metadata-only output, corruption,
directories, missing files, unavailable install syntax, no command execution,
and no database creation.

## Consequences

- Users can inspect a narrowly specified package without granting it authority.
- Producers must emit the exact deterministic profile; a package builder is a
  separate future convenience surface.
- No migration is added because validation stores no durable state.
- A later schema may add a content-addressed user Registry and immutable
  import/removal ledger, but it must reuse this parser and undergo an
  independent persistence, recovery, and selection audit.
- Import hooks and automatic scripts remain excluded. Future script assets may
  only become Artifacts and require an ordinary Go Tool Gateway, Policy, Scope,
  approval, and Sandbox decision at execution time.
