import { ProjectFindingsPanel } from '../components/project/ProjectFindingsPanel'
import { useProjectContext } from '../components/project/ProjectContext'

/**
 * The project's Findings tab — findings from the project's most recent
 * scan, grouped by engine. See ProjectFindingsPanel for the fetch/render
 * logic, shared with the global Findings page.
 */
export function ProjectFindingsPage() {
  const { project } = useProjectContext()

  return (
    <main className="mx-auto max-w-4xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-h1 text-text-primary">Findings</h1>
        <p className="text-body-sm text-text-secondary">
          Findings from this project's most recent scan.
        </p>
      </div>

      <ProjectFindingsPanel projectId={project.id} repository={project.repository} />
    </main>
  )
}
