import { type ChangeEvent, type DragEvent, type FormEvent, useEffect, useState } from 'react'
import { FileText, Link2, Trash2, UploadCloud } from 'lucide-react'
import { Button } from '../ui/Button'
import { Card, CardDescription, CardTitle } from '../ui/Card'
import { GeminiMark } from '../icons/GeminiMark'
import { ApiError } from '../../lib/apiClient'
import { cn } from '../../lib/cn'
import {
  deleteDocument,
  importDocument,
  listDocuments,
  uploadDocument,
  type Document,
} from '../../lib/projectsApi'

// Mirrors project.Service.UploadDocument's own limits
// (internal/modules/project/service.go) — checked here too so a bad upload
// fails fast client-side instead of waiting on a round trip.
const ALLOWED_EXTENSIONS = ['.md', '.txt', '.adoc', '.rst', '.pdf', '.csv']
// The 100 KB limit applies to actual text content — for every extension
// except .pdf, that's the raw file itself. A PDF is extracted to plain text
// server-side (service.go's UploadDocument), so its *raw* file is allowed up
// to the backend's transport-layer ceiling (maxUploadBytes) instead; the
// extracted-text 100 KB limit is still enforced, just after the round trip.
const MAX_SIZE_BYTES = 100 * 1024
const MAX_PDF_RAW_BYTES = 1024 * 1024
const MAX_DOCUMENTS = 20

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  return `${(bytes / 1024).toFixed(1)} KB`
}

function extOf(filename: string): string {
  return filename.slice(filename.lastIndexOf('.')).toLowerCase()
}

// One badge color per extension so the document list is scannable at a
// glance rather than every row looking identical.
const EXTENSION_BADGE_CLASSES: Record<string, string> = {
  '.pdf': 'bg-danger/10 text-danger',
  '.md': 'bg-accent/10 text-accent',
  '.csv': 'bg-success/10 text-success',
  '.txt': 'bg-bg-subtle text-text-secondary',
  '.adoc': 'bg-bg-subtle text-text-secondary',
  '.rst': 'bg-bg-subtle text-text-secondary',
}

/**
 * Document upload for docreview's (Phase 11) AI architecture/security
 * review — modeled on RepositoryAttachForm's list-plus-form shape, but for
 * *N* documents rather than one repository. Every uploaded document is
 * reviewed alongside anything docreview finds inside the repository
 * checkout itself (documentation/05-module-specifications.md §11). Covers
 * both intake paths: a file from disk, or a Google Docs/Drive share link
 * imported server-side (no OAuth — the link must be shared "anyone with the
 * link can view," and the backend fetches it directly).
 *
 * `variant="embedded"` drops the outer Card so this can be nested inline
 * (ScanLauncher, when the Docs engine is selected) without a double
 * border/shadow; the default `"card"` is the standalone Settings-page use.
 */
