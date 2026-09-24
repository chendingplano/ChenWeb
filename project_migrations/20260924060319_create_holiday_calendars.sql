-- +goose Up
-- Holiday calendar admin (openspec/changes/holiday-calendar-admin): year-independent
-- holiday definitions bound to specific dates within a (year, country, calendar_type)
-- calendar.

CREATE TABLE IF NOT EXISTS public.holiday_info (
    id              BIGSERIAL PRIMARY KEY,
    country         TEXT NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT,
    note            TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (country, name)
);

CREATE TABLE IF NOT EXISTS public.calendars (
    id              BIGSERIAL PRIMARY KEY,
    year            INT NOT NULL,
    country         TEXT NOT NULL,
    calendar_type   TEXT NOT NULL DEFAULT 'holidays',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (year, country, calendar_type)
);

CREATE TABLE IF NOT EXISTS public.calendar_holidays (
    id              BIGSERIAL PRIMARY KEY,
    calendar_id     BIGINT NOT NULL REFERENCES public.calendars(id) ON DELETE CASCADE,
    holiday_info_id BIGINT NOT NULL REFERENCES public.holiday_info(id) ON DELETE RESTRICT,
    holiday_date    DATE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (calendar_id, holiday_date)
);

CREATE INDEX IF NOT EXISTS idx_calendar_holidays_holiday_info_id
    ON public.calendar_holidays (holiday_info_id);

-- +goose Down
DROP TABLE IF EXISTS public.calendar_holidays;
DROP TABLE IF EXISTS public.calendars;
DROP TABLE IF EXISTS public.holiday_info;
