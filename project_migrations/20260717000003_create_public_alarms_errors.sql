-- +goose Up
CREATE TABLE IF NOT EXISTS alarms_errors (
    id              BIGSERIAL    PRIMARY KEY,
    occurred_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    severity        VARCHAR(20)  NOT NULL DEFAULT 'error'
                                  CHECK (severity IN ('info', 'warning', 'error', 'critical')),
    message         TEXT         NOT NULL,
    status          VARCHAR(20)  NOT NULL DEFAULT 'unsolved'
                                  CHECK (status IN ('unsolved', 'solved')),
    notes           JSONB        NOT NULL DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alarms_errors_occurred_at
    ON alarms_errors (occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_alarms_errors_status
    ON alarms_errors (status);

COMMENT ON TABLE alarms_errors IS
    'System alarms/errors shown on /semos/workspace and triaged at /semos/admin/alarms. '
    'No i18n. status/notes are admin-editable; all other fields are written by system/monitoring code. '
    'notes is an append-only JSON array of {time, user, note} objects.';

-- +goose Down
DROP INDEX IF EXISTS idx_alarms_errors_status;
DROP INDEX IF EXISTS idx_alarms_errors_occurred_at;
DROP TABLE IF EXISTS alarms_errors;
