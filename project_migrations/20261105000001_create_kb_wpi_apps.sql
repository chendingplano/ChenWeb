-- WPI apps move out of kb.wpi_descriptors into their own table, and an app no longer
-- lives inside a page (devdoc 2026102601 §3.1, §6; OpenSpec change wpi-apps-table).
-- An app descriptor names the page it opens by that page's descriptor_id
-- (`initial_page`), so one page can serve several apps. Until now the editor's
-- Create App dialog stored an app's identity in a root attribute `app` of its
-- page; this migration turns each such attribute into an app row and hoists the
-- page's own details (name, description, keywords, tags, size) to the page root.

-- +goose Up
CREATE TABLE IF NOT EXISTS kb.wpi_apps (
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

CREATE INDEX IF NOT EXISTS idx_wpi_apps_active
    ON kb.wpi_apps (descriptor_id, version DESC)
    WHERE status = 'active';

COMMENT ON TABLE kb.wpi_apps IS
    'Web Page Interpreter apps, one row per version. An app names the page it opens by '
    'that page''s descriptor_id (kb.frontend_pages); pages are not embedded in apps. '
    'Versions are append-only; the latest active version is the one in use.';
COMMENT ON COLUMN kb.wpi_apps.body IS
    'The app descriptor JSON itself; its descriptor_id and schema_version match the columns.';

-- Apps stored as kb.wpi_descriptors rows move over with every version.
INSERT INTO kb.wpi_apps (descriptor_id, version, schema_version, body, status,
                         created_by, updated_by, created_at, updated_at)
SELECT descriptor_id, version, schema_version, body, status, created_by, updated_by, created_at, updated_at
  FROM kb.wpi_descriptors
 WHERE kind = 'app'
ON CONFLICT (descriptor_id, version) DO NOTHING;

DELETE FROM kb.wpi_descriptors WHERE kind = 'app';

-- A page's root attribute `app` becomes an app row opening that page. The app's
-- name was never stored (only its slug), so the slug stands in as app_name.
-- An app that already exists under the ID is left alone.
INSERT INTO kb.wpi_apps (descriptor_id, version, schema_version, body, status,
                         created_by, updated_by, created_at, updated_at)
SELECT p.body->'app'->>'descriptor_id', 1, p.schema_version,
       jsonb_strip_nulls(jsonb_build_object(
           'descriptor_id', p.body->'app'->>'descriptor_id',
           'schema_version', p.schema_version,
           'initial_page', p.descriptor_id,
           'app_name', p.body->'app'->>'descriptor_id',
           'app_description', p.body->'app'->'description',
           'keywords', p.body->'app'->'keywords',
           'tags', p.body->'app'->'page_tags')),
       'active', p.created_by, p.updated_by, p.created_at, p.updated_at
  FROM kb.frontend_pages p
 WHERE p.status = 'active'
   AND p.body ? 'app'
   AND p.version = (SELECT MAX(version) FROM kb.frontend_pages q WHERE q.descriptor_id = p.descriptor_id)
   AND p.body->'app' ? 'descriptor_id'
ON CONFLICT (descriptor_id, version) DO NOTHING;

-- Every page version holding `app` loses it; what only `app` held moves to the
-- page root unless the page already has it there.
UPDATE kb.frontend_pages
   SET body = (body - 'app')
              || CASE WHEN body ? 'page_name'   OR NOT body->'app' ? 'page_name'   THEN '{}'::jsonb ELSE jsonb_build_object('page_name', body->'app'->'page_name') END
              || CASE WHEN body ? 'description' OR NOT body->'app' ? 'description' THEN '{}'::jsonb ELSE jsonb_build_object('description', body->'app'->'description') END
              || CASE WHEN body ? 'keywords'    OR NOT body->'app' ? 'keywords'    THEN '{}'::jsonb ELSE jsonb_build_object('keywords', body->'app'->'keywords') END
              || CASE WHEN body ? 'page_tags'   OR NOT body->'app' ? 'page_tags'   THEN '{}'::jsonb ELSE jsonb_build_object('page_tags', body->'app'->'page_tags') END
              || CASE WHEN body ? 'width'       OR NOT body->'app' ? 'width'       THEN '{}'::jsonb ELSE jsonb_build_object('width', body->'app'->'width') END
              || CASE WHEN body ? 'height'      OR NOT body->'app' ? 'height'      THEN '{}'::jsonb ELSE jsonb_build_object('height', body->'app'->'height') END
 WHERE body ? 'app';

ALTER TABLE kb.wpi_descriptors DROP CONSTRAINT IF EXISTS wpi_descriptors_kind_check;
ALTER TABLE kb.wpi_descriptors
    ADD CONSTRAINT wpi_descriptors_kind_check
    CHECK (kind IN ('element', 'action', 'page-layouter', 'element-layouter', 'validator'));

COMMENT ON TABLE kb.wpi_descriptors IS
    'Web Page Interpreter classes (element, action, page-layouter, element-layouter, validator), '
    'one row per version; apps are in kb.wpi_apps and pages in kb.frontend_pages. Versions are '
    'append-only; the latest active version is the one in use.';

-- +goose Down
ALTER TABLE kb.wpi_descriptors DROP CONSTRAINT IF EXISTS wpi_descriptors_kind_check;
ALTER TABLE kb.wpi_descriptors
    ADD CONSTRAINT wpi_descriptors_kind_check
    CHECK (kind IN ('app', 'element', 'action', 'page-layouter', 'element-layouter', 'validator'));

INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status,
                                created_by, updated_by, created_at, updated_at)
SELECT 'app', descriptor_id, version, schema_version, body, status, created_by, updated_by, created_at, updated_at
  FROM kb.wpi_apps
ON CONFLICT (kind, descriptor_id, version) DO NOTHING;

DROP INDEX IF EXISTS kb.idx_wpi_apps_active;
DROP TABLE IF EXISTS kb.wpi_apps;
