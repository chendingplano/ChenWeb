-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.site_announcements (
    id                  BIGSERIAL    PRIMARY KEY,
    group_id            BIGINT       NOT NULL,
    lang                VARCHAR(10)  NOT NULL,
    occurred_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    importance          VARCHAR(20)  NOT NULL DEFAULT 'normal',
    announcement_text   TEXT         NOT NULL,
    created_by          VARCHAR(255),
    updated_by          VARCHAR(255),
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_site_announcements_group_lang
    ON kb.site_announcements (group_id, lang);
CREATE INDEX IF NOT EXISTS idx_kb_site_announcements_occurred_at
    ON kb.site_announcements (occurred_at DESC);

COMMENT ON TABLE kb.site_announcements IS
    'Workspace announcements shown on /semos/workspace. Row-per-locale: rows sharing '
    'group_id are translations of the same logical announcement, one row per lang.';

-- Carry over the announcement previously hardcoded in
-- config/site/site-default.toml / site-default-zh-cn.toml [workspace].announcements,
-- which this migration's feature (workspace-lists-live-data) replaces.
INSERT INTO kb.site_announcements (group_id, lang, occurred_at, importance, announcement_text)
VALUES
    (1, 'en', NOW(), 'normal', 'Welcome to your SemOS workspace.'),
    (1, 'zh-cn', NOW(), 'normal', '欢迎使用 SemOS 工作台。')
ON CONFLICT (group_id, lang) DO NOTHING;

-- +goose Down
DROP INDEX IF EXISTS idx_kb_site_announcements_occurred_at;
DROP INDEX IF EXISTS idx_kb_site_announcements_group_lang;
DROP TABLE IF EXISTS kb.site_announcements;
