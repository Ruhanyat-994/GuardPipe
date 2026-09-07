import { useEffect, useState } from "react";
import { Card, CardTitle } from "../components/ui/Card";
import { MetricCard } from "../components/ui/MetricCard";
import { ApiError } from "../lib/apiClient";
import { getSystemHealth, type SystemHealth } from "../lib/adminApi";
import { formatDate } from "../lib/format";

/**
 * Screen 22 — Admin: System health (BUILD_GUIDE.md Phase 14). Every number
 * here is either real (engine job stats, jobs in flight) or explicitly
 * marked unavailable — never fabricated, the same rule AiPanel's
 * unavailable state and PartialResultBanner already follow.
 */
export function AdminSystemHealthPage() {
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [error, setError] = useState<string | null>(null);

  function refresh() {
    getSystemHealth()
      .then(setHealth)
      .catch((err: unknown) => {
        setError(
          err instanceof ApiError
            ? err.problem.detail
            : "Could not load system health.",
        );
      });
  }

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 30_000);
    return () => clearInterval(interval);
  }, []);

  return (
    <main className="mx-auto max-w-5xl px-6 py-8">
      <h1 className="text-h1 text-text-primary">System health</h1>
      <p className="mb-6 text-body-sm text-text-secondary">
        Live operational state — refreshes every 30 seconds.
        {health && <> Last checked {formatDate(health.checked_at)}.</>}
      </p>

      {error && (
        <Card className="mb-4 border-danger/30 bg-danger/5">
          <p role="alert" className="text-body-sm text-danger">
            {error}
          </p>
        </Card>
      )}

      {!health && !error && (
        <p className="text-body-sm text-text-secondary">Loading…</p>
      )}

      {health && (
        <>
          <div className="mb-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
            <MetricCard
              label="Jobs in flight"
              value={health.jobs_in_flight}
              caption="Redis queue"
            />
            <MetricCard
              label="Sandbox containers"
              value={health.sandbox_containers_running ?? "—"}
              caption={
                health.sandbox_containers_running === null
                  ? "Unavailable"
                  : "Running now"
              }
            />
            <MetricCard
              label="Gemini key pool"
              value={health.gemini.available ? health.gemini.pool_size : "—"}
              caption={
                health.gemini.available
                  ? `Currently on key #${health.gemini.current_index + 1}`
                  : "AI disabled"
              }
            />
            <MetricCard
              label="AI cache hit rate"
              value={
                health.ai_cache.available
                  ? formatHitRate(health.ai_cache.hits, health.ai_cache.misses)
                  : "—"
              }
              caption={
                health.ai_cache.available
                  ? `${health.ai_cache.hits} hits / ${health.ai_cache.misses} misses`
                  : "AI disabled"
              }
            />
          </div>

          <Card>
            <CardTitle className="text-h3">
              Engine job outcomes (last 24h)
            </CardTitle>
            {health.engine_stats.length === 0 ? (
              <p className="mt-2 text-body-sm text-text-secondary">
                No engine jobs have run in the last 24 hours.
              </p>
            ) : (
              <div className="mt-4 overflow-x-auto">
                <table className="w-full text-left text-body-sm">
                  <thead>
                    <tr className="border-b border-border-default text-caption text-text-tertiary uppercase">
                      <th className="pb-2 pr-4 font-semibold">Engine</th>
                      <th className="pb-2 pr-4 font-semibold">Succeeded</th>
                      <th className="pb-2 pr-4 font-semibold">Failed</th>
                      <th className="pb-2 font-semibold">Skipped</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border-default">
                    {health.engine_stats.map((stat) => (
                      <tr key={stat.engine}>
                        <td className="py-2.5 pr-4 font-medium text-text-primary capitalize">
                          {stat.engine}
                        </td>
                        <td className="py-2.5 pr-4 text-success">
                          {stat.succeeded}
                        </td>
                        <td className="py-2.5 pr-4 text-danger">
                          {stat.failed}
                        </td>
                        <td className="py-2.5 text-text-tertiary">
                          {stat.skipped}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        </>
      )}
    </main>
  );
}

function formatHitRate(hits: number, misses: number): string {
  const total = hits + misses;
  if (total === 0) return "—";
  return `${Math.round((hits / total) * 100)}%`;
}
