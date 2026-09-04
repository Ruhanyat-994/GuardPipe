-- +goose Up
-- BUILD_GUIDE.md Phase 15 follow-up (2026-09-02): a live, in-app invite
-- notification flow needs the invitee to be able to explicitly decline —
-- distinct from an org admin revoking it (organization.InviteRevoked),
-- so an org's pending-invites list can honestly say which happened.
-- Postgres 12+ allows ALTER TYPE ... ADD VALUE inside a transaction as
-- long as the new value isn't used in the same transaction, which this
-- migration doesn't do — safe as an ordinary goose migration.
ALTER TYPE invite_status ADD VALUE 'declined';

-- +goose Down
-- Postgres has no DROP VALUE for enums — reversing this cleanly would mean
-- recreating the whole type and every column using it, which risks data
-- loss for a value that may already be in use by the time anyone runs this
-- down migration. Deliberately a no-op; rolling back past this migration
-- while any invite has status 'declined' is not supported.
SELECT 1;
