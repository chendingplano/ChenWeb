-- WPI pages move out of kb.wpi_descriptors into their own table (devdoc
-- 2026102601 §3.2, §6; OpenSpec change wpi-pages). Pages have no classes: each
-- page is complete and is reused by copying. Every page version moves with its
-- status and audit columns; kb.wpi_descriptors keeps apps and the element and
-- action classes, and its kind check no longer allows pages.

-- +goose Up
CREATE TABLE IF NOT EXISTS kb.frontend_pages (
    id             BIGSERIAL   PRIMARY KEY,
    descriptor_id  TEXT        NOT NULL CHECK (descriptor_id ~ '^[a-z0-9][a-z0-9-]{0,127}$'),
    version        INTEGER     NOT NULL CHECK (version > 0),
    schema_version TEXT        NOT NULL,
    body           JSONB       NOT NULL CHECK (jsonb_typeof(body) = 'object'),
    status         TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('draft', 'active', 'retired')),
    created_by     TEXT        NOT NULL DEFAULT '',
    updated_by     TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (descriptor_id, version)
);

CREATE INDEX IF NOT EXISTS idx_frontend_pages_active
    ON kb.frontend_pages (descriptor_id, version DESC)
    WHERE status = 'active';

COMMENT ON TABLE kb.frontend_pages IS
    'Web Page Interpreter pages, one row per version. Pages have no classes; they are '
    'reused by copying. Versions are append-only; the latest active version is the one in use.';
COMMENT ON COLUMN kb.frontend_pages.body IS
    'The page descriptor JSON itself; its descriptor_id and schema_version match the columns.';

INSERT INTO kb.frontend_pages (descriptor_id, version, schema_version, body, status,
                               created_by, updated_by, created_at, updated_at)
SELECT descriptor_id, version, schema_version, body, status, created_by, updated_by, created_at, updated_at
  FROM kb.wpi_descriptors
 WHERE kind = 'page'
ON CONFLICT (descriptor_id, version) DO NOTHING;

DELETE FROM kb.wpi_descriptors WHERE kind = 'page';

ALTER TABLE kb.wpi_descriptors DROP CONSTRAINT IF EXISTS wpi_descriptors_kind_check;
ALTER TABLE kb.wpi_descriptors
    ADD CONSTRAINT wpi_descriptors_kind_check CHECK (kind IN ('app', 'element', 'action'));

COMMENT ON TABLE kb.wpi_descriptors IS
    'Web Page Interpreter descriptors (app, element and action classes; pages are in '
    'kb.frontend_pages), one row per version. Versions are append-only; the latest active '
    'version is the one in use.';

-- +goose Down
ALTER TABLE kb.wpi_descriptors DROP CONSTRAINT IF EXISTS wpi_descriptors_kind_check;
ALTER TABLE kb.wpi_descriptors
    ADD CONSTRAINT wpi_descriptors_kind_check CHECK (kind IN ('app', 'page', 'element', 'action'));

INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status,
                                created_by, updated_by, created_at, updated_at)
SELECT 'page', descriptor_id, version, schema_version, body, status, created_by, updated_by, created_at, updated_at
  FROM kb.frontend_pages
ON CONFLICT (kind, descriptor_id, version) DO NOTHING;

COMMENT ON TABLE kb.wpi_descriptors IS
    'Web Page Interpreter descriptors (app, page, element, action), one row per '
    'version. Versions are append-only; the latest active version is the one in use.';

DROP INDEX IF EXISTS kb.idx_frontend_pages_active;
DROP TABLE IF EXISTS kb.frontend_pages;
