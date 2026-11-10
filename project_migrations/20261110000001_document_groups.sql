-- +goose Up
-- Document groups (openspec change document-groups): a registry of documents with stable IDs,
-- groups made of directory/file members, per-group categories, menu binding and PDF exports.
-- doc_id comes from the shared ID manager (sequence doc_registry_id), not from this table.

CREATE TABLE IF NOT EXISTS kb.doc_documents (
    doc_id       BIGINT PRIMARY KEY,
    logical_key  TEXT NOT NULL,
    kind         TEXT NOT NULL CHECK (kind IN ('markdown', 'pdf')),
    title        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS doc_documents_logical_key_idx ON kb.doc_documents (logical_key);

CREATE TABLE IF NOT EXISTS kb.doc_files (
    file_id      BIGSERIAL PRIMARY KEY,
    doc_id       BIGINT NOT NULL REFERENCES kb.doc_documents (doc_id),
    path         TEXT NOT NULL UNIQUE,
    version      TEXT NOT NULL DEFAULT '',
    hash         TEXT NOT NULL,
    size_bytes   BIGINT NOT NULL DEFAULT 0,
    mtime_ns     BIGINT NOT NULL DEFAULT 0,
    missing      BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS doc_files_doc_idx ON kb.doc_files (doc_id);
CREATE INDEX IF NOT EXISTS doc_files_hash_idx ON kb.doc_files (hash);

CREATE TABLE IF NOT EXISTS kb.doc_groups (
    group_id        BIGSERIAL PRIMARY KEY,
    group_key       TEXT NOT NULL UNIQUE,
    label           JSONB NOT NULL DEFAULT '{}'::jsonb,
    parent_menu_id  TEXT NOT NULL DEFAULT 'documents',
    sort_order      INTEGER NOT NULL DEFAULT 100,
    access_role     JSONB NOT NULL DEFAULT '[]'::jsonb,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_by      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS kb.doc_group_members (
    member_id    BIGSERIAL PRIMARY KEY,
    group_id     BIGINT NOT NULL REFERENCES kb.doc_groups (group_id) ON DELETE CASCADE,
    member_type  TEXT NOT NULL CHECK (member_type IN ('dir', 'file')),
    path         TEXT NOT NULL,
    UNIQUE (group_id, member_type, path)
);

CREATE TABLE IF NOT EXISTS kb.doc_group_exclusions (
    group_id  BIGINT NOT NULL REFERENCES kb.doc_groups (group_id) ON DELETE CASCADE,
    doc_id    BIGINT NOT NULL REFERENCES kb.doc_documents (doc_id),
    PRIMARY KEY (group_id, doc_id)
);

CREATE TABLE IF NOT EXISTS kb.doc_group_categories (
    group_id       BIGINT NOT NULL REFERENCES kb.doc_groups (group_id) ON DELETE CASCADE,
    doc_id         BIGINT NOT NULL REFERENCES kb.doc_documents (doc_id),
    category       TEXT NOT NULL,
    model          TEXT NOT NULL DEFAULT '',
    classified_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, doc_id)
);

CREATE TABLE IF NOT EXISTS kb.doc_pdf_exports (
    file_id      BIGINT PRIMARY KEY REFERENCES kb.doc_files (file_id) ON DELETE CASCADE,
    source_hash  TEXT NOT NULL,
    pdf_path     TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed the two groups that replace the hard-coded Resources > Documents entries.
INSERT INTO kb.doc_groups (group_key, label, parent_menu_id, sort_order, access_role, created_by) VALUES
    ('devdocs', '{"en":"Dev Documents","zh-cn":"开发文档"}'::jsonb, 'documents', 10,
        '["admin","root","guest","dev","k_engineer","trial"]'::jsonb, 'migration'),
    ('user-manuals', '{"en":"User''s Manuals","zh-cn":"用户手册"}'::jsonb, 'documents', 20,
        '["admin","root","guest","dev","k_engineer","trial"]'::jsonb, 'migration')
ON CONFLICT (group_key) DO NOTHING;

INSERT INTO kb.doc_group_members (group_id, member_type, path)
SELECT group_id, 'dir', 'KnowledgeStore/doc-repo/devdocs' FROM kb.doc_groups WHERE group_key = 'devdocs'
ON CONFLICT DO NOTHING;
INSERT INTO kb.doc_group_members (group_id, member_type, path)
SELECT group_id, 'dir', 'KnowledgeStore/doc-repo/user-manuals' FROM kb.doc_groups WHERE group_key = 'user-manuals'
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS kb.doc_pdf_exports;
DROP TABLE IF EXISTS kb.doc_group_categories;
DROP TABLE IF EXISTS kb.doc_group_exclusions;
DROP TABLE IF EXISTS kb.doc_group_members;
DROP TABLE IF EXISTS kb.doc_groups;
DROP TABLE IF EXISTS kb.doc_files;
DROP TABLE IF EXISTS kb.doc_documents;
