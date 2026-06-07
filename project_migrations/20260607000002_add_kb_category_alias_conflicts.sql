-- +goose Up
CREATE TABLE kb.category_alias_conflicts (
    alias         text NOT NULL,
    category_type text NOT NULL,
    category_ids  jsonb DEFAULT '[]'::jsonb NOT NULL,
    detected_at   timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT category_alias_conflicts_pkey PRIMARY KEY (category_type, alias)
);

-- +goose Down
DROP TABLE IF EXISTS kb.category_alias_conflicts;
