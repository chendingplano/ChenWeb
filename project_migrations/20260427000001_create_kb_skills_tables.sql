-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.skill_categories (
    id          BIGSERIAL    PRIMARY KEY,
    "name"      VARCHAR(256) NOT NULL,
    "level"     INT          NOT NULL DEFAULT 0,
    parent_id   BIGINT       REFERENCES kb.skill_categories(id) ON DELETE SET NULL,
    "path"      TEXT[]       NOT NULL DEFAULT '{}'::text[],
    description TEXT,
    create_time TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_skill_categories_parent_id
    ON kb.skill_categories(parent_id);

CREATE INDEX IF NOT EXISTS idx_kb_skill_categories_level
    ON kb.skill_categories("level");

CREATE TABLE IF NOT EXISTS kb.skills (
    id          BIGSERIAL    PRIMARY KEY,
    "name"      VARCHAR(256) NOT NULL,
    description TEXT,
    category    TEXT[]       NOT NULL DEFAULT '{}'::text[],
    status      VARCHAR(64),
    tags        TEXT[]       NOT NULL DEFAULT '{}'::text[],
    version     VARCHAR(128),
    notes       TEXT,
    public_info JSONB,
    create_time TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_skills_status
    ON kb.skills(status);

CREATE INDEX IF NOT EXISTS idx_kb_skills_modify_time
    ON kb.skills(modify_time DESC);

-- +goose Down
DROP TABLE IF EXISTS kb.skills;
DROP INDEX IF EXISTS idx_kb_skill_categories_level;
DROP INDEX IF EXISTS idx_kb_skill_categories_parent_id;
DROP TABLE IF EXISTS kb.skill_categories;
