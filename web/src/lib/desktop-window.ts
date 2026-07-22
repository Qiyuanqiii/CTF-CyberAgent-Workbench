interface WailsWindowRuntime {
  Quit?: () => void;
  WindowMinimise?: () => void;
  WindowToggleMaximise?: () => void;
}

declare global {
  interface Window {
    runtime?: WailsWindowRuntime;
  }
}

export function minimiseDesktopWindow(): void {
  window.runtime?.WindowMinimise?.();
}

export function toggleDesktopWindowMaximised(): void {
  window.runtime?.WindowToggleMaximise?.();
}

export function closeDesktopWindow(): void {
  window.runtime?.Quit?.();
}
