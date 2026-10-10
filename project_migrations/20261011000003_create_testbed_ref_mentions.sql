-- +goose Up
-- Gold references written by the extract-references-benchmark skill (.agents/skills), the answer
-- key for extract_references. A run is one row in testbed.reference_benchmark_runs (audit record:
-- candidate ledger, coverage, reviews, the document's own identifier), its accepted references in
-- testbed.ref_mentions and its excluded/duplicate candidates in testbed.ref_mentions_dropped, all
-- inserted in one transaction. testbed.ref_mentions mirrors the detection columns of
-- kb.ref_mentions; resolution columns (resource, holding, metric state) are system-derived and
-- not benchmarked here. A run is identified by (skill_name, skill_version, model_name, run_id).
CREATE TABLE IF NOT EXISTS testbed.reference_benchmark_runs (
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
    document_identifier TEXT        NOT NULL DEFAULT '',
    coverage            JSONB       NOT NULL CHECK (jsonb_typeof(coverage) = 'array'),
    candidates          JSONB       NOT NULL CHECK (jsonb_typeof(candidates) = 'array'),
    completeness_review TEXT        NOT NULL,
    correctness_review  TEXT        NOT NULL,
    reviewed_at         TIMESTAMPTZ,
    ext_info            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (skill_name, skill_version, model_name, run_id)
);
CREATE INDEX IF NOT EXISTS idx_testbed_reference_benchmark_runs_document
    ON testbed.reference_benchmark_runs (input_record_id);

CREATE TABLE IF NOT EXISTS testbed.ref_mentions (
    id                   BIGSERIAL   PRIMARY KEY,
    input_record_id      BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    skill_name           TEXT        NOT NULL,
    skill_version        TEXT        NOT NULL,
    model_name           TEXT        NOT NULL,
    run_id               TEXT        NOT NULL,
    source_filename      TEXT        NOT NULL,
    source_sha256        TEXT        NOT NULL,
    ref_id               TEXT        NOT NULL,
    section              TEXT        NOT NULL DEFAULT '',
    source_line_spans    JSONB       NOT NULL CHECK (jsonb_typeof(source_line_spans) = 'array'),
    source_table_rows    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    evidence             TEXT        NOT NULL,
    ref_form             TEXT        NOT NULL CHECK (ref_form IN ('listed', 'in_text')),
    list_kind            TEXT        NOT NULL DEFAULT '' CHECK (list_kind IN ('', 'normative', 'informative')),
    list_label           TEXT        NOT NULL DEFAULT '',
    list_entry_ref_id    TEXT        NOT NULL DEFAULT '',
    normativity          TEXT        NOT NULL CHECK (normativity IN ('normative', 'informative', 'unlisted')),
    in_requirement       BOOLEAN     NOT NULL,
    list_mismatch        TEXT        NOT NULL DEFAULT '' CHECK (list_mismatch IN ('', 'not_listed', 'never_cited')),
    mechanism            TEXT        NOT NULL,
    cited_raw            TEXT        NOT NULL,
    identifiers          JSONB       NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(identifiers) = 'array'),
    target_type          TEXT        NOT NULL,
    target_subtype       TEXT        NOT NULL,
    target_subtype_label TEXT        NOT NULL DEFAULT '',
    target_title         TEXT        NOT NULL DEFAULT '',
    target_version       TEXT        NOT NULL DEFAULT '',
    relationships        TEXT[]      NOT NULL,
    target_scope         TEXT        NOT NULL,
    target_locator       TEXT        NOT NULL DEFAULT '',
    target_content       TEXT        NOT NULL,
    metric_name          TEXT        NOT NULL DEFAULT '',
    metric_name_en       TEXT,
    metric_subject       TEXT        NOT NULL DEFAULT '',
    metric_subject_en    TEXT,
    condition            TEXT        NOT NULL DEFAULT '',
    internal             BOOLEAN     NOT NULL,
    confidence           DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    ext_info             JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (skill_name, skill_version, model_name, run_id, ref_id),
    FOREIGN KEY (skill_name, skill_version, model_name, run_id)
        REFERENCES testbed.reference_benchmark_runs (skill_name, skill_version, model_name, run_id)
        ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_testbed_ref_mentions_document_run
    ON testbed.ref_mentions (input_record_id, skill_name, skill_version, model_name, run_id);

CREATE TABLE IF NOT EXISTS testbed.ref_mentions_dropped (
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
        REFERENCES testbed.reference_benchmark_runs (skill_name, skill_version, model_name, run_id)
        ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_testbed_ref_mentions_dropped_document_run
    ON testbed.ref_mentions_dropped (input_record_id, skill_name, skill_version, model_name, run_id);

-- +goose Down
DROP INDEX IF EXISTS testbed.idx_testbed_ref_mentions_dropped_document_run;
DROP TABLE IF EXISTS testbed.ref_mentions_dropped;
DROP INDEX IF EXISTS testbed.idx_testbed_ref_mentions_document_run;
DROP TABLE IF EXISTS testbed.ref_mentions;
DROP INDEX IF EXISTS testbed.idx_testbed_reference_benchmark_runs_document;
DROP TABLE IF EXISTS testbed.reference_benchmark_runs;
