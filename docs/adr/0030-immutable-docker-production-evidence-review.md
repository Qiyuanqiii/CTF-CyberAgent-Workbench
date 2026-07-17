# ADR 0030: Immutable Docker Production-Evidence Review

## Status

Accepted for schema v68. This decision records one operator acceptance or
rejection of an exact schema-v67 production-evidence receipt. It does not
verify process isolation and does not permit daemon writes, container start,
process execution, output export, or Artifact commit.

## 中文摘要

schema v68 为一份精确完成的 v67 harness receipt 增加一次不可变操作员审阅。
操作者必须显式确认并选择 `accepted|rejected`。接纳只能使用固定原因
`metadata_scope_accepted`；拒绝只能使用五个有界原因码。请求、数据库、事件
和 CLI 都不接收自由文本说明、原始 daemon payload、socket、路径、镜像仓库
名或容器/资源身份。

接纳只表示操作员接受这份有界元数据回执的范围，不表示生产隔离已经验证。
因此 `production_verified_count=0`、`sufficient_check_count=0`、
`blocker_count=16`，并且 start gate、容器启动、进程执行、输出导出和 Artifact
提交权限继续全部为 `false`。v68 不执行 Docker 请求，也不启动任何进程。

## Context

Schema v65 created a non-authorizing machine receipt. Schema v66 placed durable
attempt ownership and generation fencing before collection. Schema v67 then
allowed an explicitly opted-in Linux collector to perform five bounded GETs and
stored sixteen deliberately insufficient `observed_failed` results.

That receipt still needed a durable human decision boundary. Without a separate
ledger, later code could confuse “capture completed” with “an operator accepted
the scope of the captured metadata.” Combining acceptance with runtime
authorization would be unsafe because v67 starts no process and verifies none of
the sixteen production controls.

## Decision

Schema v68 stores:

- one immutable `sandbox_docker_production_evidence_review.v1` per exact v67
  evidence/attempt pair;
- one digest-only `sandbox_docker_production_evidence_review_operation.v1` for
  idempotent replay;
- one metadata-only `sandbox.docker_production_evidence_reviewed` Run event.

The Application accepts only an evidence ID, stable operation key, bounded
decision, bounded reason code, reviewer identity, and explicit confirmation.
The accepted decision permits only `metadata_scope_accepted`. Rejection permits
`integrity_concern`, `environment_concern`, `scope_concern`,
`insufficient_evidence`, or `operator_rejected`. There is no free-form reason
field.

Go rebuilds the review from authoritative Store state. It requires a completed
v67 harness result and rejects a legacy v66 inert result, a pending attempt, or
an unowned receipt. It binds the evidence, attempt, v63 blocked start-gate
review, Run, Mission, Workspace, source operation digest, evidence capture
fingerprint, harness result fingerprint, and authority, threat-model, suite,
and environment fingerprints.

## Atomicity And SQL Enforcement

The operation references the review through a deferred foreign key. The Store
inserts the operation first and then the review in one transaction. The review
insert trigger requires the exact operation, while the deferred foreign key
prevents an operation-only transaction from committing. A review-only insert
fails its trigger. Update and delete triggers make both records immutable.

SQLite independently requires:

- the exact completed v67 harness receipt and no legacy v66 attempt result;
- the exact v63 `blocked/deny_start` review and matching authority chain;
- sixteen evidence items, all `observed_failed`, observed, not production
  verified, and insufficient for start;
- zero production-verified and sufficient checks plus sixteen blockers;
- false start-gate, daemon-write, container-start, process, output, and
  Artifact authority;
- review time no earlier than the evidence and harness result;
- at most one review per evidence/attempt and at most 32 reviews per Run.

Migration creates only tables, indexes, and triggers. It does not invent a
review for any existing receipt or incomplete attempt.

## Authority Boundary

`receipt_accepted=true` is a classification of this metadata receipt only. It
is not a capability, approval grant, production verification, start permit, or
input to an existing Runner. All authority booleans remain false in the domain
object, SQL checks, event projection, and CLI output. The review service has no
Docker transport, model call, Tool Gateway execution, Shell, or host-process
dependency.

The step is operationally independent, not a cryptographic two-person rule.
`reviewed_by` is an audit identity supplied through the current local operator
surface; the project does not yet claim authenticated human identity or enforce
separation of duties. A future multi-user control plane must add authentication
before it can make that stronger claim.

## Privacy And Replay

Public projections retain bounded IDs, classifications, counts, timestamps,
and fingerprints. They omit daemon payloads and errors, sockets, paths, image
repository names, resource/container identities, raw operation keys, private
lease identities, and free-form narratives.

The raw operation key is domain-separated and stored only as a digest. A
same-key/same-semantic retry returns the existing review without a daemon call
or second event. Reusing that key for another receipt, reviewer, decision, or
reason conflicts. A second operation key cannot create a second decision for
the same evidence or attempt.

## Validation

Tests cover accepted and every rejected reason, invalid decision/reason pairs,
pending and legacy-v66 rejection, source/fingerprint tampering, explicit
confirmation, operation replay and changed-intent conflicts, one-decision
enforcement, SQL immutability, event privacy, operation-only commit failure,
v67-to-v68 migration without fabrication, CLI list/show privacy, and absence of
new daemon collection during review.

The full ordinary/race suites, vet, static analysis, dependency checks,
frontend gates, Markdown validation, and repository privacy/capability scans are
release gates. The Windows development host does not run the optional Linux
daemon integration path for this metadata-only slice.

The final local gate passed the full ordinary/race suites in 247.9s/276.3s,
vet, zero-warning staticcheck, module verification/tidy diff, zero-finding
govulncheck, 21 frontend tests, OpenAPI/build/npm audit, 57-file/74-link
Markdown validation, repository privacy/encoding/forbidden-entry/diff scans,
Linux test-binary cross-compilation, and isolated real-CLI schema-v68 smoke.
Focused Domain/Store/Application/CLI repetitions passed at 50/10/5/5 and race
repetitions at 10/3/3/3.

An independent audit additionally required the review root to persist and
self-validate the semantic request fingerprint, SQL to bind it directly to the
operation, and every Store read/replay to recompute the relation. Negative tests
now cover review-only and operation-only halves, operation mutation/deletion,
source/fingerprint/authority tampering, two-Store convergence, and the complete
rejected Store/event/Application/CLI/list/show/replay path. No unresolved
high/medium issue is known.

## Follow-Up

The next recommended product slice returns to the content-addressed external
Skill Registry: imported packages must remain inert, validated, profile-scoped,
and unable to run scripts, hooks, network requests, models, or tools during
import. A later separately reviewed runtime-verification lifecycle must produce
real start/wait/TERM/KILL/orphan evidence before Docker process authority can be
considered. Output export, atomic Artifact commit, Local OS sandboxing, Rust
analyzers, and CTF automation remain independent slices.
