-- +goose Up
-- Daily per-assistant LLM usage report, shared by coding assistants
-- (Qwen Code today; Codex, Claude Code, etc. later). One row per
-- (assistant, usage_date, model); collectors upsert idempotently.
CREATE TABLE IF NOT EXISTS kb.llm_usage (
    id               BIGSERIAL   PRIMARY KEY,
    assistant        TEXT        NOT NULL,
    usage_date       DATE        NOT NULL,
    model            TEXT        NOT NULL,
    requests         INT         NOT NULL DEFAULT 0,
    input_tokens     BIGINT      NOT NULL DEFAULT 0,
    output_tokens    BIGINT      NOT NULL DEFAULT 0,
    cached_tokens    BIGINT      NOT NULL DEFAULT 0,
    thinking_tokens  BIGINT      NOT NULL DEFAULT 0,
    total_tokens     BIGINT      NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kb_llm_usage_assistant_date_model
        UNIQUE (assistant, usage_date, model)
);

-- Daily 02:00 schedule for the collector that fills kb.llm_usage. The
-- scheduler is interval-based, so the interval is 86400s with the first run
-- pinned to the next 02:00 in the workspace timezone ([llm] workspace_timezone
-- = America/Chicago); subsequent runs follow at ~24h intervals.
INSERT INTO kb.scheduled_jobs (name, job_type, interval_seconds, params, enabled, run_once, next_run_at)
SELECT 'Collect LLM Usage (daily 2am)', 'collect_llm_usage', 86400, '{}'::jsonb, true, false,
       CASE
           WHEN (NOW() AT TIME ZONE 'America/Chicago')::time < TIME '02:00'
               THEN ((NOW() AT TIME ZONE 'America/Chicago')::date + TIME '02:00') AT TIME ZONE 'America/Chicago'
           ELSE (((NOW() AT TIME ZONE 'America/Chicago')::date + INTERVAL '1 day') + TIME '02:00') AT TIME ZONE 'America/Chicago'
       END
WHERE NOT EXISTS (SELECT 1 FROM kb.scheduled_jobs WHERE job_type = 'collect_llm_usage');

-- +goose Down
DELETE FROM kb.scheduled_jobs WHERE job_type = 'collect_llm_usage';
DROP TABLE IF EXISTS kb.llm_usage;
