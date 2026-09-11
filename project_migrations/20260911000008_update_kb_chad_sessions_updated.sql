-- +goose Up
ALTER TABLE kb.chad_sessions
    ALTER COLUMN updated DROP DEFAULT,
    ALTER COLUMN updated TYPE TIMESTAMPTZ
    USING to_timestamp(updated),
    ALTER COLUMN updated SET DEFAULT NOW();

-- +goose Down
ALTER TABLE kb.chad_sessions
    ALTER COLUMN updated TYPE DOUBLE PRECISION
    USING extract(epoch FROM updated),
    ALTER COLUMN updated SET DEFAULT 0;
