# ADR 0001: Go Is the Sole Control Plane

Date: 2026-07-10

Status: Accepted

## Context

CyberAgent Workbench may grow from a Go CLI/TUI into a multi-language product:

- Go for the agent runtime, security policy, local API, persistence, model access, workspace control, and sandbox orchestration.
- Rust for deterministic binary, object-file, archive, and packet-capture analysis.
- TypeScript for a React-based local web interface.

Without a strict ownership rule, direct cross-language calls can create multiple control planes, inconsistent security checks, and secret exposure.

## Decision

Go is the only control plane. All privileged or stateful operations are owned by Go.

```text
TypeScript UI
    |
    | HTTP / WebSocket
    v
Go control plane
    |-- LLM providers
    |-- SQLite / optional vector store
    |-- workspace and file permissions
    |-- policy and approval decisions
    |-- Docker / sandbox lifecycle
    `-- Rust analyzers over versioned JSON
```

Allowed call directions:

- TypeScript -> Go -> Rust
- TypeScript -> Go -> Docker
- TypeScript -> Go -> LLM
- Go -> Rust analyzer process -> JSON result

Disallowed call directions:

- TypeScript -> Rust directly
- TypeScript -> Docker, shell, SQLite, local secrets, or LLM directly
- Rust -> Go callbacks
- Rust -> LLM, session state, user settings, policy decisions, or Docker
- Rust -> TypeScript

## Go Responsibilities

Go owns:

- sessions, tasks, context compaction, and agent loops;
- model routing, API credentials, and provider requests;
- workspace path checks, file permissions, edit approvals, and artifact records;
- shell/tool approvals, network scope checks, policy, and audit events;
- SQLite and any optional Qdrant integration;
- Docker client access, timeouts, resource limits, and process lifecycle;
- the HTTP/WebSocket API consumed by TypeScript;
- validation of every request and response crossing a language boundary.

## Rust Responsibilities

Rust analyzers are deterministic tools, not agents. Each analyzer accepts bounded input and emits structured JSON.

Suitable Rust capabilities include:

- binary and executable parsing with `goblin` or `object`;
- ZIP/TAR inspection;
- PCAP parsing;
- CPU-heavy deterministic transforms where `rayon` is useful.

Rust must not read global application configuration or API keys. Go passes only the workspace-scoped input required for one operation. Rust output is untrusted until Go validates its protocol version, size, schema, exit status, and declared artifacts.

Initial transport is a child process with JSON on stdin/stdout. FFI is intentionally avoided. JSON-RPC or gRPC may replace the transport later without changing ownership.

Example envelope:

```json
{
  "protocol_version": "v1",
  "request_id": "req-123",
  "operation": "binary.inspect",
  "input": {"path": "attachments/sample.bin"}
}
```

```json
{
  "protocol_version": "v1",
  "request_id": "req-123",
  "ok": true,
  "result": {},
  "error": null
}
```

## TypeScript Responsibilities

TypeScript is a presentation and interaction layer. The expected stack is React/Vite, with Zustand or TanStack Query where useful. Monaco Editor and xterm.js are optional views over Go-controlled capabilities.

TypeScript must never be the enforcement point for:

- API key handling;
- Docker or shell execution;
- network-scope authorization;
- workspace path or file permission decisions;
- policy approval or audit persistence.

The browser receives redacted, least-privilege data from Go. Removing or bypassing a UI check must not grant additional authority.

## Dependency Adoption

Dependencies are added only when their owning feature is implemented:

- Cobra: adopt when migrating the growing CLI command tree provides enough value to justify the churn.
- `net/http`: already used by providers; keep it for a small API, or add chi when the local HTTP surface needs routing/middleware composition.
- SQLite: current default local store.
- Qdrant: optional behind a vector-store interface; never required for the local-first baseline.
- Docker client: add with the first real isolated runner, not for placeholder detection.
- Bubble Tea: already used for TUI.
- YAML/JSON Schema: add parsing and schema validation when configuration becomes user-editable runtime input.
- Rust crates: add with the first deterministic analyzer.
- React/Vite packages: add with the first web UI, after the Go API contract exists.

## Language Ratio

The long-term 60% Go, 20% Rust, 20% TypeScript split is a product-shape target, not a source-line quota. Ownership and security boundaries take precedence. It is acceptable for Go to remain above 60% until deterministic analyzers and the web interface justify the other languages.

## Consequences

- Security-sensitive behavior has one authoritative implementation.
- Rust tools are independently testable and replaceable.
- CLI, TUI, and web UI share the same Go application services.
- Cross-language calls have serialization and subprocess overhead.
- Go must maintain protocol schemas, cancellation, timeouts, output limits, and compatibility tests.
