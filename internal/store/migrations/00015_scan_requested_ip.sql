-- +goose Up
-- Accountability for pentest/scan usage (documentation/06-database-design.md
-- §4.9's `scans` table gains one column): `scans.triggered_by` already names
-- who queued a scan, but not from where. target_attestations.source_ip
-- (migration 00004) already does this for the one-time "I own this target"
-- attestation; this is the same idea applied per scan-execution, since a
-- second member of the same org could trigger a scan against an
-- already-attested target later, and export reports (BUILD_GUIDE.md Phase
-- 13) need a per-scan actor+IP stamp to show as the report's own
-- accountability/responsibility watermark, not just the target's one-time
-- attestation record.
ALTER TABLE scans ADD COLUMN requested_ip INET;

-- +goose Down
ALTER TABLE scans DROP COLUMN IF EXISTS requested_ip;
