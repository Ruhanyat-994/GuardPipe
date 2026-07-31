import { Link, NavLink, useNavigate } from 'react-router-dom'
import {
  Archive,
  Crosshair,
  LayoutDashboard,
  ScanSearch,
  Settings as SettingsIcon,
  ShieldAlert,
} from 'lucide-react'
import { ContextMenu } from '../ContextMenu'
import { cn } from '../../lib/cn'
import { archiveProject, type Project } from '../../lib/projectsApi'

/**
 * documentation/09-ui-ux-design-system.md §4.4 — breadcrumb + `ContextMenu`
 * + underlined tab row, sitting directly below `TopBar` on every
 * `/projects/:id/*` route. New in this pass — Phase 3 had no per-project
 * sub-navigation, everything lived on one dashboard page.
 */
const TABS = [
  { to: '', label: 'Overview', icon: LayoutDashboard, end: true },
  { to: 'scans', label: 'Scans', icon: ScanSearch, end: false },
  { to: 'findings', label: 'Findings', icon: ShieldAlert, end: false },
  { to: 'targets', label: 'Targets', icon: Crosshair, end: false },
  { to: 'settings', label: 'Settings', icon: SettingsIcon, end: false },
] as const

export function ProjectTabBar({ project }: { project: Project }) {
  const navigate = useNavigate()

  async function handleArchive() {
    // A styled confirmation Dialog is listed in §4.1 but not built yet
    // (only Button/Card/Input exist) — window.confirm still satisfies
    // §6's "require a modal naming the exact object" in substance, and
    // this is the first real destructive action in the product.
    if (
      !window.confirm(
        `Archive "${project.name}"? You can find it again from an archived filter later.`,
      )
    ) {
      return
    }
    await archiveProject(project.id)
    navigate('/projects')
  }

  return (
    <div className="border-b border-border-default bg-bg-surface px-6">
      <div className="flex h-11 items-center justify-between">
        <div className="flex items-center gap-1.5 text-body-sm">
          <Link to="/projects" className="text-text-secondary hover:text-text-primary">
            Projects
          </Link>
          <span className="text-text-tertiary">/</span>
          <span className="font-medium text-text-primary">{project.name}</span>
        </div>
        <ContextMenu
          label="Project actions"
          groups={[
            [
              {
                label: 'Archive project',
                icon: Archive,
                destructive: true,
                onClick: () => void handleArchive(),
              },
            ],
          ]}
        />
      </div>
      <nav className="-mb-px flex gap-5">
        {TABS.map(({ to, label, icon: Icon, end }) => (
          <NavLink
            key={label}
            to={to}
            end={end}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-1.5 border-b-2 px-0.5 pb-2 text-body-sm font-medium',
                isActive
                  ? 'border-accent text-accent'
                  : 'border-transparent text-text-secondary hover:text-text-primary',
              )
            }
          >
            <Icon className="h-4 w-4" aria-hidden="true" />
            {label}
          </NavLink>
        ))}
      </nav>
    </div>
  )
}
