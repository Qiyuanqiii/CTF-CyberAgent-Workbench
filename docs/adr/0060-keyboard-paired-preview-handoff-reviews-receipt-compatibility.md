# ADR 0060: Keyboard Paired Preview, Handoff Reviews, And Receipt Compatibility

- Status: Accepted
- Date: 2026-07-20
- Scope: D1-G12, D1-V11, and R9 on schema v84

## Context

ADR 0059 left three narrow follow-up gaps. The paired exact-redacted preview had visible
previous and next buttons but no region-scoped keyboard navigation or explicit close
control. Snapshot receipt reviews were durable and public through their own inventory,
but the regenerable Code Handoff did not carry those review facts. The Runner evidence-
set receipt had cross-platform canonical golden vectors but no strict compatibility gate
for malformed or future receipt envelopes.

These gaps can be closed without adding a database migration, Git authority, reviewer
identity disclosure, or a product Runner starter.

## Decision

### D1-G12: keyboard-accessible paired preview

The existing paired preview region is focusable and declares `ArrowLeft`, `ArrowRight`,
and `Escape` shortcuts. Opening it moves focus into the region. Plain left/right keys
navigate only the already returned bounded comparison candidates; modified shortcuts
are ignored, and boundary movement remains disabled. Escape and a familiar close icon
remove the preview and return focus to the exact button that opened it.

The implementation issues no new Git request. It still makes at most the two existing
exact redacted preview calls for the selected base/head/path tuple, and it makes no call
for an absent side. It grants no raw content, mutation, process, network, hook, checkout,
or reference authority.

### D1-V11: non-authorizing reviews in Code Handoff

The Code Handoff now reads schema-v84 receipt reviews under the same bounded source-
event snapshot retry used by the rest of the handoff. It publishes at most twenty exact
references and bounded confirmed/disputed totals. Each reference carries only:

- opaque review and receipt IDs;
- receipt content SHA-256;
- receipt and review event sequences;
- `metadata_confirmed` or `metadata_disputed`;
- review time.

Go validates every record, Run/Session/Workspace binding, unique review and receipt ID,
and strict descending event order. Reviewer identity, operation key, request fingerprint,
snapshot body, verification body, and private narrative remain absent. Metadata-only,
read-only, and non-authorizing are true; snapshot/result acceptance, inference, rewrite,
approval, authority, and execution are false.

JSON and Markdown exports contain the same bounded review metadata and exact digest.
The React client enforces exact keys, sequence order, counts, digest shape, source event
high-water, and every false authority field before rendering a summary. Adding this
projection does not accept a verification result, release a report, resume a Run, or
start execution.

### R9: strict evidence-set receipt compatibility rejection

An internal, side-effect-free compatibility decoder accepts one complete JSON object no
larger than 8 KiB. It requires every v1 field exactly once, rejects unknown or missing
fields, duplicate keys, trailing JSON, invalid UTF-8, oversized input, wrong JSON types,
and unsupported top-level or child-record protocols. A structurally valid envelope must
still exactly match the six typed evidence records and their canonical digest.

Versioned test vectors cover eleven rejection cases: future receipt protocol, future
record protocol, missing and unknown fields, duplicate protocol, trailing JSON, wrong
type, invalid UTF-8, oversized envelope, digest mismatch, and product-authority widening.
The valid baseline and all rejection vectors run on ordinary Go CI and Windows Desktop
CI. The decoder has no filesystem, network, subprocess, LocalRunner, Docker, or product
starter connection.

## Consequences

- Database schema remains v84; D1-V11 reads the existing immutable review table.
- Code Handoff remains regenerable from durable records and source-event high-water.
- A confirmed review remains only an operator fact about receipt metadata.
- TypeScript remains presentation and strict response validation; Go owns binding,
  persistence, event order, and authority.
- R9 establishes a future import/transport boundary without enabling such an import or
  any product process execution.

## Verification

The three-slice functional gate passed on the final implementation:

- `go test ./... -count=1`: 403.0 seconds;
- repeated focused HTTP and Runner tests, plus `go vet ./...`;
- 37 frontend test files and 130 tests, strict TypeScript, Vite production build, and
  zero high-severity npm vulnerabilities;
- deterministic OpenAPI and TypeScript generation: 75 paths, 83 operations, 182 schemas;
- secure Desktop boundary tests and a reproducible Windows dual build.

The OpenAPI SHA-256 is
`e637623b6931c88466fd8d04412da31091af36c9e57161dc7d6c3784f64f56a3`.
The unsigned portable GUI SHA-256 is
`a02843a00fc050d9eee51426fc460a0b40eb3413a256c6fb855a838b562c9a72`.
Automated Windows checks pass, while `release_ready=false` remains correct pending the
manual Windows 10/WebView2 matrix.

Review found and fixed four low-risk issues: the Markdown export initially omitted the
exact receipt digest; the Go projection relied on Store uniqueness/order without an
independent interface-boundary check; the embedded JSON export parser checked only a
subset of false authority fields; and the OpenAPI receipt-digest field lacked the same
hex pattern already enforced at runtime. No unresolved high- or medium-severity issue is
known on an enabled path.

No real Provider or API key, Shell, LocalRunner, Docker, hook, attack traffic, external
network request, installer, registry mutation, or product process start was used.
