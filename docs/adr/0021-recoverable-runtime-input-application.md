# ADR 0021: Recoverable Runtime-Input Application

## Status

Accepted for schema v61.

## Context

Schema v60 deterministically compiles the exact v57/v59 sealed input into one
canonical archive per reviewed Manifest target. It deliberately has no Docker
write transport. A later process lifecycle cannot safely consume those plans
until volume creation, archive upload, readback, target attachment, collision
handling, and crash recovery are independently durable and auditable.

Applying a projection is not permission to start the target. The write path
must therefore end at a fully inspected, never-started container and retain no
process or output capability.

## Decision

Schema v61 adds a separately confirmed
`sandbox_docker_runtime_input_application_intent.v1` before any daemon write.
The immutable intent binds the current v60 projection, v59 handoff, v56
attempt, Manifest, container plan, input Artifacts, endpoint, requester, and a
domain-separated operation-key digest. Operator confirmation and daemon-write
confirmation are both required and persisted.

An independent SQLite lease owns each application generation. Active leases
cannot be stolen. A released or expired generation may be resumed by exactly
one later generation; stale workers cannot append failures or commit a result.
The Application revalidates the complete v48-v60 authority chain after the
write-ahead transaction, recompiles the exact container specification,
resolves the writable output mount again, and recaptures the v57 sealed bundle.
The recaptured report, bytes, counts, Artifact payload identity, and v60
projection compilation must all match durable evidence before transport use.

The first transport is fixed to the local Unix Docker socket and API `1.40`.
Its closed allowlist permits only exact image/container/volume inspection,
deterministic local-volume create/delete, never-started carrier/target
create/delete, and archive PUT/GET at fixed `/cyberagent-input`. It does not
read `DOCKER_HOST`, follow redirects, use TCP, pull images, or accept a caller
selected archive path.

For each projection, the transport creates a local volume and never-started
writable carrier, uploads the transient canonical archive, reads it back
through the daemon, and verifies the complete tar semantics. It then removes
the carrier. The final target is created once with the reviewed writable output
bind and every input volume attached read-only with Docker `NoCopy`. Exact
inspection must succeed before the immutable result is committed.

Deterministic names and authority labels make retry converge. Existing objects
are reconciled only when their complete request, projection, role, item, image,
mount, security, and never-started configuration match. Foreign or ambiguous
collisions fail closed and are never deleted. Failure cleanup runs under an
independent bounded context and removes only exact owned resources. Daemon work
is cut off before lease expiry with a reserved cleanup window so an old worker
cannot normally overlap a generation takeover.

## Security Boundary

The successful status is `volumes_applied_target_never_started`. The final
container and its read-only input volumes remain present for a future,
separately authorized lifecycle. Container start, process execution, output
export, backend authorization, and Artifact commit authorization remain false
in Go and SQLite. The transport has no start, exec, attach, logs, export, pull,
build, network-mutation, forced-volume-delete, or arbitrary-request endpoint.

Raw Manifest targets, host paths, file names/content, volume/carrier/container
names and IDs, archive bytes, socket paths, operation keys, and private lease
identities are transient. Tables, events, list/show/resume output, and the
TypeScript console retain only bounded metadata, digests, counts, status, and
false authority flags.

Windows returns `application_unsupported`; it does not fall back to a host
process or caller-selected Docker endpoint.

## Recovery And Validation

Intent, failure, and result records are immutable, while the lease follows
strict generation transitions. The intent's unique digest-keyed operation
binding provides idempotent replay without a second operation table.
Completion and failure writes require the current active generation. Semantic
replay returns the existing result without recapturing input or contacting
Docker. A failed generation records only a bounded typed code, releases its
lease atomically, and can be resumed.

Focused tests cover dual confirmation, write-ahead ordering, stale-generation
fencing, released-generation recovery, replay, migration without fabricated
facts, SQL immutability, metadata privacy, foreign collisions, corrupt
readback, exact cleanup, endpoint allowlists, and the lease cleanup window. A
real-daemon Linux harness remains required before any future start gate may
depend on this evidence.

## Consequences

- Reviewed runtime inputs can now be materialized into deterministic,
  daemon-owned, readback-verified, read-only volumes.
- The retained target is inspectable and resumable but deliberately cannot run.
- Schema v62 should first add explicit metadata-only inspection and bounded
  cleanup/reconciliation for retained v61 resources.
- Start/wait/TERM/KILL/orphan recovery remains a later independent state
  machine. Output export and atomic Artifact commit remain later gates.
