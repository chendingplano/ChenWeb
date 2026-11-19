-- +goose Up
-- Release versioning (openspec release-versioning). Replaces the free-text releases tables,
-- which held no rows on any machine when this was written; refuse to drop them otherwise.
-- +goose StatementBegin
DO $$
DECLARE
    t TEXT;
    has_rows BOOLEAN;
BEGIN
    -- Dynamic SQL, so a missing table is skipped instead of failing to plan.
    FOREACH t IN ARRAY ARRAY['public.release_items', 'public.releases'] LOOP
        IF to_regclass(t) IS NOT NULL THEN
            EXECUTE format('SELECT EXISTS (SELECT 1 FROM %s)', t) INTO has_rows;
            IF has_rows THEN
                RAISE EXCEPTION '% has rows; move them before release versioning replaces the table', t;
            END IF;
        END IF;
    END LOOP;
END
$$;
-- +goose StatementEnd

DROP INDEX IF EXISTS public.release_items_release_id_idx;
DROP TABLE IF EXISTS public.release_items;
DROP TABLE IF EXISTS public.releases;

-- One row per version: a new major or minor (revision 0) or a build (revision >= 1).
-- On the build machine rows are written here; elsewhere they are imported from release files.
CREATE TABLE IF NOT EXISTS kb.releases (
    id             BIGSERIAL   PRIMARY KEY,
    project        TEXT        NOT NULL CHECK (project IN ('ChenWeb')),
    major          SMALLINT    NOT NULL CHECK (major BETWEEN 0 AND 99),
    minor          SMALLINT    NOT NULL CHECK (minor BETWEEN 0 AND 99),
    revision       SMALLINT    NOT NULL CHECK (revision BETWEEN 0 AND 99),
    kind           TEXT        NOT NULL CHECK (kind IN ('major', 'minor', 'build')),
    status         TEXT        NOT NULL CHECK (status IN ('building', 'released')),
    description    TEXT        NOT NULL DEFAULT '',
    build_number   TEXT        UNIQUE,
    release_date   DATE,
    git_rev        TEXT        NOT NULL DEFAULT '',
    git_dirty      BOOLEAN     NOT NULL DEFAULT FALSE,
    shared_git_rev TEXT        NOT NULL DEFAULT '',
    shared_git_dirty BOOLEAN   NOT NULL DEFAULT FALSE,
    created_by     TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- The build script that reserved a 'building' release; a dead process on the same
    -- host means the build was killed and the reservation can be reverted.
    builder_host   TEXT        NOT NULL DEFAULT '',
    builder_pid    INTEGER     NOT NULL DEFAULT 0,
    CONSTRAINT releases_project_version_unique UNIQUE (project, major, minor, revision),
    CHECK ((kind = 'build') = (revision > 0))
);

-- A release item waits as pending (release_id NULL) until a build takes it.
CREATE TABLE IF NOT EXISTS kb.release_items (
    id           BIGSERIAL   PRIMARY KEY,
    project      TEXT        NOT NULL CHECK (project IN ('ChenWeb')),
    release_id   BIGINT      REFERENCES kb.releases (id),
    item_type    TEXT        NOT NULL CHECK (item_type IN ('new feature', 'improvement', 'bug fix')),
    component    TEXT        NOT NULL CHECK (component IN ('ChenWeb', 'shared')),
    description  TEXT        NOT NULL CHECK (btrim(description) <> ''),
    issue        TEXT        NOT NULL DEFAULT '' CHECK (issue ~ '^[0-9]*$'),
    pull_request TEXT        NOT NULL DEFAULT '',
    notes        TEXT        NOT NULL DEFAULT '',
    created_by   TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_release_items_release ON kb.release_items (release_id, id);
CREATE INDEX IF NOT EXISTS idx_release_items_pending ON kb.release_items (project, id) WHERE release_id IS NULL;

-- This machine's current release per project: the latest version created on the build
-- machine, the running release on the deploy machine.
CREATE TABLE IF NOT EXISTS kb.release_current (
    project      TEXT        PRIMARY KEY CHECK (project IN ('ChenWeb')),
    major        SMALLINT    NOT NULL CHECK (major BETWEEN 0 AND 99),
    minor        SMALLINT    NOT NULL CHECK (minor BETWEEN 0 AND 99),
    revision     SMALLINT    NOT NULL CHECK (revision BETWEEN 0 AND 99),
    build_number TEXT        NOT NULL DEFAULT '',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE IF EXISTS kb.deployment_builds ADD COLUMN IF NOT EXISTS release_version TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS kb.deployments ADD COLUMN IF NOT EXISTS release_version TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS kb.deployments ADD COLUMN IF NOT EXISTS previous_release_version TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE IF EXISTS kb.deployments DROP COLUMN IF EXISTS previous_release_version;
ALTER TABLE IF EXISTS kb.deployments DROP COLUMN IF EXISTS release_version;
ALTER TABLE IF EXISTS kb.deployment_builds DROP COLUMN IF EXISTS release_version;
DROP TABLE IF EXISTS kb.release_current;
DROP INDEX IF EXISTS kb.idx_release_items_pending;
DROP INDEX IF EXISTS kb.idx_release_items_release;
DROP TABLE IF EXISTS kb.release_items;
DROP TABLE IF EXISTS kb.releases;

CREATE TABLE IF NOT EXISTS public.releases (
    id BIGSERIAL PRIMARY KEY,
    major_version TEXT NOT NULL,
    minor_version TEXT NOT NULL,
    release_notes TEXT NOT NULL DEFAULT '',
    release_date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT releases_version_unique UNIQUE (major_version, minor_version)
);

CREATE TABLE IF NOT EXISTS public.release_items (
    id BIGSERIAL PRIMARY KEY,
    release_id BIGINT NOT NULL REFERENCES public.releases(id) ON DELETE CASCADE,
    item_type TEXT NOT NULL CHECK (item_type IN ('bug fix', 'improvement', 'new feature')),
    description TEXT NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    pull_request TEXT NOT NULL DEFAULT '',
    ticket_num TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS release_items_release_id_idx ON public.release_items (release_id, sort_order, id);
