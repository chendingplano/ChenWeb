-- +goose Up
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

-- +goose Down
DROP INDEX IF EXISTS public.release_items_release_id_idx;
DROP TABLE IF EXISTS public.release_items;
DROP TABLE IF EXISTS public.releases;
