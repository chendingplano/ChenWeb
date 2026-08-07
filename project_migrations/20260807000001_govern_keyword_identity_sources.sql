-- +goose Up

ALTER TABLE kb.keyword_surface_evidence
    ADD COLUMN release TEXT;

ALTER TABLE kb.keyword_sources
    ADD COLUMN provider_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_subset TEXT NOT NULL DEFAULT '',
    ADD COLUMN content_checksum TEXT NOT NULL DEFAULT '',
    ADD COLUMN license_review_status TEXT NOT NULL DEFAULT 'unreviewed',
    ADD COLUMN authority_role TEXT NOT NULL DEFAULT 'context_only',
    ADD COLUMN authoritative_relations TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN allowed_scopes TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN languages TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN adapter_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN provenance_locator TEXT NOT NULL DEFAULT '',
    ADD COLUMN approved_by TEXT,
    ADD COLUMN approved_at TIMESTAMPTZ,
    ADD COLUMN identity_authority BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE kb.keyword_surface_evidence SET release = '' WHERE release IS NULL;

INSERT INTO kb.keyword_sources (source, release, license, notes)
SELECT DISTINCT source, '', '', 'legacy keyword_surface_evidence placeholder backfilled by migration 20260807000001'
FROM kb.keyword_surface_evidence
ON CONFLICT (source, release) DO NOTHING;

INSERT INTO kb.keyword_sources (source, release, license, notes)
SELECT DISTINCT source, release, '', 'legacy keyword_external_ids placeholder backfilled by migration 20260807000001'
FROM kb.keyword_external_ids
ON CONFLICT (source, release) DO NOTHING;

ALTER TABLE kb.keyword_surface_evidence
    ALTER COLUMN release SET NOT NULL,
    ALTER COLUMN release SET DEFAULT '',
    DROP CONSTRAINT keyword_surface_evidence_surface_id_source_external_id_key,
    ADD CONSTRAINT uq_keyword_surface_evidence_source_external_release UNIQUE (surface_id, source, external_id, release);

DO $$
DECLARE
    evidence_orphans BIGINT;
    mapping_orphans BIGINT;
    blank_sources BIGINT;
    blank_external_ids BIGINT;
BEGIN
    SELECT count(*) INTO evidence_orphans
    FROM kb.keyword_surface_evidence e
    LEFT JOIN kb.keyword_sources s
      ON s.source = e.source AND s.release = e.release
    WHERE s.source IS NULL;

    SELECT count(*) INTO mapping_orphans
    FROM kb.keyword_external_ids x
    LEFT JOIN kb.keyword_sources s
      ON s.source = x.source AND s.release = x.release
    WHERE s.source IS NULL;

    IF evidence_orphans <> 0 OR mapping_orphans <> 0 THEN
        RAISE EXCEPTION 'keyword identity source/release orphans: evidence=%, external_ids=%',
            evidence_orphans, mapping_orphans;
    END IF;

    SELECT count(*) INTO blank_sources
    FROM kb.keyword_sources
    WHERE btrim(source) = '';

    SELECT count(*) INTO blank_external_ids
    FROM kb.keyword_external_ids
    WHERE btrim(external_id) = '';

    IF blank_sources <> 0 OR blank_external_ids <> 0 THEN
        RAISE EXCEPTION 'blank keyword identity keys: sources=%, external_ids=%',
            blank_sources, blank_external_ids;
    END IF;
END
$$;

ALTER TABLE kb.keyword_sources
    ADD CONSTRAINT ck_keyword_sources_nonblank_source CHECK (btrim(source) <> ''),
    ADD CONSTRAINT ck_keyword_sources_authority_role CHECK (
        authority_role IN ('exact_identity_authority', 'conditional_identity_authority', 'proposal_only', 'context_only')
    ),
    ADD CONSTRAINT ck_keyword_sources_license_review_status CHECK (
        license_review_status IN ('unreviewed', 'approved', 'rejected')
    ),
    ADD CONSTRAINT ck_keyword_sources_checksum CHECK (
        content_checksum = '' OR content_checksum ~ '^[0-9a-f]{64}$'
    ),
    ADD CONSTRAINT ck_keyword_sources_identity_authority_consistent CHECK (
        identity_authority = (
            authority_role = 'exact_identity_authority'
            AND license_review_status = 'approved'
            AND 'exact_equivalent' = ANY(authoritative_relations)
            AND approved_by IS NOT NULL AND btrim(approved_by) <> ''
            AND approved_at IS NOT NULL
        )
    );

ALTER TABLE kb.keyword_external_ids
    ADD CONSTRAINT ck_keyword_external_ids_nonblank_external_id CHECK (btrim(external_id) <> '');

ALTER TABLE kb.keyword_surface_evidence
    ADD CONSTRAINT fk_keyword_surface_evidence_source_release FOREIGN KEY (source, release)
        REFERENCES kb.keyword_sources (source, release) ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE kb.keyword_external_ids
    ADD CONSTRAINT fk_keyword_external_ids_source_release FOREIGN KEY (source, release)
        REFERENCES kb.keyword_sources (source, release) ON UPDATE RESTRICT ON DELETE RESTRICT;

