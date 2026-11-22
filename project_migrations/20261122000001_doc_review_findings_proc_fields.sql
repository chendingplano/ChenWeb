-- +goose Up
-- Compliance Review redesign (openspec compliance-reviewer-redesign): each finding keeps the names of
-- the activity's cited documents, the error that stopped its review (if any), and whether the
-- reviewer's run for this aspect succeeded or failed. review_status stays the user's field.
ALTER TABLE kb.doc_review_findings ADD COLUMN IF NOT EXISTS cited_documents JSONB;
ALTER TABLE kb.doc_review_findings ADD COLUMN IF NOT EXISTS error_msg TEXT;
ALTER TABLE kb.doc_review_findings ADD COLUMN IF NOT EXISTS proc_status TEXT;

-- +goose Down
ALTER TABLE kb.doc_review_findings DROP COLUMN IF EXISTS proc_status;
ALTER TABLE kb.doc_review_findings DROP COLUMN IF EXISTS error_msg;
ALTER TABLE kb.doc_review_findings DROP COLUMN IF EXISTS cited_documents;
