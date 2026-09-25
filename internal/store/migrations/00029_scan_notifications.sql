-- +goose Up
-- Scan-completion notifications: an in-app feed (the bell) and an emailed
-- PDF report (modules/notification).
--
-- user_notification_settings: one row per user, created lazily the first
-- time they change anything — a user with no row gets the defaults (email
-- on, to their account address; live-scan emails off). report_email is the
-- *verified* address reports go to (NULL = the account email). A new
-- address first sits in pending_email until the user clicks the link sent
-- to it: without that step, anyone who got into an account could quietly
-- redirect every future vulnerability report to an outside mailbox. Only
-- the SHA-256 of the verification token is stored.
CREATE TABLE user_notification_settings (
    user_id               UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    report_email          CITEXT,
    pending_email         CITEXT,
    pending_token_hash    BYTEA,
    pending_expires_at    TIMESTAMPTZ,
    email_on_scan_complete BOOLEAN NOT NULL DEFAULT true,
    -- Off by default: every push can start a live scan.
    email_on_live_scan    BOOLEAN NOT NULL DEFAULT false,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT user_notification_settings_pending_complete CHECK (
        (pending_email IS NULL AND pending_token_hash IS NULL AND pending_expires_at IS NULL)
        OR (pending_email IS NOT NULL AND pending_token_hash IS NOT NULL AND pending_expires_at IS NOT NULL)
    )
);
CREATE UNIQUE INDEX user_notification_settings_token_idx
    ON user_notification_settings (pending_token_hash) WHERE pending_token_hash IS NOT NULL;

-- scan_report_emails: the outbox. The worker that finishes a scan only
-- inserts a row here; a separate sender (notification.Sender) builds the PDF
-- and sends it, retrying with backoff — so a slow or broken mail provider
-- can never hold up a scan. UNIQUE (scan_id, user_id) makes the enqueue
-- idempotent: a scan is reported to a given user at most once.
-- recipient is resolved at enqueue time and kept as the record of where the
-- report actually went.
CREATE TABLE scan_report_emails (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scan_id         UUID NOT NULL REFERENCES scans (id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    org_id          UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    recipient       CITEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'sending', 'sent', 'failed')),
    attempts        INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error      TEXT,
    sent_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT scan_report_emails_once UNIQUE (scan_id, user_id)
);
CREATE INDEX scan_report_emails_due_idx
    ON scan_report_emails (next_attempt_at) WHERE status IN ('pending', 'sending');

-- notifications: the bell's persistent feed. Scoped to user AND org — a
-- member of several orgs only sees the ones for the org their session is
-- currently in, like every other org-scoped read. kind is TEXT + CHECK
-- (expected to grow).
CREATE TABLE notifications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    org_id      UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('scan_completed', 'scan_failed', 'scan_cancelled')),
    scan_id     UUID REFERENCES scans (id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    body        TEXT NOT NULL,
    read_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT notifications_scan_once UNIQUE (scan_id, user_id)
);
CREATE INDEX notifications_user_org_idx ON notifications (user_id, org_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS scan_report_emails;
DROP TABLE IF EXISTS user_notification_settings;
