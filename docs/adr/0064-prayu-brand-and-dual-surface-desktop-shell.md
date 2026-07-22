# ADR 0064: Prayu Brand And Dual-Surface Desktop Shell

- Status: Accepted
- Date: 2026-07-21
- Scope: non-schema Desktop/Web presentation and user-visible product identity

## Context

The Windows shell already projected the Go-owned Run runtime, but its visual hierarchy
still resembled an operations dashboard and its user-facing name remained CyberAgent
Workbench. The product is now named Prayu. The supplied concept also distinguishes the
dark workspace shell from a separate Settings surface and gives selected navigation rows
a specific orange brush treatment.

A visible rename must not silently invalidate existing scripts, databases, provider
credentials, API clients, or Windows integration. Likewise, a richer Settings surface
must not turn TypeScript into a second control plane.

## Decision

1. `internal/buildinfo.ProductName` is the Go-owned user-visible product name. CLI help
   and version labels, TUI/Desktop titles, generated workspace text, Agent prompts, and
   React shell text use Prayu.
2. Stable compatibility identifiers remain unchanged: the `cyberagent` executable,
   `cyberagent-workbench` Go module, `.cyberagent-workbench` data directory,
   `CYBERAGENT_*` environment variables, HTTP compatibility headers, credential targets,
   current OpenAPI/SARIF identifiers, and `CyberAgentWorkbench` Windows class name.
3. The workspace uses the exact user-supplied background and wordmark. Settings uses a
   distinct approved background. These are repository assets, not runtime downloads. The
   original active-brush PNG decision is superseded by ADR 0070's CSS-generated state.
4. Selected task, Run, and Settings rows share one visual state: warm dark base, right-side
   orange brush, orange icon/accent, and cream text. The main work surface is cream with
   90% opacity so the supplied background remains visible without reducing readability.
5. Settings navigation is functional and reports bounded API, schema, product-version,
   surface, and capability facts already returned by Go. The only browser persistence is
   `prayu.ui-density`, a display-only comfortable/compact preference.
6. The shell supports desktop and mobile layouts. Fixed rails become stacked regions on
   narrow screens, selected rows remain fully visible, and top-level horizontal overflow
   is forbidden.

## Security And Authority

This decision adds no migration, HTTP route, OpenAPI operation, credential body, host
path, model call, tool call, Shell, Docker, LocalRunner, subprocess, network, approval,
Policy decision, or execution lease. Settings cannot enable a Go capability and does not
derive authorization from display data. Product execution and Rust analyzer invocation
remain closed.

The schema and generated API contract remain v84 and 75 paths / 83 operations / 182
schemas. The Prayu rename is intentionally presentation-first; a future compatibility-ID
migration would require its own versioned plan and data/credential migration tests.

## Verification

Focused Go package tests passed for build information, Agent/session prompts, workspace,
application, TUI, and Desktop application wiring. Focused React tests cover the Prayu
shell, Settings navigation/density, selected resource rows, callbacks, and runtime facts;
strict TypeScript also passes.

The integrated gate passed 400.5 seconds of uncached `go test -count=1 ./...`, full
`go vet ./...`, 38 files/137 React tests, deterministic OpenAPI regeneration, strict
TypeScript, Vite production build, a zero-vulnerability npm audit, 7+2 locked Rust tests,
Rust fmt/clippy with warnings denied, secure Desktop tests/vet, and a reproducible Windows
dual build. The unsigned Desktop SHA-256 is
`0b294a9759e216c918775f05710148da6d45cde0e4e443e773894ecad6801a9b` and release
readiness remains false. The initial dependency audit rejected transitive `js-yaml 4.2.0`;
a narrow npm override to fixed 4.3.0 then passed API generation, all Web tests, build, and
the zero-vulnerability re-audit.

The combined review also found that a denied browser storage backend could throw while
reading or writing the display-only density preference. The UI now catches both paths,
falls back to comfortable density, and has a dedicated regression test.

In-app-browser visual checks used an in-memory read-only API fixture at 1440x900 and
390x844. They confirmed distinct workspace/Settings backgrounds, the original selected-row
treatment, the cream translucent work surface, fully visible selected mobile
rows, and no top-level horizontal overflow. The fixture did not call a Provider, tool,
Runner, network target, or product execution path.

## Consequences

- Prayu now has a coherent first-party interface without breaking established local data
  and automation identifiers.
- The visual shell can evolve independently from the Go control plane, but runtime facts
  and mutations must continue to come from explicit Go contracts.
- Existing users may still see CyberAgent names in paths, environment variables, headers,
  schema/API compatibility metadata, or credential records. Documentation must identify
  those names as compatibility contracts rather than unfinished duplicate branding.
- Product progress percentages do not increase solely because of this visual batch.
