# ADR 0070: Frameless Workbench, Resizable Sidebar, And Agent Composer

- Status: Accepted
- Date: 2026-07-22
- Scope: non-schema Windows Desktop/Web presentation and existing-control composition

## Context

The Prayu shell had the correct identity and dual backgrounds, but still exposed a native
Windows title strip, a fixed oversized sidebar, and a minimal prompt box. The selected-row
prototype also depended on a cropped bitmap from the concept image, which made the result
look pasted rather than implemented. A richer composer must not turn React into a second
control plane or claim unsupported Provider settings.

## Decision

1. The Wails Windows development surface is frameless. React renders one compact titlebar;
   minimize, maximize/restore, and close call the existing Wails runtime when available and
   fail softly in browser tests.
2. The sidebar defaults to 286 px and is clamped to 232-420 px. Pointer drag, keyboard
   adjustment, and double-click reset share the same clamp. Only this presentation value is
   stored locally. The resize target is transparent and draws no orange divider.
3. Selected navigation rows use CSS gradients and clipping to create the orange brush. The
   cropped active-brush bitmap is removed; concept screenshots are references, not UI layers.
4. New Run, Run delivery, and Session chat share one Agent composer. The add menu exposes
   existing workspace attachment, target mode, Plan mode, and installed-Skill paths.
5. Model availability is queried lazily from the Go API and route changes use the existing
   controlled mutation. The renderer does not invent model names or retain credentials.
6. The context ring displays conservative Go-owned context limits. Because the current
   Provider contract has no `reasoning_effort`, Standard is the only enabled reasoning
   choice; High and Max remain visibly disabled until a versioned Go contract exists.
7. New-task text prefills the existing controlled Run-creation dialog. It does not create a
   Run, attach a workspace, or change a route without the existing Go boundary.

## Security And Authority

This decision adds no migration, HTTP route, OpenAPI operation, credential access, Policy
decision, Tool capability, Shell, LocalRunner, Docker, browser process, filesystem authority,
network authority, approval, persistence authority, installer, registry write, updater, or
startup behavior. Window controls affect only the current Prayu Wails window. TypeScript
continues to consume Go-owned facts and controlled mutations.

Schema/OpenAPI remain v84 and 75 paths / 83 operations / 182 schemas.

## Verification

Strict TypeScript, Vite production build, 41 files/143 React tests, zero-vulnerability npm
audit, full `go test ./...`, Desktop-tag tests, full `go vet ./...`, and `git diff --check`
pass. The reproducible unsigned Windows development binary has SHA-256
`28ae5b21efa7746f0bd3c6646351daca6234aeeb2e85c082982e4e915b95400b` and remains
`release_ready=false`.

Visual QA at 1440x900 confirmed the frameless titlebar, cream work surface, pure-CSS active
row, add/model/reasoning/context popovers, and complete model labels. A Settings-background
opacity issue was corrected and rebuilt. The final automated click-through yielded to
concurrent physical user input; no Codex window was controlled or closed.

## Consequences

- The workbench now matches the intended interaction hierarchy without screenshot-derived
  selected controls.
- Sidebar width and composer presentation can evolve without changing runtime authority.
- High/Max reasoning remain a visible roadmap capability, not a false active setting.
- Monaco production chunks remain large and should be split in a later performance slice.
- This remains an unsigned portable development client, not an installer or formal release.
