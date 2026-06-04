-- +goose Up
ALTER TABLE kb.artifact_connections
    ADD COLUMN IF NOT EXISTS source_record_id BIGINT,
    ADD COLUMN IF NOT EXISTS target_record_id BIGINT,
    ADD COLUMN IF NOT EXISTS source_desc TEXT,
    ADD COLUMN IF NOT EXISTS target_desc TEXT;

UPDATE kb.artifact_connections
SET
    source_record_id = COALESCE(source_record_id, input_record_id),
    target_record_id = COALESCE(target_record_id, input_record_id),
    source_desc = COALESCE(NULLIF(source_desc, ''), source_type || ':' || source_id),
    target_desc = COALESCE(NULLIF(target_desc, ''), target_type || ':' || target_id)
WHERE source_record_id IS NULL
   OR target_record_id IS NULL
   OR source_desc IS NULL
   OR source_desc = ''
   OR target_desc IS NULL
   OR target_desc = '';

ALTER TABLE kb.artifact_connections
    ALTER COLUMN source_record_id SET NOT NULL,
    ALTER COLUMN target_record_id SET NOT NULL,
    ALTER COLUMN source_desc SET NOT NULL,
    ALTER COLUMN target_desc SET NOT NULL;

ALTER TABLE kb.artifact_connections
    DROP CONSTRAINT IF EXISTS artifact_connections_input_record_id_fkey;

ALTER TABLE kb.artifact_connections
    ADD CONSTRAINT artifact_connections_source_record_id_fkey
        FOREIGN KEY (source_record_id) REFERENCES kb.inputs(id) ON DELETE CASCADE,
    ADD CONSTRAINT artifact_connections_target_record_id_fkey
        FOREIGN KEY (target_record_id) REFERENCES kb.inputs(id) ON DELETE CASCADE;

DROP INDEX IF EXISTS kb.idx_kb_artifact_connections_record;
DROP INDEX IF EXISTS kb.idx_kb_artifact_connections_source;
DROP INDEX IF EXISTS kb.idx_kb_artifact_connections_target;
DROP INDEX IF EXISTS kb.idx_kb_artifact_connections_relation;

CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_source_record
    ON kb.artifact_connections (source_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_target_record
    ON kb.artifact_connections (target_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_source
    ON kb.artifact_connections (source_record_id, source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_target
    ON kb.artifact_connections (target_record_id, target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_relation
    ON kb.artifact_connections (source_record_id, relation_name);

ALTER TABLE kb.artifact_connections
    DROP COLUMN IF EXISTS input_record_id;

-- +goose Down
ALTER TABLE kb.artifact_connections
    ADD COLUMN IF NOT EXISTS input_record_id BIGINT;

UPDATE kb.artifact_connections
SET input_record_id = COALESCE(input_record_id, source_record_id, target_record_id)
WHERE input_record_id IS NULL;

ALTER TABLE kb.artifact_connections
    ALTER COLUMN input_record_id SET NOT NULL;

ALTER TABLE kb.artifact_connections
    DROP CONSTRAINT IF EXISTS artifact_connections_source_record_id_fkey,
    DROP CONSTRAINT IF EXISTS artifact_connections_target_record_id_fkey,
    ADD CONSTRAINT artifact_connections_input_record_id_fkey
        FOREIGN KEY (input_record_id) REFERENCES kb.inputs(id) ON DELETE CASCADE;

DROP INDEX IF EXISTS kb.idx_kb_artifact_connections_source_record;
DROP INDEX IF EXISTS kb.idx_kb_artifact_connections_target_record;
DROP INDEX IF EXISTS kb.idx_kb_artifact_connections_source;
DROP INDEX IF EXISTS kb.idx_kb_artifact_connections_target;
DROP INDEX IF EXISTS kb.idx_kb_artifact_connections_relation;

CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_record
    ON kb.artifact_connections (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_source
    ON kb.artifact_connections (input_record_id, source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_target
    ON kb.artifact_connections (input_record_id, target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_connections_relation
    ON kb.artifact_connections (input_record_id, relation_name);

ALTER TABLE kb.artifact_connections
    DROP COLUMN IF EXISTS source_record_id,
    DROP COLUMN IF EXISTS target_record_id,
    DROP COLUMN IF EXISTS source_desc,
    DROP COLUMN IF EXISTS target_desc;
