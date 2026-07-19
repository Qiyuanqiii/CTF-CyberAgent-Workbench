# ADR 0050: Repository History, Verification Plans, And Handoff Export

Date: 2026-07-20

Status: Accepted

## Context

The Code workbench could show current repository state and a bounded redacted working-tree Diff, but it could not show recent local history. Verification stored operator outcomes but had no separate checklist, which risked confusing intended checks with completed evidence. Code Handoff was reconstructable in the UI but had no portable, integrity-bound export.

These additions must preserve the existing rule that Go is the sole control plane. Repository data is untrusted evidence, verification guidance is not a result, and a downloaded handoff is not an execution or mutation capability.

## Decision

1. `repository_history.v1` reads only the exact registered Workspace root through `go-git`. It never discovers a parent repository, invokes Git, reads remotes, executes hooks, or uses the network. It follows at most 50 first-parent commits and returns at most 64 local branches after scanning at most 1,024 references. Subjects are normalized, secret-redacted, and bounded; author identities, email, commit bodies, and host roots are omitted. Redirected or linked Git metadata fails closed.
2. Schema v80 stores immutable `operator_verification_plan.v1` records and ordered items. One plan carries 1-32 operator-authored checks and is exact-bound to a Code Run, active Session, Mission Workspace, operation digest, request fingerprint, metadata event, and content digests. SQLite fixes `guidance_only=true` and command execution, model assertion, result inference, approval, and authority to false. Plans and v78 verification evidence remain separate protocols and tables.
3. `code_handoff_export.v1` renders a stable `code_handoff.v1` snapshot as Markdown or JSON, capped at 256 KiB. It includes a source event high-water mark, byte count, content SHA-256, MIME type, and safe filename. The TypeScript client recomputes the digest and checks the Run/high-water binding before initiating a browser download.
4. All three views remain bounded. Repository history is read-only local metadata, plan creation is a narrow operator-authored append, and export is download-only. None can checkout, fetch, push, run a verification command, infer pass, resume a Run, accept a report, apply an edit, or start a process.

## Consequences

- The Repository tab can explain local branch and recent first-parent context without exposing developer identity or remote credentials.
- Verify now distinguishes what an operator intends to check from what an operator later observed.
- Handoffs can be carried between sessions or attached to external workflows with deterministic content integrity.
- Merge side-parent traversal, commit bodies, remote operations, automatic test execution, plan-item result inference, handoff import/resume, and all Git mutation remain deferred.
- Real Shell, LocalRunner, and Docker process execution remain disabled and require independent lifecycle and sandbox gates.

## Verification

The ordinary product gate passed the uncached full Go suite, post-audit focused Repository/Application/Store/HTTP tests, `go vet`, 124 React tests across 37 files, strict TypeScript, deterministic OpenAPI generation, and the Vite production build. Chrome-extension checks against the final Go-hosted bundle verified Repository privacy, guidance/result separation, Handoff source metadata, zero page-level horizontal overflow, and zero console warnings/errors. The audit additionally fixed exact-limit plan inventory parsing, saturated hostile Git metadata counters, rotated idempotency keys after a failed plan intent is edited, and deferred object-URL revocation until after the download click can begin.
