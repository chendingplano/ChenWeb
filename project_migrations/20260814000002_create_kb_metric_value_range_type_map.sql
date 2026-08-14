-- +goose Up
-- Governed, DB-backed replacement for CanonicalMetricValueRangeType's hardcoded Go
-- switch (server/api/ontology/assertions/metric_normalizer.go). kb.metrics.value_range_type
-- is free text from an LLM extractor (309 distinct strings across 7,036 rows observed in a
-- 2026-08-14 production survey) -- ADR 2026081401 DR1. raw_value is normalized identically to
-- CanonicalMetricValueRangeType's existing step (lowercased, trimmed, '-'/' ' -> '_') so the
-- seed rows below and the runtime lookup key are guaranteed to agree.
--
-- status: 'proposed' (auto-inserted on first sighting, not yet authoritative) / 'approved'
-- (a human confirmed or corrected canonical_bucket; used to classify a metric) / 'ambiguous'
-- (a human decided no bound direction can be inferred from the string -- stays unparsed by
-- design, same as today, but as a recorded decision instead of an implicit code gap).
CREATE TABLE IF NOT EXISTS kb.metric_value_range_type_map (
    raw_value             TEXT PRIMARY KEY,
    canonical_bucket      TEXT,
    status                TEXT NOT NULL DEFAULT 'proposed'
                              CHECK (status IN ('proposed', 'approved', 'ambiguous')),
    occurrence_count      BIGINT NOT NULL DEFAULT 0,
    first_seen_record_id  BIGINT,
    last_seen_record_id   BIGINT,
    note                  TEXT,
    create_time           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by             TEXT,
    modify_time           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by             TEXT
);

CREATE INDEX IF NOT EXISTS idx_kb_metric_value_range_type_map_status
    ON kb.metric_value_range_type_map (status);

-- DR5 seed set 1: every synonym recognized by CanonicalMetricValueRangeType's Go switch
-- (metric_normalizer.go:222-253) as of commit 56b6, status='approved' with the matching
-- canonical_bucket -- preserves current classification behavior exactly on cutover. Also
-- includes the six bucket names themselves (lower_bound/upper_bound/exact/range/qualitative/
-- limit_absent): CanonicalMetricValueRangeType's `default` case returns an already-canonical
-- string unchanged, so a kb.metrics row whose extractor emitted the bucket name directly
-- (not a synonym) classifies via that identity pass-through today -- the governed table must
-- reproduce it explicitly, since there is no switch `default` equivalent here.
INSERT INTO kb.metric_value_range_type_map (raw_value, canonical_bucket, status) VALUES
    ('lower_bound', 'lower_bound', 'approved'),
    ('upper_bound', 'upper_bound', 'approved'),
    ('qualitative', 'qualitative', 'approved'),
    ('limit_absent', 'limit_absent', 'approved'),
    -- lower_bound
    ('min', 'lower_bound', 'approved'),
    ('minimum', 'lower_bound', 'approved'),
    ('min_threshold', 'lower_bound', 'approved'),
    ('lower_limit', 'lower_bound', 'approved'),
    ('lower_threshold', 'lower_bound', 'approved'),
    ('greater_than_or_equal', 'lower_bound', 'approved'),
    ('greater_than_or_equal_to', 'lower_bound', 'approved'),
    ('greater_or_equal', 'lower_bound', 'approved'),
    ('greater_than', 'lower_bound', 'approved'),
    ('at_least', 'lower_bound', 'approved'),
    ('>=', 'lower_bound', 'approved'),
    ('>', 'lower_bound', 'approved'),
    ('≥', 'lower_bound', 'approved'),
    ('大于等于', 'lower_bound', 'approved'),
    ('下限', 'lower_bound', 'approved'),
    ('minimumormore', 'lower_bound', 'approved'),
    ('threshold_min', 'lower_bound', 'approved'),
    -- upper_bound
    ('max', 'upper_bound', 'approved'),
    ('maximum', 'upper_bound', 'approved'),
    ('max_threshold', 'upper_bound', 'approved'),
    ('upper_limit', 'upper_bound', 'approved'),
    ('upper_threshold', 'upper_bound', 'approved'),
    ('less_than_or_equal', 'upper_bound', 'approved'),
    ('less_than_or_equal_to', 'upper_bound', 'approved'),
    ('less_than', 'upper_bound', 'approved'),
    ('<=', 'upper_bound', 'approved'),
    ('<', 'upper_bound', 'approved'),
    ('≤', 'upper_bound', 'approved'),
    ('不大于', 'upper_bound', 'approved'),
    ('上限', 'upper_bound', 'approved'),
    -- exact
    ('exact', 'exact', 'approved'),
    ('exact_count', 'exact', 'approved'),
    ('exact_duration', 'exact', 'approved'),
    ('exact_ratio', 'exact', 'approved'),
    ('exact_specification', 'exact', 'approved'),
    ('single', 'exact', 'approved'),
    ('single_value', 'exact', 'approved'),
    ('integer', 'exact', 'approved'),
    ('numeric', 'exact', 'approved'),
    ('exact_value', 'exact', 'approved'),
    ('fixed', 'exact', 'approved'),
    ('固定值', 'exact', 'approved'),
    ('单值', 'exact', 'approved'),
    ('单一值', 'exact', 'approved'),
    ('点值', 'exact', 'approved'),
    ('精确值', 'exact', 'approved'),
    -- range
    ('range', 'range', 'approved'),
    ('interval', 'range', 'approved'),
    ('between', 'range', 'approved'),
    ('value_range', 'range', 'approved'),
    ('min_max', 'range', 'approved'),
    ('closed_interval', 'range', 'approved'),
    ('区间', 'range', 'approved'),
    ('范围', 'range', 'approved')
ON CONFLICT (raw_value) DO NOTHING;

-- DR5 seed set 2: direction-ambiguous strings, status='ambiguous' -- preserves today's
-- silent-defer outcome instead of manufacturing new failures. Sourced from
-- TestResolveMetricValueLeavesAmbiguousVocabularyUnparsed (metric_normalizer_test.go).
INSERT INTO kb.metric_value_range_type_map (raw_value, canonical_bucket, status) VALUES
    ('threshold', NULL, 'ambiguous'),
    ('target', NULL, 'ambiguous'),
    ('tolerance', NULL, 'ambiguous'),
    ('categorical', NULL, 'ambiguous'),
    ('ordinal', NULL, 'ambiguous'),
    ('continuous', NULL, 'ambiguous'),
    ('binary', NULL, 'ambiguous'),
    ('ratio', NULL, 'ambiguous')
ON CONFLICT (raw_value) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS kb.metric_value_range_type_map;
