-- Index fields for the reusable module catalogue, used by the find-reusable-module skill.
--
-- kb.reusable_module_capabilities lists the kinds of need modules are grouped by
-- (for example "ids": IDs and counters). Its rows, like the module rows, are catalogue
-- data maintained by the create-reusable-module skill, not seeded here.
--
-- New kb.reusable_modules columns:
--   capability    the module's capability; NULL only until a row is classified
--   use_when      needs phrased the way a developer would search for them
--   devdoc_paths  workspace-relative paths of the devdocs that explain the module

-- +goose Up
CREATE TABLE IF NOT EXISTS kb.reusable_module_capabilities (
    capability  TEXT        PRIMARY KEY CHECK (capability ~ '^[a-z][a-z0-9-]*$'),
    label       TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    sort_order  INTEGER     NOT NULL DEFAULT 0,
    created_by  TEXT        NOT NULL DEFAULT '',
    updated_by  TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE kb.reusable_modules
    ADD COLUMN IF NOT EXISTS capability   TEXT REFERENCES kb.reusable_module_capabilities (capability) ON UPDATE CASCADE,
    ADD COLUMN IF NOT EXISTS use_when     TEXT  NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS devdoc_paths JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE kb.reusable_modules DROP CONSTRAINT IF EXISTS reusable_modules_devdoc_paths_check;
ALTER TABLE kb.reusable_modules ADD CONSTRAINT reusable_modules_devdoc_paths_check
    CHECK (jsonb_typeof(devdoc_paths) = 'array');

CREATE INDEX IF NOT EXISTS idx_reusable_modules_capability ON kb.reusable_modules (capability);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_reusable_modules_capability;
ALTER TABLE kb.reusable_modules DROP CONSTRAINT IF EXISTS reusable_modules_devdoc_paths_check;
ALTER TABLE kb.reusable_modules
    DROP COLUMN IF EXISTS devdoc_paths,
    DROP COLUMN IF EXISTS use_when,
    DROP COLUMN IF EXISTS capability;
DROP TABLE IF EXISTS kb.reusable_module_capabilities;
