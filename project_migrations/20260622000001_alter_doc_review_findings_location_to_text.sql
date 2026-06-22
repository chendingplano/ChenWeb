-- +goose Up
ALTER TABLE kb.doc_review_findings
    ALTER COLUMN location TYPE TEXT USING location::text;

-- +goose Down
ALTER TABLE kb.doc_review_findings
    ALTER COLUMN location TYPE JSONB USING location::jsonb;
