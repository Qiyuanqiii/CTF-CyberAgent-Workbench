# ADR 0052: Conservative Model Context And Cumulative Handoff Memory

Date: 2026-07-20

Status: Accepted

## Context

The runtime already compacted long Sessions, but its token estimate undercounted CJK and other non-ASCII text, complete model requests had no final aggregate context-window gate, and each new summary replaced rather than incorporated the prior summary. A long-running Agent could therefore lose early decisions after repeated compaction or send a multilingual/tool-heavy request larger than the intended local budget.

Repository documents also remain a Prompt Injection boundary. Historical text must be useful as evidence without turning README notes, tool output, or model prose into new control instructions.

中文结论：上下文压缩必须累计、可校验并保留来源；文件内容仍是证据，不是控制者。默认 32K 是 Go 的保守规划值，不是远端模型规格承诺。

## Decision

1. `model_context_window.v1` defines a conservative per-model planning policy. The fallback is 32,768 total tokens, a 1,024-token safety margin, 1,024 default output tokens, and a 4,096 output cap. Exact Provider/Model overrides are generation-swapped with the Router.
2. Go estimates the complete request, including messages, ToolCalls, ToolResults, tool descriptions, and parameter schemas. ASCII uses a conservative word/character estimate; every non-ASCII UTF-8 byte counts as one token. Saturating arithmetic prevents overflow.
3. Root Supervisor and Specialist requests pass through the aggregate gate immediately before each Provider call. Only the oldest ordinary Session history is optional. Trusted control, current input, assembled memory, embedded or selected Skill context, and tool schemas are never silently dropped. Mandatory overflow fails with `RESOURCE_EXHAUSTED` before network activity.
4. Active conversation compaction still defaults to more than eight messages with the newest four preserved. `handoff_memory.v1` stores at most 4,000 characters and 12 retained provenance records, with an exact predecessor ID, content SHA-256, cumulative compacted count, omitted count, monotonic last ordinal, and Session-message ID high-water. Source references are redacted and bounded before persistence.
5. Schema v82 marks the handoff table append-only. SQLite rejects updates, deletes, non-v1 inserts after migration, stale predecessor bindings, cross-Workspace chains, and non-monotonic counts. Go recomputes envelope, record, and row integrity on every read. Concurrent stale writers fail rather than fork the chain. A retry after summary insertion but before Session-message marking reuses the previous summary when no new IDs exist and admits only IDs above the high-water when new messages are mixed in.
6. Existing rows migrate as `handoff_memory.v0`. They remain readable and are folded once into the next v1 handoff as `prior_handoff` with `instruction_authorized=false`.
7. Only provenance-confirmed operator messages and Go control records may retain instruction authority. Workspace files, README text, tools, model output, external Skills, and legacy summaries remain untrusted evidence. The runtime does not automatically rewrite or reload arbitrary `AGENTS.md`, README, or project-memory files.

## Consequences

- Repeated compaction no longer drops the earliest retained handoff facts.
- Multilingual and tool-schema-heavy requests fail conservatively instead of overflowing the local plan.
- Oldest chat history can be shed without deleting mandatory control or current-task context.
- The 32K fallback may underuse a larger Provider window until an exact override is configured; this is intentional fail-safe behavior.
- Memory remains deterministic extractive handoff rather than model-generated semantic summarization. A future semantic summarizer must preserve the same provenance and immutable-chain rules.
- The current Router override has no user-facing CLI/API control yet.

## Verification

Focused tests cover ASCII/CJK/emoji estimation, output clamping, oldest-history removal, mandatory overflow, Router generation swaps, Root and Specialist Provider delivery, repeated compaction, bounded record retention, tamper detection, stale forks, immutability, and v81-to-v82 legacy migration/folding.

The final uncached full Go suite passed in 348.5 seconds. `go vet` and staticcheck passed for the changed Go packages. TypeScript strict typecheck and all 127 Vitest tests across 37 files passed. The combined review fixed the original non-cumulative-summary defect, replaced a shared validator ceiling with a dedicated 12-record handoff cap, initialized zero-value Router maps, corrected v81 downgrade-fixture ordering, redacted and bounded source references, clamped clock rollback, and added exactly-once Session-message high-water recovery. No unresolved high or medium issue is known in this batch.

This is the first three-slice batch in a new six-slice cycle. Full-repository race, staticcheck, govulncheck, dependency/privacy, production-build, and browser robustness gates remain scheduled after the next three slices. No real Provider, API key, Shell, LocalRunner, Docker start, hook, attack traffic, or external network request was used.
