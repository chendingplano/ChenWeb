-- +goose Up
-- Profile versions are append-only snapshots. Changes create a new version;
-- mutable activation state lives in agentic_service_profiles.
CREATE TABLE IF NOT EXISTS kb.agentic_service_profile_versions (
    service_slug    TEXT NOT NULL,
    version         TEXT NOT NULL,
    profile_json    JSONB NOT NULL,
    prompt_ref      TEXT NOT NULL,
    prompt_hash     TEXT NOT NULL,
    prompt_snapshot TEXT NOT NULL,
    created_by      TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (service_slug, version),
    CHECK (BTRIM(service_slug) <> ''),
    CHECK (BTRIM(version) <> ''),
    CHECK (BTRIM(prompt_ref) <> ''),
    CHECK (BTRIM(prompt_hash) <> ''),
    CHECK (BTRIM(created_by) <> '')
);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.reject_agentic_service_profile_version_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'agentic service profile versions are immutable';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_agentic_service_profile_versions_immutable
BEFORE UPDATE OR DELETE ON kb.agentic_service_profile_versions
FOR EACH ROW EXECUTE FUNCTION kb.reject_agentic_service_profile_version_mutation();

CREATE TABLE IF NOT EXISTS kb.agentic_service_profiles (
    service_slug   TEXT PRIMARY KEY,
    active_version TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (BTRIM(service_slug) <> ''),
    FOREIGN KEY (service_slug, active_version)
        REFERENCES kb.agentic_service_profile_versions(service_slug, version)
);

-- +goose Down
DROP TABLE IF EXISTS kb.agentic_service_profiles;
DROP TRIGGER IF EXISTS trg_agentic_service_profile_versions_immutable ON kb.agentic_service_profile_versions;
DROP FUNCTION IF EXISTS kb.reject_agentic_service_profile_version_mutation();
DROP TABLE IF EXISTS kb.agentic_service_profile_versions;
