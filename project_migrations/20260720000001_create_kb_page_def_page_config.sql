-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

-- One row per configurable frontend page. page_key is the stable identity used
-- by frontend, backend, and admin tooling to reference a page's config.
CREATE TABLE IF NOT EXISTS kb.page_def (
    id           BIGSERIAL    PRIMARY KEY,
    page_key     VARCHAR(128) NOT NULL,
    route        VARCHAR(255),
    title        VARCHAR(255),
    description  TEXT,
    created_by   VARCHAR(255),
    updated_by   VARCHAR(255),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_page_def_page_key
    ON kb.page_def (page_key);

COMMENT ON TABLE kb.page_def IS
    'One row per configurable frontend page. page_key is the stable page identity; '
    'page structure (menu tree, tile layout, icons, routes) stays page-owned in the '
    'frontend — this table only records identity and admin metadata.';

-- One row per configurable entry per language. The pair (page_key, entry_key)
-- is the stable entry identity; language differentiates translations. The
-- default-language row ([languages].default) is authoritative for accessible,
-- enabled, and access_role; non-default rows carry translated content only.
CREATE TABLE IF NOT EXISTS kb.page_config (
    id           BIGSERIAL    PRIMARY KEY,
    page_key     VARCHAR(128) NOT NULL,
    entry_key    VARCHAR(128) NOT NULL,
    language     VARCHAR(10)  NOT NULL,
    content      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    access_role  JSONB,
    accessible   BOOLEAN      NOT NULL DEFAULT true,
    enabled      BOOLEAN      NOT NULL DEFAULT true,
    created_by   VARCHAR(255),
    updated_by   VARCHAR(255),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_page_config_identity
    ON kb.page_config (page_key, entry_key, language);
CREATE INDEX IF NOT EXISTS idx_kb_page_config_page_lang
    ON kb.page_config (page_key, language);

COMMENT ON TABLE kb.page_config IS
    'One row per configurable page entry per language. Identity is '
    '(page_key, entry_key); language differentiates translations. content is a '
    'JSONB payload ({label, description}). access_role is a JSON array of '
    '[system].access_roles keys; accessible/enabled are explicit switches. The '
    'default-language row is authoritative for access_role/accessible/enabled.';

-- +goose Down
DROP TABLE IF EXISTS kb.page_config;
DROP TABLE IF EXISTS kb.page_def;
