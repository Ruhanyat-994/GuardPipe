import { type FormEvent, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Check, Globe, GitBranch as GitBranchIcon, Layers } from 'lucide-react'
import { Button } from '../components/ui/Button'
import { Card, CardTitle } from '../components/ui/Card'
import { Input } from '../components/ui/Input'
import { RepositoryAttachForm } from '../components/project/RepositoryAttachForm'
import { TargetRegisterForm } from '../components/project/TargetRegisterForm'
import { ApiError } from '../lib/apiClient'
import { createProject, type Project, type Repository, type Target } from '../lib/projectsApi'
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

type ScanMode = 'repo' | 'target' | 'both'

const SCAN_MODE_OPTIONS: {
  mode: ScanMode
  icon: typeof GitBranchIcon
  label: string
  description: string
}[] = [
  {
    mode: 'repo',
    icon: GitBranchIcon,
    label: 'GitHub repository only',
    description: 'Code, dependency, container, Kubernetes, and CI/CD scans.',
  },
  {
    mode: 'target',
    icon: Globe,
    label: 'Web URL / public endpoint only',
    description: 'A black-box pentest scan against a live host or URL.',
  },
  {
    mode: 'both',
    icon: Layers,
    label: 'Both',
    description: 'Every scan this project can offer.',
  },
]

/**
 * documentation/09-ui-ux-design-system.md §5.6 + BUILD_GUIDE.md Phase 3/12:
 * "Project creation gates on an explicit 'how will this project be
 * scanned' choice, not two independently-optional attachment sections"
 * (BUILD_GUIDE.md Phase 12 frontend checklist). A new first step — GitHub
 * repository only / Web URL only / Both — narrows which of the
 * repository-attach/pentest-target-registration sections render next,
 * since a project scanned only by URL never needs the repo/PAT step and
 * vice versa. Client-side only, per that same checklist item's own scoping
 * note (the backend still happily accepts a project with neither
 * attached — hardening that is separate, optional follow-up work, not done
 * here).
 */
export function ProjectCreatePage() {
  const navigate = useNavigate()

  const [scanMode, setScanMode] = useState<ScanMode | null>(null)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [project, setProject] = useState<Project | null>(null)
  const [repository, setRepository] = useState<Repository | null>(null)
  const [target, setTarget] = useState<Target | null>(null)

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

  const showRepoStep = scanMode === 'repo' || scanMode === 'both'
  const showTargetStep = scanMode === 'target' || scanMode === 'both'
  const targetStepNumber = showRepoStep ? 4 : 3

  // "At least one must end up attached" (BUILD_GUIDE.md Phase 12) — exactly
  // the section(s) the chosen mode requires, not both regardless of mode.
  const attachmentSatisfied =
    scanMode === 'repo'
      ? !!repository
      : scanMode === 'target'
        ? !!target
        : !!repository && !!target

  return (
    <main className="mx-auto max-w-2xl px-6 py-8">
      <h1 className="text-h1 text-text-primary">New project</h1>
      <p className="mb-6 text-body-sm text-text-secondary">
        A project is what GuardPipe scans repeatedly — typically one application, repository, or
        public endpoint.
      </p>

      <Card>
        <div className="flex items-center gap-2">
          <StepBadge n={1} done={!!scanMode} />
          <CardTitle className="text-h3">How will this project be scanned?</CardTitle>
        </div>
        <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
          {SCAN_MODE_OPTIONS.map(({ mode, icon: Icon, label, description: desc }) => {
            const isSelected = scanMode === mode
            return (
              <button
                key={mode}
                type="button"
                disabled={!!project}
                aria-pressed={isSelected}
                onClick={() => setScanMode(mode)}
                className={cn(
                  'flex flex-col items-start gap-2 rounded-lg border px-4 py-3 text-left transition-colors',
                  project && 'cursor-not-allowed opacity-60',
                  isSelected
                    ? 'border-accent bg-accent/5'
                    : 'border-border-default bg-bg-surface hover:border-border-strong',
                )}
              >
                <Icon
                  className={cn('h-5 w-5', isSelected ? 'text-accent' : 'text-text-tertiary')}
                  aria-hidden="true"
                />
                <span className="text-body-sm font-semibold text-text-primary">{label}</span>
                <span className="text-caption text-text-tertiary">{desc}</span>
              </button>
            )
          })}
        </div>
      </Card>

      {scanMode && (
        <div className="animate-reveal mt-4">
          <Card>
            <div className="flex items-center gap-2">
              <StepBadge n={2} done={!!project} />
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
                <label
                  htmlFor="description"
                  className="mb-1 block text-body-sm text-text-secondary"
                >
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
        </div>
      )}

      {project && showRepoStep && (
        <div className="animate-reveal mt-4">
          <div className="mb-2 flex items-center gap-2">
            <StepBadge n={3} done={!!repository} />
            <span className="text-body-sm font-medium text-text-secondary">Step 3</span>
          </div>
          <RepositoryAttachForm projectId={project.id} onAttached={setRepository} />
        </div>
      )}
      {project && showTargetStep && (
        <div className="animate-reveal mt-4">
          <div className="mb-2 flex items-center gap-2">
            <StepBadge n={targetStepNumber} done={!!target} />
            <span className="text-body-sm font-medium text-text-secondary">
              Step {targetStepNumber}
            </span>
          </div>
          <TargetRegisterForm projectId={project.id} onRegistered={setTarget} />
        </div>
      )}

      {project && (
        <div className="animate-reveal mt-6 flex items-center justify-between gap-4">
          <Link to="/projects" className="text-body-sm text-text-secondary underline">
            Back to projects
          </Link>
          <div className="flex flex-col items-end gap-1">
            {!attachmentSatisfied && (
              <span className="text-caption text-text-tertiary">
                {scanMode === 'both'
                  ? 'Attach a repository and register a target to continue.'
                  : scanMode === 'repo'
                    ? 'Attach a repository to continue.'
                    : 'Register a target to continue.'}
              </span>
            )}
            <Button
              onClick={() => navigate(`/projects/${project.id}`)}
              disabled={!attachmentSatisfied}
            >
              Go to project
            </Button>
          </div>
        </div>
      )}
    </main>
  )
}
