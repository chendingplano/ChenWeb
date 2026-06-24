-- +goose Up
-- Records every reviewer correction action taken on a Document Review Report:
--   auto_fix          – LLM Auto Fix applied to a finding's offending line(s)
--   edit_tool         – manual Edit Tool save of a finding's line(s)
--   finding_delete    – finding removed via the Delete button (review_status='deleted')
--   structure_modify  – Document Structure line edited (line type / content / coords)
--   structure_split   – Document Structure line split into multiple lines
--   structure_delete  – Document Structure line deleted
CREATE TABLE IF NOT EXISTS kb.doc_review_activities (
    id              BIGSERIAL    PRIMARY KEY,
    activity_type   TEXT         NOT NULL,
    input_record_id BIGINT       NOT NULL,
    review_run_id   TEXT,
    report_id       BIGINT,
    finding_id      BIGINT,
    page_number     INT,
    line_number     INT,
    location        TEXT,
    old_content     TEXT,
    new_content     TEXT,
    detail          JSONB,
    actor           TEXT,
    create_time     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_doc_review_activities_record
    ON kb.doc_review_activities (input_record_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_activities_run
    ON kb.doc_review_activities (review_run_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_activities_report
    ON kb.doc_review_activities (report_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_activities_type
    ON kb.doc_review_activities (activity_type);

-- +goose Down
DROP INDEX IF EXISTS idx_doc_review_activities_type;
DROP INDEX IF EXISTS idx_doc_review_activities_report;
DROP INDEX IF EXISTS idx_doc_review_activities_run;
DROP INDEX IF EXISTS idx_doc_review_activities_record;
DROP TABLE IF EXISTS kb.doc_review_activities;
