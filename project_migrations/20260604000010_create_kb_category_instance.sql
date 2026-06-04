-- +goose Up
-- +goose StatementBegin
ALTER TABLE kb.artifact_categories
    ADD CONSTRAINT uq_kb_artifact_categories_category_id UNIQUE (category_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS kb.category_instance (
    category_id     BIGINT      NOT NULL REFERENCES kb.artifact_categories(category_id) ON DELETE CASCADE,
    artifact_id     TEXT        NOT NULL,
    input_record_id BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    extra_info      JSONB       NULL,
    create_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_kb_category_instance PRIMARY KEY (category_id, artifact_id)
);
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_kb_category_instance_artifact_id     ON kb.category_instance (artifact_id);
CREATE INDEX IF NOT EXISTS idx_kb_category_instance_input_record_id ON kb.category_instance (input_record_id);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_category_instance_input_record_id;
DROP INDEX IF EXISTS idx_kb_category_instance_artifact_id;
DROP TABLE IF EXISTS kb.category_instance;
ALTER TABLE kb.artifact_categories
    DROP CONSTRAINT IF EXISTS uq_kb_artifact_categories_category_id;
