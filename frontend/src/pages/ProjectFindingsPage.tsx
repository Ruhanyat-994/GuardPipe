import { Link, useSearchParams } from 'react-router-dom'
import { ProjectFindingsPanel } from '../components/project/ProjectFindingsPanel'
import { useProjectContext } from '../components/project/ProjectContext'

/**
 * The project's Findings tab — findings from the project's most recent
 * scan, grouped by engine. See ProjectFindingsPanel for the fetch/render
 * logic, shared with the global Findings page. An optional `?severity=`
 * (set when arriving from a DashboardPage severity-tile click) narrows the
 * list to just that severity.
 */
export function ProjectFindingsPage() {
  const { project } = useProjectContext()
  const [searchParams, setSearchParams] = useSearchParams()
  const severity = searchParams.get('severity')

  return (
    <main className="mx-auto max-w-4xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-h1 text-text-primary">Findings</h1>
        <p className="text-body-sm text-text-secondary">
          Findings from this project's most recent scan.
        </p>
      </div>

      {severity && (
        <p className="mb-4 text-body-sm text-text-secondary">
          Filtered to <span className="font-medium capitalize text-text-primary">{severity}</span> ·{' '}
          <Link
            to="?"
            replace
            className="text-accent hover:underline"
            onClick={(e) => {
              e.preventDefault()
              setSearchParams({})
            }}
          >
            Clear filter
          </Link>
        </p>
      )}

      <ProjectFindingsPanel
        projectId={project.id}
        repository={project.repository}
        severityFilter={severity ?? undefined}
      />
    </main>
  )
}
