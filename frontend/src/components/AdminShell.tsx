import { type ReactNode, useState } from 'react'
import { Link, NavLink } from 'react-router-dom'
import {
  Activity,
  ArrowLeft,
  Building2,
  Menu,
  PanelLeftClose,
  ScrollText,
  ShieldAlert,
} from 'lucide-react'
import { cn } from '../lib/cn'
import { Logo } from './Logo'
import { UserMenu } from './UserMenu'

/**
 * The platform-operator control plane's own chrome (BUILD_GUIDE.md
 * Phase 14) — deliberately NOT a section of AppShell/SidebarNav/TopBar. A
 * screenshot of this should never read as something a customer could
 * stumble into by mistake, the same reasoning that already keeps the
 * public AuthShell visually distinct from the authenticated AppShell. Still
 * built on the same design tokens (documentation/09-ui-ux-design-system.md)
 * — the distinction is content and an explicit "Platform Admin" label, an
 * amber accent strip, and a different icon set, not a second design system.
 */

const NAV_ITEMS = [
  { to: '/admin/organizations', label: 'Organizations', icon: Building2 },
  { to: '/admin/pentest-flags', label: 'Pentest misuse queue', icon: ShieldAlert },
  { to: '/admin/audit-log', label: 'Audit log', icon: ScrollText },
  { to: '/admin/system-health', label: 'System health', icon: Activity },
] as const

export function AdminShell({ children }: { children: ReactNode }) {
  const [collapsed, setCollapsed] = useState(false)

  return (
    <div className="flex min-h-screen flex-col bg-bg-base">
      <div className="h-1 shrink-0 bg-warning" aria-hidden="true" />
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
          to="/admin/organizations"
          className="flex items-center gap-2 text-h3 font-semibold text-chrome-text"
        >
          <Logo className="h-6 w-auto" />
          GuardPipe
        </Link>

        <span className="rounded-full bg-warning/20 px-2.5 py-0.5 text-caption font-semibold tracking-wide text-warning uppercase">
          Platform Admin
        </span>

        <div className="ml-auto flex items-center gap-1.5">
          <Link
            to="/dashboard"
            className="flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-body-sm text-chrome-text-secondary hover:bg-chrome-hover hover:text-chrome-text"
          >
            <ArrowLeft className="h-4 w-4" aria-hidden="true" />
            Back to GuardPipe
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
          aria-label="Platform admin"
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
                      ? 'bg-warning/10 text-warning'
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
          </nav>
        </aside>

        <div className="min-w-0 flex-1 overflow-y-auto">{children}</div>
      </div>
    </div>
  )
}
