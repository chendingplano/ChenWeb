-- +goose Up
CREATE TABLE IF NOT EXISTS site_tenants (
    tenant_id       VARCHAR(128) PRIMARY KEY,
    tenant_name     TEXT NOT NULL DEFAULT '',
    config_filename TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE site_tenants IS
    'Tenant registry for the SemOS customer-facing frontend (ADR 2026071102). '
    'config_filename is the per-tenant site-config TOML path relative to the ChenWeb repo root. '
    'Minimal placeholder shape; full RBAC/tenant schema deferred to a future auth ADR.';

-- +goose Down
DROP TABLE IF EXISTS site_tenants;
