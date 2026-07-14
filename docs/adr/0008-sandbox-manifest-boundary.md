# ADR 0008: Go-Owned Sandbox Manifest Boundary

Status: Accepted
Date: 2026-07-14

## Context / 背景

CyberAgent Workbench needs one protocol between a Run and future process backends. That protocol must eventually support `TS -> Go -> Docker` and `Go -> Rust`, but describing a command must never be confused with authorizing or starting it.

CyberAgent Workbench 需要一份连接 Run 与未来进程后端的统一协议。该协议以后要承载 `TS -> Go -> Docker` 和 `Go -> Rust`，但“描述命令”绝不能被解释成“已经授权或启动命令”。

## Decision / 决策

1. `sandbox_manifest.v1` is a strict, size-bounded JSON protocol owned by Go. It defines backend intent, executable and ordered argv, a sandbox working directory, workspace-relative mounts, exact network scope, resource limits, environment literals or secret references, input Artifact identities, output capture/paths, timeout, and cancellation grace.
2. Unknown fields, duplicate JSON keys, trailing data, invalid UTF-8, traversal, overlapping mounts, wildcard targets, non-canonical CIDRs, literal secret variables, credential-shaped argv, and values outside hard limits fail closed.
3. The Manifest is transient input. SQLite schema v48 stores immutable preparation, validation, and digest-keyed operation facts only. Raw executable, argv, paths, environment values, secret references, targets, and Manifest JSON do not enter those tables or Run events.
4. Go binds the normalized Manifest fingerprint to one non-terminal Run, Mission, persisted Workspace root, Mission Scope, Policy decision, optional exact approval, and a generated cancellation identity. A requested network allowlist must be an exact subset of the Mission allowlist.
5. Any Docker/Local backend, write mount, network access, or secret reference requires approval when Policy allows it. Permanent Policy denial cannot be overridden. An approved record still does not authorize execution in v48.
6. `NoopRunner` performs deterministic validation. `LocalRunner` and `DockerRunner` remain fail-closed. Schema checks, Go types, events, and CLI output all fix `backend_enabled=false` and `execution_authorized=false`; no host or container process is started.

1. `sandbox_manifest.v1` 是由 Go 主控、严格且有大小上限的 JSON 协议。它描述后端意图、可执行文件与有序 argv、沙箱工作目录、工作区相对挂载、精确网络 Scope、资源限制、环境字面量或密钥引用、输入 Artifact、输出捕获/路径、超时和取消宽限。
2. 未知字段、重复 JSON key、尾随数据、非法 UTF-8、路径穿越、重叠挂载、通配网络目标、非规范 CIDR、密钥变量字面量、疑似凭证 argv 和越过硬上限的值全部关闭失败。
3. Manifest 只在当前调用内存中存在。SQLite schema v48 仅保存不可变 preparation、validation 与摘要幂等操作事实；可执行文件、argv、路径、环境值、密钥引用、目标和 Manifest JSON 原文不进入这些表或 Run 事件。
4. Go 将规范化 Manifest 指纹绑定到一个非终态 Run、Mission、持久化 Workspace 根、Mission Scope、Policy 决策、可选精确审批和 Go 生成的取消身份。Manifest 网络白名单必须是 Mission 白名单的精确子集。
5. Docker/Local 后端、写挂载、网络访问或密钥引用在 Policy 允许时仍强制要求审批；永久 Policy 拒绝不可被覆盖。v48 中即使审批已通过，也不代表允许执行。
6. `NoopRunner` 只做确定性校验；`LocalRunner` 与 `DockerRunner` 继续关闭失败。Schema、Go 类型、事件和 CLI 均固定 `backend_enabled=false`、`execution_authorized=false`，不会启动宿主机或容器进程。

## Consequences / 影响

- Users can inspect a deterministic Manifest and durably attach its metadata-only validation to a Run through `sandbox validate` and `run sandbox prepare|list|show`.
- Same-key retries and concurrent processes converge on one preparation; changed intent conflicts. Rejected intent remains auditable without retaining command content.
- Future execution must require the caller to resupply a Manifest whose fingerprint matches the preparation, then pass a separate approval/execution audit. Schema v48 alone can never be treated as an execution permit.
- A later Docker slice must re-resolve every workspace path, recheck Scope/Policy/approval/budget/lease, materialize secret references without persistence, and prove cleanup/cancellation before enabling a process.

- 用户可以通过 `sandbox validate` 和 `run sandbox prepare|list|show` 检查确定性 Manifest，并把不含原文的校验事实绑定到 Run。
- 相同 key 的重试和跨进程并发会收敛到同一 preparation，异意图复用返回冲突；被拒绝的意图可审计但不保留命令正文。
- 未来执行必须重新提交与 preparation 指纹一致的 Manifest，并通过独立审批/执行审计；schema v48 本身永远不是执行许可证。
- 后续 Docker 切片必须重新解析所有工作区路径，重新检查 Scope、Policy、审批、预算与租约，在不持久化密钥的前提下解析 secret reference，并先证明取消与清理可靠，再允许启动进程。
