-- +goose Up
-- Compliance review storage (openspec compliance-review-storage; KnowledgeStore design 2026112301).
-- A compliance finding keeps a small summary here; its content, activities, candidate
-- assessments, searches and run context live in the tables below and load on demand.
-- storage_version 1 = legacy (everything in metadata and description), 2 = normalized.
ALTER TABLE kb.doc_review_findings ADD COLUMN IF NOT EXISTS storage_version SMALLINT NOT NULL DEFAULT 1;
ALTER TABLE kb.doc_review_findings ADD COLUMN IF NOT EXISTS line_start INTEGER;
ALTER TABLE kb.doc_review_findings ADD COLUMN IF NOT EXISTS line_end INTEGER;
ALTER TABLE kb.doc_review_findings ADD COLUMN IF NOT EXISTS activity_count INTEGER;
ALTER TABLE kb.doc_review_findings ADD COLUMN IF NOT EXISTS search_result_count INTEGER;
ALTER TABLE kb.doc_review_findings ADD COLUMN IF NOT EXISTS selected_candidate_count INTEGER;
ALTER TABLE kb.doc_review_findings ADD COLUMN IF NOT EXISTS relevant_candidate_count INTEGER;
ALTER TABLE kb.doc_review_findings ADD COLUMN IF NOT EXISTS judged_candidate_count INTEGER;
ALTER TABLE kb.doc_review_findings ADD COLUMN IF NOT EXISTS verdict_counts JSONB;

-- Summary pages: run, aspect, the API's severity order, then id.
CREATE INDEX IF NOT EXISTS idx_doc_review_findings_run_aspect_severity
    ON kb.doc_review_findings (run_id, aspect,
        (CASE severity WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END), id);

