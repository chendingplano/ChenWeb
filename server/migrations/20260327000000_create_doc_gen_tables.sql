-- server/migrations/20260327000000_create_doc_gen_tables.sql
-- +goose Up
CREATE TABLE IF NOT EXISTS doc_gen_queries (
    id            BIGSERIAL PRIMARY KEY,
    name          VARCHAR(255) NOT NULL UNIQUE,
    description   TEXT,
    sql_statement TEXT NOT NULL,
    created_by    VARCHAR(255) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS doc_gen_jobs (
    job_id         BIGSERIAL PRIMARY KEY,
    request_name   VARCHAR(255) NOT NULL UNIQUE,
    purpose        VARCHAR(255) NOT NULL,
    remarks        TEXT,
    sql_query_id   BIGINT REFERENCES doc_gen_queries(id) ON DELETE SET NULL,
    sql_statement  TEXT NOT NULL,
    template_type  VARCHAR(16) NOT NULL,
    template_path  TEXT NOT NULL,
    converter      JSONB NOT NULL,
    output_dir     TEXT NOT NULL,
    output_format  VARCHAR(16) NOT NULL,
    status         VARCHAR(32) NOT NULL DEFAULT 'pending',
    CONSTRAINT chk_doc_gen_jobs_status CHECK (status IN ('pending','processing','completed','failed')),
    total_count    INT NOT NULL DEFAULT 0,
    success_count  INT NOT NULL DEFAULT 0,
    fail_count     INT NOT NULL DEFAULT 0,
    error_msg      TEXT,
    created_by     VARCHAR(255) NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS doc_gen_log (
    id            BIGSERIAL PRIMARY KEY,
    job_id        BIGINT NOT NULL REFERENCES doc_gen_jobs(job_id) ON DELETE CASCADE,
    request_name  VARCHAR(255) NOT NULL,
    customer_id   VARCHAR(128) NOT NULL,
    customer_name VARCHAR(255) NOT NULL,
    email         VARCHAR(255) NOT NULL,
    phone_num     VARCHAR(64),
    purpose       VARCHAR(255) NOT NULL,
    filename      VARCHAR(512) NOT NULL,
    status        VARCHAR(32) NOT NULL,
    CONSTRAINT chk_doc_gen_log_status CHECK (status IN ('generated','failed')),
    error_msg     TEXT,
    remarks       TEXT,
    created_by    VARCHAR(255) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for doc_gen_jobs
CREATE INDEX IF NOT EXISTS idx_doc_gen_jobs_status     ON doc_gen_jobs(status);
CREATE INDEX IF NOT EXISTS idx_doc_gen_jobs_created_by ON doc_gen_jobs(created_by);
CREATE INDEX IF NOT EXISTS idx_doc_gen_jobs_created_at ON doc_gen_jobs(created_at DESC);

-- Indexes for doc_gen_log
CREATE INDEX IF NOT EXISTS idx_doc_gen_log_job_id      ON doc_gen_log(job_id);
CREATE INDEX IF NOT EXISTS idx_doc_gen_log_customer_id ON doc_gen_log(customer_id);
CREATE INDEX IF NOT EXISTS idx_doc_gen_log_created_at  ON doc_gen_log(created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_doc_gen_log_created_at;
DROP INDEX IF EXISTS idx_doc_gen_log_customer_id;
DROP INDEX IF EXISTS idx_doc_gen_log_job_id;
DROP INDEX IF EXISTS idx_doc_gen_jobs_created_at;
DROP INDEX IF EXISTS idx_doc_gen_jobs_created_by;
DROP INDEX IF EXISTS idx_doc_gen_jobs_status;
DROP TABLE IF EXISTS doc_gen_log;
DROP TABLE IF EXISTS doc_gen_jobs;
DROP TABLE IF EXISTS doc_gen_queries;
