-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.recent_activities (
    id              BIGSERIAL    PRIMARY KEY,
    group_id        BIGINT       NOT NULL,
    lang            VARCHAR(10)  NOT NULL,
    occurred_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    activity_type   VARCHAR(50)  NOT NULL DEFAULT 'general',
    activity_text   TEXT         NOT NULL,
    created_by      VARCHAR(255),
    updated_by      VARCHAR(255),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_recent_activities_group_lang
    ON kb.recent_activities (group_id, lang);
CREATE INDEX IF NOT EXISTS idx_kb_recent_activities_occurred_at
    ON kb.recent_activities (occurred_at DESC);

COMMENT ON TABLE kb.recent_activities IS
    'Workspace recent-activity feed shown on /semos/workspace. Row-per-locale: rows sharing '
    'group_id are translations of the same logical activity entry, one row per lang.';

-- +goose Down
DROP INDEX IF EXISTS idx_kb_recent_activities_occurred_at;
DROP INDEX IF EXISTS idx_kb_recent_activities_group_lang;
DROP TABLE IF EXISTS kb.recent_activities;
