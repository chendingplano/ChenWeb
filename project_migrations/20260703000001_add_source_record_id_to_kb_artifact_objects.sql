-- +goose Up
ALTER TABLE kb.artifact_objects
    ADD COLUMN IF NOT EXISTS source_record_id BIGINT;

UPDATE kb.artifact_objects
SET source_record_id = COALESCE(source_record_id, input_record_id)
WHERE source_record_id IS NULL;

ALTER TABLE kb.artifact_objects
    ALTER COLUMN source_record_id SET NOT NULL;

ALTER TABLE kb.artifact_objects
    DROP CONSTRAINT IF EXISTS artifact_objects_source_record_id_fkey;

ALTER TABLE kb.artifact_objects
    ADD CONSTRAINT artifact_objects_source_record_id_fkey
        FOREIGN KEY (source_record_id) REFERENCES kb.inputs(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_kb_artifact_objects_source_artifact
    ON kb.artifact_objects (source_record_id, artifact_type, artifact_id);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_artifact_objects_source_artifact;

ALTER TABLE kb.artifact_objects
    DROP CONSTRAINT IF EXISTS artifact_objects_source_record_id_fkey;

ALTER TABLE kb.artifact_objects
    DROP COLUMN IF EXISTS source_record_id;
