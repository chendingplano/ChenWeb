-- +goose Up
-- day_kind distinguishes a holiday day off ('holiday') from an adjusted working
-- day that belongs to the same holiday ('adjusted'), e.g. a Sunday worked to make
-- up for a CN New Year break. Existing rows are days off.
ALTER TABLE public.calendar_holidays
    ADD COLUMN IF NOT EXISTS day_kind TEXT NOT NULL DEFAULT 'holiday'
        CONSTRAINT calendar_holidays_day_kind_check CHECK (day_kind IN ('holiday', 'adjusted'));

-- +goose Down
ALTER TABLE public.calendar_holidays
    DROP COLUMN IF EXISTS day_kind;
