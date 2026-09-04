-- +goose Up
-- BUILD_GUIDE.md Phase 15. Reserved, unused schema — this migration
-- deliberately ships no application logic. Google OIDC / domain-restricted
-- SSO was pulled out of this pass at the user's explicit request (basic
-- password-based org membership only, for now); these tables exist purely
-- so adding real SSO later is a config/logic change, not a schema
-- migration — the same "reserve the column now" reasoning BUILD_GUIDE.md
-- already applies to the SAML columns below. Nothing in this codebase
-- reads or writes these tables yet.

CREATE TABLE organization_domains (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    domain              TEXT NOT NULL,
    verification_token  TEXT NOT NULL,
    verified_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, domain)
);

CREATE TABLE organization_sso_settings (
    org_id                              UUID PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
    google_sso_enabled                  BOOLEAN NOT NULL DEFAULT false,
    restrict_signup_to_verified_domain  BOOLEAN NOT NULL DEFAULT false,
    -- Reserved for the Stretch SAML item (documentation's Phase 15 note) —
    -- Okta/Entra ID/OneLogin/Workspace-as-SAML-IdP. NULL/false until built.
    sso_provider                        TEXT,
    saml_metadata_url                   TEXT,
    saml_enabled                        BOOLEAN NOT NULL DEFAULT false,
    updated_at                          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS organization_sso_settings;
DROP TABLE IF EXISTS organization_domains;
