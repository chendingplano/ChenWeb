-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.doc_process_plans (
    id                  BIGSERIAL    PRIMARY KEY,
    run_id              BIGINT       NOT NULL,
    record_id           BIGINT       NOT NULL,
    plan_facts          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    plan_steps          JSONB        NOT NULL DEFAULT '[]'::jsonb,
    pipeline_selection  JSONB        NOT NULL DEFAULT '{}'::jsonb,
    pipeline_binding    JSONB        NOT NULL DEFAULT '{}'::jsonb,
    create_time         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_doc_process_plans_run
        FOREIGN KEY (run_id) REFERENCES kb.doc_process_runs (id)
        ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_doc_process_plans_run_id
    ON kb.doc_process_plans (run_id);
CREATE INDEX IF NOT EXISTS idx_kb_doc_process_plans_record_id
    ON kb.doc_process_plans (record_id);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_doc_process_plans_record_id;
DROP INDEX IF EXISTS idx_kb_doc_process_plans_run_id;
DROP TABLE IF EXISTS kb.doc_process_plans;
