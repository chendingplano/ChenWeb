-- +goose Up
-- Marks groups whose categories were imported once from the directory-based builder's manifests
-- (<dir>/.docbuilder/manifest.json), so a forced reclassification is not undone by a re-import.
ALTER TABLE kb.doc_groups ADD COLUMN IF NOT EXISTS legacy_imported BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE kb.doc_groups DROP COLUMN IF EXISTS legacy_imported;
