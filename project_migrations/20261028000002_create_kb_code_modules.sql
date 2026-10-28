-- The WPI code modules: the code that draws elements, performs actions or lays out
-- pages (devdoc 2026102601 §3.5.2; OpenSpec change wpi-code-modules). An element
-- class names its module in code_module_id; the class is valid only while that
-- module has a row here with module_type 'element' and status 'active', and an
-- active class also needs the module's code in the interpreter. Each module is
-- documented in KnowledgeStore doc-repo/code_modules/<module_doc>. There is no
-- write API: rows come from migrations or psql, like kb.stored_stmts.
--
-- Seeds the six element modules that exist today; their code is registered under
-- the same IDs in web/src/lib/wpi/elements/index.ts and server/api/wpi/validate.go.

-- +goose Up
CREATE TABLE IF NOT EXISTS kb.code_modules (
    code_module_id TEXT        PRIMARY KEY CHECK (code_module_id ~ '^[a-z0-9][a-z0-9-]{0,127}$'),
    module_name    TEXT        NOT NULL UNIQUE CHECK (module_name <> ''),
    description    TEXT        NOT NULL DEFAULT '',
    module_type    TEXT        NOT NULL CHECK (module_type IN ('element', 'action', 'page', 'layouter')),
    module_doc     TEXT        NOT NULL DEFAULT '',
    notes          TEXT        NOT NULL DEFAULT '',
    status         TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    creator        TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE kb.code_modules IS
    'WPI code modules. An element class names its module in code_module_id; only an active '
    'element module may be named (devdoc 2026102601 §3.5.2).';
COMMENT ON COLUMN kb.code_modules.module_type IS
    'What the module implements: element, action, page or layouter.';
COMMENT ON COLUMN kb.code_modules.module_doc IS
    'File name of the module''s document in KnowledgeStore doc-repo/code_modules.';
COMMENT ON COLUMN kb.code_modules.status IS
    'active, or suspended: classes may not name a suspended module, and apps using such a class do not load.';

INSERT INTO kb.code_modules (code_module_id, module_name, description, module_type, module_doc, creator) VALUES
    ('text',   'Text',   'One paragraph of text: a fixed string or the value of an input doc attribute.', 'element', 'text.md',   'migration:wpi-code-modules'),
    ('input',  'Input',  'A caption above a one-line field that shows and edits one input doc attribute.', 'element', 'input.md',  'migration:wpi-code-modules'),
    ('select', 'Select', 'A caption above a pulldown menu of fixed options.',                              'element', 'select.md', 'migration:wpi-code-modules'),
    ('button', 'Button', 'A button that runs an action when it is clicked.',                               'element', 'button.md', 'migration:wpi-code-modules'),
    ('list',   'List',   'Records from a query, one per row, under a header of column labels.',           'element', 'list.md',   'migration:wpi-code-modules'),
    ('menu',   'Menu',   'A tree of items and folders; items open pages.',                                 'element', 'menu.md',   'migration:wpi-code-modules')
ON CONFLICT (code_module_id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS kb.code_modules;
