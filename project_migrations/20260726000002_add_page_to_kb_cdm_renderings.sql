-- +goose Up
--
-- kb.cdm_renderings originally keyed one row per (document, content_version,
-- renderer, renderer_version), assuming a rendering is a single artifact.
-- That holds for the .typ source and a PDF export, but not for paginated SVG
-- (spec §5.7): a document renders to one SVG file per page, and all of them
-- share the same renderer/renderer_version. Adding `page` (0 for
-- non-paginated renderers such as "typst" source or "pdf") and widening the
-- unique key to include it.
ALTER TABLE kb.cdm_renderings
    ADD COLUMN IF NOT EXISTS page INTEGER NOT NULL DEFAULT 0;

ALTER TABLE kb.cdm_renderings
    DROP CONSTRAINT IF EXISTS uq_kb_cdm_renderings;

ALTER TABLE kb.cdm_renderings
    ADD CONSTRAINT uq_kb_cdm_renderings
        UNIQUE (document_id, content_version, renderer, renderer_version, page);

-- +goose Down
ALTER TABLE kb.cdm_renderings
    DROP CONSTRAINT IF EXISTS uq_kb_cdm_renderings;

ALTER TABLE kb.cdm_renderings
    ADD CONSTRAINT uq_kb_cdm_renderings
        UNIQUE (document_id, content_version, renderer, renderer_version);

ALTER TABLE kb.cdm_renderings
    DROP COLUMN IF EXISTS page;