CREATE TABLE kb.keyword_source_artifacts (
    source             TEXT NOT NULL,
    release            TEXT NOT NULL,
    artifact_id        TEXT NOT NULL,
    content_checksum   TEXT NOT NULL CHECK (content_checksum ~ '^[0-9a-f]{64}$'),
    media_type         TEXT NOT NULL DEFAULT '',
    provenance_locator TEXT NOT NULL,
    payload            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source, release, artifact_id),
    FOREIGN KEY (source, release) REFERENCES kb.keyword_sources (source, release)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE TABLE kb.keyword_catalog_entries (
    source             TEXT NOT NULL,
    release            TEXT NOT NULL,
    external_id        TEXT NOT NULL CHECK (btrim(external_id) <> ''),
    entry_status       TEXT NOT NULL DEFAULT '',
    provenance_locator TEXT NOT NULL DEFAULT '',
    native_payload     JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (source, release, external_id),
    FOREIGN KEY (source, release) REFERENCES kb.keyword_sources (source, release)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE TABLE kb.keyword_catalog_labels (
    source             TEXT NOT NULL,
    release            TEXT NOT NULL,
    external_id        TEXT NOT NULL,
    language           TEXT NOT NULL,
    label_role         TEXT NOT NULL,
    label               TEXT NOT NULL,
    provenance_locator TEXT NOT NULL DEFAULT '',
    native_payload     JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (source, release, external_id, language, label_role, label),
    FOREIGN KEY (source, release, external_id)
        REFERENCES kb.keyword_catalog_entries (source, release, external_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE TABLE kb.keyword_catalog_relations (
    source                      TEXT NOT NULL,
    release                     TEXT NOT NULL,
    subject_external_id         TEXT NOT NULL,
    relation                    TEXT NOT NULL,
    object_external_id          TEXT NOT NULL,
    provenance_locator          TEXT NOT NULL DEFAULT '',
    native_payload              JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (source, release, subject_external_id, relation, object_external_id),
    FOREIGN KEY (source, release, subject_external_id)
        REFERENCES kb.keyword_catalog_entries (source, release, external_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE TABLE kb.keyword_catalog_negative_decisions (
    source              TEXT NOT NULL,
    release             TEXT NOT NULL,
    subject_external_id TEXT NOT NULL,
    object_external_id  TEXT NOT NULL,
    relation            TEXT NOT NULL,
    reason              TEXT NOT NULL,
    provenance_locator  TEXT NOT NULL DEFAULT '',
    native_payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (source, release, subject_external_id, object_external_id, relation),
    FOREIGN KEY (source, release, subject_external_id)
        REFERENCES kb.keyword_catalog_entries (source, release, external_id)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE TABLE kb.keyword_ucum_codes (
    source             TEXT NOT NULL,
    release            TEXT NOT NULL,
    code               TEXT NOT NULL,
    print_symbol       TEXT NOT NULL DEFAULT '',
    dimension          TEXT NOT NULL DEFAULT '',
    provenance_locator TEXT NOT NULL DEFAULT '',
    native_payload     JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (source, release, code),
    FOREIGN KEY (source, release) REFERENCES kb.keyword_sources (source, release)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE TABLE kb.keyword_identity_deployments (
    deployment_key TEXT PRIMARY KEY,
    source         TEXT NOT NULL,
    release        TEXT NOT NULL,
    enabled        BOOLEAN NOT NULL,
    changed_by     TEXT NOT NULL CHECK (btrim(changed_by) <> ''),
    changed_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (source, release) REFERENCES kb.keyword_sources (source, release)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE TABLE kb.keyword_identity_deployment_history (
    history_id     BIGSERIAL PRIMARY KEY,
    deployment_key TEXT NOT NULL,
    source         TEXT NOT NULL,
    release        TEXT NOT NULL,
    enabled        BOOLEAN NOT NULL,
    action         TEXT NOT NULL CHECK (action IN ('activate', 'disable', 'rollback')),
    changed_by     TEXT NOT NULL CHECK (btrim(changed_by) <> ''),
    changed_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (source, release) REFERENCES kb.keyword_sources (source, release)
        ON UPDATE RESTRICT ON DELETE RESTRICT
);

CREATE FUNCTION kb.reject_keyword_identity_immutable_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% is immutable; register a new source release or snapshot', TG_TABLE_NAME;
END
$$;

CREATE TRIGGER keyword_sources_immutable
BEFORE UPDATE OR DELETE ON kb.keyword_sources
FOR EACH ROW EXECUTE FUNCTION kb.reject_keyword_identity_immutable_mutation();

CREATE TRIGGER keyword_source_artifacts_immutable
BEFORE UPDATE OR DELETE ON kb.keyword_source_artifacts
FOR EACH ROW EXECUTE FUNCTION kb.reject_keyword_identity_immutable_mutation();

CREATE TRIGGER keyword_catalog_entries_immutable
BEFORE UPDATE OR DELETE ON kb.keyword_catalog_entries
FOR EACH ROW EXECUTE FUNCTION kb.reject_keyword_identity_immutable_mutation();

CREATE TRIGGER keyword_catalog_labels_immutable
BEFORE UPDATE OR DELETE ON kb.keyword_catalog_labels
FOR EACH ROW EXECUTE FUNCTION kb.reject_keyword_identity_immutable_mutation();

CREATE TRIGGER keyword_catalog_relations_immutable
BEFORE UPDATE OR DELETE ON kb.keyword_catalog_relations
FOR EACH ROW EXECUTE FUNCTION kb.reject_keyword_identity_immutable_mutation();

CREATE TRIGGER keyword_catalog_negative_decisions_immutable
BEFORE UPDATE OR DELETE ON kb.keyword_catalog_negative_decisions
FOR EACH ROW EXECUTE FUNCTION kb.reject_keyword_identity_immutable_mutation();

CREATE TRIGGER keyword_ucum_codes_immutable
BEFORE UPDATE OR DELETE ON kb.keyword_ucum_codes
FOR EACH ROW EXECUTE FUNCTION kb.reject_keyword_identity_immutable_mutation();

CREATE TRIGGER keyword_identity_deployment_history_append_only
BEFORE UPDATE OR DELETE ON kb.keyword_identity_deployment_history
FOR EACH ROW EXECUTE FUNCTION kb.reject_keyword_identity_immutable_mutation();

-- +goose Down

DO $$
DECLARE
    collapsing_groups BIGINT;
BEGIN
    SELECT count(*) INTO collapsing_groups
    FROM (
        SELECT surface_id, source, external_id
        FROM kb.keyword_surface_evidence
        GROUP BY surface_id, source, external_id
        HAVING count(*) > 1
    ) duplicates;

    IF collapsing_groups <> 0 THEN
        RAISE EXCEPTION 'cannot drop release: % duplicate (surface_id, source, external_id) groups would collapse',
            collapsing_groups;
    END IF;
END
$$;

DROP TRIGGER keyword_identity_deployment_history_append_only ON kb.keyword_identity_deployment_history;
DROP TRIGGER keyword_ucum_codes_immutable ON kb.keyword_ucum_codes;
DROP TRIGGER keyword_catalog_negative_decisions_immutable ON kb.keyword_catalog_negative_decisions;
DROP TRIGGER keyword_catalog_relations_immutable ON kb.keyword_catalog_relations;
DROP TRIGGER keyword_catalog_labels_immutable ON kb.keyword_catalog_labels;
DROP TRIGGER keyword_catalog_entries_immutable ON kb.keyword_catalog_entries;
DROP TRIGGER keyword_source_artifacts_immutable ON kb.keyword_source_artifacts;
DROP TRIGGER keyword_sources_immutable ON kb.keyword_sources;

DROP TABLE kb.keyword_identity_deployment_history;
DROP TABLE kb.keyword_identity_deployments;
DROP TABLE kb.keyword_ucum_codes;
DROP TABLE kb.keyword_catalog_negative_decisions;
DROP TABLE kb.keyword_catalog_relations;
DROP TABLE kb.keyword_catalog_labels;
DROP TABLE kb.keyword_catalog_entries;
DROP TABLE kb.keyword_source_artifacts;
DROP FUNCTION kb.reject_keyword_identity_immutable_mutation();

ALTER TABLE kb.keyword_surface_evidence
    DROP CONSTRAINT fk_keyword_surface_evidence_source_release;
ALTER TABLE kb.keyword_external_ids
    DROP CONSTRAINT fk_keyword_external_ids_source_release;
ALTER TABLE kb.keyword_sources
    DROP CONSTRAINT ck_keyword_sources_nonblank_source,
    DROP CONSTRAINT ck_keyword_sources_authority_role,
    DROP CONSTRAINT ck_keyword_sources_license_review_status,
    DROP CONSTRAINT ck_keyword_sources_checksum,
    DROP CONSTRAINT ck_keyword_sources_identity_authority_consistent;
ALTER TABLE kb.keyword_external_ids
    DROP CONSTRAINT ck_keyword_external_ids_nonblank_external_id;
ALTER TABLE kb.keyword_surface_evidence
    DROP CONSTRAINT uq_keyword_surface_evidence_source_external_release,
    ADD CONSTRAINT keyword_surface_evidence_surface_id_source_external_id_key UNIQUE (surface_id, source, external_id),
    DROP COLUMN release;
ALTER TABLE kb.keyword_sources
    DROP COLUMN provider_id,
    DROP COLUMN source_subset,
    DROP COLUMN content_checksum,
    DROP COLUMN license_review_status,
    DROP COLUMN authority_role,
    DROP COLUMN authoritative_relations,
    DROP COLUMN allowed_scopes,
    DROP COLUMN languages,
    DROP COLUMN adapter_version,
    DROP COLUMN provenance_locator,
    DROP COLUMN approved_by,
    DROP COLUMN approved_at,
    DROP COLUMN identity_authority;

-- Migration-created keyword_sources placeholders are deliberately retained as provenance.
