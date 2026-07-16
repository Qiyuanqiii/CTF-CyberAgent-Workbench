# ADR 0029: Bounded Linux Read-Only Docker Evidence Harness

## Status

Accepted for schema v67. This decision permits narrowly bounded read-only
Docker daemon contact on Linux after explicit operator and environment opt-in.
It does not permit daemon writes, image pulls, container start, process
execution, output export, Artifact commit, or evidence acceptance.

## 中文摘要

schema v67 在 v66 写前 attempt、当前 generation lease 和零读取控制检查点
全部持久化后，增加独立不可变 harness intent。Linux 操作者还必须显式设置
`CYBERAGENT_DOCKER_PRODUCTION_EVIDENCE=1`。Go 先对固定本机 Unix endpoint
执行一次按 attempt label 精确过滤的容器清单 GET；只有 owned scope 为空时，
才持久化 daemon-aware reconciliation。随后依次执行 `_ping`、`version`、
`info` 和精确已存在 image digest 的 inspect。

该协议最多执行五次 GET，每次最多四秒，整体不超过 v66 attempt 的 30 秒
deadline。它不读取 `DOCKER_HOST`，不 pull，也没有 create、start、exec、
remove 或 delete 方法。receipt 仅证明 daemon 已被只读访问；16 项检查全部
固定为 `observed_failed`、`production_verified_count=0` 和
`sufficient_for_start=false`，不能解锁 v63 start gate。

## Context

Schema v65 established a non-authorizing evidence ledger. Schema v66 then
placed durable attempt ownership, generation fencing, and a control-plane
reconciliation checkpoint before collector invocation. The built-in collector
still could not contact a daemon, so the product had no bounded path for
capturing real machine metadata through that recovery boundary.

A daemon-aware collector must distinguish three facts:

1. Go owns the current attempt generation before any external call.
2. No resource carrying this attempt's ownership label already exists before
   capture resumes.
3. Reading daemon and image metadata is not proof that any process-isolation
   control works in production.

## Decision

Schema v67 stores three new immutable records:

- `sandbox_docker_production_evidence_harness_intent.v1`, binding the exact
  attempt, blocked review, container plan, pre-existing OCI image digest,
  fixed local endpoint, label selector, operator, and all authority flags;
- `sandbox_docker_production_evidence_harness_reconciliation.v1`, binding one
  real-daemon inventory GET to the current generation and its v66 control
  reconciliation;
- `sandbox_docker_production_evidence_harness_result.v1`, binding the same
  generation, reconciliation, v65 receipt, four capture GETs, and zero
  production-verified checks.

The Application sequence is:

1. Acquire or recover the v66 attempt under a current generation lease.
2. Persist that generation's zero-read v66 control reconciliation.
3. Rebuild the exact v67 intent from the immutable review and container plan,
   then persist or revalidate it.
4. GET `/containers/json` with `all=1` and an exact
   `cyberagent.workbench.production-evidence-attempt=<attempt-id>` label
   filter.
5. Require an empty result and persist the daemon-aware reconciliation.
6. GET `/_ping`, `/version`, `/info`, and
   `/images/<exact-sha256-digest>/json`, in that order.
7. Normalize bounded metadata into fingerprints and atomically commit the v65
   evidence, v67 result, operation, lease release, and metadata-only events.

The resource-list response is bounded to 256 entries and the common response
body bound remains 2 MiB. Duplicate JSON keys, duplicate resource IDs,
mismatched labels, redirects, non-GET methods, extra query fields, non-local
endpoint classes, malformed metadata, and digest mismatch fail closed. Raw
resource IDs are fingerprinted only in memory and are not persisted.

## Authority Boundary

The harness transport embeds the existing read-only transport and adds only
the exact labeled container-list operation. Its interface contains no pull,
create, start, run, exec, attach, log, archive, export, stop, kill, remove, or
delete method. The local implementation dials only `/var/run/docker.sock`,
ignores proxy and `DOCKER_HOST`, disables redirects and keep-alives, and uses a
bounded client timeout.

All sixteen probe items are deliberately `observed_failed`. The GET responses
show that selected metadata was readable, but no container was started and no
runtime isolation behavior was exercised. Go validation and SQLite checks fix:

- `production_verified_count = 0`;
- `sufficient_check_count = 0` and `blocker_count = 16`;
- `start_gate_passed = false`;
- daemon-write, container-start, process-execution, output-export, and
  Artifact-commit authority to false.

The old v66 inert result path is rejected once a v67 harness intent exists.
The v67 operation gate accepts either a complete legacy/inert v66 result or a
complete v67 result, never an unowned receipt. Reconciliation and completion
must match the current generation and full private lease identity.

## Failure, Recovery, And Privacy

A failure before daemon-aware reconciliation may have attempted a daemon read
without obtaining a trustworthy response. Audit output therefore reports
daemon contact as `not_confirmed`, not falsely as `false`. A persisted harness
reconciliation reports contact as confirmed. Failures remain bounded typed
codes and never store raw transport errors or payloads.

On restart, a persisted v67 intent forces the harness path; removing the
environment opt-in cannot silently downgrade it to the v66 inert collector.
A released or expired attempt resumes only at generation `N+1`, writes a new
v66 control reconciliation, and performs a new empty-scope reconciliation
before capture. A stale generation cannot commit evidence.

Tables, events, and CLI projections omit sockets, daemon payloads, image
repository names, container/resource IDs, paths, raw operation keys, lease IDs,
and lease owners. They retain only bounded classifications, counts, protocol
versions, fingerprints, and authority booleans.

## Compatibility

Migration creates no harness intent, reconciliation, or result for existing
v65 receipts or v66 attempts. An in-flight v66 attempt upgrades in place and
continues on its original inert path unless the product explicitly prepares a
v67 intent under a current lease. Existing v66 terminal results remain valid.

## Validation

Tests cover exact GET ordering and count, fixed label query encoding, response
and duplicate rejection, owned-resource collision, per-call cancellation,
zero production verification, absence of mutation methods, write-ahead intent
and reconciliation ordering visible inside the collector, v66 fallback
rejection, generation fencing, replay without new calls, migration from an
in-flight v66 attempt, row immutability, CLI privacy, and all false authority
bits. The ordinary repository suite, race detector, vet, static analysis,
dependency checks, frontend gates, and repository scans are release gates.

The development host is Windows, so the default validation does not contact a
real Docker daemon. A Linux integration run remains an explicit environment
operation and must use an already-present exact image digest. It must not pull
or start a container.

## Follow-Up

Schema v68 should add an independent operator evidence-acceptance/rejection
ledger. Acceptance of this v67 receipt alone must still leave all sixteen
production checks blocked because v67 records zero runtime verification. A
later, separately audited start/wait/TERM/KILL/orphan lifecycle must generate
its own production evidence before any process authority can be considered.
Output export, atomic Artifact commit, Local OS sandboxing, external Skill
installation, Rust analyzers, and CTF automation remain separate slices.
