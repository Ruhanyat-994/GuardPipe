import { create } from 'zustand'

/**
 * Light / dark / system theme mode
 * (documentation/09-ui-ux-design-system.md §2.10). "System" is not a
 * fourth visual design — it always resolves to `light` or `dark` at
 * runtime by reading `prefers-color-scheme`, and stays live-synced to it
 * for as long as the user hasn't picked an explicit mode.
 *
 * The actual "no flash of the wrong theme on load" work happens in
 * `index.html`'s inline script, which runs before this module (before
 * React even mounts) and applies the same class this store computes —
 * this store is what the UI (the `ThemeSubmenu`, §4.4) reads and writes
 * after that.
 */

export type ThemeMode = 'light' | 'dark' | 'system'
export type ResolvedTheme = 'light' | 'dark'

const STORAGE_KEY = 'guardpipe-theme'

function systemPrefersDark(): boolean {
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

function resolve(mode: ThemeMode): ResolvedTheme {
  return mode === 'system' ? (systemPrefersDark() ? 'dark' : 'light') : mode
}

function applyTheme(resolved: ResolvedTheme): void {
  document.documentElement.classList.toggle('dark', resolved === 'dark')
}

function loadStoredMode(): ThemeMode {
  const stored = localStorage.getItem(STORAGE_KEY)
  return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system'
}

interface ThemeState {
  mode: ThemeMode
  resolvedTheme: ResolvedTheme
  setMode: (mode: ThemeMode) => void
}

const initialMode = loadStoredMode()
// Idempotent with the inline boot script in index.html — safe to re-apply,
// and this is what keeps the DOM class and this store's state from ever
// disagreeing.
applyTheme(resolve(initialMode))

export const useThemeStore = create<ThemeState>((set) => ({
  mode: initialMode,
  resolvedTheme: resolve(initialMode),
  setMode: (mode) => {
    localStorage.setItem(STORAGE_KEY, mode)
    const resolved = resolve(mode)
    applyTheme(resolved)
    set({ mode, resolvedTheme: resolved })
  },
}))

// Live-follow the OS theme while mode is "system" — changing the OS theme
// with GuardPipe open updates the app immediately, no reload
// (documentation/09-ui-ux-design-system.md §2.10).
window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
  if (useThemeStore.getState().mode !== 'system') return
  const resolved = resolve('system')
  applyTheme(resolved)
  useThemeStore.setState({ resolvedTheme: resolved })
})
