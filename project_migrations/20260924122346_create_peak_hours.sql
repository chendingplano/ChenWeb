-- +goose Up
-- Peak hours admin (openspec/changes/peak-hours-admin): named, timezone-aware
-- windows of active hours with day-of-week/day-of-month applicability and
-- weekend/holiday/specific-date exclusions.

CREATE TABLE IF NOT EXISTS public.peak_hours (
    id                BIGSERIAL PRIMARY KEY,
    name              TEXT NOT NULL UNIQUE,
    hours             JSONB NOT NULL,
    timezone          TEXT NOT NULL,
    applicable_days   JSONB NOT NULL,
    exclude_days      JSONB NOT NULL DEFAULT '[]',
    country           TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS public.peak_hours;
