import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Building2, Search } from 'lucide-react'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { Input } from '../components/ui/Input'
import { EmptyState } from '../components/ui/EmptyState'
import { ApiError } from '../lib/apiClient'
import { listOrganizations, type OrganizationSummary } from '../lib/adminApi'
import { formatDate } from '../lib/format'

/**
 * Screen 18 — Admin: Organizations (BUILD_GUIDE.md Phase 14). The one place
 * in the product that deliberately shows every tenant at once — see
 * AdminShell's own doc comment for why this lives outside AppShell.
 */
export function AdminOrganizationsPage() {
  const navigate = useNavigate()
  const [orgs, setOrgs] = useState<OrganizationSummary[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  useEffect(() => {
    const handle = setTimeout(() => {
      listOrganizations(search)
        .then((res) => setOrgs(res.data))
        .catch((err: unknown) => {
          setError(err instanceof ApiError ? err.problem.detail : 'Could not load organizations.')
        })
    }, 250) // debounce search input, same order of magnitude as other filter inputs in this app
    return () => clearTimeout(handle)
  }, [search])

  return (
    <main className="mx-auto max-w-5xl px-6 py-8">
      <h1 className="text-h1 text-text-primary">Organizations</h1>
      <p className="mb-6 text-body-sm text-text-secondary">
        Every tenant on the platform — suspend an organization whose account is under
        investigation, or reinstate one once it's resolved.
      </p>

      {error && (
        <Card className="mb-4 border-danger/30 bg-danger/5">
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        </Card>
      )}

      <div className="mb-4 flex items-center gap-2">
        <Search className="h-4 w-4 text-text-tertiary" aria-hidden="true" />
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by organization name…"
          className="max-w-sm"
        />
      </div>

      <Card>
        <CardTitle className="text-h3">
          Organizations {orgs !== null && `(${orgs.length})`}
        </CardTitle>

        {orgs === null && !error && <CardDescription className="mt-2">Loading…</CardDescription>}

        {orgs !== null && orgs.length === 0 && (
          <EmptyState
            icon={Building2}
            title="No organizations found"
            description={search ? 'No organization matches this search.' : 'No organizations yet.'}
          />
        )}

        {orgs !== null && orgs.length > 0 && (
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-left text-body-sm">
              <thead>
                <tr className="border-b border-border-default text-caption text-text-tertiary uppercase">
                  <th className="pb-2 pr-4 font-semibold">Name</th>
                  <th className="pb-2 pr-4 font-semibold">Members</th>
                  <th className="pb-2 pr-4 font-semibold">Projects</th>
                  <th className="pb-2 pr-4 font-semibold">Scans</th>
                  <th className="pb-2 pr-4 font-semibold">Status</th>
                  <th className="pb-2 font-semibold">Created</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border-default">
                {orgs.map((org) => (
                  <tr
                    key={org.id}
                    className="cursor-pointer hover:bg-bg-subtle"
                    onClick={() => navigate(`/admin/organizations/${org.id}`)}
                  >
                    <td className="py-2.5 pr-4 font-medium text-text-primary">{org.name}</td>
                    <td className="py-2.5 pr-4 text-text-secondary">{org.member_count}</td>
                    <td className="py-2.5 pr-4 text-text-secondary">{org.project_count}</td>
                    <td className="py-2.5 pr-4 text-text-secondary">{org.scan_count}</td>
                    <td className="py-2.5 pr-4">
                      {org.suspended_at ? (
                        <span className="inline-flex items-center rounded-full bg-danger/10 px-2 py-0.5 text-caption font-semibold text-danger">
                          Suspended
                        </span>
                      ) : (
                        <span className="inline-flex items-center rounded-full bg-success/10 px-2 py-0.5 text-caption font-semibold text-success">
                          Active
                        </span>
                      )}
                    </td>
                    <td className="py-2.5 text-text-tertiary">{formatDate(org.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </main>
  )
}
