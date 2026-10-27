-- Named SQL statements and named JSON files for the WPI actions query and
-- load-file (devdoc 2026102601 §3.2.2; OpenSpec change wpi-pages). A descriptor
-- names a row; the SQL text and the file content stay on the server, because
-- some customers' firewalls block traffic that carries SQL text. There is no
-- write API: rows come from migrations or psql. access_controls lists the user
-- roles that may use a row; NULL, empty or containing 'all' means every
-- logged-in user.

-- +goose Up
CREATE TABLE IF NOT EXISTS kb.stored_stmts (
    stmt_name       TEXT        PRIMARY KEY CHECK (stmt_name ~ '^[a-z0-9][a-z0-9-]{0,127}$'),
    sql_stmt        TEXT        NOT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    creator         TEXT        NOT NULL DEFAULT '',
    access_controls TEXT[],
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE kb.stored_stmts IS
    'SQL statements that WPI query actions name by stmt_name. Run read-only, with the '
    'action''s params bound to $1..$n; each row comes back as a JSON object.';
COMMENT ON COLUMN kb.stored_stmts.access_controls IS
    'User roles that may run the statement; NULL, empty or containing ''all'' means everyone.';

CREATE TABLE IF NOT EXISTS kb.stored_files (
    file_name       TEXT        PRIMARY KEY CHECK (file_name ~ '^[a-z0-9][a-z0-9-]{0,127}$'),
    file_content    JSONB       NOT NULL CHECK (jsonb_typeof(file_content) = 'object'),
    description     TEXT        NOT NULL DEFAULT '',
    creator         TEXT        NOT NULL DEFAULT '',
    access_controls TEXT[],
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE kb.stored_files IS
    'JSON documents that WPI load-file actions name by file_name; the content becomes a '
    'panel''s input doc.';
COMMENT ON COLUMN kb.stored_files.access_controls IS
    'User roles that may read the file; NULL, empty or containing ''all'' means everyone.';

-- +goose Down
DROP TABLE IF EXISTS kb.stored_files;
DROP TABLE IF EXISTS kb.stored_stmts;
