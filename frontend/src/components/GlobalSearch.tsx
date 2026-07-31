import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { FolderKanban, Search } from 'lucide-react'
import { Popover } from './ui/Popover'
import { listProjects, type Project } from '../lib/projectsApi'

/**
 * documentation/09-ui-ux-design-system.md §4.4 — expands the top-bar
 * search pill into an overlay. There's no cross-entity search backend yet
 * (findings/rules search is well beyond Phase 3), so this ships as a real,
 * working client-side filter over the caller's own projects rather than a
 * fabricated "AI-interpreted query" row — honest about the current scope,
 * same discipline as `NotificationPanel`.
 */
export function GlobalSearch() {
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [projects, setProjects] = useState<Project[] | null>(null)
  const [loading, setLoading] = useState(false)

  function ensureLoaded() {
    if (projects !== null || loading) return
    setLoading(true)
    listProjects()
      .then((res) => setProjects(res.data))
      .catch(() => setProjects([]))
      .finally(() => setLoading(false))
  }

  const filtered = (projects ?? []).filter((p) =>
    p.name.toLowerCase().includes(query.trim().toLowerCase()),
  )

  return (
    <Popover
      align="left"
      panelClassName="w-[420px]"
      trigger={(open, toggle) => (
        <button
          type="button"
          onClick={() => {
            toggle()
            ensureLoaded()
          }}
          aria-haspopup="menu"
          aria-expanded={open}
          className="flex h-8 w-64 items-center gap-2 rounded-full border border-white/15 bg-white/10 px-3 text-body-sm text-chrome-text-secondary hover:bg-white/15"
        >
          <Search className="h-4 w-4" aria-hidden="true" />
          Search…
        </button>
      )}
    >
      {(close) => (
        <div className="p-3">
          <div className="flex items-center gap-2 rounded-md border border-border-default px-3 py-2">
            <Search className="h-4 w-4 text-text-tertiary" aria-hidden="true" />
            <input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search projects…"
              className="w-full bg-transparent text-body-sm text-text-primary outline-none placeholder:text-text-tertiary"
            />
          </div>

          <div className="mt-2 max-h-72 overflow-y-auto">
            {loading && <p className="px-2 py-3 text-body-sm text-text-tertiary">Loading…</p>}
            {!loading && filtered.length === 0 && (
              <p className="px-2 py-3 text-body-sm text-text-tertiary">
                {query ? 'No projects match.' : 'Type to search your projects.'}
              </p>
            )}
            {filtered.map((p) => (
              <button
                key={p.id}
                type="button"
                onClick={() => {
                  close()
                  setQuery('')
                  navigate(`/projects/${p.id}`)
                }}
                className="flex w-full items-center gap-2.5 rounded-md px-2 py-2 text-left text-body-sm text-text-primary hover:bg-bg-subtle"
              >
                <FolderKanban className="h-4 w-4 shrink-0 text-text-tertiary" aria-hidden="true" />
                <span className="truncate">{p.name}</span>
              </button>
            ))}
          </div>

          <p className="mt-2 border-t border-border-default pt-2 text-caption text-text-tertiary">
            Searching projects only — findings and rules search land in a later phase.
          </p>
        </div>
      )}
    </Popover>
  )
}
