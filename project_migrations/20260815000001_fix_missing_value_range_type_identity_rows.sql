-- +goose Up
-- Corrects a gap from 20260814000002: the identity-bucket rows
-- (lower_bound/upper_bound/qualitative/limit_absent -- raw_value equal to its
-- own canonical_bucket, per CanonicalMetricValueRangeType's `default` pass-
-- through) were added to that migration's file after air had already applied
-- an earlier, incomplete copy of it (project_db_migration recorded
-- 20260814000002 as applied at 2026-08-14 19:02:32, before the identity-row
-- edit landed on disk) -- a known hazard of this workspace's live air-managed
-- dev server re-running goose migrations on every Go file save. Confirmed
-- live: `lower_bound`/`upper_bound`/`qualitative` had already been
-- auto-inserted as `status='proposed'` by real extract_metrics traffic on
-- input record 416 before this fix landed (ValueRangeTypeMapper's normal
-- miss-handling, working as designed against the incomplete seed data).
--
-- ON CONFLICT DO UPDATE (not DO NOTHING) deliberately overrides an existing
-- 'proposed' row for exactly these four raw values -- unlike a genuinely
-- ambiguous string, a raw value that already equals its own canonical bucket
-- name has no legitimate alternative classification a human could have
-- intended, so there is nothing to lose by correcting it here instead of
-- waiting for manual triage.
INSERT INTO kb.metric_value_range_type_map (raw_value, canonical_bucket, status) VALUES
    ('lower_bound', 'lower_bound', 'approved'),
    ('upper_bound', 'upper_bound', 'approved'),
    ('qualitative', 'qualitative', 'approved'),
    ('limit_absent', 'limit_absent', 'approved')
ON CONFLICT (raw_value) DO UPDATE
    SET canonical_bucket = EXCLUDED.canonical_bucket,
        status = 'approved',
        modify_time = NOW()
    WHERE kb.metric_value_range_type_map.status != 'approved';

-- +goose Down
-- No down: reverting would re-break already-corrected classifications for
-- new future writers, and would not restore the 'proposed' rows' original
-- occurrence_count/first_seen_record_id (not captured by this migration).
