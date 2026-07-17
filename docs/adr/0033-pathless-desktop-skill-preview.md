# ADR 0033: Pathless Desktop Skill Package Preview

## Status

Accepted as non-schema Desktop D1-A groundwork. The Go bridge and its protocol
remain the path/privacy authority. ADR 0034 has since connected them to a Wails
v2.13.0 native dialog and read-first renderer; HTTP upload, installation
mutation, and installer remain absent. SQLite remains at schema v71.

## 中文摘要

Desktop D1-A 复用现有严格 `skill_package.v1` 解析器，但不把本地文件路径
交给 TypeScript 或浏览器。D0-A 原生文件选择器把路径只交给一个 Go 闭包；Go
立即完成普通文件身份、64 KiB 上限、前后身份稳定性、确定性 ZIP 与 Manifest
校验，然后只把经过投影的风险元数据放入内存。

渲染层只能得到一个 256-bit URL-safe 不透明句柄。句柄五分钟过期、最多同时
保留 16 个、只允许消费一次；它不包含路径，也不引用磁盘文件。预览不返回
Manifest description、content path、正文或 content digest，不安装、不持久化、
不执行、不联网、不调用模型/工具，也不授予任何能力。

## Context

The existing CLI already validates a package through a bounded ordinary-file
read and the pure in-memory parser. Reimplementing that parser in TypeScript
would create a second security decision point. Exposing an HTTP endpoint that
accepts an arbitrary local path would be worse: any renderer or allowed browser
origin holding a token could turn Go into a local-file oracle.

At the time of this decision the project did not yet have a desktop shell. This
slice established the native boundary before Wails integration could
accidentally expose paths. ADR 0034 now supplies the visible shell without
changing this protocol.

## Decision

`skills.ReadPackageFile` is now the shared CLI/Desktop file boundary. It:

- rejects empty or whitespace-rewritten paths;
- accepts only a non-symlink ordinary file between 1 and 64 KiB;
- compares the pre-open, opened, and post-read file identities, sizes, and
  modification times;
- bounds the read before calling the existing deterministic ZIP parser;
- honors cancellation at every safe local boundary; and
- returns stable errors that never include the supplied source path.

`skills.ValidatePackageFile` returns only the immutable parser preview. CLI
validation and import now use the same reader, preserving their existing
outputs and exit-code behavior.

`desktop.NewSkillPackagePreviewBoundary` returns two separate values:

1. `NativeSkillPackageSelector`, a Go function retained by the future native
   shell. It accepts the path selected by the operating-system dialog, validates
   it immediately, projects the result, and then forgets the path and body.
2. `SkillPackagePreviewBridge`, the only object intended for renderer binding.
   Its sole method accepts a validated opaque handle, never a path or bytes.

The broker stores at most sixteen metadata projections in process memory. Each
handle contains 32 cryptographically random bytes encoded with unpadded URL-safe
base64, expires after five minutes, and is atomically consumed once. Expired
entries are removed before issue and lookup. Cancellation before lookup does
not consume a handle. A renderer restart therefore cannot turn a stale handle
into a durable file capability.

The renderer-safe `desktop_skill_package_preview.v1` includes only constrained
name/version/Profile values, validated declared-tool names and count, bounded
byte/token counts, archive and semantic package digests, fixed trust/risk codes,
and explicit false authority fields. It excludes:

- source path and file name;
- package body and Manifest description;
- Manifest content path and content digest;
- API keys, operator identity, operation key, or database identity; and
- any install, command, hook, network, Provider, tool, or capability grant.

## Capability Boundary

At D1-A acceptance the new package was not referenced by CLI startup, HTTP,
OpenAPI, React, or any production command. ADR 0034 later bound only the
selector/preview methods inside the read-first Desktop shell. The preview
boundary itself still has no Store, model, Tool Gateway, Sandbox, Docker,
process, or network call and creates neither an HTTP handler nor a database
connection.

The Wails adapter implemented by ADR 0034 binds only a no-argument method that
opens the native dialog and invokes the Go selector. It does not bind a method
accepting a renderer-provided path, expose the selector closure, or add a
JSON/HTTP path parameter. Reflection tests lock that allowlist.

## Non-Goals

The D1-A decision itself did not add:

- a Wails dependency or `cyberagent-desktop` executable;
- a browser `<input type=file>`, multipart endpoint, or arbitrary path API;
- package installation, removal, selection, or persistence;
- scripts, install hooks, executable assets, URL/Git/Marketplace download;
- model, tool, Shell, host-process, Docker, or network execution; or
- Windows installer, registry, auto-start, protocol, or update behavior.

## Validation

Focused tests cover valid CLI/Desktop reuse, path-free errors, blank,
whitespace-rewritten, missing, directory, symlink, empty, oversized, malformed,
and pre-cancelled inputs. Desktop tests prove JSON field allowlists, omission of
the path/body/description/content metadata, snapshot stability after the source
file changes, expiry and bounded capacity, random-source failure, cancellation
without consumption, exact one-time replay, and 32-way concurrent consumption
with exactly one success. Repeated ordinary and race tests, vet, and staticcheck
pass for the affected packages.

## Follow-Up

ADR 0034 completed the Wails v2.13.0 read-first shell, embedded React console,
and native dialog adapter without adding an installer. Any package installation
or HTTP upload remains a separate mutation slice requiring an independent
control token, strict Host/Origin and content type, streaming bounds,
idempotency, CSRF resistance, cancellation, and audit.