export function DocumentUploadForm({
  projectId,
  variant = 'card',
}: {
  projectId: string
  variant?: 'card' | 'embedded'
}) {
  const [documents, setDocuments] = useState<Document[] | null>(null)
  const [uploading, setUploading] = useState(false)
  const [dragging, setDragging] = useState(false)
  const [driveUrl, setDriveUrl] = useState('')
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [deletingId, setDeletingId] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    listDocuments(projectId)
      .then((res) => {
        if (!cancelled) setDocuments(res.data)
      })
      .catch((err: unknown) => {
        if (!cancelled)
          setError(err instanceof ApiError ? err.problem.detail : 'Could not load documents.')
      })
    return () => {
      cancelled = true
    }
  }, [projectId])

  const atLimit = (documents?.length ?? 0) >= MAX_DOCUMENTS

  async function processFile(file: File) {
    setError(null)
    const ext = extOf(file.name)
    if (!ALLOWED_EXTENSIONS.includes(ext)) {
      setError(`"${file.name}" isn't a supported type — use ${ALLOWED_EXTENSIONS.join(', ')}.`)
      return
    }
    if (ext === '.pdf') {
      if (file.size > MAX_PDF_RAW_BYTES) {
        setError(`"${file.name}" is larger than the 1 MB limit for PDFs.`)
        return
      }
    } else if (file.size > MAX_SIZE_BYTES) {
      setError(`"${file.name}" is larger than the 100 KB limit.`)
      return
    }
    if ((documents?.length ?? 0) >= MAX_DOCUMENTS) {
      setError(`This project already has the maximum of ${MAX_DOCUMENTS} documents.`)
      return
    }

    setUploading(true)
    try {
      const doc = await uploadDocument(projectId, file)
      setDocuments((prev) => [doc, ...(prev ?? [])])
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not upload the document.')
    } finally {
      setUploading(false)
    }
  }

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = '' // allow re-selecting the same filename after a delete
    if (file) void processFile(file)
  }

  const dropDisabled = uploading || atLimit

  function handleDragOver(e: DragEvent<HTMLLabelElement>) {
    e.preventDefault()
    if (!dropDisabled) setDragging(true)
  }

  function handleDragLeave(e: DragEvent<HTMLLabelElement>) {
    e.preventDefault()
    setDragging(false)
  }

  function handleDrop(e: DragEvent<HTMLLabelElement>) {
    e.preventDefault()
    setDragging(false)
    if (dropDisabled) return
    const file = e.dataTransfer.files?.[0]
    if (file) void processFile(file)
  }

  async function handleImport(e: FormEvent) {
    e.preventDefault()
    const url = driveUrl.trim()
    if (!url) return

    setError(null)
    if ((documents?.length ?? 0) >= MAX_DOCUMENTS) {
      setError(`This project already has the maximum of ${MAX_DOCUMENTS} documents.`)
      return
    }

    setImporting(true)
    try {
      const doc = await importDocument(projectId, url)
      setDocuments((prev) => [doc, ...(prev ?? [])])
      setDriveUrl('')
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not import the document.')
    } finally {
      setImporting(false)
    }
  }

  async function handleDelete(doc: Document) {
    if (!window.confirm(`Remove "${doc.filename}"?`)) return
    setDeletingId(doc.id)
    setError(null)
    try {
      await deleteDocument(doc.id)
      setDocuments((prev) => (prev ?? []).filter((d) => d.id !== doc.id))
    } catch (err) {
      setError(err instanceof ApiError ? err.problem.detail : 'Could not remove the document.')
    } finally {
      setDeletingId(null)
    }
  }

  const Wrapper = variant === 'embedded' ? 'div' : Card
  const wrapperProps =
    variant === 'embedded' ? { className: 'mt-4 border-t border-border-default pt-4' } : {}

  return (
    <Wrapper {...wrapperProps}>
      <div className="flex items-center gap-2">
        <FileText className="h-4 w-4 text-text-tertiary" aria-hidden="true" />
        <CardTitle className="text-h3">Documents</CardTitle>
        <GeminiMark />
      </div>
      <CardDescription className="mt-1">
        Design/requirements documents reviewed for architectural and security gaps —{' '}
        {ALLOWED_EXTENSIONS.join(', ')}, up to 100 KB of text each (1 MB for a PDF, before
        extraction), {MAX_DOCUMENTS} per project.
      </CardDescription>

      {/* The dropzone is the primary call to action — large, illustrated,
          and impossible to miss, rather than a small text link buried under
          the document list. Drag-and-drop and click-to-browse both work;
          the whole box is the label for the hidden file input. */}
      <label
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        className={cn(
          'mt-4 flex flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed px-6 py-8 text-center transition-colors',
          dropDisabled
            ? 'cursor-not-allowed border-border-default bg-bg-subtle opacity-50'
            : 'cursor-pointer border-border-default bg-bg-subtle hover:border-accent hover:bg-accent/5',
          dragging && !dropDisabled && 'border-accent bg-accent/5',
        )}
      >
        <UploadCloud
          className={cn('h-8 w-8', dragging ? 'text-accent' : 'text-text-tertiary')}
          aria-hidden="true"
        />
        <p className="text-body font-medium text-text-primary">
          {uploading ? 'Uploading…' : 'Drag & drop a document here'}
        </p>
        {!uploading && (
          <p className="text-body-sm text-text-secondary">
            or <span className="font-medium text-accent">click to browse</span> —{' '}
            {ALLOWED_EXTENSIONS.join(', ')}
          </p>
        )}
        <input
          type="file"
          accept={ALLOWED_EXTENSIONS.join(',')}
          className="sr-only"
          disabled={dropDisabled}
          onChange={handleFileChange}
        />
      </label>

      <div className="my-4 flex items-center gap-3">
        <div className="h-px flex-1 bg-border-default" />
        <span className="text-caption font-medium text-text-tertiary">OR</span>
        <div className="h-px flex-1 bg-border-default" />
      </div>

      <form onSubmit={(e) => void handleImport(e)} className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-0 flex-1">
          <Link2
            className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-text-tertiary"
            aria-hidden="true"
          />
          <input
            type="url"
            value={driveUrl}
            onChange={(e) => setDriveUrl(e.target.value)}
            placeholder="Paste a Google Docs or Drive share link…"
            disabled={importing || atLimit}
            aria-label="Google Docs or Drive share link"
            className="w-full rounded-md border border-border-default bg-bg-surface py-2 pl-8 pr-3 text-body-sm text-text-primary placeholder:text-text-tertiary disabled:cursor-not-allowed disabled:opacity-50"
          />
        </div>
        <Button
          type="submit"
          variant="secondary"
          loading={importing}
          disabled={importing || atLimit || !driveUrl.trim()}
        >
          Import from Google
        </Button>
      </form>
      <p className="mt-1.5 text-caption text-text-tertiary">
        Sharing must be set to &quot;anyone with the link can view&quot; — GuardPipe fetches it
        directly, no Google sign-in required.
      </p>

      {error && (
        <p role="alert" className="mt-3 text-body-sm text-danger">
          {error}
        </p>
      )}

      {documents === null ? (
        <p className="mt-4 text-body-sm text-text-secondary">Loading…</p>
      ) : documents.length > 0 ? (
        <ul className="mt-4 flex flex-col divide-y divide-border-default">
          {documents.map((doc) => {
            const ext = extOf(doc.filename)
            return (
              <li key={doc.id} className="flex items-center justify-between gap-3 py-2.5">
                <div className="flex min-w-0 items-center gap-2.5">
                  <span
                    className={cn(
                      'shrink-0 rounded px-1.5 py-0.5 text-caption font-semibold uppercase',
                      EXTENSION_BADGE_CLASSES[ext] ?? 'bg-bg-subtle text-text-secondary',
                    )}
                  >
                    {ext.slice(1)}
                  </span>
                  <span className="truncate text-body-sm text-text-primary">{doc.filename}</span>
                  <span className="shrink-0 text-caption text-text-tertiary">
                    {formatSize(doc.size_bytes)}
                  </span>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  loading={deletingId === doc.id}
                  onClick={() => void handleDelete(doc)}
                  aria-label={`Remove ${doc.filename}`}
                >
                  <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                </Button>
              </li>
            )
          })}
        </ul>
      ) : (
        <p className="mt-4 text-body-sm text-text-tertiary">No documents uploaded yet.</p>
      )}
    </Wrapper>
  )
}
