# ADR 0031: Content-Addressed Inert User Skill Registry

## Status

Accepted for schema v69. This decision adds local import, immutable metadata
history, content-addressed package storage, list/show, and removal tombstones for
validated user Skill packages. It does not authorize Run selection, prompt
injection, tools, network access, Provider calls, scripts, hooks, or commands.

## 中文摘要

schema v69 允许操作者把已经通过 `skill_package.v1` 严格校验的本地 ZIP
导入 Code 或 Cyber 用户目录。Go 先持久化不可变安装意图，再把原始包写入
内容寻址对象目录，完整回读复核后才写入完成结果；崩溃后使用同一 operation
key 可继续未完成导入。SQLite 只保留有界元数据和摘要，对象正文不进入事件。

外部包始终标记为 `operator_installed_untrusted`。导入、列表、详情与移除都不
读取正文作为指令，不调用模型、网络、工具、Shell 或安装钩子，也不授予 Run
选择或上下文注入权限。Cyber 目录第一版只接受精确的 `script` Profile。移除
只追加 tombstone，对象继续保留；已被 Run 固定的版本禁止移除。

## Context

ADR 0024 fixed a deterministic two-entry ZIP and a pure-memory validator. That
boundary could prove package structure and metadata but could not retain a
validated package, recover an interrupted import, distinguish Code and Cyber
catalogs, or preserve an immutable installation history.

Executing package content during import would turn an untrusted documentation
format into a software installer. Loading the body into a model immediately
would also let repository-style indirect prompt injection become authority.
Schema v69 therefore stores and audits the package while deliberately keeping
it unavailable to Runs.

## Decision

The Go control plane now provides:

- `skill import <package.zip> --surface code|cyber --operation-key <key>
  --confirm-untrusted-skill`;
- `skill installed`, filtered by surface/Profile and optionally including
  removed history;
- `skill installed show <name>@<version>`;
- `skill remove <name>@<version> --operation-key <key> --confirm-remove`.

An import accepts only bytes that pass the complete ADR-0024 parser. Built-in
Skill names are reserved. A Code package may declare its validated compatible
Profiles. A Cyber package must declare exactly `script`; it cannot inherit
Code, Review, or Learn guidance. The registry is bounded to 64 historical
package identities and eight versions per name.

All installed packages retain these fixed conclusions:

- trust class `operator_installed_untrusted`;
- risks `untrusted_instructions` and `declared_tools_not_capabilities`;
- zero executable assets and install hooks;
- `import_command_execution=false`;
- `import_network_access=false`;
- `import_provider_calls=false`;
- `tool_capability_grant=false`;
- `run_selection_authorized=false`;
- `context_injection_authorized=false`.

The CLI prints bounded metadata, hashes, object identity, status, and the false
authority conclusions. It never prints the Skill body, source path, raw
operation key, or a command derived from the package. Before SQLite persistence,
the only free-text Manifest metadata field (`description`) passes through the
shared credential redactor; archive/package fingerprints still bind the exact
operator-supplied object bytes.

## Object Storage And Recovery

Validated archive bytes are stored below:

`$CYBERAGENT_HOME/skill-registry/objects/sha256/<prefix>/<digest>.zip`

`PackageObjectStore` exposes only `Put` and `Verify`; it has no execute or
delete method. The local implementation uses an `os.Root`, same-directory
exclusive temporary file, file sync, hard-link publication, and full readback
validation. A pre-existing object is accepted only after size, SHA-256,
deterministic ZIP, and semantic package fingerprint verification. Symlinks,
missing objects, corruption, replacement during read, and mismatched receipts
fail closed. Directory sync is best effort for Windows portability; the
write-ahead SQLite intent makes an interrupted publish recoverable.

Every list, show, and completed import replay verifies the object again. The
filesystem remains under the local operator's account rather than being a
cryptographic trust root, so same-user tampering is detected on use rather than
claimed impossible.

## Immutable Ledger

Schema v69 adds installation operations, installation intents, installation
results, removal operations, and removal tombstones. Operation keys are
domain-separated SHA-256 digests; raw keys are not stored. The operation is
inserted before its root record in one transaction. Deferred foreign keys and
reciprocal triggers prevent either half from committing alone. Update and
delete triggers make every record append-only.

The installation intent commits before object publication. Completion binds
the exact installation fingerprint, archive digest, semantic package
fingerprint, object key, byte count, and completion time. Same-key/same-intent
replay returns the original record; changed intent conflicts. Competing SQLite
connections converge on one intent and one completion.

Removal appends a tombstone and never deletes the content-addressed object.
Historical recovery is therefore preserved, while future selection is fixed
false. Go and SQLite both reject removal when an exact name, version, and
content hash is already pinned by `run_skill_selection_items`. Reinstall and
restore after a tombstone require a future explicit protocol; they are not
silently inferred.

## Authority And Prompt-Injection Boundary

Package Markdown and descriptions are untrusted evidence, not policy or
operator instruction. Schema v69 provides no content reader to the Agent
runtime and does not merge external packages into the embedded Registry.
Declared tool dependencies remain metadata only. Confirmation acknowledges
storage of untrusted instructions; it is not approval to execute or inject
them.

A later schema must separately define exact external version selection,
Code/Cyber compatibility, redaction, token budgets, provenance, root/Specialist
minimization, removal interaction, and first-model-call atomicity. Until that
protocol is implemented, an installed external Skill cannot affect a Run.

## Validation

Tests cover strict confirmation, Code/Cyber separation, reserved names,
operation replay/conflict, pending-import recovery, independent-Store
convergence, immutable SQL, operation-only commit rejection, object
publication races, corruption, symlinks, cancellation, forged object receipts,
independently generated concurrent request identities, credential redaction in
free-text installation metadata, metadata-only CLI output, tombstone replay,
retained objects, Run-pinned
removal rejection in both Go and SQL, and v68-to-v69 migration without
fabricated installations.

The final local gate passed the full ordinary and race suites in 259.7s and
275.3s. Vet, zero-warning staticcheck, module verification/tidy diff,
zero-finding govulncheck, OpenAPI drift checks, 21 frontend tests, the Vite
production build, and zero-vulnerability npm audit are green. No real model,
network request, Shell, Docker operation, installer hook, or host process was
run by this slice. Two real Services generating independent candidate IDs and
timestamps converged through 20 ordinary and 10 race repetitions for both
import and removal. Initial Linux CI run `29556933994` exposed concurrent nested
directory preparation through `os.Root.MkdirAll`; publication now creates and
`Lstat`-verifies every path component and rejects symlink redirection. Twelve
independent Stores passed 100 ordinary and 20 race repetitions. No unresolved
high- or medium-severity issue is known.

## Follow-Up

The next recommended slice is schema v70: immutable external-Skill selection
for a Run followed by a separately budgeted, provenance-preserving, redacted
root/Specialist load path. It must consume only the exact verified object and
must not grant declared tools. HTTP/Desktop import, signatures, team catalogs,
Marketplace distribution, restore/garbage collection, Rust analyzers, real
Sandbox execution, and CTF automation remain separate later gates.
