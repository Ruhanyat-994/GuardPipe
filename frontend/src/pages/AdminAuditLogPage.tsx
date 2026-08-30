import { useEffect, useState } from 'react'
import { ScrollText } from 'lucide-react'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { Input } from '../components/ui/Input'
import { EmptyState } from '../components/ui/EmptyState'
import { ApiError } from '../lib/apiClient'
import { listAuditLog, type AuditEntry } from '../lib/adminApi'
import { formatDate } from '../lib/format'

/**
 * Screen 20 — Admin: Audit log (BUILD_GUIDE.md Phase 14). Reuses
 * FindingsTable's filter-input conventions rather than a second version of
 * the same "big filterable table" shape.
 */
export function AdminAuditLogPage() {
  const [entries, setEntries] = useState<AuditEntry[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [action, setAction] = useState('')
  const [orgId, setOrgId] = useState('')

  useEffect(() => {
    const handle = setTimeout(() => {
      listAuditLog({ action: action || undefined, orgId: orgId || undefined })
        .then((res) => setEntries(res.data))
        .catch((err: unknown) => {
          setError(err instanceof ApiError ? err.problem.detail : 'Could not load the audit log.')
        })
    }, 250)
    return () => clearTimeout(handle)
  }, [action, orgId])

  return (
    <main className="mx-auto max-w-6xl px-6 py-8">
      <h1 className="text-h1 text-text-primary">Audit log</h1>
      <p className="mb-6 text-body-sm text-text-secondary">
        Every recorded action across every organization — the one screen where the normal per-org
        scoping is deliberately lifted.
      </p>

      {error && (
        <Card className="mb-4 border-danger/30 bg-danger/5">
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        </Card>
      )}

      <Card className="mb-4">
        <div className="flex flex-wrap items-center gap-3">
          <Input
            value={action}
            onChange={(e) => setAction(e.target.value)}
            placeholder="Filter by action (e.g. org.suspended)…"
            className="max-w-xs"
          />
          <Input
            value={orgId}
            onChange={(e) => setOrgId(e.target.value)}
            placeholder="Filter by organization ID…"
            className="max-w-xs"
          />
        </div>
      </Card>

      <Card>
        <CardTitle className="text-h3">
          Entries {entries !== null && `(${entries.length})`}
        </CardTitle>

        {entries === null && !error && <CardDescription className="mt-2">Loading…</CardDescription>}

        {entries !== null && entries.length === 0 && (
          <EmptyState
            icon={ScrollText}
            title="No matching entries"
            description="No audit log entries match these filters."
          />
        )}

        {entries !== null && entries.length > 0 && (
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-left text-body-sm">
              <thead>
                <tr className="border-b border-border-default text-caption text-text-tertiary uppercase">
                  <th className="pb-2 pr-4 font-semibold">When</th>
                  <th className="pb-2 pr-4 font-semibold">Action</th>
                  <th className="pb-2 pr-4 font-semibold">Organization</th>
                  <th className="pb-2 pr-4 font-semibold">Actor</th>
                  <th className="pb-2 pr-4 font-semibold">Resource</th>
                  <th className="pb-2 font-semibold">Detail</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border-default">
                {entries.map((entry) => (
                  <tr key={entry.id}>
                    <td className="py-2.5 pr-4 whitespace-nowrap text-text-tertiary">
                      {formatDate(entry.created_at)}
                    </td>
                    <td className="py-2.5 pr-4 font-mono text-text-primary">{entry.action}</td>
                    <td className="py-2.5 pr-4 font-mono text-caption text-text-tertiary">
                      {entry.org_id ? `${entry.org_id.slice(0, 8)}…` : '—'}
                    </td>
                    <td className="py-2.5 pr-4 font-mono text-caption text-text-tertiary">
                      {entry.actor_id ? `${entry.actor_id.slice(0, 8)}…` : 'system'}
                    </td>
                    <td className="py-2.5 pr-4 text-text-secondary">
                      {entry.resource_type ?? '—'}
                    </td>
                    <td className="py-2.5 max-w-xs truncate font-mono text-caption text-text-tertiary">
                      {Object.keys(entry.detail).length > 0 ? JSON.stringify(entry.detail) : '—'}
                    </td>
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
