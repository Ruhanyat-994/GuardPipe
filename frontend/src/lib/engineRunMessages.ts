/**
 * Human-readable copy for the machine-coded `error_reason` strings the
 * orchestrator persists (internal/modules/orchestrator/worker.go's `reason`
 * values — "engine_not_registered", "workspace_unavailable",
 * "credential_invalid", "timeout", "engine_error"). `skip_reason` needs no
 * equivalent table: every engine's own `Applicable()` (domain/engine.go)
 * already returns a full, human-readable sentence, not a machine code — see
 * e.g. containerscan's "no Dockerfile found".
 */
const ERROR_REASON_MESSAGES: Record<string, string> = {
  engine_not_registered: "This check isn't available in this build yet.",
  workspace_unavailable: "The project's repository couldn't be prepared for scanning.",
  credential_invalid:
    'The stored repository credential is no longer valid — check the project settings.',
  timeout: 'This check took longer than its time budget and was stopped.',
  engine_error: 'This check hit an unexpected error while running.',
}

export function humanizeErrorReason(reason: string | null | undefined): string {
  if (!reason) return 'This check failed to complete.'
  return ERROR_REASON_MESSAGES[reason] ?? reason.replace(/_/g, ' ')
}
