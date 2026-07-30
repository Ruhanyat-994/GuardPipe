-- +goose Up
-- Extensions and enum types, declared once up front so every later
-- migration can reference them (documentation/06-database-design.md §3,
-- §11 planned migration sequence).

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TYPE user_role      AS ENUM ('admin', 'member', 'viewer');
CREATE TYPE project_status AS ENUM ('active', 'archived');
CREATE TYPE target_status  AS ENUM ('awaiting_attestation', 'attested', 'blocked', 'revoked');
CREATE TYPE scan_type      AS ENUM ('full_supply_chain', 'partial', 'pentest_only');
CREATE TYPE scan_status    AS ENUM ('queued', 'running', 'completed', 'failed', 'cancelled');
CREATE TYPE job_status     AS ENUM ('queued', 'running', 'succeeded', 'failed', 'skipped', 'cancelled');
CREATE TYPE engine_id      AS ENUM ('docreview', 'codescan', 'depscan', 'containerscan',
                                    'k8sscan', 'cicdscan', 'pentest');
-- severity is declared worst-first so `ORDER BY severity` sorts
-- critical -> informational with no CASE expression (§3 ordering note).
CREATE TYPE severity       AS ENUM ('critical', 'high', 'medium', 'low', 'informational');
CREATE TYPE confidence     AS ENUM ('high', 'medium', 'low');
CREATE TYPE finding_status AS ENUM ('open', 'acknowledged', 'suppressed', 'fixed', 'false_positive');
CREATE TYPE finding_source AS ENUM ('rule', 'ai');
CREATE TYPE verdict        AS ENUM ('pass', 'warn', 'block');
CREATE TYPE patch_status   AS ENUM ('verified', 'unverified', 'not_applicable');

-- +goose Down
DROP TYPE IF EXISTS patch_status;
DROP TYPE IF EXISTS verdict;
DROP TYPE IF EXISTS finding_source;
DROP TYPE IF EXISTS finding_status;
DROP TYPE IF EXISTS confidence;
DROP TYPE IF EXISTS severity;
DROP TYPE IF EXISTS engine_id;
DROP TYPE IF EXISTS job_status;
DROP TYPE IF EXISTS scan_status;
DROP TYPE IF EXISTS scan_type;
DROP TYPE IF EXISTS target_status;
DROP TYPE IF EXISTS project_status;
DROP TYPE IF EXISTS user_role;

DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS pgcrypto;
