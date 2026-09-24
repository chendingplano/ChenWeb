-- +goose Up
ALTER TABLE public.holiday_info
    ADD COLUMN IF NOT EXISTS display_seqno INT;

WITH numbered AS (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY country ORDER BY id)::INT AS seqno
    FROM public.holiday_info
)
UPDATE public.holiday_info h
SET display_seqno = numbered.seqno
FROM numbered
WHERE h.id = numbered.id
  AND h.display_seqno IS NULL;

ALTER TABLE public.holiday_info
    ALTER COLUMN display_seqno SET NOT NULL,
    ALTER COLUMN display_seqno SET DEFAULT 1,
    ADD CONSTRAINT holiday_info_display_seqno_positive CHECK (display_seqno > 0),
    ADD CONSTRAINT holiday_info_country_display_seqno_unique UNIQUE (country, display_seqno);

-- +goose Down
ALTER TABLE public.holiday_info
    DROP CONSTRAINT IF EXISTS holiday_info_country_display_seqno_unique,
    DROP CONSTRAINT IF EXISTS holiday_info_display_seqno_positive,
    ALTER COLUMN display_seqno DROP DEFAULT,
    DROP COLUMN IF EXISTS display_seqno;
