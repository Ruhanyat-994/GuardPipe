import { type FormEvent, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { CheckCircle2 } from 'lucide-react'
import { Button } from '../components/ui/Button'
import { Card, CardTitle } from '../components/ui/Card'
import { Input } from '../components/ui/Input'
import { RepositoryAttachForm } from '../components/project/RepositoryAttachForm'
import { TargetRegisterForm } from '../components/project/TargetRegisterForm'
import { ApiError } from '../lib/apiClient'
import { createProject, type Project } from '../lib/projectsApi'

/**
 * documentation/09-ui-ux-design-system.md §5.6 + BUILD_GUIDE.md Phase 3:
 * "Project creation form + repository/pentest-target attachment UI (PAT
 * input must never echo back or log the token)." Three stages on one page:
 * project details, then (once the project exists) an optional repository
 * attach and an optional pentest-target registration — both are safe to
 * skip and finish later from the project's own Settings/Targets tabs
 * (`ProjectSettingsPage`, `ProjectTargetsPage`).
 */
export function ProjectCreatePage() {
  const navigate = useNavigate()

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [project, setProject] = useState<Project | null>(null)

  async function handleCreate(e: FormEvent) {
    e.preventDefault()
    setCreateError(null)
    setCreating(true)
    try {
      const created = await createProject({
        name,
        description: description.trim() || undefined,
      })
      setProject(created)
    } catch (err) {
      setCreateError(
        err instanceof ApiError ? err.problem.detail : 'Something went wrong. Please try again.',
      )
    } finally {
      setCreating(false)
    }
  }

  return (
    <main className="mx-auto max-w-2xl px-6 py-8">
      <h1 className="text-h1 text-text-primary">New project</h1>
      <p className="mb-6 text-body-sm text-text-secondary">
        A project is what GuardPipe scans repeatedly — typically one application or repository.
      </p>

      <Card>
        <CardTitle>Project details</CardTitle>
        <form onSubmit={handleCreate} className="mt-4 flex flex-col gap-4">
          <div>
            <label htmlFor="name" className="mb-1 block text-body-sm text-text-secondary">
              Name
            </label>
            <Input
              id="name"
              required
              maxLength={120}
              disabled={!!project}
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Payments API"
            />
          </div>
          <div>
            <label htmlFor="description" className="mb-1 block text-body-sm text-text-secondary">
              Description <span className="text-text-tertiary">(optional)</span>
            </label>
            <Input
              id="description"
              disabled={!!project}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Core payment service"
            />
          </div>

          {createError && (
            <p role="alert" className="text-body-sm text-danger">
              {createError}
            </p>
          )}

          {!project && (
            <Button type="submit" loading={creating} className="self-start">
              Create project
            </Button>
          )}
          {project && (
            <p className="flex items-center gap-1.5 text-body-sm text-success">
              <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
              Project created.
            </p>
          )}
        </form>
      </Card>

      {project && (
        <div className="mt-4">
          <RepositoryAttachForm projectId={project.id} />
        </div>
      )}
      {project && (
        <div className="mt-4">
          <TargetRegisterForm projectId={project.id} />
        </div>
      )}

      {project && (
        <div className="mt-6 flex items-center justify-between">
          <Link to="/projects" className="text-body-sm text-text-secondary underline">
            Back to projects
          </Link>
          <Button onClick={() => navigate(`/projects/${project.id}`)}>Go to project</Button>
        </div>
      )}
    </main>
  )
}
