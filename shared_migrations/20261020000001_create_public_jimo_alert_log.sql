-- +goose Up
-- JimoLogger Warn/Error events (written by shared/go/api/sysdatastores
-- StartAlertLogWriter), plus the triage fields used by the
-- System Admin > Logs > Warnings and Errors page.
CREATE TABLE IF NOT EXISTS public.jimo_alert_log (
    id           BIGSERIAL    PRIMARY KEY,
    level        VARCHAR(8)   NOT NULL,
    message      TEXT         NOT NULL,
    req_id       VARCHAR(64),
    loc          VARCHAR(128),
    call_flow    TEXT,
    attrs        JSONB,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    status       VARCHAR(16)  NOT NULL DEFAULT 'open',
    owner        VARCHAR(255),
    resolution   TEXT,
    resolver     VARCHAR(255),
    resolve_time TIMESTAMPTZ
);

-- Table may already exist from the earlier version without triage columns.
ALTER TABLE public.jimo_alert_log ADD COLUMN IF NOT EXISTS status       VARCHAR(16) NOT NULL DEFAULT 'open';
ALTER TABLE public.jimo_alert_log ADD COLUMN IF NOT EXISTS owner        VARCHAR(255);
ALTER TABLE public.jimo_alert_log ADD COLUMN IF NOT EXISTS resolution   TEXT;
ALTER TABLE public.jimo_alert_log ADD COLUMN IF NOT EXISTS resolver     VARCHAR(255);
ALTER TABLE public.jimo_alert_log ADD COLUMN IF NOT EXISTS resolve_time TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_jimo_alert_log_created_at ON public.jimo_alert_log (created_at);
CREATE INDEX IF NOT EXISTS idx_jimo_alert_log_level ON public.jimo_alert_log (level, created_at);
CREATE INDEX IF NOT EXISTS idx_jimo_alert_log_status ON public.jimo_alert_log (status, created_at);

-- +goose Down
DROP TABLE IF EXISTS public.jimo_alert_log;
