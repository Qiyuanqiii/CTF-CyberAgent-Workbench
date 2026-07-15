# ADR 0022: Retained Runtime-Input Resource Lifecycle

## Status

Accepted for schema v62.

## Context

Schema v61 deliberately leaves one fully inspected, never-started target and
its deterministic read-only input volumes in Docker. A process lifecycle must
not assume those objects still exist or still belong to the Run after a restart.
Operators also need a recoverable way to remove exact-owned residue without a
generic Docker delete capability.

Resource observation and deletion are separate authorities. A prior successful
inspection cannot make a later delete safe by itself because daemon state may
change between operations. Neither operation is permission to start the target.

## Decision

Schema v62 introduces a transient
`sandbox_docker_runtime_input_resource_descriptor.v1` reconstructed from the
current completed v61 result, its v60 projection, the resupplied Manifest, and
the revalidated v48-v61 authority chain. Descriptor reconstruction recompiles
the exact container specification and resolves the reviewed writable output
mount, but does not probe or recapture the input bundle.

An explicitly confirmed read-only inspector uses the fixed local Unix Docker
endpoint to inspect the target and every deterministic volume. It records one
immutable metadata-only result as exact-owned and complete, partial/absent, or
unsafe because of a foreign or changed resource. Never-started, read-only, and
`NoCopy` evidence is true only when the exact target and every volume are all
present. An unsafe inspection is persisted for audit and returned as a failed
precondition. Inspection grants no write authority.

Cleanup requires a cleanup-eligible inspection, a separate operator cleanup
confirmation, and daemon-write confirmation. Before any DELETE, Go commits an
immutable cleanup intent and active generation lease. The transport then
re-inspects the target and every volume. If any object is foreign or changed,
it returns without issuing a DELETE. Otherwise it removes the target by its
inspected container ID, removes only exact-owned deterministic volumes with
non-forced deletion, and performs a final full inspection requiring every
resource to be absent.

Failures append only bounded typed codes and release the lease. A later owner
may acquire a new generation after release or expiry; stale generations cannot
record failure or completion. A completed operation or completed resume is a
metadata-only replay and does not contact Docker.

Lease rows cannot be deleted, and their only accepted update paths are release
or generation increment after release/expiry. Failure and result timestamps
must fall within the matching active lease window and cannot be in the future.

## Security Boundary

The transport exposes only exact container/volume GET and DELETE operations on
the fixed local Unix endpoint. It has no create, start, exec, attach, logs,
archive, pull, build, export, network mutation, forced volume deletion, or
caller-selected request path. Windows returns an explicit unsupported error and
does not fall back to host execution.

Tables, events, and CLI output contain bounded identities, digests, counts,
states, generations, and false authority bits. They do not retain Manifest
bodies, resource names, container IDs, host paths, socket paths, raw operation
keys, or private lease identities.

Cleanup success means only `exact_owned_resources_absent`. It is not process
isolation, backend availability, execution evidence, output-export authority,
or Artifact-commit authority. Container start remains absent.

## Recovery And Validation

Go constructors, Application revalidation, Store transactions, and SQLite
constraints independently bind inspection and cleanup to the current v61
application result. Migration creates no inspection or cleanup fact for older
rows. Tests cover confirmation-before-contact, semantic replay, write-ahead
ordering, failure and takeover, stale-worker fencing, SQL immutability,
metadata privacy, partial and absent resources, foreign-collision zero-delete,
target-before-volume deletion, final absence, endpoint allowlists, Windows
unsupported behavior, lease deletion/timestamp tampering, and v61-to-v62
migration.

The opt-in Linux real-daemon harness now extends the v57/v59/v60/v61 chain
through v62 inspection and cleanup. It still requires an already-present exact
image and never pulls or starts a container.

## Consequences

- Retained v61 resources can be audited after restart and safely reclaimed.
- A foreign collision is durable evidence but never cleanup authority.
- Crash recovery converges through a separate cleanup generation lease.
- A later start/wait/TERM/KILL/orphan lifecycle remains an independent design
  and release gate.
