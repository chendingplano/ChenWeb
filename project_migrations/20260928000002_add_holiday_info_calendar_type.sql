-- +goose Up
-- Holiday definitions belong to (country, calendar_type), and are reused across years.
-- Existing rows all belong to the only calendar type so far, 'holidays'.
ALTER TABLE public.holiday_info
    ADD COLUMN IF NOT EXISTS calendar_type TEXT NOT NULL DEFAULT 'holidays';

ALTER TABLE public.holiday_info
    DROP CONSTRAINT IF EXISTS holiday_info_country_name_key,
    DROP CONSTRAINT IF EXISTS holiday_info_country_display_seqno_unique,
    ADD CONSTRAINT holiday_info_country_type_name_key UNIQUE (country, calendar_type, name),
    ADD CONSTRAINT holiday_info_country_type_display_seqno_unique UNIQUE (country, calendar_type, display_seqno);

-- +goose Down
-- Fails if two calendar types of one country share a holiday name or display_seqno.
ALTER TABLE public.holiday_info
    DROP CONSTRAINT IF EXISTS holiday_info_country_type_display_seqno_unique,
    DROP CONSTRAINT IF EXISTS holiday_info_country_type_name_key,
    ADD CONSTRAINT holiday_info_country_display_seqno_unique UNIQUE (country, display_seqno),
    ADD CONSTRAINT holiday_info_country_name_key UNIQUE (country, name);

ALTER TABLE public.holiday_info
    DROP COLUMN IF EXISTS calendar_type;
