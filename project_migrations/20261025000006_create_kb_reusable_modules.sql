-- Registry of reusable modules (frontend components, backend packages, scripts).
-- Each row is documented by a doc in KnowledgeStore/doc-repo/modules/ and is
-- maintained by the create-reusable-module skill.

-- +goose Up
CREATE TABLE IF NOT EXISTS kb.reusable_modules (
    id            BIGSERIAL   PRIMARY KEY,
    module_id     TEXT        NOT NULL UNIQUE CHECK (module_id ~ '^[0-9]{8}-[0-9]{2}$'),
    module_name   TEXT        NOT NULL,
    module_type   TEXT        NOT NULL CHECK (module_type IN ('frontend', 'backend', 'script')),
    prog_language TEXT        NOT NULL,
    description   TEXT        NOT NULL DEFAULT '',
    files         JSONB       NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(files) = 'array'),
    version       TEXT        NOT NULL DEFAULT '1.0.0',
    config_json   JSONB       NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(config_json) = 'object'),
    status        TEXT        NOT NULL DEFAULT 'under-dev' CHECK (status IN ('active', 'under-dev', 'retired')),
    doc_path      TEXT        NOT NULL DEFAULT '',
    created_by    TEXT        NOT NULL DEFAULT '',
    updated_by    TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_reusable_modules_name ON kb.reusable_modules (module_name);
CREATE INDEX IF NOT EXISTS idx_reusable_modules_type_status ON kb.reusable_modules (module_type, status);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_reusable_modules_type_status;
DROP INDEX IF EXISTS kb.idx_reusable_modules_name;
DROP TABLE IF EXISTS kb.reusable_modules;
