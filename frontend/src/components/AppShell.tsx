import { type ReactNode, useEffect, useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import {
  ArrowLeftCircle,
  BookOpen,
  Crosshair,
  FolderKanban,
  HelpCircle,
  LayoutGrid,
  Menu,
  PanelLeftClose,
  Plus,
  ScanSearch,
  Settings,
  ShieldAlert,
  Users,
} from 'lucide-react'
import { cn } from '../lib/cn'
import { GlobalSearch } from './GlobalSearch'
import { Logo } from './Logo'
import { NotificationPanel } from './NotificationPanel'
import { UserMenu } from './UserMenu'
import { Button } from './ui/Button'
import { getProject } from '../lib/projectsApi'
import { useAuthStore } from '../stores/authStore'
import { useIdleLogout } from '../hooks/useIdleLogout'

/**
 * The authenticated app's persistent chrome — dark `TopBar` + light
 * collapsible `SidebarNav` (documentation/09-ui-ux-design-system.md §2.8,
 * §4.4) — Jira-inspired: the top bar is always the dark/inverse chrome
 * colour regardless of the app's own light/dark theme mode (§2.10), a
 * constant landmark exactly like Jira's own black bar. Every authenticated
 * route renders inside this shell.
 */

const NAV_ITEMS = [
  { to: '/dashboard', label: 'Overview', icon: LayoutGrid },
  { to: '/projects', label: 'Projects', icon: FolderKanban },
  { to: '/scans', label: 'Scans', icon: ScanSearch },
  { to: '/findings', label: 'Findings', icon: ShieldAlert },
  { to: '/targets', label: 'Targets', icon: Crosshair },
  { to: '/rules', label: 'Rules', icon: BookOpen },
  // Team Dashboard (BUILD_GUIDE.md Phase 15) — org-wide assignment × gate-
  // verdict matrix.
  { to: '/team', label: 'Team', icon: Users },
] as const

export function AppShell({ children }: { children: ReactNode }) {
  useIdleLogout()

  const [collapsed, setCollapsed] = useState(false)
  const location = useLocation()
  const navigate = useNavigate()

  // Context-dependent primary action (§4.4's TopBar spec): a project-scoped
  // route gets "New Scan" (routes to that project's Scans tab, Phase 6),
  // the projects list gets "New Project". Every other route (Findings/
  // Rules/etc. placeholders) has nothing to create yet, so the slot is
  // simply empty rather than showing a button that does nothing.
  const projectMatch = /^\/projects\/([^/]+)/.exec(location.pathname)
  const inProject = projectMatch !== null
  const onProjectsList = location.pathname === '/projects'

  // project-collaborators follow-up — a session switched into one shared
  // project (authStore.switchProject) gets a persistent banner instead of
  // its usual org-wide nav: no Team Dashboard (org-wide), no New Project
  // (server-side blocked anyway, project.Service.requireUnscoped). "Return
  // to your dashboard" is just switchOrg back to the account's own home
  // org — always available, whether this session got here via switch-org
  // or switch-project.
  const scopedProjectId = useAuthStore((s) => s.user?.scopedProjectId ?? null)
  const homeOrgId = useAuthStore((s) => s.user?.homeOrgId ?? '')
  const switchOrg = useAuthStore((s) => s.switchOrg)
  const [scopedProjectName, setScopedProjectName] = useState<string | null>(null)
  const [returning, setReturning] = useState(false)

  useEffect(() => {
    if (!scopedProjectId) return
    let cancelled = false
    getProject(scopedProjectId)
      .then((p) => {
        if (!cancelled) setScopedProjectName(p.name)
      })
      .catch(() => {
        if (!cancelled) setScopedProjectName(null)
      })
    return () => {
      cancelled = true
    }
  }, [scopedProjectId])

  async function handleReturnToDashboard() {
    if (!homeOrgId) return
    setReturning(true)
    try {
      await switchOrg(homeOrgId)
      navigate('/dashboard')
    } finally {
      setReturning(false)
    }
  }

  const navItems = scopedProjectId ? NAV_ITEMS.filter((item) => item.to !== '/team') : NAV_ITEMS

  return (
    <div className="flex min-h-screen flex-col bg-bg-base">
      {scopedProjectId && (
        <div className="flex shrink-0 items-center gap-2 border-b border-accent/30 bg-accent/10 px-3 py-1.5 text-body-sm text-text-primary">
          <FolderKanban className="h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
          <span className="min-w-0 truncate">
            Viewing <span className="font-semibold">{scopedProjectName ?? 'a shared project'}</span>{' '}
            — shared with you as a collaborator. You can&rsquo;t see the rest of this organization.
          </span>
          <Button
            size="sm"
            variant="secondary"
            loading={returning}
            onClick={() => void handleReturnToDashboard()}
            className="ml-auto shrink-0"
          >
            <ArrowLeftCircle className="h-3.5 w-3.5" aria-hidden="true" />
            Return to your dashboard
          </Button>
        </div>
      )}
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

        <Link
          to="/dashboard"
          className="flex items-center gap-2 text-h3 font-semibold text-chrome-text"
        >
          <Logo className="h-6 w-auto" />
          GuardPipe
        </Link>

        <div className="ml-2">
          <GlobalSearch />
        </div>

        <div className="ml-auto flex items-center gap-1.5">
          {onProjectsList && !scopedProjectId && (
            <Button size="sm" onClick={() => navigate('/projects/new')}>
              <Plus className="h-4 w-4" aria-hidden="true" />
              New Project
            </Button>
          )}
          {inProject && (
            <Button size="sm" onClick={() => navigate(`/projects/${projectMatch[1]}/scans`)}>
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
            {navItems.map(({ to, label, icon: Icon }) => (
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
