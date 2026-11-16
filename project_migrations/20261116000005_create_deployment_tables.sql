-- +goose Up
-- Deployment page (openspec deployment-page): one row per Build Deployment click on the
-- build machine, one row per Deploy on the deployment machine, and the steps of each run.
-- kb.build_activities (one row per built executable) is separate; build_number links them.
CREATE TABLE IF NOT EXISTS kb.deployment_builds (
    id             BIGSERIAL   PRIMARY KEY,
    build_number   TEXT        NOT NULL UNIQUE,
    build_name     TEXT        NOT NULL,
    target_machine TEXT        NOT NULL,
    os_info        TEXT        NOT NULL,
    description    TEXT        NOT NULL DEFAULT '',
    status         TEXT        NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    archive_name   TEXT        NOT NULL DEFAULT '',
    error_message  TEXT        NOT NULL DEFAULT '',
    requested_by   TEXT        NOT NULL DEFAULT '',
    runner_host    TEXT        NOT NULL DEFAULT '',
    runner_pid     INTEGER     NOT NULL DEFAULT 0,
    started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at    TIMESTAMPTZ
);

-- At most one active build per database (one build machine per database).
CREATE UNIQUE INDEX IF NOT EXISTS idx_deployment_builds_one_active
    ON kb.deployment_builds ((true)) WHERE status = 'running';

CREATE TABLE IF NOT EXISTS kb.deployments (
    id                    BIGSERIAL   PRIMARY KEY,
    archive_name          TEXT        NOT NULL,
    build_number          TEXT        NOT NULL DEFAULT '',
    previous_build_number TEXT        NOT NULL DEFAULT '',
    status                TEXT        NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    error_message         TEXT        NOT NULL DEFAULT '',
    requested_by          TEXT        NOT NULL DEFAULT '',
    runner_pid            INTEGER     NOT NULL DEFAULT 0,
    requested_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at            TIMESTAMPTZ,
    finished_at           TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_deployments_one_active
    ON kb.deployments ((true)) WHERE status IN ('queued', 'running');

CREATE TABLE IF NOT EXISTS kb.deployment_steps (
    id               BIGSERIAL   PRIMARY KEY,
    build_id         BIGINT      REFERENCES kb.deployment_builds (id) ON DELETE CASCADE,
    deployment_id    BIGINT      REFERENCES kb.deployments (id) ON DELETE CASCADE,
    seq              INTEGER     NOT NULL,
    step_key         TEXT        NOT NULL,
    step_arg         TEXT        NOT NULL DEFAULT '',
    status           TEXT        NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'skipped')),
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ,
    output           TEXT        NOT NULL DEFAULT '',
    output_truncated BOOLEAN     NOT NULL DEFAULT FALSE,
    CHECK (num_nonnulls(build_id, deployment_id) = 1)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_deployment_steps_build_seq
    ON kb.deployment_steps (build_id, seq) WHERE build_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_deployment_steps_deployment_seq
    ON kb.deployment_steps (deployment_id, seq) WHERE deployment_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS kb.idx_deployment_steps_deployment_seq;
DROP INDEX IF EXISTS kb.idx_deployment_steps_build_seq;
DROP TABLE IF EXISTS kb.deployment_steps;
DROP INDEX IF EXISTS kb.idx_deployments_one_active;
DROP TABLE IF EXISTS kb.deployments;
DROP INDEX IF EXISTS kb.idx_deployment_builds_one_active;
DROP TABLE IF EXISTS kb.deployment_builds;
