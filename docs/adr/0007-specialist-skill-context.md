# ADR 0007: Minimal Specialist Skill Context

- Status: Accepted
- Date: 2026-07-14

## Context

Schema v40 delivers a Run's pinned built-in Skill selection to the root Supervisor, but Specialist children previously received only their instructions, owned memory, and ordinary bounded history. Copying the full root selection into every child would waste context, blur Code and Cyber catalogs, and let planning guidance influence an execution worker that cannot choose a direction. Allowing an assignment or model to choose its own Skill would also turn descriptive text into an authority channel.

## Decision

Schema v47 adds Go-owned `specialist_skill_context.v1` delivery.

- The source of truth is the parent Run's immutable `skill_selection.v1`, current immutable `run_mode.v1`, and embedded versioned Registry. Child assignment text is never a selection input.
- Each active Specialist Attempt receives at most one already-pinned guide under a separate 1,024-token default budget and 2,048-token hard maximum.
- On the Code surface, the selected guide must exactly match the Run Profile: `code`, `review`, `learn`, or `script`.
- On the Cyber surface, Code/Review/Learn guidance is omitted. Only the Script Profile may receive the narrow `script` guide.
- `plan-delivery` remains root-only. A child cannot select, widen, or reorder the subset.
- The assignment's durable Agent identity, parent, Profile, limits, and delegated capabilities enter a fingerprint so a changed assignment cannot reuse an old preparation. A child must hold `model.chat` before any model context is prepared.
- Registry validation rechecks the pinned name, version, source hash, bytes, Profile, redacted delivery hash, and conservative token accounting in memory.
- SQLite persists one immutable metadata-only preparation per AgentAttempt and one immutable commit bound to the first durable Specialist model start. Preparation, commit, and model-start event ordering is recoverable and transactional.
- Skill bodies, paths, names, versions, source hashes, and delivered hashes do not enter the v47 tables or Run events. The exact body exists only in the current Go-owned Provider request.

## Invariants

1. Built-in Skill guidance is subordinate to Policy, Scope, budgets, approval, Tool Gateway, and Run lifecycle. It grants no tool or execution capability.
2. Core delegation remains capped at two depth-one children. Schema v47 does not change admission, operator review/application/schedule gates, Fan-out tiers, or recursive-spawn limits.
3. A Run with a persisted parent selection cannot start a Specialist model call until the active Attempt has a matching preparation. Runs without a historical selection retain their existing no-Skill behavior.
4. Concurrent or restarted preparation converges on the same immutable record. A changed mode, assignment, selection, Attempt, or reconstructed metadata conflicts or fails closed.
5. A failed model-start transaction leaves no model call, Skill commit, or committed event. Prepared metadata remains available for retry.
6. External repository content, child instructions, model output, HTTP clients, and Tool Gateway calls cannot register or choose built-in Skills.

## Consequences

Specialists gain focused workflow guidance without receiving the full root prompt or a new authority source. Code and Cyber execution surfaces remain distinct, context cost is bounded, and delivery can be audited after restart without storing the prompt body. A pre-v47 in-flight Specialist call that has a persisted parent selection but no preparation fails closed and must be recovered as a fresh Attempt; migration does not invent historical prompt provenance. Real Sandbox execution, Rust analyzer processes, and CTF automation remain separate later gates.
