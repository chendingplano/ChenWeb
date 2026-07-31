-- +goose Up
ALTER TABLE kb.doc_process_plans
    ADD COLUMN IF NOT EXISTS excluded_by_policy TEXT[] NOT NULL DEFAULT '{}'::text[];

-- +goose Down
ALTER TABLE kb.doc_process_plans
    DROP COLUMN IF EXISTS excluded_by_policy;
