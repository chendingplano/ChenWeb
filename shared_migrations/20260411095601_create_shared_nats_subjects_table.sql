-- +goose Up
CREATE SCHEMA IF NOT EXISTS shared;

CREATE TABLE IF NOT EXISTS shared.nats_subjects (
    id           BIGSERIAL      PRIMARY KEY,
    subject      VARCHAR(255)   NOT NULL,
    description  TEXT           DEFAULT NULL,
    is_active    BOOLEAN        NOT NULL DEFAULT TRUE,
    created_by   VARCHAR(255)   DEFAULT NULL,
    created_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_nats_subjects_subject_lower_unique
    ON shared.nats_subjects (LOWER(subject));

CREATE INDEX IF NOT EXISTS idx_nats_subjects_is_active
    ON shared.nats_subjects (is_active);

-- +goose Down
DROP INDEX IF EXISTS shared.idx_nats_subjects_is_active;
DROP INDEX IF EXISTS shared.idx_nats_subjects_subject_lower_unique;
DROP TABLE IF EXISTS shared.nats_subjects;
