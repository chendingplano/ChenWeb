-- +goose Up
-- Decision-model policies: task-specific instruction text that apps place in
-- a decision request's state. Versions are immutable; see
-- shared/openspec/changes/add-decision-policy-store/design.md.
CREATE SCHEMA IF NOT EXISTS shared;

CREATE TABLE IF NOT EXISTS shared.decision_policies (
    id               BIGSERIAL     PRIMARY KEY,
    name             VARCHAR(128)  NOT NULL,
    description      TEXT          NOT NULL DEFAULT '',
    current_version  INT           NOT NULL,
    latest_version   INT           NOT NULL,
    status           VARCHAR(16)   NOT NULL DEFAULT 'active'
                                   CHECK (status IN ('active', 'deleted')),
    created_by       VARCHAR(255)  NOT NULL DEFAULT '',
    updated_by       VARCHAR(255)  NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at       TIMESTAMPTZ   NULL,
    CHECK (current_version BETWEEN 1 AND latest_version)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_decision_policies_name_active_unique
    ON shared.decision_policies (LOWER(name))
    WHERE status = 'active';

CREATE TABLE IF NOT EXISTS shared.decision_policy_versions (
    id          BIGSERIAL     PRIMARY KEY,
    policy_id   BIGINT        NOT NULL REFERENCES shared.decision_policies (id),
    version     INT           NOT NULL CHECK (version >= 1),
    content     TEXT          NOT NULL CHECK (btrim(content) <> ''),
    note        TEXT          NOT NULL DEFAULT '',
    created_by  VARCHAR(255)  NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE (policy_id, version)
);

-- +goose Down
DROP TABLE IF EXISTS shared.decision_policy_versions;
DROP INDEX IF EXISTS shared.idx_decision_policies_name_active_unique;
DROP TABLE IF EXISTS shared.decision_policies;
