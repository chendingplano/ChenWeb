-- +goose Up
-- Evolve kb.artifact_categories to support the category resolution design
-- (see KnowledgeStore/Capsules/coding-capsules/categories/category-mgmt-spec.md):
--   * surrogate primary key on category_id (was category_key)
--   * uniqueness scoped to (category_type, category_key) -- the upsert conflict target
--   * ontology columns: aliases, acronyms, category_desc, category_keywords
--   * match_keys (deterministic exact/alias lookup) -- search_document already exists
--   * GIN indexes for the exact/alias and hybrid (lexical) match channels
--
-- kb.category_instance already enforces UNIQUE(category_id, artifact_id) via its
-- PRIMARY KEY (pk_kb_category_instance), so no change is needed there.

-- New ontology / lookup columns.
ALTER TABLE kb.artifact_categories
    ADD COLUMN IF NOT EXISTS aliases           JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS acronyms          JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS category_desc     TEXT  NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS category_keywords JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS match_keys        JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Seed match_keys for existing rows with the normalized category_key so they
-- remain resolvable via the deterministic exact/alias layer.
UPDATE kb.artifact_categories
SET    match_keys = jsonb_build_array(regexp_replace(lower(btrim(category_key)), '\s+', ' ', 'g'))
WHERE  match_keys = '[]'::jsonb;

-- Uniqueness scoped to (category_type, category_key). Every category_key is
-- currently globally unique (it is the PK), so these pairs are already unique.
ALTER TABLE kb.artifact_categories
    ADD CONSTRAINT artifact_categories_type_key_uniq UNIQUE (category_type, category_key);

-- Swap the primary key from category_key to the surrogate category_id.
-- The current PK constraint name varies by environment (the table was renamed
-- from kb.categories, so live DBs carry the legacy name `categories_pkey`),
-- so drop whatever PK currently exists rather than a hard-coded name.
-- The FK from kb.category_instance depends on uq_kb_artifact_categories_category_id,
-- which is left in place; the new PK is added alongside it.
-- +goose StatementBegin
DO $$
DECLARE pk_name text;
BEGIN
    SELECT conname INTO pk_name
    FROM   pg_constraint
    WHERE  conrelid = 'kb.artifact_categories'::regclass AND contype = 'p';
    IF pk_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE kb.artifact_categories DROP CONSTRAINT %I', pk_name);
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE kb.artifact_categories
    ADD CONSTRAINT artifact_categories_pkey PRIMARY KEY (category_id);

-- Match channels.
CREATE INDEX IF NOT EXISTS idx_kb_artifact_categories_match_keys
    ON kb.artifact_categories USING gin (match_keys jsonb_path_ops);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_categories_search_fts
    ON kb.artifact_categories USING gin (to_tsvector('simple', search_document));

-- +goose Down
DROP INDEX IF EXISTS idx_kb_artifact_categories_search_fts;
DROP INDEX IF EXISTS idx_kb_artifact_categories_match_keys;

-- Restore category_key as the primary key (dropping whatever PK exists first).
-- +goose StatementBegin
DO $$
DECLARE pk_name text;
BEGIN
    SELECT conname INTO pk_name
    FROM   pg_constraint
    WHERE  conrelid = 'kb.artifact_categories'::regclass AND contype = 'p';
    IF pk_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE kb.artifact_categories DROP CONSTRAINT %I', pk_name);
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE kb.artifact_categories
    ADD CONSTRAINT categories_pkey PRIMARY KEY (category_key);

ALTER TABLE kb.artifact_categories
    DROP CONSTRAINT IF EXISTS artifact_categories_type_key_uniq;

ALTER TABLE kb.artifact_categories
    DROP COLUMN IF EXISTS match_keys,
    DROP COLUMN IF EXISTS category_keywords,
    DROP COLUMN IF EXISTS category_desc,
    DROP COLUMN IF EXISTS acronyms,
    DROP COLUMN IF EXISTS aliases;
