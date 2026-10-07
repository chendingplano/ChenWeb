-- +goose Up
-- One row per extract-metrics-benchmark run. The run's audit record (candidate ledger, coverage,
-- reviews) used to be copied into ext_info of every metric row of the run (~30 KB per row); it is
-- stored here once. testbed.metrics.ext_info keeps only per-row data: evidence, annotation_notes
-- and candidate_ids (the ledger entries that produced or duplicate the row).
CREATE TABLE IF NOT EXISTS testbed.metric_benchmark_runs (
    skill_name TEXT NOT NULL,
    skill_version TEXT NOT NULL,
    model_name TEXT NOT NULL,
    benchmark_run_id TEXT NOT NULL CHECK (benchmark_run_id ~ '^[0-9]{8}_[0-9]{6}$'),
    input_record_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    source_filename TEXT NOT NULL,
    source_sha256 TEXT NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    rules_sha256 TEXT,          -- NULL for runs saved before tooling recorded it (rules 1.x)
    language TEXT NOT NULL,
    title TEXT,
    doc_no TEXT,
    coverage JSONB NOT NULL CHECK (jsonb_typeof(coverage) = 'array'),
    candidates JSONB NOT NULL CHECK (jsonb_typeof(candidates) = 'array'),
    completeness_review TEXT NOT NULL,
    correctness_review TEXT NOT NULL,
    reviewed_at TIMESTAMPTZ,
    ext_info JSONB NOT NULL DEFAULT '{}'::jsonb,  -- e.g. legacy_benchmark_run_id
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (skill_name, skill_version, model_name, benchmark_run_id)
);
CREATE INDEX IF NOT EXISTS idx_testbed_metric_benchmark_runs_document
    ON testbed.metric_benchmark_runs (input_record_id);

-- Every row of a run must carry the same audit record; refuse to collapse runs that do not.
-- +goose StatementBegin
DO $$
DECLARE bad TEXT;
BEGIN
    SELECT string_agg(DISTINCT benchmark_run_id, ', ') INTO bad FROM (
        SELECT benchmark_run_id FROM testbed.metrics
         GROUP BY skill_name, skill_version, model_name, benchmark_run_id
        HAVING count(DISTINCT jsonb_build_array(input_record_id, source_filename, source_sha256,
                   ext_info->'candidates', ext_info->'coverage', ext_info->'completeness_review',
                   ext_info->'correctness_review', ext_info->'rules_sha256', ext_info->'language',
                   ext_info->'title', ext_info->'doc_no', ext_info->'legacy_benchmark_run_id')) > 1) r;
    IF bad IS NOT NULL THEN
        RAISE EXCEPTION 'testbed.metrics runs with differing audit records: %', bad;
    END IF;
END $$;
-- +goose StatementEnd

INSERT INTO testbed.metric_benchmark_runs
    (skill_name, skill_version, model_name, benchmark_run_id, input_record_id, source_filename,
     source_sha256, rules_sha256, language, title, doc_no, coverage, candidates,
     completeness_review, correctness_review, reviewed_at, ext_info, created_at)
SELECT DISTINCT ON (skill_name, skill_version, model_name, benchmark_run_id)
       skill_name, skill_version, model_name, benchmark_run_id, input_record_id, source_filename,
       source_sha256, ext_info->>'rules_sha256', COALESCE(ext_info->>'language', ''),
       ext_info->>'title', ext_info->>'doc_no',
       COALESCE(ext_info->'coverage', '[]'::jsonb), COALESCE(ext_info->'candidates', '[]'::jsonb),
       COALESCE(ext_info->>'completeness_review', ''), COALESCE(ext_info->>'correctness_review', ''),
       (ext_info->>'reviewed_at')::timestamptz,
       jsonb_strip_nulls(jsonb_build_object('legacy_benchmark_run_id', ext_info->'legacy_benchmark_run_id')),
       created_at
  FROM testbed.metrics
 ORDER BY skill_name, skill_version, model_name, benchmark_run_id, id
ON CONFLICT DO NOTHING;

UPDATE testbed.metrics m
   SET ext_info = (m.ext_info - 'candidates' - 'coverage' - 'completeness_review'
                   - 'correctness_review' - 'rules_sha256' - 'language' - 'title' - 'doc_no'
                   - 'reviewed_at' - 'legacy_benchmark_run_id')
                  || jsonb_build_object('schema_version', '3', 'candidate_ids', COALESCE(
                       (SELECT jsonb_agg(c->>'candidate_id' ORDER BY c->>'candidate_id')
                          FROM jsonb_array_elements(m.ext_info->'candidates') c
                         WHERE c->'metric_ids' ? m.metric_id), '[]'::jsonb))
 WHERE m.ext_info ? 'candidates';

ALTER TABLE testbed.metrics ADD CONSTRAINT metrics_benchmark_run_fkey
    FOREIGN KEY (skill_name, skill_version, model_name, benchmark_run_id)
    REFERENCES testbed.metric_benchmark_runs (skill_name, skill_version, model_name, benchmark_run_id)
    ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE testbed.metrics DROP CONSTRAINT IF EXISTS metrics_benchmark_run_fkey;
UPDATE testbed.metrics m
   SET ext_info = (m.ext_info - 'candidate_ids')
                  || jsonb_strip_nulls(jsonb_build_object(
                       'schema_version', '2', 'rules_sha256', r.rules_sha256, 'language', r.language,
                       'title', r.title, 'doc_no', r.doc_no, 'reviewed_at', r.reviewed_at,
                       'coverage', r.coverage, 'candidates', r.candidates,
                       'completeness_review', r.completeness_review,
                       'correctness_review', r.correctness_review))
                  || r.ext_info
  FROM testbed.metric_benchmark_runs r
 WHERE (m.skill_name, m.skill_version, m.model_name, m.benchmark_run_id)
     = (r.skill_name, r.skill_version, r.model_name, r.benchmark_run_id);
DROP INDEX IF EXISTS testbed.idx_testbed_metric_benchmark_runs_document;
DROP TABLE IF EXISTS testbed.metric_benchmark_runs;
