-- +goose Up
ALTER TABLE kb.doc_review_runs ADD COLUMN IF NOT EXISTS user_id TEXT;

-- +goose Down
ALTER TABLE kb.doc_review_runs DROP COLUMN IF EXISTS user_id;
