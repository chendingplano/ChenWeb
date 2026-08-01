-- +goose Up
-- A comparison scope freezes the target, metric, governing releases, and
-- evidence-closure policy used to produce one reproducible DR21/DR22 run.
CREATE TABLE IF NOT EXISTS kb.ontology_comparison_scopes (
    comparison_scope_id TEXT PRIMARY KEY,
    target_object_ids   JSONB NOT NULL,
    metric_keys         JSONB NOT NULL,
    as_of_date          DATE NOT NULL,
    module_releases     JSONB NOT NULL,
    profile_releases    JSONB NOT NULL,
    precedence_policy   JSONB NOT NULL DEFAULT '{}',
    closed_dimensions   JSONB NOT NULL DEFAULT '{}',
    selected_by         TEXT,
    selection_reason    TEXT,
    create_time         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS kb.ontology_comparison_runs (
    id                  BIGSERIAL PRIMARY KEY,
    comparison_scope_id TEXT NOT NULL REFERENCES kb.ontology_comparison_scopes(comparison_scope_id) ON DELETE RESTRICT,
    input_record_id     BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    assertion_watermark TEXT NOT NULL,
    comparator_version  TEXT NOT NULL,
    create_time         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ontology_comparison_runs_scope
    ON kb.ontology_comparison_runs (comparison_scope_id, id);

-- Each cell is explicitly directional: subject (enterprise) is compared to
-- one authority family. Assertion lists retain every cited input even when a
-- precedence policy chooses one representative.
CREATE TABLE IF NOT EXISTS kb.ontology_comparison_cells (
    id                                      BIGSERIAL PRIMARY KEY,
    comparison_run_id                       BIGINT NOT NULL REFERENCES kb.ontology_comparison_runs(id) ON DELETE CASCADE,
    target_object_id                        TEXT NOT NULL,
    metric_key                              TEXT NOT NULL,
    subject_family                          TEXT NOT NULL,
    subject_representative_assertion_id     BIGINT REFERENCES kb.semantic_assertions(id) ON DELETE RESTRICT,
    subject_assertions                      JSONB NOT NULL DEFAULT '[]',
    subject_remainder_count                 INTEGER NOT NULL DEFAULT 0 CHECK (subject_remainder_count >= 0),
    authority_family                        TEXT NOT NULL,
    authority_representative_assertion_id   BIGINT REFERENCES kb.semantic_assertions(id) ON DELETE RESTRICT,
    authority_assertions                    JSONB NOT NULL DEFAULT '[]',
    authority_remainder_count               INTEGER NOT NULL DEFAULT 0 CHECK (authority_remainder_count >= 0),
    verdict                                 TEXT NOT NULL CHECK (verdict IN ('identical', 'equivalent', 'stronger', 'weaker', 'conflict', 'incomparable', 'qualitative_only', 'limit_absent', 'standard_absent', 'not_applicable', 'indeterminate')),
    direction                               TEXT NOT NULL CHECK (direction = 'subject_to_authority'),
    rationale                               TEXT NOT NULL,
    create_time                             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (comparison_run_id, target_object_id, metric_key, subject_family, authority_family)
);

CREATE INDEX IF NOT EXISTS idx_ontology_comparison_cells_run
    ON kb.ontology_comparison_cells (comparison_run_id, target_object_id, metric_key);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_comparison_cells;
DROP TABLE IF EXISTS kb.ontology_comparison_runs;
DROP TABLE IF EXISTS kb.ontology_comparison_scopes;
