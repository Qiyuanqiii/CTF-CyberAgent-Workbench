# ADR 0071: Inert Browser Executable Identity, Profile Lifecycle, And Sealed CDP

- Status: Accepted
- Date: 2026-07-23
- Scope: non-schema Go browser-runtime adapters with no product execution

## Context

ADR 0069 fixed browser profiles, exact target scope, and a non-authorizing Session plan. The
next layer must identify a browser installation, reserve disposable state, and describe CDP
results without accidentally turning metadata into launch authority. PATH lookup, `--version`
subprocesses, personal Chrome profiles, model-selected directories, generic recursive cleanup,
and an externally implemented transport would each weaken that boundary.

An executable digest is also not proof that a file is a trusted browser. Publisher signature,
open-handle launch revalidation, process-tree ownership, durable approval, and sandbox evidence
remain separate gates.

## Decision

1. B1 discovers Edge, Chrome, and Chromium only below Windows Known Folder installation roots
   and fixed Go-owned relative paths. It never searches PATH or executes a candidate. Existing
   candidates must be non-link regular PE files below the admitted root. Identity binds product,
   channel, canonical path, relative path, host/target architecture, exact byte count, SHA-256,
   and read-only Windows file-version metadata when available. The path is checked again after
   hashing and version inspection.
2. Executable identity explicitly records publisher-signature verification and launch trust as
   false. The canonical path is not approved for persistence, and all runtime authority remains
   false. P11-C must perform stronger handle-bound and publisher-policy checks before launch.
3. B2 derives one directory name from the Session Plan's 64-character profile token under an
   exact `browser-profiles` root. Planning does not create, read, rename, write, or delete that
   directory. Observations are limited to absent, exact active/stale/released owner, foreign
   owner, or corrupt marker metadata.
4. Exact stale ownership can advance only from generation N to N+1. The directory path remains
   fixed, but owner token, marker digest, and ancestry fingerprint change. Old generations are
   then treated as foreign. Only an exact released current owner can produce a cleanup candidate;
   the candidate remains delete-blocked and forbids wildcards and model-owned cleanup.
5. B3 exposes a package-sealed CDP transport. Production builds admit only an inert Disabled
   transport and a deterministic in-memory Fake transport. The bridge revalidates Session,
   executable, ownership, exact URL scope, action limits, cancellation, and deadline.
6. Navigation, DOM snapshot, PNG/JPEG screenshot, and request-capture contracts return metadata
   only: canonical URL, media type, byte count, SHA-256, and bounded request count. Raw DOM,
   pixels, headers, cookies, bodies, and request records are absent. Fake outcomes are explicitly
   synthetic. Process, network, profile-write, mutation, replay, Artifact, and product-execution
   facts remain false.
7. CDP outcome JSON is bounded, strict, and canonical. Unknown, missing, duplicate, reordered,
   or trailing schema data fails closed. No CLI, HTTP, Desktop, Store, Event, or Artifact adapter
   is added by this decision.

## Security And Authority

This batch does not launch a browser or any other process, connect to a target or proxy, resolve
DNS, create a profile root, write an owner marker, delete a directory, read a credential, mutate
or replay a request, or commit an Artifact. Wails WebView2 remains only the Prayu renderer.

The executable identity is discovery evidence, not an allowlist or TOCTOU-safe launch handle.
The cleanup plan is an exact candidate, not deletion permission. A Fake CDP success is synthetic
contract evidence, not proof of browser, process, network, DOM, screenshot, or capture activity.
Schema/OpenAPI remain v84 and 75 paths / 83 operations / 182 schemas.

## Verification

Focused `browserruntime` tests, race tests, vet, staticcheck, source-capability scans, mutation
tests, cancellation/deadline tests, byte/path drift tests, recovery-generation fencing, and
canonical codec tests pass. Package statement coverage is 77.8%.

The integrated gate passes `go test ./... -count=1` in 378.0 seconds, full `go vet ./...`, 41
React test files / 143 tests, strict TypeScript, Vite production build, zero-vulnerability npm
audit, and patch hygiene. No real browser, process, network, profile directory, credential,
Docker, Shell, Provider, Store/Event, or Artifact path ran.

## Consequences

- Prayu can now describe installed-browser identity, disposable-profile ownership, recovery,
  cleanup, and CDP result shapes without claiming an operational built-in browser.
- The next batch must close publisher trust, handle-bound revalidation, durable launch ownership,
  process-tree cleanup, and operator review before any real browser start is considered.
- Real Safe Web navigation, DOM, screenshots, CTF interception/mutation/replay, and instrumented
  security relaxation remain unavailable.
