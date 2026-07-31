import { type FormEvent, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Check } from 'lucide-react'
import { Button } from '../components/ui/Button'
import { Card, CardTitle } from '../components/ui/Card'
import { Input } from '../components/ui/Input'
import { RepositoryAttachForm } from '../components/project/RepositoryAttachForm'
import { TargetRegisterForm } from '../components/project/TargetRegisterForm'
import { ApiError } from '../lib/apiClient'
import { createProject, type Project } from '../lib/projectsApi'
import { cn } from '../lib/cn'

/** Numbered step marker (documentation/09-ui-ux-design-system.md §6,
 * "progressive disclosure") — a filled accent circle with the step number
 * while pending, a success checkmark once that step is actually done. Steps
 * 2/3 are optional and skippable, so they stay numbered rather than ever
 * showing "done". */
function StepBadge({ n, done }: { n: number; done: boolean }) {
  return (
    <span
      className={cn(
        'flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-caption font-semibold',
        done ? 'bg-success/10 text-success' : 'bg-accent/10 text-accent',
      )}
    >
      {done ? <Check className="h-3.5 w-3.5" aria-hidden="true" /> : n}
    </span>
  )
}

/**
 * documentation/09-ui-ux-design-system.md §5.6 + BUILD_GUIDE.md Phase 3:
 * "Project creation form + repository/pentest-target attachment UI (PAT
 * input must never echo back or log the token)." Three numbered stages on
 * one page: project details, then (once the project exists) an optional
 * repository attach and an optional pentest-target registration — both
 * reveal with a short transition and are safe to skip and finish later
 * from the project's own Settings/Targets tabs (`ProjectSettingsPage`,
 * `ProjectTargetsPage`).
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
        <div className="flex items-center gap-2">
          <StepBadge n={1} done={!!project} />
          <CardTitle className="text-h3">Project details</CardTitle>
        </div>
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
        </form>
      </Card>

      {project && (
        <div className="animate-reveal mt-4">
          <div className="mb-2 flex items-center gap-2">
            <StepBadge n={2} done={false} />
            <span className="text-body-sm font-medium text-text-secondary">Step 2</span>
          </div>
          <RepositoryAttachForm projectId={project.id} />
        </div>
      )}
      {project && (
        <div className="animate-reveal mt-4">
          <div className="mb-2 flex items-center gap-2">
            <StepBadge n={3} done={false} />
            <span className="text-body-sm font-medium text-text-secondary">Step 3</span>
          </div>
          <TargetRegisterForm projectId={project.id} />
        </div>
      )}

      {project && (
        <div className="animate-reveal mt-6 flex items-center justify-between">
          <Link to="/projects" className="text-body-sm text-text-secondary underline">
            Back to projects
          </Link>
          <Button onClick={() => navigate(`/projects/${project.id}`)}>Go to project</Button>
        </div>
      )}
    </main>
  )
}
