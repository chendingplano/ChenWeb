-- +goose Up
-- Immutable, reproducible normative-review input. Selected profile release
-- ids and applicability/precedence facts are frozen as JSON snapshots.
CREATE TABLE IF NOT EXISTS kb.ontology_review_scopes (
    review_scope_id       TEXT PRIMARY KEY,
    reviewed_document_ids JSONB NOT NULL DEFAULT '[]',
    target_object_ids     JSONB NOT NULL DEFAULT '[]',
    target_class_term_ids JSONB NOT NULL DEFAULT '[]',
    as_of_date            DATE NOT NULL,
    jurisdiction          TEXT NOT NULL,
    operating_context     TEXT,
    selected_profiles     JSONB NOT NULL,
    selection_mode        TEXT NOT NULL CHECK (selection_mode IN ('explicit', 'deterministic_rule')),
    precedence_policy     JSONB NOT NULL DEFAULT '{}',
    closed_dimensions     JSONB NOT NULL DEFAULT '[]',
    selected_by           TEXT,
    selection_reason      TEXT,
    create_time           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_review_scopes;
