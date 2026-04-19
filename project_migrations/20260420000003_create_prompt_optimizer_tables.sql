-- +goose Up

-- Models: per-user LLM provider configurations. api_key_encrypted stores
-- AES-256-GCM ciphertext (base64); the plaintext key never touches the DB.
CREATE TABLE IF NOT EXISTS prompt_optimizer_models (
    id                VARCHAR(64)  PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id           VARCHAR(64)  NOT NULL,
    name              TEXT         NOT NULL,
    provider          TEXT         NOT NULL,
    base_url          TEXT,
    model             TEXT         NOT NULL,
    api_key_encrypted TEXT         NOT NULL,
    default_params    JSONB        NOT NULL DEFAULT '{}'::jsonb,
    enabled           BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS prompt_optimizer_models_user_idx
    ON prompt_optimizer_models(user_id);

-- Templates: user-owned prompt templates. user_id IS NULL marks built-in
-- seeds loaded by the handler on first read.
CREATE TABLE IF NOT EXISTS prompt_optimizer_templates (
    id          VARCHAR(64)  PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id     VARCHAR(64),
    kind        TEXT         NOT NULL
        CHECK (kind IN ('optimize','iterate','test','analyze','user_optimize','image_optimize')),
    name        TEXT         NOT NULL,
    description TEXT,
    content     TEXT         NOT NULL,
    language    TEXT         NOT NULL DEFAULT 'en',
    variables   JSONB        NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS prompt_optimizer_templates_user_idx
    ON prompt_optimizer_templates(user_id);
CREATE INDEX IF NOT EXISTS prompt_optimizer_templates_kind_idx
    ON prompt_optimizer_templates(kind);

-- History: every optimize/iterate/test/analyze run.
CREATE TABLE IF NOT EXISTS prompt_optimizer_history (
    id               VARCHAR(64)  PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id          VARCHAR(64)  NOT NULL,
    kind             TEXT         NOT NULL
        CHECK (kind IN ('optimize','iterate','test','analyze')),
    original_prompt  TEXT,
    optimized_prompt TEXT,
    test_input       TEXT,
    test_output      TEXT,
    template_id      VARCHAR(64)
        REFERENCES prompt_optimizer_templates(id) ON DELETE SET NULL,
    model_id         VARCHAR(64)
        REFERENCES prompt_optimizer_models(id) ON DELETE SET NULL,
    params           JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS prompt_optimizer_history_user_idx
    ON prompt_optimizer_history(user_id);
CREATE INDEX IF NOT EXISTS prompt_optimizer_history_created_idx
    ON prompt_optimizer_history(created_at DESC);

-- Favorites: pins over history rows or templates.
CREATE TABLE IF NOT EXISTS prompt_optimizer_favorites (
    id         VARCHAR(64)  PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id    VARCHAR(64)  NOT NULL,
    ref_kind   TEXT         NOT NULL CHECK (ref_kind IN ('history','template')),
    ref_id     VARCHAR(64)  NOT NULL,
    note       TEXT,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, ref_kind, ref_id)
);

-- Variables: named values substituted into templates at run time.
CREATE TABLE IF NOT EXISTS prompt_optimizer_variables (
    id         VARCHAR(64)  PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id    VARCHAR(64)  NOT NULL,
    name       TEXT         NOT NULL,
    value      TEXT         NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, name)
);

-- +goose Down
DROP TABLE IF EXISTS prompt_optimizer_variables;
DROP TABLE IF EXISTS prompt_optimizer_favorites;
DROP TABLE IF EXISTS prompt_optimizer_history;
DROP TABLE IF EXISTS prompt_optimizer_templates;
DROP TABLE IF EXISTS prompt_optimizer_models;
