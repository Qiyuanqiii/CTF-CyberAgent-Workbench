# ADR 0020: Deterministic Runtime-Input Projection Plan

## Status

Accepted for schema v60.

## Context

Schema v59 proves that Docker consumed the exact v57 sealed bundle, read the
same bytes back, mounted the temporary carrier volume read-only, and removed all
handoff resources. It deliberately does not define how a future container sees
each reviewed input at its exact Manifest target. Starting a bootstrap process
inside the container, making the reviewed root writable, or accepting a generic
archive destination would widen authority before process isolation is proven.

The runtime therefore needs a deterministic, inspectable projection plan before
any new daemon write is introduced.

## Decision

Schema v60 adds `sandbox_docker_runtime_input_projection_plan.v1` as an
immutable, operator-confirmed, metadata-only plan. Application may create it
only from a completed v59 handoff and completed v56 attempt after revalidating
the complete v48-v59 authority chain and resubmitted Manifest.

Go recaptures the v57 sealed bundle from the descriptor-safe provider. The
report fingerprint, archive digest, byte count, input counts, and Artifact
payload digest must exactly match the durable v57/v59 evidence. A strict parser
then requires the byte stream to be the canonical v57 PAX tar:

- only regular files and directories with fixed modes, ownership, and epoch
  timestamps are accepted;
- links, devices, traversal, duplicate paths, missing parents, unexpected
  roots, non-canonical PAX data, empty Artifacts, and trailing bytes fail closed;
- the parsed entries are re-encoded and compared byte-for-byte with the sealed
  input;
- every read-only Manifest source must have a directory root in the first
  profile; single-file mount roots remain unsupported by this projection gate;
- input Artifacts map to the fixed `/cyberagent-input/artifacts` target.

Each accepted source root becomes one deterministic relative tar projection.
Targets and future volume names exist only in transient memory. The volume name
derivation includes the completed handoff fingerprint, so retries converge while
identical inputs in different Runs cannot collide. Persisted items retain only
bounded counts, byte totals, digests, and target/archive/volume fingerprints.

The Store commits plan, ordered items, aggregate completion marker, operation
binding, and audit event in one Run-locked SQLite transaction. An operation key
is stored only as a domain-separated digest. Exactly one completed projection
plan may bind a handoff, and replay returns the existing immutable plan without
recapturing input.

## Security Boundary

Schema v60 has no Docker transport. It does not create a volume, upload an
archive, recreate a container, start a process, export output, or commit an
Artifact. The durable status is `compiled_not_applied`; daemon contacted,
daemon applied, container started, process executed, output exported, backend
enabled, execution authorized, and Artifact commit authorized are all fixed to
false in Go and SQLite.

Raw Manifest targets, host paths, volume names, archive roots, file names,
content, bundle bytes, container IDs, and raw operation keys are excluded from
tables, events, and CLI output. TypeScript, providers, model output, Skills, and
external documents cannot apply or widen this plan.

## Recovery And Validation

The completion table prevents partial item sets from appearing through normal
queries. Its insert trigger verifies contiguous ordinals, kind counts, and all
aggregate sums. Root, item, completion, and operation rows are immutable.
Cross-Run tests cover identical input content, and migration from v59 creates no
projection facts.

## Consequences

- The exact mapping and bytes for future read-only input volumes are now
  deterministic and independently auditable.
- Directory-root mounts are supported; a safe single-file projection design is
  deferred rather than silently changing mount semantics.
- Schema v61 must introduce a separate write-ahead, generation-fenced transport
  that applies these volumes and verifies them while the container remains
  never-started.
- Start/wait/TERM/KILL/orphan recovery, output export, and atomic production
  Artifact commit remain later independent gates.
