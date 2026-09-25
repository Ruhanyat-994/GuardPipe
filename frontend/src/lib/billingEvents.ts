import { useBillingStore } from '../stores/billingStore'

/** Call after anything that spends or adds tokens (scan started, job
 * refunded, checkout paid) so the token bar animates right away instead of
 * waiting for its next poll. */
export function notifyBillingChanged(): void {
  void useBillingStore.getState().refresh()
}
