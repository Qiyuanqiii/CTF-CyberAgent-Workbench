# ADR 0063: Inert Analyzer Registry, ZIP Inventory, And Shared Vectors

- Status: Accepted
- Date: 2026-07-21
- Scope: P10-B1, P10-B2, and P10-B3 on schema v84

## Context

ADR 0062 fixed the Go-owned analyzer request/result/error protocol and introduced a
development-only Rust metadata fixture. The next analyzer must exercise a useful binary
format without creating a product process bridge, accepting host paths, decompressing
untrusted data, or allowing Rust to become a second control plane.

Archive inspection is a suitable first deterministic domain because central-directory
metadata can expose traversal, duplicate-name, declared-size, and compression-ratio risk
without reading entry bodies. The contract still needs hard output/allocation limits and
byte-exact Go/Rust compatibility before any Run or Artifact integration is considered.

## Decision

### P10-B1: Go owns an inert descriptor Registry

`internal/analyzer` defines `analyzer_descriptor.v1` and a fixed `BuiltinRegistry` with
exactly two descriptors:

- `fixture.digest.v1` -> `analyzer_result.v1`, accepting any syntactically valid media
  type through the explicit `*/*` descriptor marker;
- `archive.zip.inventory.v1` -> `archive.inventory.v1`, accepting only
  `application/zip`.

Registry listing is analyzer-ID sorted, lookups return cloned media-type slices, and no
registration or mutation API exists. Strict descriptor decoding rejects unknown,
duplicate, missing, future, or widened fields. Every filesystem, network, subprocess,
environment, product-invocation, process-start, file-input, and Artifact-commit value is
fixed false. The descriptor type deliberately has no executable, command, path, URL,
argument, or starter field.

### P10-B2: ZIP inventory is central-directory-only metadata

Go defines and strictly validates `archive.inventory.v1`. It accepts only the existing
canonical Base64 inline request, globally bounded to 64 KiB decoded input and 16 KiB
output. The ZIP-specific limits are:

- at most 32 entries;
- at most 128 UTF-8 bytes per non-empty/control-free name;
- at most 2,048 aggregate name bytes;
- 8 MiB declared uncompressed bytes per entry as a risk threshold;
- 32 MiB declared aggregate uncompressed bytes as a risk threshold;
- 100,000 ratio-milli, or 100:1, as a compression-risk threshold;
- 1,000,000,000 ratio-milli as the deterministic reported-value ceiling.

The implementation uses `archive/zip.NewReader` only to obtain central-directory
`FileHeader` metadata and never calls `File.Open`. It does not decompress, extract, or
write. Integer ratio calculation uses overflow-safe multiplication/division, declared
size totals saturate at `uint64` maximum, and no declared size controls an allocation.

Each entry carries its input order, name, deterministic `file|directory` kind, compressed
and uncompressed declarations, ratio-milli, lowercase declared CRC-32, and sorted risk codes.
The eight versioned risks are `absolute_path`, `backslash_separator`,
`compression_ratio`, `declared_entry_size`, `declared_total_size`,
`directory_has_data`, `duplicate_name`, and `parent_traversal`. The result also fixes
`metadata_only=true`, `central_directory_only=true`, `entry_contents_read=false`,
`extraction_performed=false`, and every capability-used bit false.

Go result decoding does not trust those claims. It rejects unknown, duplicate, missing,
oversized, or future JSON and recomputes every index, count, total, ratio, kind, risk,
risk count, limit, and false semantic before accepting the result.

### P10-B3: Rust mirrors the pure function, not product authority

The Rust fixture pins `rawzip = 0.5.1`. Its locked package metadata declares MIT, its
crate source uses `#![forbid(unsafe_code)]`, and `cargo tree` shows no dependency beneath
`rawzip`. Rust parses `ZipArchive::from_slice`, checks the EOCD entry hint before bounded
allocation, iterates only `entries()` central records, and never requests an entry body.
It imports no filesystem, network, process, environment, model, Store, Run, or Artifact
API.

