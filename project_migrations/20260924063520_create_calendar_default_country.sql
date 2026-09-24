-- +goose Up
-- Holiday calendar admin (openspec/changes/holiday-calendar-admin): a per-deployment
-- default country for the Calendar admin page, since different ChenWeb deployments
-- want a different (or no) default country selection. Singleton row: at most one
-- default at a time, enforced by the fixed id=1 primary key.

CREATE TABLE IF NOT EXISTS public.calendar_default_country (
    id          INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    country     TEXT NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS public.calendar_default_country;
