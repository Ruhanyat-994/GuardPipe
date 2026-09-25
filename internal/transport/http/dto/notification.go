package dto

import (
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/notification"
)

// NotificationSettingsResponse matches `GET /me/notification-settings`.
type NotificationSettingsResponse struct {
	AccountEmail        string     `json:"account_email"`
	ReportEmail         string     `json:"report_email"`
	UsingAccountEmail   bool       `json:"using_account_email"`
	PendingEmail        *string    `json:"pending_email"`
	PendingExpiresAt    *time.Time `json:"pending_expires_at"`
	EmailOnScanComplete bool       `json:"email_on_scan_complete"`
	EmailOnLiveScan     bool       `json:"email_on_live_scan"`
	EmailEnabled        bool       `json:"email_enabled"`
}

func FromNotificationSettings(v *notification.SettingsView) NotificationSettingsResponse {
	return NotificationSettingsResponse{
		AccountEmail: v.AccountEmail, ReportEmail: v.ReportEmail, UsingAccountEmail: v.UsingAccountEmail,
		PendingEmail: v.PendingEmail, PendingExpiresAt: v.PendingExpiresAt,
		EmailOnScanComplete: v.EmailOnScanComplete, EmailOnLiveScan: v.EmailOnLiveScan,
		EmailEnabled: v.EmailEnabled,
	}
}

// UpdateNotificationSettingsRequest is `PUT /me/notification-settings` —
// every field optional. report_email "" means "use my account email".
type UpdateNotificationSettingsRequest struct {
	EmailOnScanComplete *bool   `json:"email_on_scan_complete"`
	EmailOnLiveScan     *bool   `json:"email_on_live_scan"`
	ReportEmail         *string `json:"report_email" validate:"omitempty,max=254"`
}

// VerifyReportEmailRequest is `POST /notification-settings/verify`.
type VerifyReportEmailRequest struct {
	Token string `json:"token" validate:"required,max=128"`
}

// VerifyReportEmailResponse names the address that was just confirmed.
type VerifyReportEmailResponse struct {
	ReportEmail string `json:"report_email"`
}

// TestEmailResponse names where the test email went.
type TestEmailResponse struct {
	SentTo string `json:"sent_to"`
}

// NotificationResponse is one entry of `GET /notifications`.
type NotificationResponse struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	ScanID    *string    `json:"scan_id"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// NotificationListResponse matches `GET /notifications`.
type NotificationListResponse struct {
	Data        []NotificationResponse `json:"data"`
	UnreadCount int                    `json:"unread_count"`
}

func FromNotifications(items []notification.Notification, unread int) NotificationListResponse {
	out := NotificationListResponse{Data: make([]NotificationResponse, len(items)), UnreadCount: unread}
	for i, n := range items {
		r := NotificationResponse{ID: n.ID.String(), Kind: n.Kind, Title: n.Title, Body: n.Body, ReadAt: n.ReadAt, CreatedAt: n.CreatedAt}
		if n.ScanID != nil {
			s := n.ScanID.String()
			r.ScanID = &s
		}
		out.Data[i] = r
	}
	return out
}