`analyzers/testdata/archive_inventory_v1_vectors.json` pins the descriptor/request/result
protocol IDs, all archive limits and risk codes, five exact Base64 ZIP inputs, semantic
JSON, output byte counts, and SHA-256 values:

- benign inventory: 1,036 bytes,
  `f690bdc6b23dbb6da759fd16dd5f5dbe4d47cab5cc9288d7fe15a223945d2c22`;
- parent traversal: 906 bytes,
  `ce3792a2ec6590cb6412ce6c8ee8f7d13269ddb12a4cc4ced12162ea6dae15af`;
- duplicate name: 1,063 bytes,
  `46e0e7bfcdb39c9ef9d291c039da7bb3a4ac49ce2dba0802c9b6daf45037d99c`;
- oversized declaration: 977 bytes,
  `6942aedffb50c36a6c614b0899b21f5303929e09d43bf6e08e0aed211bcf3e2b`;
- compression ratio: 927 bytes,
  `a0f0d0d5376b5f951d46c0e8a85c89fd74f8fb0f211a18ed3d5d4f398887ff14`.

Go and Rust load the file independently; neither test invokes the other implementation.
The Go CI compatibility step now names both shared-vector suites.

## Consequences

- Go remains the sole control plane and protocol authority.
- Schema remains v84; OpenAPI remains 75 paths, 83 operations, and 182 schemas.
- TypeScript and Desktop gain no analyzer surface or authority.
- The only accepted content source is bounded inline Base64. Paths, URLs, commands, raw
  environment values, and API keys remain absent.
- A suspicious archive produces metadata risks only. It is never extracted or treated as
  permission to invoke another tool.
- There is still no Go-to-Rust product transport, `os/exec`, Run/Event/SQLite binding,
  result persistence, Artifact candidate/commit, CLI/HTTP/Desktop invocation, or process
  lifecycle evidence.
- The next batch may define only a disabled/fake transport and fault contract. A real
  subprocess adapter remains a separate approval, timeout, cancellation, kill/orphan,
  executable-identity, stdout/stderr-limit, and Artifact-transaction gate.

## Verification And Review

The cumulative six-slice gate passed the 380.2/401.5-second uncached ordinary/race Go
suites, twenty additional analyzer race repetitions, about 12.99 million protocol fuzz evaluations, full vet,
zero-warning staticcheck, govulncheck with zero reachable findings, module verify/tidy,
seven Rust unit tests, two independent shared-vector integration tests, Rust fmt and
zero-warning clippy, 37 frontend files and 134 tests, strict TypeScript, deterministic
OpenAPI generation, Vite build, zero npm vulnerabilities, secure Desktop tests/vet, and
a reproducible Windows dual build. The unsigned GUI SHA-256 is
`871c6270de44f3d6aecd31064127cdbfb400c5d6e6936e44698bcc30b0c611db` and remains
`release_ready=false`.

RustSec loaded 1,166 official advisories and scanned 42 locked crate dependencies with
zero known vulnerabilities. Go/npm audits also passed, and `rawzip` adds no transitive
crate.

Review fixed five low-risk classes before delivery: empty JSON risk arrays had to remain
non-null through strict recomputation; hostile `uint64` declarations needed explicit
saturating-total/capped-ratio parity tests and invalid-name classification; a Rust refactor
left needless borrows that violated the zero-warning gate; a deterministic ZIP test used
a deprecated timestamp helper; and the unverified central-directory checksum was renamed
from `crc32` to `declared_crc32` so later callers cannot mistake it for verified content.
No known
unresolved high/medium issue exists on an enabled path.

No real Provider or API key, Shell, LocalRunner, Docker, hook, attack traffic, installer,
registry mutation, product analyzer process, Run event, SQLite write, or Artifact commit
was used.
