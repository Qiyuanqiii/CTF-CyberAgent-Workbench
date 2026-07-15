# ADR 0019: Daemon-Owned Host-Input Handoff

## Status

Accepted for schema v59.

## Context

Schema v57 captures read-only workspace inputs into a descriptor-pinned,
kernel-sealed in-memory archive. Schema v58 makes that capture requirement
immutable before daemon stage. Neither milestone proves that Docker received the
same bytes.

Docker Engine archive extraction writes into a container filesystem. The Engine
API rejects that operation when the destination is a read-only root filesystem
or read-only volume. Making the target writable would weaken the reviewed v54
authority. The handoff therefore needs a separate writable carrier whose only
purpose is to populate a daemon-owned volume. Docker API behavior is pinned to
[Engine API v1.40](https://docs.docker.com/reference/api/engine/version/v1.40/).

## Decision

Schema v59 adds three immutable facts:

1. `sandbox_docker_host_input_handoff_requirement.v1` is created in the same
   transaction as every new rehearsal attempt and its first lease. A required
   handoff also requires the v58 capture fact.
2. `sandbox_docker_host_input_handoff_intent.v1` commits before archive or volume
   mutation and binds the attempt, stopped-container fingerprint, v57 staging
   result, current lease generation, plan, requester, and authority fingerprints.
3. `sandbox_docker_host_input_handoff.v1` records only a completed, readback-
   verified, fully cleaned, never-started result. Required attempts cannot record
   cleanup or completion before this result exists.

The Linux transport remains default-disabled and is selected only after separate
daemon-write, descriptor-staging, and handoff confirmations. It accepts only the
fixed local Unix endpoint and Docker API `1.40`. Its operation set is closed to:

- exact image and container inspection;
- deterministic local-volume create, inspect, and non-forced delete;
- deterministic never-started carrier and target create/inspect/delete;
- archive PUT to `/cyberagent-input` and archive GET for the fixed
  `/cyberagent-input/bundle.tar` file.

The transport wraps the exact v57 archive as one read-only `bundle.tar`, uploads
it through a temporary writable carrier, reads it back through Docker, and
verifies both byte length and SHA-256. It then removes the carrier and original
stopped target, recreates the target with the verified volume mounted read-only,
verifies the full container configuration, and removes both target and volume.
No container is started.

`/cyberagent-input` and its ancestor/descendant mount tree are reserved for the
Go control plane during this handoff. A Manifest mount that overlaps the reserved
tree is rejected before daemon mutation. The target read-only root and all
reviewed Manifest mount permissions remain unchanged.

## Recovery And Collision Rules

Names derive from the immutable request fingerprint. A retry may remove only a
carrier, volume, original target, or read-only target whose complete labels,
configuration, and identity match the same request. Exact carrier/volume/final-
target residue converges before a new attempt. A same-name foreign or changed
resource fails closed and is never deleted.

The failure cleanup context is independent from the caller context. It removes
only exact owned resources, including the original stopped target when failure
occurs before archive upload. The write-ahead intent remains pending, so a later
lease generation can recapture the same sealed bytes and retry without another
v55 stage create.

## Security Boundary

Schema v59 adds narrowly typed archive and local-volume operations, not generic
Docker access. It adds no start, exec, attach, logs, export, image pull/build,
network mutation, arbitrary archive path, forced volume deletion, caller-selected
endpoint, or Artifact writer. Raw paths, content, descriptors, container IDs,
carrier/volume names, socket details, operation keys, and private lease identities
are not persisted or emitted in events.

Every result fixes container start, process execution, output export, backend
enablement, production verification, execution authority, and Artifact commit
authority to false. This evidence proves only a bounded daemon handoff followed
by complete cleanup.

## Consequences

- v57 sealed bytes can now be proven as daemon-consumed and read back without
  weakening the target container.
- Restart recovery has an immutable intent and deterministic resource identity.
- The fixed volume carries `bundle.tar`; schema v59 does not yet define how a
  future running process consumes or projects its entries at Manifest targets.
- A Linux real-daemon opt-in harness is still required before any production
  start gate can rely on this behavior. Windows remains explicitly unsupported.
- The next gate must design start/wait/terminate/orphan handling and input
  projection separately, while keeping output export and Artifact commit closed
  until their own transaction is audited.
