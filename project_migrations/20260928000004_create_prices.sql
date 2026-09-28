-- +goose Up
CREATE TABLE IF NOT EXISTS public.price_defs (
    id BIGSERIAL PRIMARY KEY,
    price_def_name TEXT NOT NULL,
    price_type TEXT NOT NULL CHECK (price_type IN ('service', 'llm')),
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT price_defs_name_unique UNIQUE (price_def_name)
);

CREATE TABLE IF NOT EXISTS public.price_items (
    id BIGSERIAL PRIMARY KEY,
    price_def_id BIGINT NOT NULL REFERENCES public.price_defs(id) ON DELETE CASCADE,
    item_name TEXT NOT NULL,
    item_type TEXT NOT NULL CHECK (item_type IN ('input', 'output')),
    cache TEXT NOT NULL DEFAULT '' CHECK (cache IN ('', 'hit', 'miss')),
    time_span TEXT NOT NULL DEFAULT '' CHECK (time_span IN ('', 'peak', 'off-peak')),
    unit TEXT NOT NULL,
    currency TEXT NOT NULL,
    value NUMERIC NOT NULL CHECK (value >= 0),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT price_items_name_unique UNIQUE (price_def_id, item_name)
);

CREATE INDEX IF NOT EXISTS price_items_price_def_id_idx ON public.price_items (price_def_id, sort_order, id);

-- +goose Down
DROP INDEX IF EXISTS public.price_items_price_def_id_idx;
DROP TABLE IF EXISTS public.price_items;
DROP TABLE IF EXISTS public.price_defs;
