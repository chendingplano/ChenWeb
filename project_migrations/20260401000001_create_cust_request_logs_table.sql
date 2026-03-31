-- +goose Up
CREATE TABLE IF NOT EXISTS cust_request_logs (
    id          BIGSERIAL       PRIMARY KEY,
    cust_name   VARCHAR(256)    NOT NULL,
    cust_email  VARCHAR(256)    NOT NULL,
    "desc"      TEXT            DEFAULT NULL,
    purpose     TEXT            DEFAULT NULL,
    create_at   TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    update_at   TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS cust_request_logs;
