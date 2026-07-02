-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.artifact_objects (
    id                   BIGSERIAL PRIMARY KEY,
    input_record_id      BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    artifact_type        TEXT        NOT NULL,
    artifact_id          TEXT        NOT NULL,
    object_id            TEXT,
    object_name          TEXT        NOT NULL DEFAULT '',
    object_name_en       TEXT,
    object_name_zh       TEXT,
    language             TEXT,
    object_type          TEXT        NOT NULL DEFAULT 'other',
    object_role          TEXT        NOT NULL DEFAULT 'other',
    aliases              JSONB       NOT NULL DEFAULT '[]'::jsonb,
    acronyms             JSONB       NOT NULL DEFAULT '[]'::jsonb,
    normalized_names     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    description          TEXT,
    evidence_quote       TEXT,
    source_line_spans    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    confidence           DOUBLE PRECISION NOT NULL DEFAULT 0,
    reconcile_status     TEXT        NOT NULL DEFAULT 'pending',
    reconcile_confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    ext_info             JSONB       NOT NULL DEFAULT '{}'::jsonb,
    create_time          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_kb_artifact_objects_reconcile_status
        CHECK (reconcile_status IN ('pending', 'matched', 'new', 'ambiguous', 'rejected'))
);

CREATE INDEX IF NOT EXISTS idx_kb_artifact_objects_artifact
    ON kb.artifact_objects (input_record_id, artifact_type, artifact_id);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_objects_object_id
    ON kb.artifact_objects (object_id);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_objects_type_name
    ON kb.artifact_objects (artifact_type, object_name);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_objects_normalized_names
    ON kb.artifact_objects USING GIN (normalized_names);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.touch_artifact_objects_modify_time()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.modify_time := NOW();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_touch_artifact_objects_modify_time ON kb.artifact_objects;
CREATE TRIGGER trg_touch_artifact_objects_modify_time
BEFORE UPDATE ON kb.artifact_objects
FOR EACH ROW
EXECUTE FUNCTION kb.touch_artifact_objects_modify_time();

-- +goose Down
DROP TRIGGER IF EXISTS trg_touch_artifact_objects_modify_time ON kb.artifact_objects;
DROP FUNCTION IF EXISTS kb.touch_artifact_objects_modify_time();
DROP INDEX IF EXISTS kb.idx_kb_artifact_objects_normalized_names;
DROP INDEX IF EXISTS kb.idx_kb_artifact_objects_type_name;
DROP INDEX IF EXISTS kb.idx_kb_artifact_objects_object_id;
DROP INDEX IF EXISTS kb.idx_kb_artifact_objects_artifact;
DROP TABLE IF EXISTS kb.artifact_objects;
