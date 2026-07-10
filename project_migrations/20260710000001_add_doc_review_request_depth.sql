-- +goose Up
ALTER TABLE IF EXISTS kb.doc_review_requests
    ADD COLUMN IF NOT EXISTS review_depth INT NOT NULL DEFAULT 1;

ALTER TABLE IF EXISTS kb.doc_review_requests
    DROP CONSTRAINT IF EXISTS chk_doc_review_requests_review_depth;

ALTER TABLE IF EXISTS kb.doc_review_requests
    ADD CONSTRAINT chk_doc_review_requests_review_depth
    CHECK (review_depth BETWEEN 1 AND 3);

-- +goose Down
ALTER TABLE IF EXISTS kb.doc_review_requests
    DROP CONSTRAINT IF EXISTS chk_doc_review_requests_review_depth;

ALTER TABLE IF EXISTS kb.doc_review_requests
    DROP COLUMN IF EXISTS review_depth;
