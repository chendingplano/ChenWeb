-- +goose Up
-- Gold provisions written by the extract-provisions-benchmark skill (.agents/skills), the answer
-- key for extract_provisions. A run is one row in testbed.provision_benchmark_runs (audit record:
-- candidate ledger, coverage, reviews), its accepted provisions in testbed.provisions and its
-- excluded/duplicate candidates in testbed.provisions_dropped, all inserted in one transaction.
-- testbed.provisions keeps the kb.provisions names for the fields the scorer checks; section,
-- provision_kind, source_table_rows, evidence and confidence are gold-only. A run is identified by
-- (skill_name, skill_version, model_name, run_id).
CREATE TABLE IF NOT EXISTS testbed.provision_benchmark_runs (
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
CREATE INDEX IF NOT EXISTS idx_testbed_provision_benchmark_runs_document
    ON testbed.provision_benchmark_runs (input_record_id);

CREATE TABLE IF NOT EXISTS testbed.provisions (
    id                   BIGSERIAL   PRIMARY KEY,
    input_record_id      BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    skill_name           TEXT        NOT NULL,
    skill_version        TEXT        NOT NULL,
    model_name           TEXT        NOT NULL,
    run_id               TEXT        NOT NULL,
    source_filename      TEXT        NOT NULL,
    source_sha256        TEXT        NOT NULL,
    prov_id              TEXT        NOT NULL,
    section              TEXT        NOT NULL DEFAULT '',
    source_line_spans    JSONB       NOT NULL CHECK (jsonb_typeof(source_line_spans) = 'array'),
    source_table_rows    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    evidence             TEXT        NOT NULL,
    provision            TEXT        NOT NULL,
    provision_en         TEXT        NOT NULL DEFAULT '',
    prov_name            TEXT        NOT NULL,
    prov_name_en         TEXT,
    provision_type       TEXT        NOT NULL CHECK (provision_type IN ('mandatory', 'recommended', 'optional')),
    provision_kind       TEXT        NOT NULL CHECK (provision_kind IN
                             ('requirement', 'prohibition', 'instruction', 'recommendation', 'permission')),
    provision_subject    TEXT        NOT NULL,
    provision_subject_en TEXT,
    location_type        TEXT        NOT NULL CHECK (location_type IN
                             ('sentence', 'paragraph', 'bullet', 'table_row', 'table_cell', 'heading_context', 'mixed')),
    is_explicit          BOOLEAN     NOT NULL,
    confidence           DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    ext_info             JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (skill_name, skill_version, model_name, run_id, prov_id),
    FOREIGN KEY (skill_name, skill_version, model_name, run_id)
        REFERENCES testbed.provision_benchmark_runs (skill_name, skill_version, model_name, run_id)
        ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_testbed_provisions_document_run
    ON testbed.provisions (input_record_id, skill_name, skill_version, model_name, run_id);

CREATE TABLE IF NOT EXISTS testbed.provisions_dropped (
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
        REFERENCES testbed.provision_benchmark_runs (skill_name, skill_version, model_name, run_id)
        ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_testbed_provisions_dropped_document_run
    ON testbed.provisions_dropped (input_record_id, skill_name, skill_version, model_name, run_id);

-- +goose Down
DROP INDEX IF EXISTS testbed.idx_testbed_provisions_dropped_document_run;
DROP TABLE IF EXISTS testbed.provisions_dropped;
DROP INDEX IF EXISTS testbed.idx_testbed_provisions_document_run;
DROP TABLE IF EXISTS testbed.provisions;
DROP INDEX IF EXISTS testbed.idx_testbed_provision_benchmark_runs_document;
DROP TABLE IF EXISTS testbed.provision_benchmark_runs;
