# ADR 0062: Go-Owned Analyzer Protocol, Rust Fixture, And Shared Vectors

- Status: Accepted
- Date: 2026-07-20
- Scope: P10-A1, P10-A2, and P10-A3 on schema v84

## Context

ADR 0001 reserved Rust for deterministic analysis behind the Go control plane, but the
repository previously contained no Rust source or executable protocol. Introducing a
real archive, object, or PCAP analyzer before fixing ownership, limits, error semantics,
and cross-language compatibility would allow implementation details to become an
accidental security contract.

The first Rust slice therefore needs to be useful as a protocol reference while
remaining disconnected from Runs, tools, Artifacts, files, networks, models, and every
product process starter.

## Decision

### P10-A1: Go owns `analyzer_protocol.v1`

`internal/analyzer` defines strict request, success-result, and error envelopes. The
request carries one opaque safe request ID, the exact `fixture.digest.v1` analyzer ID,
one media type, canonical standard Base64 inline content, explicit limits, four explicit
false capabilities, and `metadata_only=true`.

The fixed limits are:

- request envelope: 96 KiB;
- decoded input: 64 KiB;
- result envelope: 512-byte declared minimum and 16 KiB global maximum;
- timeout declaration: 100 to 30,000 milliseconds;
- request ID and media type: 128 bytes each;
- JSON duplicate-key scan: at most eight nested levels.

Unknown, duplicate, missing, trailing, invalid-UTF-8, non-canonical Base64, malformed,
oversized, future-version, unsupported-analyzer, or authority-widening requests fail
closed. Error messages are fixed metadata and never echo parser details or unsafe IDs.
Fourteen versioned error codes cover protocol rejection plus future Go-side process
deadline, process-failure, oversized-result, invalid-result, and internal classifications.

The Go package exposes only strict parsing, result/error validation, and a pure reference
evaluation. It imports no application, Store, Runner, Artifact, Tool, HTTP, or process
package and contains no `os/exec` or equivalent starter.

### P10-A2: Rust is a deterministic fixture tool

`analyzers/fixture` is an Apache-2.0 Rust crate using locked `clap`, `serde`,
`serde_json`, `base64`, and `sha2` dependencies. The binary has no path, URL, command,
Run, Session, key, config, or capability argument. It reads at most 96 KiB plus one byte
from stdin and writes one bounded JSON envelope to stdout.

Successful evaluation returns only media type, input byte count, SHA-256, UTF-8 status,
and logical line count. Raw content is never returned. Filesystem, network, subprocess,
and environment capabilities must all be false and capabilities-used is always false.
The tool does not own policy, approval, persistence, or lifecycle state.

Rust is pinned by `analyzers/rust-toolchain.toml` to 1.97.1. The local bootstrap used
rustup 1.29.0; its 12,814,336-byte installer matched the Winget-published SHA-256
`86478e53f769379d7f0ebfa7c9aa97cb76ca92233f79aa2cc0dbee2efaac73c7` before execution.
The local host is `x86_64-pc-windows-gnu` with Cargo 1.97.1 and WinLibs GCC 16.1.0.

### P10-A3: one contract, two independent validators

`analyzers/testdata/analyzer_protocol_v1_vectors.json` is the shared compatibility
source. It pins all protocol IDs, limits, exit codes, fourteen error codes, and five
vectors: empty success, UTF-8 metadata success, capability denial, input-limit denial,
and future-protocol denial.

Go and Rust independently load that file. Neither invokes the other. Both require exact
exit code, semantic JSON, stdout byte length, and stdout SHA-256. The five output hashes
are:

- empty success: `454d988b988627333805ba90a476379a8694e7123b81023c51473a36866861b8`;
- text success: `e2961f362118d99bfce58847b0376546bdc8aa06eb9e710be08e0f8292dd9048`;
- capability denial: `5cf3777222e5a372bf7deecf25d91108f51b31fa3ad4c8caacf2cc585c9965a0`;
- input-limit denial: `78cb7eb62e028b808e2e39c48bf4dcfe14a66b37d840d000c8f0cfa40c97cff9`;
- future protocol denial: `d91cc0896cc3b5182685efe41be9d531dabf5715168e146cd2a6ced2e0b59824`.

GitHub Actions now has a separate pinned Rust job for format, locked tests, and
zero-warning clippy. The ordinary Go job explicitly runs the same shared-vector test
before the full repository suite.

## Consequences

- Go remains the only control plane; TypeScript has no new surface.
- Database schema remains v84 and OpenAPI remains unchanged at 75 paths, 83 operations,
  and 182 schemas.
- The repository now contains Rust source, but no product analyzer Registry, bridge,
  invocation, result persistence, Artifact commit, file access, or execution authority.
- `target/` is ignored and Rust/TOML/lock files are normalized to LF.
- The next real analyzer can evolve behind a fixed protocol rather than redefining the
  control boundary.
- A future process bridge must still independently enforce wall deadlines, stdout/stderr
  bounds, exit status, result validation, executable identity, policy, approval, and
  Artifact transactions. Timeout fields in this request are declarations, not current
  OS enforcement.

## Verification And Review

The three-slice functional gate includes the 394.6-second uncached full-repository Go
suite, full vet, Go analyzer regression and fuzzing, locked Rust tests, format,
zero-warning clippy, the real fixture CLI success/denial smoke, 37 frontend files and 134
tests, deterministic OpenAPI generation, Vite build, npm audit, RustSec audit, secure
Desktop tests/vet, and a reproducible Windows dual build. The unsigned GUI SHA-256 is
`69ed40aede0cfc23e075df824fecf6c1ef7b4b0586a8f4b685b7d8aa95dde3b4` and remains
`release_ready=false`. RustSec loaded 1,166 official advisories and found zero known
vulnerabilities in 41 locked crate dependencies.

Protocol fuzzing executed more than one million evaluations in the first pass and a
second post-review pass without panic, unbounded output, or non-strict response.

Review found and fixed five dormant or low-risk issues before product wiring: shared
vectors initially did not pin all constants/error codes; missing explicit-false fields
needed parity tests; Go result validation allowed non-empty UTF-8 metadata with zero
lines; oversized error-envelope classification differed from success results; and the
Go duplicate-key walk needed an explicit nesting bound. Build-output ignore and LF rules
were also added for the new ecosystem. No known unresolved high/medium issue exists on
an enabled path.

One non-product Rust fixture process was run for explicit success and denial smoke tests.
No real Provider or API key, Shell, LocalRunner, Docker, hook, attack traffic, product
analyzer process, installer, registry mutation, Run event, SQLite write, or Artifact
commit was used.
