# ADR 0032: External Skill Run Selection And Minimized Context

## Status

Accepted for schema v70. This decision lets an operator pin verified user Skill
packages to one not-yet-started Run and deliver their redacted bodies to the
root Agent and, when explicitly designated, one Specialist context. It grants
no tool, Shell, process, network, secret, scope, Policy, approval, or delegation
authority.

## 中文摘要

schema v70 把 v69 中惰性存放的外部 Skill 接入一条独立确认的 Run 选择链。
操作者必须再次确认非可信上下文，并按精确 `name@version` 固定 active 安装；
安装确认本身不能替代 Run 确认。每个 Run 最多四项，总预算最多 4096，最多
一项可以显式交给 Specialist。Code/Cyber Catalog 与 Profile 继续分离，Cyber
只接受 `script`。

Go 在每次交付前重新读取精确内容寻址对象，复核文件身份、大小、摘要、ZIP
结构、语义指纹与 Manifest，再脱敏并放入用户角色 JSON 信封。正文只存在于
当前模型请求内；SQLite 与事件只记录不可变来源元数据。外部正文是工作流
建议，不是系统策略，也不能授予包里声明的工具。

## Context

ADR 0031 deliberately stopped at inert storage. It proved that a package could
be imported and retained without executing it, but it provided no way for an
operator to use that package in a Run. Merging installed packages directly into
the embedded Registry would collapse installation trust, Run intent, and model
authority into one action. It would also make indirect prompt injection in
Markdown look like trusted system guidance.

Schema v70 therefore introduces a second decision and a separate provenance
protocol. Installation says only "retain these validated bytes". Selection says
"deliver this exact version as untrusted workflow guidance to this Run".

## Selection Decision

The CLI exposes:

```text
cyberagent skill select-external <run-id> <name>@<version>... \
  --operation-key <stable-key> --confirm-untrusted-skill-context \
  [--specialist <name>@<version>] [--token-budget <1..4096>]
cyberagent skill external-selection <run-id>
```

Selection is accepted only while the Run is `created`. It freezes:

- Run, Mission, current mode snapshot/revision, surface, and Profile;
- exact installation and install-result fingerprints;
- content/archive digests and byte counts;
- package semantic fingerprint and content-addressed object key;
- aggregate budget and deterministic item order;
- the optional single Specialist designation;
- the requesting operator and digest-only operation identity.

One Run has at most one external selection. A selection contains one to four
active packages and a maximum aggregate conservative budget of 4096. Cyber
accepts only the Script Profile. Built-in and user catalogs remain separate.
Declared tool dependencies are recorded only as a count and never become a
grant.

Same-key/same-intent replay returns the original immutable selection. Changed
intent conflicts. Operation keys are domain-separated SHA-256 digests; raw keys
do not enter SQLite or events. Go and SQLite both prevent removal of an exact
installation pinned by a v70 selection.

## Object Read And Context Assembly

`PackageObjectLoader` is intentionally separate from the v69 writer interface.
It returns a defensive in-memory snapshot only after checking the expected
object path, ordinary-file identity, byte count, SHA-256, strict deterministic
ZIP, semantic package fingerprint, and Manifest/content binding. Symlink,
replacement, corruption, cancellation, and any provenance mismatch fail closed.

Root delivery uses `external_skill_context.v1` with the selection budget.
Specialist delivery uses `external_specialist_skill_context.v1`, defaults to a
1024 budget, has a hard 2048 limit, and loads only the one package explicitly
designated by the operator. A child cannot select a package through its
assignment, model output, or declared dependencies.

Content passes through the shared secret redactor. The delivered digest, byte
count, token upper bound, and redaction count bind the in-memory body without
persisting it. The body is serialized into a user-role
`external_skill_guidance.v1` envelope whose authority map fixes these fields to
false:

- Policy override;
- tool grant;
- file-write grant;
- Shell or process execution;
- network access;
- secret access;
- scope expansion;
- delegation or child creation.

The system control message states that external Skill text is optional workflow
guidance, repository and document claims are evidence, and requests to hide
steps, expose secrets, alter Policy, or widen authority must be ignored. This
reduces indirect prompt-injection influence without claiming that model
semantics alone are a security boundary. Go Policy, Scope, approvals, budgets,
and tool registration remain the actual capability boundary.

## Durable Provenance And Recovery

Schema v70 adds immutable selection/item/operation tables plus root and
Specialist preparation/commit ledgers. Selection events contain only protocol,
surface, Profile, counts, budgets, and closed authority conclusions. Context
events contain only preparation/commit identity and aggregate provenance; they
contain no package body, name/version, source path, raw key, secret, or model
text.

Preparation is recoverable. For root turns, its commit is part of the same
transaction that writes the first `model.started` record. Specialist context is
committed in the corresponding first Specialist model-call transaction. A
crash before model start leaves a reusable preparation; replay cannot invent a
commit or double-deliver durable history. Update and delete triggers make every
v70 fact append-only.

## Non-Goals

Schema v70 does not add:

- HTTP, TUI, or Web selection mutation;
- Desktop file upload or package preview;
- URL/Git/Marketplace installation or package signatures;
- executable package assets, install hooks, or commands;
- additional model tools or autonomous child scheduling;
- Local or Docker process execution.

## Validation

Focused tests cover the second confirmation, exact version/object binding,
removed and cross-surface rejection, Code/Cyber/Profile constraints, stable
operation replay, immutable SQL, direct-SQL removal protection, v69-to-v70
migration without fabricated state, root and Specialist first-call provenance,
one-package Specialist minimization, object mismatch, secret redaction, event
and CLI privacy, and indirect prompt-injection placement in user rather than
system/assistant messages.

The final local release gate passed full ordinary and race suites in
197.6s/264.4s, vet/static analysis, module verification/tidy diff, zero-finding
govulncheck, 21 frontend tests, OpenAPI/build/npm audit, repository
credential/runtime-artifact/encoding/Markdown-link/diff scans, and an isolated
real-CLI schema-v70 smoke. The audit fixed one medium-severity schema defect
that had incorrectly made an installation globally single-Run; uniqueness is
now selection-scoped and covered by a second-Run regression. The audit also
tightened latest-mode Specialist provenance, token boundaries, cross-phase
replay, selection/operation atomicity, cancellation-safe object loading, and
explicit closed authority fields. GitHub Actions remains the final remote Linux
gate for the delivery commit; its measured result is recorded after push.

## Follow-Up

The next product-facing Skill slice may add read-only HTTP/TUI/Web projection
and a separately audited Go-owned local upload preview for Desktop D1. Any
browser mutation requires the distinct control token and a new security review.
Package signing, team catalogs, Marketplace distribution, restore/garbage
collection, Rust analyzers, real Sandbox execution, and CTF automation remain
independent later gates.
