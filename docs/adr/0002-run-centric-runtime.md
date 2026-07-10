# ADR 0002: Run-Centric Resumable Agent Runtime

Date: 2026-07-10

Status: Accepted

## Context

The current scaffold has useful boundaries for tasks, sessions, tools, file edits, context compaction, workspaces, policy, and SQLite. They are not yet joined by one durable execution aggregate. Long-running execution, cancellation, resume, multi-agent work, budget enforcement, structured findings, and headless mode would otherwise each invent separate lifecycle state.

The public [Strix repository](https://github.com/usestrix/strix) demonstrates several useful product ideas: a top-level resumable run, one coordinator for an addressable agent graph, per-agent planning memory, explicit root/child completion, sandbox lifecycle ownership, streamed UI events, and structured findings/reports.

We want the ideas, not a port. CyberAgent Workbench has different constraints: Go is the sole control plane, actions are approval-first, SQLite is authoritative, Rust is a deterministic tool boundary, and CTF behavior is deferred.

## Decision

Adopt a run-centric architecture:

- `Mission` stores stable user intent and authorization scope.
- `Run` stores one resumable execution attempt and its immutable config snapshot.
- `RunSupervisor` owns startup, resume, budget, cancellation, sandbox, and finalization.
- `AgentCoordinator` is the single owner of agent graph state and inbox delivery.
- `WorkItem` and `Note` provide structured memory outside chat history.
- root and child agents complete through distinct structured lifecycle actions.
- normalized events form the common interface for CLI, TUI, API, and CI.
- findings require evidence and validation state before acceptance.

## Adopted Ideas

- resumable execution with explicit durable state;
- root and specialist agents with parent/child ownership;
- single coordinator instead of direct mutable cross-agent access;
- per-agent task planning and run-level notes;
- bounded budgets, cancellation, and cleanup;
- sandbox backend abstraction and per-run environment;
- progressive skill loading;
- structured reports and machine-readable artifacts;
- interactive and headless execution using the same core.

## Adaptations

- SQLite event/state tables replace scattered JSON files and process-global report state.
- all privileged tools pass through Go policy and approval services;
- multi-agent concurrency is opt-in and added after single-agent recovery works;
- deterministic finding fingerprints are primary; model-assisted deduplication is optional;
- scope is a signed or operator-approved system snapshot that prompts cannot expand;
- Rust analyzers run behind a versioned JSON process protocol;
- generic coding/review/learning profiles come before CTF/offensive profiles.

## Rejected or Deferred

- copying Strix source, prompts, or package structure;
- Python as a second control plane;
- direct agent access to Docker or host shell;
- default autonomous public-network testing;
- mutable global report state;
- requiring Docker for basic local-first chat and code review;
- adding multi-agent orchestration before run resume and cancellation are reliable.

## Consequences

- Existing services require gradual adapters into the Run/Event model.
- Database migrations must become versioned before schema expansion.
- The UI becomes simpler because it consumes one event stream.
- Recovery tests and state-machine tests become core quality gates.
- More durable state is stored, but execution can survive process restarts.
- Feature work is reordered around vertical run slices rather than isolated commands.

## Migration Rule

No big-bang rewrite. Each milestone must keep existing commands working while one end-to-end path moves to the new architecture. Compatibility code is removed only after its replacement passes unit, integration, resume, and CLI smoke tests.
