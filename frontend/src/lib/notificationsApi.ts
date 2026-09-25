/**
 * Typed wrappers for the scan-completion notification feed (the bell) and
 * each user's own report-email settings — modules/notification on the
 * backend.
 */

import { apiClient } from './apiClient'

export type NotificationKind = 'scan_completed' | 'scan_failed' | 'scan_cancelled'

export interface AppNotification {
  id: string
  kind: NotificationKind
  scan_id: string | null
  title: string
  body: string
  read_at: string | null
  created_at: string
}

export interface NotificationList {
  data: AppNotification[]
  unread_count: number
}

export function listNotifications(): Promise<NotificationList> {
  return apiClient.get<NotificationList>('/notifications')
}

export function markNotificationRead(id: string): Promise<void> {
  return apiClient.post<void>(`/notifications/${id}/read`)
}

export function markAllNotificationsRead(): Promise<void> {
  return apiClient.post<void>('/notifications/read-all')
}

export interface NotificationSettings {
  account_email: string
  // Where reports go right now: the verified override, or the account email.
  report_email: string
  using_account_email: boolean
  // A new address waiting for its verification link to be clicked.
  pending_email: string | null
  pending_expires_at: string | null
  email_on_scan_complete: boolean
  email_on_live_scan: boolean
  // false when the server has no mail backend configured at all.
  email_enabled: boolean
}

export interface UpdateNotificationSettingsInput {
  email_on_scan_complete?: boolean
  email_on_live_scan?: boolean
  // "" switches back to the account email.
  report_email?: string
}

export function getNotificationSettings(): Promise<NotificationSettings> {
  return apiClient.get<NotificationSettings>('/me/notification-settings')
}

export function updateNotificationSettings(
  input: UpdateNotificationSettingsInput,
): Promise<NotificationSettings> {
  return apiClient.put<NotificationSettings>('/me/notification-settings', input)
}

export function resendReportEmailVerification(): Promise<void> {
  return apiClient.post<void>('/me/notification-settings/resend-verification')
}

export function sendTestEmail(): Promise<{ sent_to: string }> {
  return apiClient.post<{ sent_to: string }>('/me/notification-settings/test')
}

export function verifyReportEmail(token: string): Promise<{ report_email: string }> {
  return apiClient.post<{ report_email: string }>('/notification-settings/verify', { token })
}
