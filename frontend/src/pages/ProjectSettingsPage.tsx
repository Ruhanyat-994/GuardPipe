import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AlertTriangle, CheckCircle2 } from 'lucide-react'
import { Button } from '../components/ui/Button'
import { Card, CardDescription, CardTitle } from '../components/ui/Card'
import { Input } from '../components/ui/Input'
import { RepositoryAttachForm } from '../components/project/RepositoryAttachForm'
import { useProjectContext } from '../components/project/ProjectContext'
import { ApiError } from '../lib/apiClient'
import { archiveProject, updateProject } from '../lib/projectsApi'

/** Real project editing + the Archive danger zone — the same
 * `project.Service.Update`/`Archive` endpoints `ContextMenu`'s "Archive
 * project" also calls, just with a second, more deliberate entry point. */
export function ProjectSettingsPage() {
  const { project, refetch } = useProjectContext()
  const navigate = useNavigate()

  const [name, setName] = useState(project.name)
  const [description, setDescription] = useState(project.description ?? '')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  const [archiving, setArchiving] = useState(false)
  const [archiveError, setArchiveError] = useState<string | null>(null)

  async function handleSave(e: FormEvent) {
    e.preventDefault()
    setSaveError(null)
    setSaved(false)
    setSaving(true)
    try {
      await updateProject(project.id, { name, description: description.trim() || undefined })
      refetch()
      setSaved(true)
    } catch (err) {
      setSaveError(
        err instanceof ApiError ? err.problem.detail : 'Something went wrong. Please try again.',
      )
    } finally {
      setSaving(false)
    }
  }

  async function handleArchive() {
    if (!window.confirm(`Archive "${project.name}"?`)) return
    setArchiveError(null)
    setArchiving(true)
    try {
      await archiveProject(project.id)
      navigate('/projects')
    } catch (err) {
      setArchiveError(
        err instanceof ApiError ? err.problem.detail : 'Something went wrong. Please try again.',
      )
      setArchiving(false)
    }
  }

  return (
    <main className="mx-auto max-w-2xl px-6 py-8">
      <h1 className="mb-6 text-h1 text-text-primary">Project settings</h1>

      <Card className="mb-4">
        <CardTitle className="text-h3">Details</CardTitle>
        <form onSubmit={handleSave} className="mt-4 flex flex-col gap-4">
          <div>
            <label htmlFor="settings-name" className="mb-1 block text-body-sm text-text-secondary">
              Name
            </label>
            <Input
              id="settings-name"
              required
              maxLength={120}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div>
            <label
              htmlFor="settings-description"
              className="mb-1 block text-body-sm text-text-secondary"
            >
              Description
            </label>
            <Input
              id="settings-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          {saveError && (
            <p role="alert" className="text-body-sm text-danger">
              {saveError}
            </p>
          )}
          <div className="flex items-center gap-3">
            <Button type="submit" loading={saving} className="self-start">
              Save changes
            </Button>
            {saved && (
              <span className="flex items-center gap-1.5 text-body-sm text-success">
                <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
                Saved
              </span>
            )}
          </div>
        </form>
      </Card>

      <div className="mb-4">
        <RepositoryAttachForm
          projectId={project.id}
          existing={project.repository}
          onAttached={refetch}
        />
      </div>

      <Card className="mb-4">
        <CardTitle className="text-h3">Credential</CardTitle>
        <CardDescription className="mt-1">
          {project.has_credential
            ? 'A GitHub personal access token is attached to this project.'
            : 'No credential attached. Required only for private repositories.'}
        </CardDescription>
      </Card>

      <Card className="border-danger/30">
        <div className="flex items-center gap-2">
          <AlertTriangle className="h-4 w-4 text-danger" aria-hidden="true" />
          <CardTitle className="text-h3">Danger zone</CardTitle>
        </div>
        <CardDescription className="mt-1">
          Archiving keeps the project's full scan history but removes it from the active project
          list (FR-PRJ-008).
        </CardDescription>
        {archiveError && (
          <p role="alert" className="mt-2 text-body-sm text-danger">
            {archiveError}
          </p>
        )}
        <Button
          variant="destructive"
          loading={archiving}
          onClick={() => void handleArchive()}
          className="mt-4"
        >
          Archive project
        </Button>
      </Card>
    </main>
  )
}
