# ADR 0061: Exact Receipt Review Navigation, Audit Facts, And Envelope Golden Vectors

- Status: Accepted
- Date: 2026-07-20
- Scope: D1-G13, D1-V12, and R10 on schema v84

## Context

ADR 0060 made immutable receipt-review metadata available in Code Handoff and added a
strict internal receipt-envelope decoder. Three narrow gaps remained. A Handoff review
reference could not open the corresponding Verify projection, Code Journey did not show
the bounded review audit facts, and accepted encoded envelopes did not have byte-level
golden vectors distinct from the canonical evidence-set vectors.

These gaps do not require a new API, database migration, mutation, or product process
starter. Navigation must treat Handoff data as an untrusted reference to independently
validated Verify inventories, never as authority or as a substitute verification result.

## Decision

### D1-G13: exact Handoff-to-Verify navigation

Each Handoff receipt-review row has an icon-only navigation command. The in-memory target
contains the existing opaque review and receipt IDs, receipt SHA-256, receipt and review
event sequences, closed decision, and review time. It is not written to a URL, browser
storage, SQLite, or an event.

Verify independently loads its existing strict review, receipt, verification-plan, and
coverage projections. A target opens only when all review fields match, the exact receipt
matches its ID/digest/event sequence, and the receipt's plan and item digests match the
current coverage projection. Only then does the existing plan item expand and the exact
receipt row receive focus. Missing, truncated, stale, or mismatched data displays a
bounded unavailable state and never falls back to another receipt. Leaving Verify clears
the target, so later ordinary navigation does not silently reuse it.

This is presentation navigation only. It does not accept a snapshot or result, approve a
change, resume a Run, grant authority, or execute anything.

### D1-V12: bounded review facts in Code Journey

Code Journey reuses the existing strict `code_handoff.v1` query key and displays at most
three receipt-review references. The compact audit section shows confirmed/disputed
counts, exact event/time metadata, explicit `metadata only` and `non-authorizing` badges,
and source truncation even when all three displayed references fit. Its navigation uses
the same exact Verify target and fail-closed matching as Handoff.

No private body, reviewer identity, operation key, request fingerprint, model assertion,
or execution state is added. TypeScript remains a presentation client over Go-owned
validated projections.

### R10: accepted-envelope golden vectors

The internal non-product Runner adds two versioned golden vectors for the encoded
`runner_evidence_set_receipt.v1` compatibility envelope. Both reuse the existing normal
empty-exit and forced-timeout bounded-metadata typed fixtures. Each encoded envelope is
exactly 660 bytes. Their SHA-256 values are:

- normal empty exit: `38dda925152009b0284af5b870e155f2bd4c332ac0cbf45742b632341b02f28c`;
- forced timeout: `34bda56ca0ff73ded18233e0b92b5273baefeba07c2f8ddf4358d792c24e20d4`.

The test requires strict decode, full compatibility validation, and byte-identical
re-encoding. Its name matches the existing Linux and Windows CI selector. It reads only
repository testdata and calls pure encoding/validation functions; it starts no process,
contacts no network or Docker daemon, and remains disconnected from every product Runner
entry point.

## Consequences

- Database schema remains v84 and the OpenAPI contract remains unchanged.
- Handoff and Journey references cannot override or weaken Verify validation.
- Receipt-review navigation is ephemeral UI state and cannot become durable authority.
- Truncated inventories are visible and fail closed instead of selecting a nearby item.
- R10 pins transport encoding in addition to R8's canonical evidence-set digest and R9's
  malformed/future rejection behavior.
- Real Local/Docker execution, xterm input, installer hooks, and product envelope import
  remain closed.

## Verification

The cumulative six-slice robustness gate passed on the final implementation:

- uncached `go test -count=1 ./...`: 421 seconds;
- uncached `go test -race -count=1 ./...`: 509.5 seconds;
- full vet, zero-warning staticcheck, module verification/tidy, and both govulncheck paths
  with zero reachable findings;
- 37 frontend files and 134 tests, strict TypeScript, deterministic OpenAPI generation,
  Vite production build, and zero npm vulnerabilities;
- secure Desktop tests/vet and a reproducible Windows dual build.

OpenAPI remains 75 paths, 83 operations, and 182 schemas, with SHA-256
`e637623b6931c88466fd8d04412da31091af36c9e57161dc7d6c3784f64f56a3`. Generated
TypeScript SHA-256 is
`339498e9d72c1fc70b44bf5e799996e255368d89a45b80989e4a31d6015f8578`. The unsigned
portable GUI SHA-256 is
`7ae75f36c2291fbf9e7d9e72071ae8d8534f4e27dd56c6d34bd04dc064f47a19` and remains
`release_ready=false` pending the manual Windows 10/WebView2 matrix.

Review found and fixed five low-risk state/diagnostic issues: a delimiter-based focus key
could theoretically collide; receipt navigation did not initially cross-bind current
plan/item digests; ordinary Verify navigation could retain an earlier exact target; an
exactly-three-item truncated source did not show its truncation badge; and a row could
remain visually focused after a later projection drift. No unresolved high- or
medium-severity issue is known on an enabled path.

No real Provider or API key, Shell, LocalRunner, Docker, hook, attack traffic, external
network request, installer, registry mutation, or product process start was used.
