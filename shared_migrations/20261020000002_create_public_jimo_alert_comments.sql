-- +goose Up
-- Developer comments on rows of public.jimo_alert_log.
CREATE TABLE IF NOT EXISTS public.jimo_alert_comments (
    id         BIGSERIAL    PRIMARY KEY,
    alert_id   BIGINT       NOT NULL REFERENCES public.jimo_alert_log (id) ON DELETE CASCADE,
    author     VARCHAR(255) NOT NULL,
    body       TEXT         NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_jimo_alert_comments_alert ON public.jimo_alert_comments (alert_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS public.jimo_alert_comments;
