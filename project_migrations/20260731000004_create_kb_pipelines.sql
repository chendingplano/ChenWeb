-- +goose Up
CREATE TABLE IF NOT EXISTS kb.pipelines (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL UNIQUE,
    display_name TEXT,
    processors TEXT[] NOT NULL DEFAULT '{}'::text[],
    legacy_equivalent BOOLEAN NOT NULL DEFAULT false,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seeded with the same three pipelines the P1 code-seeded registry shipped
-- with (defaultProductionPipelines in pipeline_selection.go), so authoring
-- this table does not change effective behavior: legacy_default stays the
-- legacy-equivalent system default. Idempotent so air re-running it is safe.
INSERT INTO kb.pipelines (name, display_name, legacy_equivalent) VALUES
    ('legacy_default', 'Legacy Default', true),
    ('store_default', 'Store Default', false),
    ('request_override', 'Request Override', false)
ON CONFLICT (name) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS kb.pipelines;
