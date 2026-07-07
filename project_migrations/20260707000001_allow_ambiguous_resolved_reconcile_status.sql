-- +goose Up
ALTER TABLE kb.artifact_objects
    DROP CONSTRAINT IF EXISTS chk_kb_artifact_objects_reconcile_status;

ALTER TABLE kb.artifact_objects
    ADD CONSTRAINT chk_kb_artifact_objects_reconcile_status
        CHECK (reconcile_status IN ('pending', 'matched', 'new', 'ambiguous', 'ambiguous_resolved', 'rejected'));

-- +goose Down
ALTER TABLE kb.artifact_objects
    DROP CONSTRAINT IF EXISTS chk_kb_artifact_objects_reconcile_status;

ALTER TABLE kb.artifact_objects
    ADD CONSTRAINT chk_kb_artifact_objects_reconcile_status
        CHECK (reconcile_status IN ('pending', 'matched', 'new', 'ambiguous', 'rejected'));
