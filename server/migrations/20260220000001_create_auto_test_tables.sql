-- +goose Up
-- Create auto_test_runs table
CREATE TABLE IF NOT EXISTS auto_test_runs (
    id          BIGSERIAL PRIMARY KEY,
    run_id      VARCHAR(64)  NOT NULL UNIQUE,
    started_at  TIMESTAMPTZ  NOT NULL,
    ended_at    TIMESTAMPTZ,
    status      VARCHAR(20)  NOT NULL DEFAULT 'running'
                CHECK (status IN ('running','completed','failed','partial')),
    env         VARCHAR(40)  NOT NULL DEFAULT 'local',
    seed        BIGINT       NOT NULL DEFAULT 0,
    config_json JSONB,
    env_json    JSONB,
    total       INTEGER      NOT NULL DEFAULT 0,
    passed      INTEGER      NOT NULL DEFAULT 0,
    failed      INTEGER      NOT NULL DEFAULT 0,
    skipped     INTEGER      NOT NULL DEFAULT 0,
    errored     INTEGER      NOT NULL DEFAULT 0,
    duration_ms BIGINT,
    report_path VARCHAR(512),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Create auto_test_results table
CREATE TABLE IF NOT EXISTS auto_test_results (
    id                   BIGSERIAL    PRIMARY KEY,
    run_id               VARCHAR(64)  NOT NULL,
    test_case_id         VARCHAR(200) NOT NULL,
    tester_name          VARCHAR(128) NOT NULL,
    status               VARCHAR(20)  NOT NULL
                         CHECK (status IN ('pass','fail','skip','error')),
    message              TEXT,
    error                TEXT,
    start_time           TIMESTAMPTZ  NOT NULL,
    end_time             TIMESTAMPTZ  NOT NULL,
    duration_ms          BIGINT       NOT NULL,
    retry_count          INTEGER      NOT NULL DEFAULT 0,
    actual_value_json    JSONB,
    side_effects         TEXT[],
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_auto_test_results_run
        FOREIGN KEY (run_id) REFERENCES auto_test_runs(run_id) ON DELETE CASCADE
);

-- Create auto_test_logs table
CREATE TABLE IF NOT EXISTS auto_test_logs (
    id           BIGSERIAL    PRIMARY KEY,
    run_id       VARCHAR(64)  NOT NULL,
    test_case_id VARCHAR(200),
    tester_name  VARCHAR(128) NOT NULL,
    log_level    VARCHAR(10)  NOT NULL CHECK (log_level IN ('DEBUG','INFO','WARN','ERROR')),
    message      TEXT         NOT NULL,
    context_json JSONB,
    logged_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_auto_test_logs_run
        FOREIGN KEY (run_id) REFERENCES auto_test_runs(run_id) ON DELETE CASCADE
);

-- Create indexes for auto_test_runs
CREATE INDEX IF NOT EXISTS idx_atr_started_at ON auto_test_runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_atr_status ON auto_test_runs(status);
CREATE INDEX IF NOT EXISTS idx_atr_env ON auto_test_runs(env);

-- Create indexes for auto_test_results
CREATE INDEX IF NOT EXISTS idx_atres_run_id ON auto_test_results(run_id);
CREATE INDEX IF NOT EXISTS idx_atres_tester ON auto_test_results(tester_name);
CREATE INDEX IF NOT EXISTS idx_atres_status ON auto_test_results(status);
CREATE INDEX IF NOT EXISTS idx_atres_case_id ON auto_test_results(test_case_id);
CREATE INDEX IF NOT EXISTS idx_atres_start_time ON auto_test_results(start_time DESC);

-- Create indexes for auto_test_logs
CREATE INDEX IF NOT EXISTS idx_atlog_run_id ON auto_test_logs(run_id);
CREATE INDEX IF NOT EXISTS idx_atlog_case_id ON auto_test_logs(test_case_id);
CREATE INDEX IF NOT EXISTS idx_atlog_level ON auto_test_logs(log_level);

-- +goose Down
-- Drop indexes
DROP INDEX IF EXISTS idx_atlog_level;
DROP INDEX IF EXISTS idx_atlog_case_id;
DROP INDEX IF EXISTS idx_atlog_run_id;
DROP INDEX IF EXISTS idx_atres_start_time;
DROP INDEX IF EXISTS idx_atres_case_id;
DROP INDEX IF EXISTS idx_atres_status;
DROP INDEX IF EXISTS idx_atres_tester;
DROP INDEX IF EXISTS idx_atres_run_id;
DROP INDEX IF EXISTS idx_atr_env;
DROP INDEX IF EXISTS idx_atr_status;
DROP INDEX IF EXISTS idx_atr_started_at;

-- Drop tables (in reverse order of creation)
DROP TABLE IF EXISTS auto_test_logs;
DROP TABLE IF EXISTS auto_test_results;
DROP TABLE IF EXISTS auto_test_runs;
