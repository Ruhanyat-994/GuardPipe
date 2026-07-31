import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Check, ChevronRight, LogOut, Monitor, Moon, Settings, Sun, User } from 'lucide-react'
import { Popover } from './ui/Popover'
import { cn } from '../lib/cn'
import { useAuthStore } from '../stores/authStore'
import { useThemeStore, type ThemeMode } from '../stores/themeStore'

const THEME_OPTIONS: { value: ThemeMode; label: string; icon: typeof Sun }[] = [
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
  { value: 'system', label: 'System', icon: Monitor },
]

function initialsOf(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  return (parts[0][0] + (parts[1]?.[0] ?? '')).toUpperCase()
}

/**
 * Avatar → identity header → Profile/Account settings/`ThemeSubmenu`/Log
 * out (documentation/09-ui-ux-design-system.md §4.4) — the concrete home
 * for the light/dark/system requirement (§2.10), exactly Jira's own
 * pattern: a nested `Theme` row, not a top-bar sun/moon toggle button.
 */
export function UserMenu() {
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const navigate = useNavigate()
  const mode = useThemeStore((s) => s.mode)
  const setMode = useThemeStore((s) => s.setMode)
  const [themeOpen, setThemeOpen] = useState(false)

  async function handleLogout() {
    await logout()
    navigate('/login', { replace: true })
  }

  const initials = initialsOf(user?.displayName ?? user?.email ?? '?')

  return (
    <Popover
      panelClassName="w-72"
      trigger={(open, toggle) => (
        <button
          type="button"
          onClick={toggle}
          aria-haspopup="menu"
          aria-expanded={open}
          aria-label="Account menu"
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-accent text-body-sm font-semibold text-text-inverse"
        >
          {initials}
        </button>
      )}
    >
      {(close) => (
        <div className="py-1">
          <div className="flex items-center gap-3 px-3 py-3">
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-accent text-body-sm font-semibold text-text-inverse">
              {initials}
            </div>
            <div className="min-w-0">
              <div className="truncate text-body-sm font-medium text-text-primary">
                {user?.displayName}
              </div>
              <div className="truncate text-caption text-text-tertiary">{user?.email}</div>
            </div>
          </div>

          <div className="border-t border-border-default py-1">
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                close()
                navigate('/settings')
              }}
              className="flex w-full items-center gap-2.5 px-3 py-2 text-left text-body-sm text-text-primary hover:bg-bg-subtle"
            >
              <User className="h-4 w-4" aria-hidden="true" />
              Profile
            </button>
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                close()
                navigate('/settings')
              }}
              className="flex w-full items-center gap-2.5 px-3 py-2 text-left text-body-sm text-text-primary hover:bg-bg-subtle"
            >
              <Settings className="h-4 w-4" aria-hidden="true" />
              Account settings
            </button>

            <button
              type="button"
              onClick={() => setThemeOpen((o) => !o)}
              aria-expanded={themeOpen}
              className="flex w-full items-center justify-between gap-2.5 px-3 py-2 text-left text-body-sm text-text-primary hover:bg-bg-subtle"
            >
              <span className="flex items-center gap-2.5">
                <Monitor className="h-4 w-4" aria-hidden="true" />
                Theme
              </span>
              <ChevronRight
                className={cn('h-4 w-4 transition-transform', themeOpen && 'rotate-90')}
                aria-hidden="true"
              />
            </button>
            {themeOpen && (
              <div
                role="menu"
                aria-label="Theme"
                className="ml-3 border-l border-border-default pl-2"
              >
                {THEME_OPTIONS.map((opt) => (
                  <button
                    key={opt.value}
                    type="button"
                    role="menuitemradio"
                    aria-checked={mode === opt.value}
                    onClick={() => setMode(opt.value)}
                    className="flex w-full items-center gap-2.5 rounded-md px-2 py-2 text-left text-body-sm text-text-primary hover:bg-bg-subtle"
                  >
                    <opt.icon className="h-4 w-4 text-text-tertiary" aria-hidden="true" />
                    {opt.label}
                    {mode === opt.value && (
                      <Check className="ml-auto h-4 w-4 text-accent" aria-hidden="true" />
                    )}
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="border-t border-border-default py-1">
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                close()
                void handleLogout()
              }}
              className="flex w-full items-center gap-2.5 px-3 py-2 text-left text-body-sm text-text-primary hover:bg-bg-subtle"
            >
              <LogOut className="h-4 w-4" aria-hidden="true" />
              Log out
            </button>
          </div>
        </div>
      )}
    </Popover>
  )
}
