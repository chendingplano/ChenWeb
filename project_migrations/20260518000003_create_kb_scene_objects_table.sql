-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS kb.scene_objects (
    id               BIGSERIAL PRIMARY KEY,
    object_id        TEXT        NOT NULL,
    input_record_id  BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    event_id         TEXT        NOT NULL DEFAULT '',
    scene_id         TEXT        NOT NULL DEFAULT '',
    scene_type       TEXT        NOT NULL DEFAULT '',
    title            TEXT        NOT NULL DEFAULT '',
    summary          TEXT        NOT NULL DEFAULT '',
    actors           JSONB       NOT NULL DEFAULT '[]'::jsonb,
    resources        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    preconditions    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    triggers         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    states           JSONB       NOT NULL DEFAULT '[]'::jsonb,
    actions          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    constraints      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    decisions        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    outcomes         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    failure_modes    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    root_causes      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    resolutions      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    relationships    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    discriminators   JSONB       NOT NULL DEFAULT '[]'::jsonb,
    keywords         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    confidence       DOUBLE PRECISION NOT NULL DEFAULT 0,
    source_refs      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    model_name       TEXT        NOT NULL DEFAULT '',
    prompt_name      TEXT        NOT NULL DEFAULT '',
    ext_info         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    create_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kb_scene_objects_input_object_id UNIQUE (input_record_id, object_id)
);
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_kb_scene_objects_input_record_id
    ON kb.scene_objects(input_record_id);

CREATE INDEX IF NOT EXISTS idx_kb_scene_objects_event_id
    ON kb.scene_objects(event_id) WHERE event_id != '';

CREATE INDEX IF NOT EXISTS idx_kb_scene_objects_keywords_gin
    ON kb.scene_objects USING GIN (keywords);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_scene_objects_keywords_gin;
DROP INDEX IF EXISTS kb.idx_kb_scene_objects_event_id;
DROP INDEX IF EXISTS kb.idx_kb_scene_objects_input_record_id;
DROP TABLE IF EXISTS kb.scene_objects;
