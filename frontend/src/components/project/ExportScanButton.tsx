import { useState } from 'react'
import { Download, FileJson, FileSpreadsheet, FileText } from 'lucide-react'
import { Popover } from '../ui/Popover'
import { cn } from '../../lib/cn'
import { ApiError } from '../../lib/apiClient'
import { exportScan, type ExportFormat } from '../../lib/scansApi'

const FORMATS: {
  format: ExportFormat
  label: string
  description: string
  icon: typeof FileText
}[] = [
  {
    format: 'pdf',
    label: 'PDF report',
    description: 'Cover page, coverage, findings',
    icon: FileText,
  },
  { format: 'csv', label: 'CSV', description: 'Findings table only', icon: FileSpreadsheet },
  { format: 'json', label: 'JSON', description: 'Full machine-readable snapshot', icon: FileJson },
]

/**
 * "Export report" — a self-contained report snapshot distinct from the live
 * scan view (coverage per engine, every finding's remediation, an
 * AI-authored executive summary when available), `GET /scans/{id}/export`.
 * A plain button that triggers the browser's native save, not a page of its
 * own — the report is generated on demand, not pre-built.
 */
export function ExportScanButton({ scanId }: { scanId: string }) {
  const [downloading, setDownloading] = useState<ExportFormat | null>(null)
  const [error, setError] = useState<string | null>(null)

  async function handleExport(format: ExportFormat, close: () => void) {
    close()
    setError(null)
    setDownloading(format)
    try {
      await exportScan(scanId, format)
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not generate the report.')
    } finally {
      setDownloading(null)
    }
  }

  return (
    <div className="relative">
      <Popover
        trigger={(open, toggle) => (
          <button
            type="button"
            onClick={toggle}
            aria-haspopup="menu"
            aria-expanded={open}
            disabled={downloading !== null}
            className={cn(
              'inline-flex h-8 items-center gap-1.5 rounded-md bg-accent px-3',
              'text-body-sm font-medium text-text-inverse shadow-sm hover:opacity-90 active:opacity-80',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            {downloading ? (
              <span
                className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-current border-t-transparent"
                aria-hidden="true"
              />
            ) : (
              <Download className="h-4 w-4" aria-hidden="true" />
            )}
            Export report
          </button>
        )}
      >
        {(close) => (
          <div className="py-1">
            {FORMATS.map(({ format, label, description, icon: Icon }) => (
              <button
                key={format}
                type="button"
                role="menuitem"
                onClick={() => void handleExport(format, close)}
                className="flex w-full items-start gap-2.5 px-3 py-2 text-left hover:bg-bg-subtle"
              >
                <Icon className="mt-0.5 h-4 w-4 shrink-0 text-text-secondary" aria-hidden="true" />
                <span>
                  <span className="block text-body-sm font-medium text-text-primary">{label}</span>
                  <span className="block text-caption text-text-tertiary">{description}</span>
                </span>
              </button>
            ))}
          </div>
        )}
      </Popover>
      {error && (
        <p role="alert" className="absolute right-0 top-full mt-1 w-64 text-caption text-danger">
          {error}
        </p>
      )}
    </div>
  )
}
