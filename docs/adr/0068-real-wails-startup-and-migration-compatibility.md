# ADR 0068: Real Wails Startup and Migration Compatibility

## Status

Accepted for the non-schema Desktop startup repair on 2026-07-22. SQLite remains at v84.

## Context

The portable Windows binary compiled reproducibly and passed its prior unit, frontend, and
build checks, but a real launch exposed two blockers that those checks did not model:

- Wails v2 converts intercepted WebView requests into server-form `http.Request` values before
  invoking the custom AssetServer handler. `URL.Scheme` and `URL.Host` are empty, while the
  verified authority remains in `Request.Host` and Wails appends the `wails.io` user-agent token.
  The previous Prayu guard expected the pre-adapter absolute URL and returned `403 Forbidden`.
- one Windows preview database had an already-recorded schema-v30 checksum that differed from
  the canonical committed v30 checksum. The database itself passed SQLite integrity checking,
  but strict migration validation prevented startup before v31-v84 could run.

Build success was therefore insufficient evidence of a usable Desktop product.

## Decision

The Desktop request boundary now accepts the exact Wails server-form request only when all of
the following hold:

- `Request.Host` is exactly `wails.localhost`, case-insensitively, with no port;
- `User-Agent` contains `wails.io` as a complete whitespace-delimited token;
- URL user info, fragment, raw fragment, and opaque data are absent; and
- URL scheme and host are either both empty (the real Wails adapter form) or both form the exact
  `http://wails.localhost` pre-adapter shape used by focused compatibility tests.

After validation, Prayu still clones the request, rebuilds `RequestURI`, clears URL authority,
and pins Host/RemoteAddr to loopback before entering the Go API. Missing markers, substring-only
user agents, wrong hosts, ports, partial authority, user info, fragments, and opaque URLs fail
closed.

Migration validation continues to require exact version, name, contiguity, and checksum. It now
accepts one additional known v30 checksum:

`bef87a078e337c7c78f020b0470b5d3c8a889a42c3f5993ef62f16e761018ae7`

The alias applies only to migration 30 and still requires the exact migration name. Unknown
checksums remain rejected. The canonical committed v30 checksum is pinned by test as
`cfd24a5af6bd9b0994984887ab568c3833f1971da8285642905c83a73ae0e0f5`.

## Verification

Regression tests reproduce the upstream Wails request transformation and all rejection cases.
A migration test creates a real v30 schema with user data, substitutes the known preview
checksum, opens it through the normal Store path, verifies upgrade to v84, and proves the data
survives. An unknown checksum test remains fail-closed.

The existing user database was first copied to
`~/.cyberagent-workbench/backups/cyberagent-v30-before-v84-20260722.db` with SHA-256
`e092efbe22665bcfa310f9d5fd984241b974daedacc9065c58261e6bf55f6970`. A separate copy upgraded
to v84 with `integrity_check=ok` before the original was opened. The rebuilt executable then
rendered the full Prayu workbench and the distinct Settings page on Windows 11. After graceful
close, the real database reported `integrity_check=ok` and schema v84.

The final local gate passed `go test ./...`, secure Desktop tests, 38 frontend files / 137 tests,
strict TypeScript, Vite production build, and reproducible dual Windows builds. The unsigned
binary SHA-256 is `91edfec6e209419e8416f28a3279ac75196c9dd6429a5942df39e7dd02f9c4b5`.

## Consequences

- A real rendered window, not compilation alone, is now required Desktop release evidence.
- Historical local data upgrades in place; Prayu never deletes or resets it to recover startup.
- The compatibility alias is deliberately narrow and cannot authorize arbitrary migration
  history.
- Windows 10 coverage, signing, and installer work remain release blockers; `release_ready`
  remains false.
