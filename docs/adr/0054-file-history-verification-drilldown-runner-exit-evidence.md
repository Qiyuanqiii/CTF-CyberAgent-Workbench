# ADR 0054: Exact File History, Verification Drilldown, And Runner Exit Evidence

- Status: Accepted
- Date: 2026-07-20
- Schema: unchanged at v82
- Slices: D1-G6, D1-V5, R3

## Context

The previous Code workbench could inspect a commit and a redacted file at that commit,
but it could not follow one exact path through local history. Verification coverage
showed aggregate references for a Run, so the Run-wide 100-reference cap could hide
the evidence for one checklist item. The non-product Runner contract proved process-
tree cleanup but had no bounded stdout/stderr or exit-evidence protocol.

These gaps must close without adding raw Git content, private verification bodies,
aggregate result inference, or any product process-start path.

## Decision

### D1-G6: bounded exact-file history

`repository_file_history.v1` accepts one canonical repository-relative path at the
exact registered Workspace root. Pure Go follows at most 512 commits on the current
HEAD first-parent chain and returns at most 50 commits where that exact path changed.
Each entry contains only object/short hashes, a bounded secret-redacted subject,
commit time, added/modified/deleted state, regular/executable/symlink/submodule mode
transitions, and content/mode-change booleans.

The projection does not infer renames and returns no author identity, commit body,
blob, patch, remote, host root, checkout, reference update, process, network, hook, or
authority. Missing or malformed objects and unsupported file modes fail closed. Git
graph order is authoritative; commit timestamps are not required to be monotonic.

### D1-V5: exact verification-plan-item drilldown

`operator_verification_plan_item_coverage.v1` reads one exact Run, immutable plan, and
item ordinal. It returns the exact plan/item digests, explicit aggregate pass/fail/
unknown counts, and at most 100 newest evidence-association references. References
contain opaque IDs, explicit outcome, evidence/association event sequences, and time.
Private plan guidance, evidence bodies, and operator identity are excluded.

Go, SQLite, OpenAPI, and TypeScript validate the Run/Session/Workspace/plan/item
binding, digests, unique association and evidence IDs, strict descending event order,
aggregate counts, truncation, and every false-authority field. A dedicated Store query
filters by Run + plan + ordinal, so a Run-wide inventory cap cannot hide an item's
first page. The response has no aggregate verdict and cannot mutate, approve, call a
model, execute a command, or grant authority.

### R3: metadata-only output and exit evidence

`runner_exit_evidence.v1` extends only the internal `NonProductOnly` lifecycle
contract. After a process tree has exited and been proven reaped, a backend may return
stdout/stderr metadata containing total observed bytes, at most 64 KiB of captured-
prefix length, SHA-256 of that prefix, and a truncation flag. The result contains no
raw output. Observed bytes are bounded at 64 MiB; larger or inconsistent evidence is
rejected.

Exit evidence exact-binds the exit code and reaped state and fixes `metadata_only=true`,
`raw_output_included=false`, and `product_execution_enabled=false`. Evidence collection
uses a separate bounded post-reap context, so invalid evidence is reported as an
evidence failure rather than falsely claiming that a process tree remains alive.
Windows Job Object and Unix process-group adapters remain entirely in `_test.go` and
start only the current Go test binary. No CLI, HTTP, Desktop, Agent, Sandbox,
LocalRunner, Docker, approval, or profile path can construct them.

## Verification

- Uncached `go test -count=1 ./...`: passed in 373.3 seconds.
- Focused race tests for Runner, Repository, Application, Store, and HTTP: passed.
- Full `go vet`, affected-package zero-warning `staticcheck`, module verify/tidy: passed.
- Web strict TypeScript, 37 Vitest files / 127 tests, and Vite production build: passed.
- OpenAPI and generated TypeScript regeneration were byte-deterministic. OpenAPI is
  71 paths, 77 operations, and 170 schemas. SHA-256 values are
  `C78A701600F8535A9C2398C12B3AAA7A695A93AD58913010D8904ADEED121625` and
  `977B8EEE7E9A268040453E0ADFB6FFB4C58489D4B90B94177473DC4B882E4740`.
- `npm audit --audit-level=high`: zero vulnerabilities.
- Windows Desktop tagged tests and reproducible dual build passed. The unsigned binary
  SHA-256 is `c96047d7f3ea0afbe3b2f54f1c4ded197a861b29d644cb2edb449c8b3e46b031`;
  `release_ready=false`, installer absent, and registry writes false remain correct.
- The Linux amd64 Runner test binary cross-compiled successfully. Test fixtures are the
  only credential-shaped strings found; user test keys are absent from the diff.

The combined audit fixed two medium robustness issues before acceptance: TypeScript
no longer assumes first-parent commit timestamps are monotonic, and truncated
verification detail now rejects duplicate evidence or returned outcome counts that
exceed the aggregate. No unresolved high or medium issue is known on an enabled path.

## Consequences

Users can inspect one path's bounded history and drill into the explicit evidence for
one verification check without widening content or execution authority. R3 gives a
future Runner an exact evidence contract, but it is not a product Runner and does not
authorize process start.

The next candidate batch is D1-G7 history-to-commit navigation, D1-V6 opaque pagination
for exact item evidence, and R4 non-product stdin/descriptor/resource evidence. Real
Local/Docker start, xterm input, installers/signing, Rust analyzers, network grants,
and CTF solving remain separately gated.
