-- Category trees, their change log and revisions, and artifact bindings
-- (devdoc 2026102003-devdoc-categories §8, openspec change category-trees).

-- +goose Up
CREATE TABLE IF NOT EXISTS kb.category_trees (
    id             TEXT        PRIMARY KEY,
    canonical_name TEXT        NOT NULL UNIQUE,
    namespace      TEXT        NOT NULL,
    pref_labels    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    definitions    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    source         TEXT        NOT NULL DEFAULT '',
    current_seq    BIGINT      NOT NULL DEFAULT 0,
    created_by     TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS kb.category_tree_types (
    artifact_type TEXT        PRIMARY KEY,
    tree_id       TEXT        NOT NULL REFERENCES kb.category_trees(id),
    updated_by    TEXT        NOT NULL DEFAULT '',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS kb.category_nodes (
    id             TEXT        PRIMARY KEY,
    tree_id        TEXT        NOT NULL REFERENCES kb.category_trees(id),
    parent_id      TEXT        REFERENCES kb.category_nodes(id),
    canonical_name TEXT        NOT NULL,
    is_others      BOOLEAN     NOT NULL DEFAULT FALSE,
    pref_labels    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    alt_labels     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    hidden_labels  JSONB       NOT NULL DEFAULT '{}'::jsonb,
    definitions    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    scope_notes    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    source         TEXT        NOT NULL DEFAULT '',
    matches        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retired_at     TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_category_nodes_tree_name
    ON kb.category_nodes (tree_id, canonical_name) WHERE retired_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_category_nodes_parent ON kb.category_nodes (parent_id);

CREATE TABLE IF NOT EXISTS kb.category_tree_changes (
    id            BIGSERIAL   PRIMARY KEY,
    tree_id       TEXT        NOT NULL REFERENCES kb.category_trees(id),
    change_set_id TEXT        NOT NULL,
    seq           BIGINT      NOT NULL,
    operation     TEXT        NOT NULL CHECK (operation IN ('add', 'delete', 'move', 'update')),
    node_id       TEXT        NOT NULL,
    before_value  JSONB,
    after_value   JSONB,
    author        TEXT        NOT NULL DEFAULT '',
    reason        TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tree_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_category_tree_changes_set ON kb.category_tree_changes (change_set_id);

CREATE TABLE IF NOT EXISTS kb.category_tree_revisions (
    id         BIGSERIAL   PRIMARY KEY,
    tree_id    TEXT        NOT NULL REFERENCES kb.category_trees(id),
    name       TEXT        NOT NULL,
    seq        BIGINT      NOT NULL,
    note       TEXT        NOT NULL DEFAULT '',
    created_by TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tree_id, name)
);

CREATE TABLE IF NOT EXISTS kb.category_classifications (
    id              BIGSERIAL   PRIMARY KEY,
    artifact_type   TEXT        NOT NULL,
    artifact_id     TEXT        NOT NULL,
    record_id       BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    tree_id         TEXT        NOT NULL REFERENCES kb.category_trees(id),
    tree_seq        BIGINT      NOT NULL,
    status          TEXT        NOT NULL
        CHECK (status IN ('pending', 'published', 'failed', 'in_review', 'unclassified')),
    reason          TEXT        NOT NULL DEFAULT '',
    proposals       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    evidence        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    model_name      TEXT        NOT NULL DEFAULT '',
    model_profile   TEXT        NOT NULL DEFAULT '',
    prompt_refs     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    reviewed_by     TEXT        NOT NULL DEFAULT '',
    review_note     TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_category_classifications_artifact
    ON kb.category_classifications (artifact_type, artifact_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_category_classifications_record
    ON kb.category_classifications (record_id, artifact_type);
CREATE INDEX IF NOT EXISTS idx_category_classifications_status
    ON kb.category_classifications (status) WHERE status IN ('failed', 'in_review');

CREATE TABLE IF NOT EXISTS kb.category_bindings (
    id                BIGSERIAL   PRIMARY KEY,
    node_id           TEXT        NOT NULL REFERENCES kb.category_nodes(id),
    artifact_type     TEXT        NOT NULL,
    artifact_id       TEXT        NOT NULL,
    record_id         BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    classification_id BIGINT      REFERENCES kb.category_classifications(id) ON DELETE SET NULL,
    tree_seq          BIGINT      NOT NULL,
    stale             BOOLEAN     NOT NULL DEFAULT FALSE,
    stale_change_id   BIGINT      REFERENCES kb.category_tree_changes(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (artifact_type, artifact_id, node_id)
);

CREATE INDEX IF NOT EXISTS idx_category_bindings_node ON kb.category_bindings (node_id);
CREATE INDEX IF NOT EXISTS idx_category_bindings_record ON kb.category_bindings (record_id, artifact_type);
CREATE INDEX IF NOT EXISTS idx_category_bindings_stale ON kb.category_bindings (artifact_type) WHERE stale;

-- +goose Down
DROP TABLE IF EXISTS kb.category_bindings;
DROP TABLE IF EXISTS kb.category_classifications;
DROP TABLE IF EXISTS kb.category_tree_revisions;
DROP TABLE IF EXISTS kb.category_tree_changes;
DROP TABLE IF EXISTS kb.category_nodes;
DROP TABLE IF EXISTS kb.category_tree_types;
DROP TABLE IF EXISTS kb.category_trees;
