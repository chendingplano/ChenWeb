-- +goose Up
ALTER TABLE kb.doc_review_status
    ADD COLUMN IF NOT EXISTS progress DOUBLE PRECISION NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE kb.doc_review_status
    DROP COLUMN IF EXISTS progress;
