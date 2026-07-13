# ADR 0004: Go-Owned Plan/Delivery Workflow

Status: Accepted
Date: 2026-07-13

## Context / 背景

CyberAgent Workbench already separates a stable Mission from resumable Runs, and schema v41 gives each Run an immutable `code|cyber` surface plus an operator-controlled `plan|deliver` phase. Product-style planning guidance alone is not a reliable state or authorization boundary: model text can be malformed, repeated after restart, or attempt to imply selection and execution in one response.

CyberAgent Workbench 已经区分稳定 Mission 与可恢复 Run，schema v41 也为每个 Run 固定了 `code|cyber` 工作面，并由操作者控制 `plan|deliver` 阶段。仅靠产品式提示词无法承担状态或授权边界：模型输出可能畸形、重启后重复，也可能把“提出方案、选择方案、执行方案”混在一次响应中。

## Decision / 决策

Schema v42 introduces the strict `plan_delivery.v1` domain protocol, implemented and controlled by Go.

schema v42 引入由 Go 实现和控制的严格 `plan_delivery.v1` 领域协议。

- A root model may persist a proposal only during an active Plan turn. The proposal has exactly three directions; each has 1-8 ordered modules, acceptance criteria, bounded tradeoffs, and backward-only dependencies.
- root 模型只能在活动 Plan turn 中记录提案。提案必须恰好包含三个方向；每个方向包含 1-8 个有序模块、验收条件、有界权衡，以及只能指向前序模块的依赖。
- Proposal creation does not select a direction, change phase, execute work, grant tools, approve Shell/network/file access, or admit a child Agent.
- 创建提案不会选择方向、切换阶段、执行工作、授予工具、批准 Shell/网络/文件访问，也不会接纳 child Agent。
- Only an operator CLI command may select direction 1, 2, or 3, and only while the Run is paused in Plan with no active execution lease.
- 只有操作者 CLI 命令可以选择方向 1、2 或 3，并且 Run 必须暂停在 Plan 且没有活动 execution lease。
- One selection transaction creates the immutable selection, chosen WorkItem dependency graph, pinned decision Note, digest-only idempotency fact, and metadata-only events.
- 一次选择事务同时创建不可变选择、对应 WorkItem 依赖图、置顶 decision Note、仅摘要化的幂等事实和仅元数据事件。
- Selection leaves the Run in Plan. Delivery requires a separate explicit phase transition governed by schema v41.
- 选择后 Run 仍停留在 Plan；进入 Delivery 必须另行执行受 schema v41 约束的显式阶段切换。
- HTTP, TUI, and React are read-only projections. The embedded `plan-delivery` Skill supplies workflow guidance but never replaces Go validation or grants capability.
- HTTP、TUI 和 React 都是只读投影。内置 `plan-delivery` Skill 只提供工作流指导，不替代 Go 校验，也不授予能力。

## Invariants / 不变量

1. Exactly one accepted direction can exist for a proposal and one Plan selection can exist for a Run.
2. Operation keys are normalized, bounded, and stored only as domain-separated digests; changed-intent replay conflicts.
3. Proposal and selection records are append-only and protected by Go validation plus SQLite constraints and triggers.
4. Selection cannot race an active Supervisor lease or target a stale mode revision.
5. WorkItems and the handoff Note are committed atomically with selection, so no partial accepted plan is visible.
6. Public projections expose no raw operation key, lease/fencing identity, private requester details, model text, or capability grant.

1. 每个提案最多接受一个方向，每个 Run 最多存在一份 Plan 选择。
2. operation key 必须规范化且有界，落库时只保存域分隔摘要；相同 key 的异意图重放必须冲突。
3. 提案和选择记录只追加，由 Go 校验与 SQLite 约束/trigger 双层保护。
4. 选择不能与活动 Supervisor lease 竞争，也不能绑定过期模式 revision。
5. WorkItems、交接 Note 与选择原子提交，外部永远看不到半份已接受计划。
6. 公共投影不公开原始 operation key、lease/fencing 身份、私有 requester 信息、模型正文或能力授权。

## Consequences / 影响

The workflow reuses WorkItems and Notes instead of introducing a second planner database. It remains resumable and auditable across processes, while the operator retains a visible decision point. The next increment can add Delivery checkpoint and audit gates against the selected WorkItems without changing proposal or selection authority.

该工作流复用 WorkItems 与 Notes，不引入第二套规划数据库；它可以跨进程恢复和审计，同时保留清晰可见的人工决策点。下一增量可直接围绕已选择 WorkItems 增加 Delivery checkpoint 与审计门禁，而无需改变提案和选择的权限边界。

The tradeoff is deliberate friction: the model cannot immediately execute its preferred direction, and the operator must perform both selection and phase transition. This is acceptable because those actions carry different meanings and should remain separately auditable.

代价是有意保留的摩擦：模型不能立即执行自己偏好的方向，操作者还必须分别完成方向选择与阶段切换。这两类动作含义不同，分开审计是合理且必要的。
