-- server/migrations/20260313120000_create_flows.sql
-- +goose Up
CREATE TABLE IF NOT EXISTS flows (
    flow_id           BIGSERIAL PRIMARY KEY,
    user_id           BIGINT NOT NULL,
    flow_name         VARCHAR(255) NOT NULL,
    flow_desc         TEXT,
    is_default        BOOLEAN NOT NULL DEFAULT FALSE,
    is_shared         BOOLEAN NOT NULL DEFAULT FALSE,
    is_template       BOOLEAN NOT NULL DEFAULT FALSE,
    template_category VARCHAR(100),
    flow_data         JSONB NOT NULL DEFAULT '{"nodes":[],"edges":[]}',
    thumbnail_svg     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS flows_user_default_idx
    ON flows (user_id) WHERE is_default = TRUE;

-- +goose Down
DROP INDEX IF EXISTS flows_user_default_idx;
DROP TABLE IF EXISTS flows;
