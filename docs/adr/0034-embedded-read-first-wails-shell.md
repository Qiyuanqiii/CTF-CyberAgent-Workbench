# ADR 0034: Embedded Read-First Wails Desktop Shell

## Status

Accepted as non-schema Desktop D0-A. CyberAgent Workbench now builds a Windows
development/portable-test desktop executable with Wails v2.13.0, an embedded
React production bundle, an in-process Go HTTP adapter, and the ADR 0033 native
Skill package picker. SQLite remains at schema v71. This is not an installer or
a production distribution channel.

## 中文摘要

Desktop D0-A 固定使用已审计的 Wails v2.13.0 稳定版；v3 仍处于 Alpha，暂不进入
主线。`cyberagent-desktop` 把 Vite production bundle 编译进可执行文件，并把现有
Go `api.v1` Handler 直接接入 Wails AssetServer，不监听 TCP 端口，也不建立第二套
业务 API。

桌面端默认只生成内存 read token。只有操作者显式使用
`--enable-profile-control` 时才生成独立 control token，而且它仍只能调用 schema
v64 的非授权执行档位控制。React 无权启动进程、Shell、Docker、安装 Skill、提交
本地路径或放宽 Policy。原生文件对话框只接受 `.zip`，路径只在 Go 内部短暂存在；
渲染层仍只获得五分钟、单次消费的不透明句柄和有界风险元数据。

## Context

The existing browser console already consumes a same-origin Go HTTP/OpenAPI
contract, while ADR 0033 established a pathless native preview boundary. A
desktop shell should reuse those surfaces instead of moving Policy, SQLite,
tokens, paths, or process authority into TypeScript.

Wails v2.13.0 is the current stable v2 release. Wails v3 is still marked Alpha,
so adopting it would add avoidable lifecycle and API churn. Wails is MIT
licensed and is compatible with this repository's Apache-2.0 source license;
binary distribution must carry the applicable third-party notice. Relevant
upstream references are the [v2.13.0 release](https://github.com/wailsapp/wails/releases/tag/v2.13.0),
[application options](https://wails.io/docs/reference/options/),
[Windows guidance](https://wails.io/docs/guides/windows/), and
[native dialog API](https://wails.io/docs/reference/runtime/dialog/).

Wails v2 does not support streamed AssetServer responses on Windows. The
ordinary browser console therefore keeps SSE, while the desktop renderer uses
bounded cursor pagination to poll the same persisted Run events.

## Decision

The desktop entry point is `cmd/cyberagent-desktop`, compiled only with the
`windows,desktop` build constraints. `web/assets_desktop.go` embeds `web/dist`
under the `desktop` tag, so a desktop build fails unless the audited Vite
production build exists. `webui.LoadEmbeddedFS` validates and snapshots the
embedded index and content-hashed assets under the same count, type, and size
bounds used by ordinary Go UI hosting.

The Wails AssetServer receives the existing `httpapi.API` Handler directly. It
opens no TCP listener. A narrow adapter clones each Wails request, pins Host and
RemoteAddr to loopback, canonicalizes Wails' empty root path, and normalizes only
the exact no-body GET/HEAD `ContentLength=-1` representation observed in Wails
v2.13. Unknown bodies retain their unknown length and continue through the
existing strict API checks.

`DesktopBridge` is the complete renderer binding surface and has exactly three
exported methods:

1. `Bootstrap`, which returns the same-origin `/api/v1` base, bounded version
   metadata, UI digest, and ephemeral memory-only tokens;
2. `SelectSkillPackage`, which opens one serialized native `.zip` dialog and
   returns only a native-issued opaque handle or explicit cancellation; and
3. `PreviewSkillPackage`, which consumes that handle once and returns the
   existing pathless metadata projection.

No bound method accepts a renderer path, file bytes, URL, command, package
installation request, network target, Scope, Policy decision, or authority
field. Tokens are generated at process startup, remain only in Go/renderer
memory, are never placed in browser storage, SQLite, logs, command output, or
the Windows registry, and must be distinct. The bootstrap fixes process,
Shell, Docker, Skill installation, and renderer path-input authority to false.

The desktop defaults to read-only. `--enable-profile-control` is an explicit
local launch flag that exposes only the already audited schema-v64 control
token; every profile still reports `process_enabled=false`,
`execution_authorized=false`, and `capability_grant=false`.

The shell uses one application instance, restores the first window when a
second instance starts, disables WebView file/drop input and the default context
menu, keeps renderer code integrity enabled, and does not add remote links.
Wails binding-origin validation remains restricted to the generated start
origin. The existing Go UI Handler supplies CSP, same-origin resource policy,
frame denial, no-referrer, permissions policy, and no-store headers.

Startup failures are shown through a bounded native Windows dialog containing
only a stable typed error code; local paths and raw causes remain on the trusted
diagnostic side. Normal GUI users no longer see a silent process exit.

## Capability Boundary

D0-A does not add:

- Run, Session, Plan, queue, approval, Diff, or Skill installation mutations;
- LocalRunner, unrestricted `os/exec`, Docker start/exec, Shell, or terminal;
- browser file upload, renderer-provided paths, drag/drop, or clipboard import;
- API-key entry, Provider settings, network Scope changes, or secret access;
- installer, portable release ZIP, registry data, file association, protocol
  handler, auto-start, background service, updater, or signing flow; or
- a new schema, Store fact, Run event, model call, or CTF capability.

Closing the window closes only this read-first client process. It does not own
or implicitly cancel a Run execution lease.

## Validation

Go tests lock the three-method binding allowlist, exact bootstrap JSON fields,
closed authority, token separation, path-free native errors, cancellation,
single-dialog serialization, one-time handle consumption, embedded-bundle
bounds, source-request immutability, and the two observed Wails request-shape
compatibility cases. TypeScript tests lock exact bridge objects, same-origin
bootstrap, authority-widening rejection, no path fields, memory-only automatic
connection, desktop event polling, native cancellation, and a preview UI with
no install action.

The production-tag Windows binary was built and launched against an isolated
schema-v71 home. Visual inspection covered the 1440x900 workbench, empty Run and
Session states, the Skill preview modal, and the native `.zip` dialog. No
overlap, horizontal overflow, blank WebView, or stale disconnect action remained.
GitHub Actions now has a separate Windows Desktop build/test job in addition to
the existing Go/Linux and TypeScript jobs. Run `29602281365` passed implementation
commit `2c0b81c` with Go/Linux in 4m57s, TypeScript in 26s, and Windows Desktop in
4m27s.

## Follow-Up

Desktop D0-B should test simultaneous CLI/Desktop database access, crash/reopen
behavior, single-instance recovery, WebView2 prerequisite diagnostics, and
bounded event-poll resumption on Windows. It must remain read-first. The first
Desktop mutation is a later D1 slice and must add a Go-owned control route,
idempotency, state fencing, and dedicated UI tests instead of widening the
native bridge.
