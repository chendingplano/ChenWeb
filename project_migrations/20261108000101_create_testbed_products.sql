-- +goose Up
-- Gold product relations written by the extract-products-benchmark skill (.agents/skills), the
-- answer key for extract_products. A run is one row in testbed.product_benchmark_runs (audit
-- record: candidate ledger, coverage, reviews), its gold rows in testbed.products and its
-- excluded/duplicate candidates in testbed.products_dropped, all inserted in one transaction.
-- One gold row is one (product, relation_type) pair, like kb.products; rows of one product share
-- product_key. testbed.products keeps the kb.products names for the fields the scorer checks;
-- product_key, section, rule_id and ext_info are gold-only. A run is identified by
-- (skill_name, skill_version, model_name, run_id).
CREATE TABLE IF NOT EXISTS testbed.product_benchmark_runs (
    skill_name          TEXT        NOT NULL,
    skill_version       TEXT        NOT NULL,
    model_name          TEXT        NOT NULL,
    run_id              TEXT        NOT NULL CHECK (run_id ~ '^[0-9]{8}_[0-9]{6}$'),
    input_record_id     BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    source_filename     TEXT        NOT NULL,
    source_sha256       TEXT        NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    rules_sha256        TEXT        NOT NULL CHECK (rules_sha256 ~ '^[0-9a-f]{64}$'),
    language            TEXT        NOT NULL,
    title               TEXT,
    doc_no              TEXT,
    coverage            JSONB       NOT NULL CHECK (jsonb_typeof(coverage) = 'array'),
    candidates          JSONB       NOT NULL CHECK (jsonb_typeof(candidates) = 'array'),
    completeness_review TEXT        NOT NULL,
    correctness_review  TEXT        NOT NULL,
    reviewed_at         TIMESTAMPTZ,
    ext_info            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (skill_name, skill_version, model_name, run_id)
);
CREATE INDEX IF NOT EXISTS idx_testbed_product_benchmark_runs_document
    ON testbed.product_benchmark_runs (input_record_id);

CREATE TABLE IF NOT EXISTS testbed.products (
    id                  BIGSERIAL   PRIMARY KEY,
    input_record_id     BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    skill_name          TEXT        NOT NULL,
    skill_version       TEXT        NOT NULL,
    model_name          TEXT        NOT NULL,
    run_id              TEXT        NOT NULL,
    source_filename     TEXT        NOT NULL,
    source_sha256       TEXT        NOT NULL,
    product_rel_id      TEXT        NOT NULL,
    product_key         TEXT        NOT NULL,
    section             TEXT        NOT NULL DEFAULT '',
    product_name        TEXT        NOT NULL,
    product_name_en     TEXT,
    canonical_name      TEXT        NOT NULL,
    canonical_name_en   TEXT,
    product_type        TEXT        NOT NULL CHECK (product_type IN
                            ('specific_product', 'product_class', 'component', 'material', 'software',
                             'system', 'equipment', 'consumable', 'packaging', 'other')),
    relation_type       TEXT        NOT NULL CHECK (relation_type IN
                            ('scope', 'regulated_object', 'requirement_target', 'performance_requirement',
                             'design_requirement', 'material_requirement', 'testing_requirement',
                             'certification_requirement', 'usage_condition', 'installation_requirement',
                             'maintenance_requirement', 'storage_requirement', 'prohibited_product',
                             'exempted_product', 'component_of', 'contains_product', 'compatible_with',
                             'replacement_or_alternative', 'measurement_object', 'risk_source', 'other')),
    relation_summary    TEXT        NOT NULL,
    relation_summary_en TEXT        NOT NULL DEFAULT '',
    evidence_quote      TEXT        NOT NULL,
    evidence_lines      JSONB       NOT NULL CHECK (jsonb_typeof(evidence_lines) = 'array'),
    obligation_level    TEXT        NOT NULL CHECK (obligation_level IN
                            ('mandatory', 'recommended', 'permitted', 'prohibited', 'conditional',
                             'descriptive', 'unknown')),
    requirement_text    TEXT        NOT NULL DEFAULT '',
    requirement_text_en TEXT        NOT NULL DEFAULT '',
    conditions          JSONB       NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(conditions) = 'array'),
    exceptions          JSONB       NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(exceptions) = 'array'),
    parameters          JSONB       NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(parameters) = 'array'),
    related_products    JSONB       NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(related_products) = 'array'),
    responsible_actor   TEXT        NOT NULL DEFAULT '',
    confidence          DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    rule_id             TEXT        NOT NULL,
    ext_info            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (skill_name, skill_version, model_name, run_id, product_rel_id),
    UNIQUE (skill_name, skill_version, model_name, run_id, product_key, relation_type),
    FOREIGN KEY (skill_name, skill_version, model_name, run_id)
        REFERENCES testbed.product_benchmark_runs (skill_name, skill_version, model_name, run_id)
        ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_testbed_products_document_run
    ON testbed.products (input_record_id, skill_name, skill_version, model_name, run_id);

CREATE TABLE IF NOT EXISTS testbed.products_dropped (
    id              BIGSERIAL   PRIMARY KEY,
    input_record_id BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    skill_name      TEXT        NOT NULL,
    skill_version   TEXT        NOT NULL,
    model_name      TEXT        NOT NULL,
    run_id          TEXT        NOT NULL,
    candidate_id    TEXT        NOT NULL,
    decision        TEXT        NOT NULL CHECK (decision IN ('excluded', 'duplicate')),
    drop_reason     TEXT        NOT NULL,
    candidate_data  JSONB       NOT NULL CHECK (jsonb_typeof(candidate_data) = 'object'),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (skill_name, skill_version, model_name, run_id, candidate_id),
    FOREIGN KEY (skill_name, skill_version, model_name, run_id)
        REFERENCES testbed.product_benchmark_runs (skill_name, skill_version, model_name, run_id)
        ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_testbed_products_dropped_document_run
    ON testbed.products_dropped (input_record_id, skill_name, skill_version, model_name, run_id);

-- +goose Down
DROP INDEX IF EXISTS testbed.idx_testbed_products_dropped_document_run;
DROP TABLE IF EXISTS testbed.products_dropped;
DROP INDEX IF EXISTS testbed.idx_testbed_products_document_run;
DROP TABLE IF EXISTS testbed.products;
DROP INDEX IF EXISTS testbed.idx_testbed_product_benchmark_runs_document;
DROP TABLE IF EXISTS testbed.product_benchmark_runs;
