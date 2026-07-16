# ADR 0025: Protected Delete Command Guard

## Status

Accepted as a non-schema, defense-in-depth Policy boundary. Real Local and
container-process execution remain disabled.

## 中文摘要

Go Policy 现在会在审批之前永久拒绝显式的高风险文件删除命令，包括递归删除、绝对或越界路径、通配目标、环境变量或命令替换目标，以及常见 PowerShell、`cmd`、Python 和 Node 删除形式。该检查只作用于 Shell、ScriptProcess、Sandbox 等可执行意图；README、日志、模型说明和其他只读证据不会因为包含示例命令而自动获得执行语义或被这条执行规则误判。永久拒绝不能被逐次审批或 Session Grant 覆盖。

这是一层纵深防御，不是宿主 Full Access 的安全证明。间接脚本、构建工具、编码载荷和解释器可以隐藏文件副作用，因此未来真实执行仍必须依赖操作系统或容器隔离，使用户主目录和控制面数据根本不可见或只读；工作区删除应由单独的类型化工具完成，而不是放开任意 Shell。

## Context

The original default Policy rejected literal `rm -rf /` and `rm -rf *`, but it
did not classify equivalent targets such as `$HOME`, `${HOME}`, `~`,
`$env:USERPROFILE`, `%USERPROFILE%`, an indirect variable, or common
PowerShell and interpreter forms. Current builds could not execute those
commands because Local execution is fail-closed and Shell/ScriptProcess are
dry-run only, but enabling a future backend without another gate would create
an avoidable safety gap.

The guard must distinguish executable intent from untrusted evidence. A README
that warns about `rm -rf $HOME` is data, not a request to execute or authorize
that command.

## Decision

`policy.DefaultChecker` owns a protected-delete classifier that runs before the
ordinary deny patterns for executable tool calls. It covers raw Shell requests
and decodes the bounded command envelope supplied by `script_process.v1` and
`sandbox_manifest.v1`.

The classifier permanently denies:

1. recursive deletion through common Unix, PowerShell, Windows, Python, Node,
   and `find -delete` forms;
2. deletion whose target is absolute, parent-traversing, wildcarded, based on
   an environment variable or command substitution, or contains the current
   user-home path;
3. equivalent structured executable/argv intents, including literal Sandbox
   environment bindings.

The returned reason is stable and path-free, has `critical` risk, and never
sets `NeedsApproval`. Argument-map traversal is sorted so a Policy fingerprint
does not depend on Go map iteration order. A simple relative, non-recursive
delete can still be proposed, but it remains dry-run in the current product.

Only known executable surfaces receive this classification. Ordinary model
responses, repository text, logs, and read-only tool content retain their
existing untrusted-evidence treatment and do not become executable authority.

## Security Boundary

The guard is intentionally conservative and cannot prove arbitrary programs
side-effect free. It does not inspect a referenced script file, decode opaque
payloads, understand every shell dialect, or replace filesystem permissions.
No future release may enable a real host process merely because this classifier
passes.

Before production execution is authorized, the runtime must independently
prove that protected host roots are absent or read-only, workspace mounts are
descriptor-safe, output is isolated, and a process cannot escape through
links, junctions, interpreters, build hooks, or child processes. Any future
delete feature must use a Go-owned typed operation with canonical
workspace-relative resolution, bounded scope, explicit review, audit facts,
and preferably quarantine/recovery semantics. Protected-root denial must
remain impossible to override by approval.

## Validation

Focused tests cover `$HOME`, indirect variables, PowerShell, `cmd`, Python,
Node, recursive relative deletion, traversal, absolute Windows paths,
`find -delete`, the actual current home path without reason disclosure,
structured Sandbox and ScriptProcess envelopes, non-executable evidence, safe
relative non-recursive cases, durable Gateway denial, and failed operator
override. The existing LocalRunner, NoopRunner, approval, workspace path, and
Sandbox disabled-start tests remain the outer fail-closed layers.

## Consequences

- The screenshot-style `$HOME` accident is now rejected by Policy even before
  the existing dry-run/disabled-runner boundary.
- This slice adds no migration, tool, process, filesystem mutation, or product
  execution capability.
- Some opaque or highly dynamic cleanup commands may be rejected
  conservatively; safe deletion should move to a typed workspace tool rather
  than accumulate shell exceptions.
- Schema v64 is now used by the independently reviewed Run execution-profile
  selection control plane. The content-addressed external Skill Registry moves
  to schema v65 or later.
