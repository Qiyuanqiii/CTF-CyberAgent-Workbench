# ADR 0072: Workbench Docks And Operator-Confirmed Workspace Opening

- Status: Accepted
- Date: 2026-07-23
- Scope: non-schema Desktop workbench controls and a pathless native workspace opener

## Context

Prayu needs the same compact workbench affordances users expect from a modern coding agent:
an application chooser, a read-only environment summary, a bottom tool panel, and a right
sidecar for review, files, browser, terminal, and side-task views. These controls must reuse the
existing Go APIs and must not imply that the currently blocked Shell or browser runtimes are
operational.

Opening the registered Workspace in File Explorer, an editor, or a terminal is useful, but a
generic renderer-supplied path/command bridge would create a host-process escape hatch. The
operation therefore needs a separate native, operator-owned contract instead of reusing Agent,
Runner, Tool Gateway, or Shell execution.

## Decision

1. The workbench header exposes four compact controls: an Open Workspace menu, Summary toggle,
   Bottom Panel toggle, and Right Sidecar toggle. Summary, bottom panel, and sidecar are
   independent and remain usable together at bounded desktop widths.
2. Summary is read-only and projects only existing repository, Run, Session, and Workspace
   metadata through current Go HTTP APIs. Review reuses the existing redacted repository diff;
   Files reuses the bounded Workspace Explorer; Side Tasks reuses current Run work items.
3. Browser and terminal tabs are honest inert states. The bottom panel is a layout surface only;
   it does not create xterm input, a PTY, a Shell, a subprocess, or browser/network authority.
4. The Wails bridge adds pathless `desktop_workspace_launcher_list.v1` and
   `desktop_workspace_open.v1` contracts. Renderer input is limited to a normalized registered
   Workspace ID and one fixed launcher ID. Paths, commands, environment variables, arbitrary
   arguments, and authority flags are not accepted or returned.
5. Go resolves the Workspace root from SQLite and keeps it native. The Windows implementation
   discovers only a fixed catalog of recognized applications from Known Folders and recognized
   uninstall metadata. It validates the selected executable and registered directory before and
   after the confirmation dialog.
6. A native Wails confirmation shows the exact Workspace root and executable path. Cancellation
   starts nothing. At most one confirmation can be active. File Explorer and editors receive only
   the registered root; an external terminal receives no command or argument and starts with that
   root as its working directory.
7. A successful open starts one operator-selected external application and releases its process
   handle. It grants no Agent, model, child, HTTP, Tool, Runner, or Shell authority. The existing
   `process_execution_enabled`, `shell_execution_enabled`, and browser start gates remain false.
8. Schema and OpenAPI remain v84 and 75 paths / 83 operations / 182 schemas. The new bridge
   capability is Desktop-only and independently advertised as `workspace_open_enabled`.

## Security And Authority

The native opener is a manual convenience boundary, not a sandbox. The external application may
read or modify the Workspace according to its own operating-system permissions after the operator
confirms it. Prayu does not supervise, terminate, audit the child application's later behavior, or
claim publisher/signature trust for a discovered launcher. The confirmation deliberately exposes
the executable and root so the operator can reject unexpected installation metadata.

The renderer never receives the registered host path and cannot supply a replacement path,
command line, environment, or Shell payload. Unknown launchers, malformed identities, duplicate
catalog entries, executable drift, missing directories, concurrent confirmations, and malformed
native receipts fail closed. Errors returned to the renderer omit host paths.

Review also tightened the resolver/launcher contract so an absolute but non-canonical root (for
example one retaining a `.` or `..` segment) is rejected at both the bridge and native boundary,
even though the production SQLite resolver already cleans registered roots.

## Verification

The final gate passes serial full Go tests in 512 seconds and serial full race tests in 603
seconds. Parallel full Go testing on this Windows machine exposes an existing SQLite migration
contention problem, so the authoritative local full-suite command uses `-p 1`; all packages pass
and no business deadlock is present. Full vet and staticcheck, ordinary and race Desktop tests,
secure Desktop-tag test/vet, two zero-reachable-finding govulncheck paths, module verification,
42 React test files / 148 tests, strict TypeScript, deterministic OpenAPI generation, Vite build,
and zero-vulnerability npm audits pass.

Rust fmt, 7+2 locked tests, Clippy with warnings denied, and an offline RustSec scan of 1,166
cached advisories / 42 locked crates pass. The online RustSec refresh failed because GitHub's Git
transport returned an I/O error; the cached database was used and this is not represented as a
fresh online scan. The reproducible Windows dual build passes with unsigned GUI SHA-256
`8aaf3365e3c4d2e41b6f6b6dbf75f1b580a48d24419ba288d4235a41b5549cb8` and
`release_ready=false`.

## Consequences

- The requested Codex-style toolbar and dock layout now has real state and existing read-only
  data behind it instead of screenshot placeholders.
- Users can manually open an exact registered Workspace in an available external application
  without giving the renderer or Agent a generic process bridge.
- Embedded terminal, PTY input, browser launch/navigation, publisher trust, child-process
  lifecycle ownership, and Agent-controlled host execution remain future independently audited
  work.
