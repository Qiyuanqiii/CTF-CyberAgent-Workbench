# ADR 0018: Durable Pre-Stage Host-Input Requirement

## Status

Accepted for schema v58.

## Context

Schema v57 prepares a descriptor-pinned, kernel-sealed host-input bundle after the
v56 stopped-container stage. The v57 staging intent then prevents completion until
the bundle evidence is durable. A narrow recovery gap remains between those two
checkpoints: if the process exits after v56 stage but before the v57 intent, a
resume that omits the staging flags can complete the optional v56-only rehearsal.
This cannot start a container or widen authority, but it loses the operator's
original staging choice.

The original roadmap also proposed handing the sealed archive directly to the
stopped container in schema v58. [Docker Engine API v1.40](https://docs.docker.com/reference/api/engine/version/v1.40/)
rejects archive extraction
when the destination volume or container root filesystem is read-only. Relaxing
`ReadonlyRootfs`, writing through the existing output bind, or retaining a host
temporary file would violate the current sandbox boundary. Therefore selection
durability and daemon handoff are separate gates.

## Decision

Schema v58 adds `sandbox_docker_host_input_requirement.v1` and persists exactly one
requirement in the same SQLite transaction that creates a new v56 attempt and its
first lease. The record fixes:

- the attempt, plan, Run, Mission, Workspace, requester, and operation digest;
- the attempt, request, Manifest, mount, input, authority, and plan fingerprints;
- the read-only mount and input Artifact counts;
- whether host-input staging is required and whether the operator confirmed it.

`required` and `operator_confirmed` must be equal. Required staging needs at least
one read-only mount; an explicit false requirement remains valid for a zero-input
plan. Row identity and timestamps are excluded from the semantic fingerprint so
independent candidates converge on the same durable fact.

Application recovery and operation-key replay derive the effective choice from the
durable requirement. A true requirement cannot be omitted to downgrade the attempt,
and a false requirement cannot be widened later. Legacy v57 attempts have no
fabricated requirement and retain their prior conservative behavior. During the
v58 migration, their existing attempt IDs are copied into a separate immutable
compatibility set before its insert trigger is installed. The marker records no
choice and cannot be added, changed, or removed after migration.

Go and SQLite both prevent a required attempt from completing without v57 staging
evidence. SQLite also rejects v57 staging for a durable false requirement. Every
post-migration stage, staging intent, and completion must reference either one
durable requirement or one migration-created legacy marker, so a direct writer
cannot manufacture a new requirement-free v58 attempt. The requirement, marker,
event, and CLI projections contain metadata only. They do not store host paths,
file content, descriptors, raw container or volume identities, socket paths, raw
operation keys, or private lease identities.

## Deferred Daemon Handoff

Schema v58 does not add archive, volume, start, exec, pull, build, or export
endpoints. `daemon_consumed=false` and `execution_evidence=false` remain true for
v57 evidence.

The next separately reviewed gate is schema v59. Its candidate design must use a
dedicated daemon-managed input volume and a never-started carrier container, with:

1. a write-ahead handoff intent bound to the v58 requirement and v57 bundle digest;
2. one fixed archive destination and exact endpoint/method/query/media-type checks;
3. bounded upload from the still-sealed handle followed by daemon archive readback
   and canonical digest verification;
4. removal of the writable carrier and recreation of a never-started target that
   mounts the same volume read-only;
5. exact label/configuration/volume reconciliation, generation fencing, and
   idempotent container plus volume cleanup;
6. no network, environment, secrets, process start, output export, or Artifact
   commit authority.

The v59 implementation must prove Docker-version behavior with an opt-in Linux
daemon test before any production claim is recorded.

## Consequences

- The v57 post-stage/pre-intent downgrade window is closed for all new attempts.
- Recovery no longer depends on resubmitting the staging flags for a v58 attempt.
- Existing v57 databases upgrade without inventing historical operator intent.
- A closed immutable legacy set preserves those attempts without becoming a
  post-migration bypass for new attempts.
- Daemon consumption remains intentionally unimplemented until a writable carrier
  can be isolated from the final read-only input mount.
