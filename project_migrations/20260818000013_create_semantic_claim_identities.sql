-- +goose Up
-- ADR 2026081701 DR5: canonical claim identity is versioned independently of
-- source occurrences and assertion revisions. Claim IDs stay stable while
-- later key-version cutovers run in explicit shadow mode.

CREATE TABLE IF NOT EXISTS kb.semantic_canonical_key_versions (
    key_version         TEXT PRIMARY KEY,
    serializer_name     TEXT NOT NULL,
    serializer_version  TEXT NOT NULL,
    status              TEXT NOT NULL CHECK (status IN ('draft', 'shadow', 'active', 'retired')),
    definition          JSONB NOT NULL DEFAULT '{}'::jsonb,
    create_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by           TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_semantic_canonical_key_versions_active
    ON kb.semantic_canonical_key_versions ((status = 'active'))
    WHERE status = 'active';

CREATE TABLE IF NOT EXISTS kb.semantic_claim_identities (
    claim_id            TEXT PRIMARY KEY CHECK (BTRIM(claim_id) <> ''),
    key_version         TEXT NOT NULL REFERENCES kb.semantic_canonical_key_versions (key_version),
    canonical_key       BYTEA NOT NULL,
    class_term_id       TEXT REFERENCES kb.ontology_term_headers (term_id),
    identity_payload    JSONB NOT NULL DEFAULT '{}'::jsonb,
    create_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by           TEXT,
    UNIQUE (key_version, canonical_key)
);

CREATE INDEX IF NOT EXISTS idx_kb_semantic_claim_identities_class
    ON kb.semantic_claim_identities (class_term_id) WHERE class_term_id IS NOT NULL;

CREATE OR REPLACE FUNCTION kb.reject_semantic_claim_identity_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'semantic claim identity is immutable';
END;
$$;

CREATE TRIGGER kb_semantic_claim_identities_immutable
BEFORE UPDATE OR DELETE ON kb.semantic_claim_identities
FOR EACH ROW EXECUTE FUNCTION kb.reject_semantic_claim_identity_mutation();

-- +goose Down
DROP TRIGGER IF EXISTS kb_semantic_claim_identities_immutable ON kb.semantic_claim_identities;
DROP FUNCTION IF EXISTS kb.reject_semantic_claim_identity_mutation();
DROP TABLE IF EXISTS kb.semantic_claim_identities;
DROP TABLE IF EXISTS kb.semantic_canonical_key_versions;
