import { type ReactNode, useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import {
  BookOpen,
  Crosshair,
  FolderKanban,
  HelpCircle,
  Menu,
  PanelLeftClose,
  Plus,
  ScanSearch,
  Settings,
  ShieldAlert,
} from 'lucide-react'
import { cn } from '../lib/cn'
import { GlobalSearch } from './GlobalSearch'
import { NotificationPanel } from './NotificationPanel'
import { UserMenu } from './UserMenu'
import { Button } from './ui/Button'

/**
 * The authenticated app's persistent chrome — dark `TopBar` + light
 * collapsible `SidebarNav` (documentation/09-ui-ux-design-system.md §2.8,
 * §4.4) — Jira-inspired: the top bar is always the dark/inverse chrome
 * colour regardless of the app's own light/dark theme mode (§2.10), a
 * constant landmark exactly like Jira's own black bar. Every authenticated
 * route renders inside this shell.
 */

const NAV_ITEMS = [
  { to: '/projects', label: 'Projects', icon: FolderKanban },
  { to: '/scans', label: 'Scans', icon: ScanSearch },
  { to: '/findings', label: 'Findings', icon: ShieldAlert },
  { to: '/targets', label: 'Targets', icon: Crosshair },
  { to: '/rules', label: 'Rules', icon: BookOpen },
] as const

export function AppShell({ children }: { children: ReactNode }) {
  const [collapsed, setCollapsed] = useState(false)
  const location = useLocation()
  const navigate = useNavigate()

  // Context-dependent primary action (§4.4's TopBar spec): a project-scoped
  // route gets the not-yet-real "New Scan" (Phase 6), the projects list
  // gets the real "New Project". Every other route (Findings/Rules/etc.
  // placeholders) has nothing to create yet, so the slot is simply empty
  // rather than showing a button that does nothing.
  const inProject = /^\/projects\/[^/]+/.test(location.pathname)
  const onProjectsList = location.pathname === '/projects'

  return (
    <div className="flex min-h-screen flex-col bg-bg-base">
      <header className="flex h-12 shrink-0 items-center gap-3 border-b border-chrome-border bg-chrome-bg px-3 text-chrome-text">
        <button
          type="button"
          onClick={() => setCollapsed((c) => !c)}
          className="rounded-md p-1.5 text-chrome-text-secondary hover:bg-chrome-hover hover:text-chrome-text"
          aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          {collapsed ? (
            <Menu className="h-5 w-5" aria-hidden="true" />
          ) : (
            <PanelLeftClose className="h-5 w-5" aria-hidden="true" />
          )}
        </button>

        <Link to="/projects" className="text-h3 font-semibold text-chrome-text">
          GuardPipe
        </Link>

        <div className="ml-2">
          <GlobalSearch />
        </div>

        <div className="ml-auto flex items-center gap-1.5">
          {onProjectsList && (
            <Button size="sm" onClick={() => navigate('/projects/new')}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              New Project
            </Button>
          )}
          {inProject && (
            <Button size="sm" disabled title="Lands with the orchestrator, Phase 6">
              <Plus className="h-4 w-4" aria-hidden="true" />
              New Scan
            </Button>
          )}

          <NotificationPanel />

          <Link
            to="/guides"
            className="rounded-md p-2 text-chrome-text-secondary hover:bg-chrome-hover hover:text-chrome-text"
            aria-label="Guides and help"
            title="Guides"
          >
            <HelpCircle className="h-5 w-5" aria-hidden="true" />
          </Link>

          <UserMenu />
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        <aside
          className={cn(
            'flex shrink-0 flex-col border-r border-border-default bg-bg-surface transition-[width]',
            collapsed ? 'w-16' : 'w-60',
          )}
          style={{ transitionDuration: 'var(--duration-base)' }}
          aria-label="Primary"
        >
          <nav className="flex flex-1 flex-col gap-1 p-3">
            {NAV_ITEMS.map(({ to, label, icon: Icon }) => (
              <NavLink
                key={to}
                to={to}
                className={({ isActive }) =>
                  cn(
                    'flex items-center gap-3 rounded-md px-3 py-2 text-body-sm font-medium transition-colors',
                    isActive
                      ? 'bg-accent/10 text-accent'
                      : 'text-text-secondary hover:bg-bg-subtle hover:text-text-primary',
                  )
                }
                style={{ transitionDuration: 'var(--duration-fast)' }}
                title={collapsed ? label : undefined}
              >
                <Icon className="h-4.5 w-4.5 shrink-0" aria-hidden="true" />
                <span className={collapsed ? 'sr-only' : undefined}>{label}</span>
              </NavLink>
            ))}

            <div className="my-2 border-t border-border-default" role="separator" />

            <NavLink
              to="/settings"
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-3 rounded-md px-3 py-2 text-body-sm font-medium transition-colors',
                  isActive
                    ? 'bg-accent/10 text-accent'
                    : 'text-text-secondary hover:bg-bg-subtle hover:text-text-primary',
                )
              }
              style={{ transitionDuration: 'var(--duration-fast)' }}
              title={collapsed ? 'Settings' : undefined}
            >
              <Settings className="h-4.5 w-4.5 shrink-0" aria-hidden="true" />
              <span className={collapsed ? 'sr-only' : undefined}>Settings</span>
            </NavLink>
          </nav>
        </aside>

        <div className="min-w-0 flex-1 overflow-y-auto">{children}</div>
      </div>
    </div>
  )
}
