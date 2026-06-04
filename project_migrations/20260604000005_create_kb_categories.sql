-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS kb.artifact_categories (
    category_key    TEXT        NOT NULL,
    category_type   TEXT        NOT NULL,
    status          TEXT        NOT NULL DEFAULT 'pending_review',
    canonical_of    TEXT        NULL,
    display_names   JSONB       NOT NULL DEFAULT '[]'::jsonb,
    required_attrs  JSONB       NOT NULL DEFAULT '[]'::jsonb,
    specs           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    plausible_ranges JSONB      NOT NULL DEFAULT '{}'::jsonb,
    embedding       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    seen_count      BIGINT      NOT NULL DEFAULT 0,
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT artifact_categories_pkey PRIMARY KEY (category_key),
    CONSTRAINT artifact_categories_status_check CHECK (status = ANY (ARRAY['pending_review'::text, 'approved'::text, 'rejected'::text, 'merged'::text]))
);
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_kb_artifact_categories_canonical_of ON kb.artifact_categories USING btree (canonical_of);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_categories_seen_count   ON kb.artifact_categories USING btree (seen_count DESC);
CREATE INDEX IF NOT EXISTS idx_kb_artifact_categories_status       ON kb.artifact_categories USING btree (status);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_artifact_categories_status;
DROP INDEX IF EXISTS idx_kb_artifact_categories_seen_count;
DROP INDEX IF EXISTS idx_kb_artifact_categories_canonical_of;
DROP TABLE IF EXISTS kb.artifact_categories;
