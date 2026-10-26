-- Web Page Interpreter (WPI) descriptors: apps, pages, elements and actions, one
-- row per version. The interpreter uses the latest active version of each
-- (kind, descriptor_id). Design: KnowledgeStore devdoc 2026102601 and the
-- ChenWeb OpenSpec change wpi-pilot.

-- +goose Up
CREATE TABLE IF NOT EXISTS kb.wpi_descriptors (
    id             BIGSERIAL   PRIMARY KEY,
    kind           TEXT        NOT NULL CHECK (kind IN ('app', 'page', 'element', 'action')),
    descriptor_id  TEXT        NOT NULL CHECK (descriptor_id ~ '^[a-z0-9][a-z0-9-]{0,127}$'),
    version        INTEGER     NOT NULL CHECK (version > 0),
    schema_version TEXT        NOT NULL,
    body           JSONB       NOT NULL CHECK (jsonb_typeof(body) = 'object'),
    status         TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('draft', 'active', 'retired')),
    created_by     TEXT        NOT NULL DEFAULT '',
    updated_by     TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (kind, descriptor_id, version)
);

CREATE INDEX IF NOT EXISTS idx_wpi_descriptors_active
    ON kb.wpi_descriptors (kind, descriptor_id, version DESC)
    WHERE status = 'active';

COMMENT ON TABLE kb.wpi_descriptors IS
    'Web Page Interpreter descriptors (app, page, element, action), one row per '
    'version. Versions are append-only; the latest active version is the one in use.';
COMMENT ON COLUMN kb.wpi_descriptors.body IS
    'The descriptor JSON itself; its descriptor_id and schema_version match the columns.';

-- +goose Down
DROP INDEX IF EXISTS kb.idx_wpi_descriptors_active;
DROP TABLE IF EXISTS kb.wpi_descriptors;