-- Full content and execution details of one finding.
CREATE TABLE IF NOT EXISTS kb.doc_review_finding_details (
    finding_id                 BIGINT PRIMARY KEY REFERENCES kb.doc_review_findings(id) ON DELETE CASCADE,
    description                TEXT NOT NULL DEFAULT '',
    evidence                   TEXT NOT NULL DEFAULT '',
    suggestion                 TEXT NOT NULL DEFAULT '',
    canonical_language         TEXT NOT NULL DEFAULT '',
    canonical_origin           TEXT NOT NULL DEFAULT '',
    source_language            TEXT NOT NULL DEFAULT '',
    source_language_confidence DOUBLE PRECISION,
    error_message              TEXT NOT NULL DEFAULT '',
    -- The model of each stage's last call in this finding (relevance, judgement, verify).
    stage_models               JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- Bounded extra details, e.g. the unchecked-activities notice's limits and counts.
    extra                      JSONB NOT NULL DEFAULT '{}'::jsonb,
    create_time                TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One activity occurrence in one review. activity_id comes from the shared ID manager
-- (sequence compliance_activity_id); local_label is the a1, a2 label used in model calls.
CREATE TABLE IF NOT EXISTS kb.doc_review_compliance_activities (
    id                   BIGSERIAL PRIMARY KEY,
    activity_id          BIGINT NOT NULL UNIQUE,
    finding_id           BIGINT NOT NULL REFERENCES kb.doc_review_findings(id) ON DELETE CASCADE,
    local_label          TEXT NOT NULL,
    display_order        INTEGER NOT NULL,
    kind                 TEXT NOT NULL,
    label                TEXT NOT NULL DEFAULT '',
    line_start           INTEGER,
    line_end             INTEGER,
    source_artifact_type TEXT NOT NULL DEFAULT '',
    source_artifact_id   TEXT NOT NULL DEFAULT '',
    source_row_id        BIGINT,
    source_text          TEXT NOT NULL DEFAULT '',
    details              JSONB NOT NULL DEFAULT '{}'::jsonb,
    create_time          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT doc_review_compliance_activities_label_key UNIQUE (finding_id, local_label)
);
CREATE INDEX IF NOT EXISTS idx_doc_review_compliance_activities_finding
    ON kb.doc_review_compliance_activities (finding_id, display_order);
CREATE INDEX IF NOT EXISTS idx_doc_review_compliance_activities_source
    ON kb.doc_review_compliance_activities (source_artifact_type, source_artifact_id);

-- One candidate assessed for one activity: every pair sent to the relevance check.
-- status: rejected | relevance_failed | pending | judgement_failed | judged; only judged
-- rows carry a verdict and binding.
CREATE TABLE IF NOT EXISTS kb.doc_review_compliance_candidates (
    id                  BIGSERIAL PRIMARY KEY,
    activity_id         BIGINT NOT NULL REFERENCES kb.doc_review_compliance_activities(activity_id) ON DELETE CASCADE,
    candidate_type      TEXT NOT NULL,
    candidate_id        TEXT NOT NULL,
    candidate_record_id BIGINT,
    document_title      TEXT NOT NULL DEFAULT '',
    cosine              DOUBLE PRECISION,
    relevance_answer    TEXT NOT NULL DEFAULT '',
    relevance_score     SMALLINT,
    kept                BOOLEAN NOT NULL DEFAULT FALSE,
    status              TEXT NOT NULL,
    verdict             TEXT,
    binding             TEXT,
    summary             TEXT NOT NULL DEFAULT '',
    note                TEXT NOT NULL DEFAULT '',
    error_message       TEXT NOT NULL DEFAULT '',
    evidence            JSONB NOT NULL DEFAULT '{}'::jsonb,
    display_order       INTEGER NOT NULL,
    judgement_order     INTEGER,
    create_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT doc_review_compliance_candidates_pair_key UNIQUE (activity_id, candidate_type, candidate_id),
    CONSTRAINT doc_review_compliance_candidates_status_check
        CHECK (status IN ('rejected', 'relevance_failed', 'pending', 'judgement_failed', 'judged')),
    CONSTRAINT doc_review_compliance_candidates_verdict_check
        CHECK ((status = 'judged') = (verdict IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_doc_review_compliance_candidates_activity
    ON kb.doc_review_compliance_candidates (activity_id, verdict, display_order);

-- One search made for one activity.
CREATE TABLE IF NOT EXISTS kb.doc_review_compliance_searches (
    id                    BIGSERIAL PRIMARY KEY,
    activity_id           BIGINT NOT NULL REFERENCES kb.doc_review_compliance_activities(activity_id) ON DELETE CASCADE,
    ordinal               INTEGER NOT NULL,
    purpose               TEXT NOT NULL,
    query                 TEXT NOT NULL DEFAULT '',
    reference             TEXT NOT NULL DEFAULT '',
    cited_document        TEXT NOT NULL DEFAULT '',
    keywords              JSONB NOT NULL DEFAULT '[]'::jsonb,
    filters               JSONB NOT NULL DEFAULT '{}'::jsonb,
    result_limit          INTEGER NOT NULL DEFAULT 0,
    potentially_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    create_time           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT doc_review_compliance_searches_ordinal_key UNIQUE (activity_id, ordinal)
);

-- One ranked result of one search. The title is a snapshot of what the search returned.
CREATE TABLE IF NOT EXISTS kb.doc_review_compliance_search_results (
    id                  BIGSERIAL PRIMARY KEY,
    search_id           BIGINT NOT NULL REFERENCES kb.doc_review_compliance_searches(id) ON DELETE CASCADE,
    rank                INTEGER NOT NULL,
    candidate_type      TEXT NOT NULL,
    candidate_id        TEXT NOT NULL,
    candidate_record_id BIGINT,
    document_title      TEXT NOT NULL DEFAULT '',
    score               DOUBLE PRECISION,
    diagnostics         JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT doc_review_compliance_search_results_rank_key UNIQUE (search_id, rank)
);

-- Shared configuration of one compliance review run. review_flags lists differences found
-- between findings when legacy findings were migrated.
CREATE TABLE IF NOT EXISTS kb.doc_review_compliance_run_context (
    run_id          BIGINT PRIMARY KEY REFERENCES kb.doc_review_runs(id) ON DELETE CASCADE,
    assessment_date TEXT NOT NULL DEFAULT '',
    schema_version  INTEGER NOT NULL DEFAULT 1,
    prompts         JSONB NOT NULL DEFAULT '{}'::jsonb,
    models          JSONB NOT NULL DEFAULT '{}'::jsonb,
    settings        JSONB,
    review_flags    JSONB NOT NULL DEFAULT '[]'::jsonb,
    create_time     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- An additional-language version of a finding (never a copy of the canonical language).
CREATE TABLE IF NOT EXISTS kb.doc_review_finding_translations (
    id          BIGSERIAL PRIMARY KEY,
    finding_id  BIGINT NOT NULL REFERENCES kb.doc_review_findings(id) ON DELETE CASCADE,
    language    TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    suggestion  TEXT NOT NULL DEFAULT '',
    provenance  TEXT NOT NULL DEFAULT '',
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT doc_review_finding_translations_language_key UNIQUE (finding_id, language)
);

-- +goose Down
DROP TABLE IF EXISTS kb.doc_review_finding_translations;
DROP TABLE IF EXISTS kb.doc_review_compliance_run_context;
DROP TABLE IF EXISTS kb.doc_review_compliance_search_results;
DROP TABLE IF EXISTS kb.doc_review_compliance_searches;
DROP TABLE IF EXISTS kb.doc_review_compliance_candidates;
DROP TABLE IF EXISTS kb.doc_review_compliance_activities;
DROP TABLE IF EXISTS kb.doc_review_finding_details;
DROP INDEX IF EXISTS kb.idx_doc_review_findings_run_aspect_severity;
ALTER TABLE kb.doc_review_findings DROP COLUMN IF EXISTS verdict_counts;
ALTER TABLE kb.doc_review_findings DROP COLUMN IF EXISTS judged_candidate_count;
ALTER TABLE kb.doc_review_findings DROP COLUMN IF EXISTS relevant_candidate_count;
ALTER TABLE kb.doc_review_findings DROP COLUMN IF EXISTS selected_candidate_count;
ALTER TABLE kb.doc_review_findings DROP COLUMN IF EXISTS search_result_count;
ALTER TABLE kb.doc_review_findings DROP COLUMN IF EXISTS activity_count;
ALTER TABLE kb.doc_review_findings DROP COLUMN IF EXISTS line_end;
ALTER TABLE kb.doc_review_findings DROP COLUMN IF EXISTS line_start;
ALTER TABLE kb.doc_review_findings DROP COLUMN IF EXISTS storage_version;
