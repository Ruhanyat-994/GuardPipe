import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Building2,
  Check,
  ChevronRight,
  LogOut,
  Monitor,
  Moon,
  Settings,
  ShieldCheck,
  Sun,
  User,
} from 'lucide-react'
import { Popover } from './ui/Popover'
import { cn } from '../lib/cn'
import { ApiError } from '../lib/apiClient'
import { listMemberOrgs, type MemberOrg } from '../lib/organizationApi'
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
  const switchOrg = useAuthStore((s) => s.switchOrg)
  const navigate = useNavigate()
  const mode = useThemeStore((s) => s.mode)
  const setMode = useThemeStore((s) => s.setMode)
  const [themeOpen, setThemeOpen] = useState(false)

  // BUILD_GUIDE.md Phase 15 — the org-switcher submenu, same nested-row
  // pattern as Theme above. orgsLoaded gates the fetch to once per popover
  // open rather than once per render; a member of only their own home org
  // (the common case, no invites yet) still shows a one-row "your org
  // (current)" list rather than an empty state, so switching is
  // discoverable even before it does anything useful.
  const [orgSwitcherOpen, setOrgSwitcherOpen] = useState(false)
  const [orgs, setOrgs] = useState<MemberOrg[]>([])
  const [orgsLoaded, setOrgsLoaded] = useState(false)
  const [switching, setSwitching] = useState<string | null>(null)
  const [orgError, setOrgError] = useState<string | null>(null)

  useEffect(() => {
    if (!orgSwitcherOpen || orgsLoaded) return
    listMemberOrgs()
      .then((res) => setOrgs(res.data))
      .catch(() => setOrgError('Could not load your organizations.'))
      .finally(() => setOrgsLoaded(true))
  }, [orgSwitcherOpen, orgsLoaded])

  async function handleSwitchOrg(orgId: string, close: () => void) {
    setSwitching(orgId)
    setOrgError(null)
    try {
      await switchOrg(orgId)
      close()
      navigate('/dashboard')
    } catch (err) {
      setOrgError(err instanceof ApiError ? err.problem.detail : 'Could not switch organizations.')
    } finally {
      setSwitching(null)
    }
  }

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
              onClick={() => setOrgSwitcherOpen((o) => !o)}
              aria-expanded={orgSwitcherOpen}
              className="flex w-full items-center justify-between gap-2.5 px-3 py-2 text-left text-body-sm text-text-primary hover:bg-bg-subtle"
            >
              <span className="flex items-center gap-2.5">
                <Building2 className="h-4 w-4" aria-hidden="true" />
                Switch organization
              </span>
              <ChevronRight
                className={cn('h-4 w-4 transition-transform', orgSwitcherOpen && 'rotate-90')}
                aria-hidden="true"
              />
            </button>
            {orgSwitcherOpen && (
              <div
                role="menu"
                aria-label="Organizations"
                className="ml-3 border-l border-border-default pl-2"
              >
                {!orgsLoaded ? (
                  <p className="px-2 py-2 text-caption text-text-tertiary">Loading…</p>
                ) : orgError ? (
                  <p role="alert" className="px-2 py-2 text-caption text-danger">
                    {orgError}
                  </p>
                ) : (
                  orgs.map((org) => (
                    <button
                      key={org.org_id}
                      type="button"
                      role="menuitemradio"
                      disabled={switching !== null}
                      onClick={() => void handleSwitchOrg(org.org_id, close)}
                      className="flex w-full items-center gap-2.5 rounded-md px-2 py-2 text-left text-body-sm text-text-primary hover:bg-bg-subtle disabled:opacity-60"
                    >
                      <span className="min-w-0 flex-1 truncate">
                        {org.name}
                        {org.is_home && (
                          <span className="ml-1.5 text-caption text-text-tertiary">(home)</span>
                        )}
                      </span>
                      {switching === org.org_id && (
                        <span
                          className="h-3 w-3 shrink-0 animate-spin rounded-full border-2 border-current border-t-transparent"
                          aria-hidden="true"
                        />
                      )}
                    </button>
                  ))
                )}
              </div>
            )}
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

          {/* Platform Admin (BUILD_GUIDE.md Phase 14) — rendered only for an
              actual platform operator; a non-operator (including an org's
              own role: 'admin') never sees this entry exists at all, not
              just a 403 clicking it. */}
          {user?.isPlatformOperator && (
            <div className="border-t border-border-default py-1">
              <button
                type="button"
                role="menuitem"
                onClick={() => {
                  close()
                  navigate('/admin/organizations')
                }}
                className="flex w-full items-center gap-2.5 px-3 py-2 text-left text-body-sm text-text-primary hover:bg-bg-subtle"
              >
                <ShieldCheck className="h-4 w-4 text-warning" aria-hidden="true" />
                Platform Admin
              </button>
            </div>
          )}

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
