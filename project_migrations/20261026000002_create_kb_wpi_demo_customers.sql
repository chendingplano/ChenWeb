-- Demo table for the Web Page Interpreter pilot (OpenSpec change wpi-pilot): the
-- customer-admin WPI app lists, inserts and updates these rows.

-- +goose Up
CREATE TABLE IF NOT EXISTS kb.wpi_demo_customers (
    id           BIGSERIAL   PRIMARY KEY,
    user_name    TEXT        NOT NULL,
    phone_number TEXT        NOT NULL DEFAULT '',
    email        TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO kb.wpi_demo_customers (user_name, phone_number, email)
SELECT v.user_name, v.phone_number, v.email
FROM (VALUES
    ('alice', '+1 415 555 0101', 'alice@example.com'),
    ('bob', '+44 20 7946 0102', 'bob@example.com'),
    ('chen', '+86 10 5555 0103', 'chen@example.com')
) AS v (user_name, phone_number, email)
WHERE NOT EXISTS (SELECT 1 FROM kb.wpi_demo_customers);

COMMENT ON TABLE kb.wpi_demo_customers IS
    'Demo customers for the WPI pilot app customer-admin. Not used by anything else.';

-- +goose Down
DROP TABLE IF EXISTS kb.wpi_demo_customers;
