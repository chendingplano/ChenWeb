-- +goose Up
CREATE TABLE IF NOT EXISTS kb.build_activities (
    id             BIGSERIAL PRIMARY KEY,
    build_number   TEXT NOT NULL,
    build_name     TEXT NOT NULL,
    executable     TEXT NOT NULL,
    target_machine TEXT NOT NULL,
    os_info        TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    activity       TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_build_activities_build_number
    ON kb.build_activities (build_number);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_build_activities_build_number;
DROP TABLE IF EXISTS kb.build_activities;
