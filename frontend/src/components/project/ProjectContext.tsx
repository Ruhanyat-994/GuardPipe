import { createContext, useContext } from 'react'
import type { Project } from '../../lib/projectsApi'

/** Shared across every tab of `ProjectLayout` (Overview/Scans/Findings/
 * Targets/Settings) so each tab page doesn't re-fetch the project itself. */
export interface ProjectContextValue {
  project: Project
  refetch: () => void
}

export const ProjectContext = createContext<ProjectContextValue | null>(null)

export function useProjectContext(): ProjectContextValue {
  const ctx = useContext(ProjectContext)
  if (!ctx) {
    throw new Error('useProjectContext must be used within a ProjectLayout route')
  }
  return ctx
}
