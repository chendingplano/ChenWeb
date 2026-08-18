// Package main implements the ADR 2026081801 Phase 0 corpus baseline.
//
// Phase 0 gates every later phase: no additive schema work may begin until the
// current pipeline has been characterized across the full corpus and the
// capacity model has been approved. This file holds the queries and the
// projection; main.go holds the CLI and rendering.
package main

import (
	"context"
	"database/sql"
	"fmt"
)

// Baseline is the whole Phase 0 report. Every field maps to a numbered item in
// the ADR's Phase 0 list or to a required pre-cutover report in section 6, so
// the same struct serves as the pre-cutover comparison basis after cutover.
type Baseline struct {
	Corpus          CorpusCounts
	PerRecord       []RecordCounts
	MappingStates   []MappingStateCount
	DeferralReasons []ReasonCount
	AssertionStates []ReasonCount
	StageCoverage   StageCoverage
	Capacity        CapacityModel
	ClassFoundation ClassFoundationCapacity
	TermReaders     TermReaderAudit
}

// CorpusCounts is the corpus-wide summary (Phase 0 item 3).
type CorpusCounts struct {
	InputRecords            int64
	RecordsWithMetrics      int64
	MetricOccurrences       int64
	MetricsWithRangeType    int64
	MetricsWithoutRangeType int64
	MetricsWithRangeError   int64
	DecisionCandidates      int64
	Assertions              int64
	ActiveEvidenceLinks     int64
	MetricSupportLinks      int64
	// DuplicateMetricSupport counts metric occurrences already carrying more
	// than one active supporting link. These must be resolved auditably before
	// uq_assertion_evidence_current_metric_support can be created (Phase 3).
	DuplicateMetricSupport int64
	// UnreachableMetrics is the losslessness gap: metric occurrences with no
	// current supporting assertion link at all.
	UnreachableMetrics int64
}

// RecordCounts is one input record's row in the per-record table.
type RecordCounts struct {
	InputRecordID      int64
	Filename           string
	Metrics            int64
	DecisionCandidates int64
	SupportedMetrics   int64
	UnreachableMetrics int64
	ReviewVisible      int64
}

// MappingStateCount groups metric occurrences by governed range-type mapping
// state, using the same normalization the runtime lookup uses.
type MappingStateCount struct {
	MappingState string
	Metrics      int64
	DistinctRaw  int64
}

// ReasonCount is a generic grouped count (deferral reasons, assertion states).
type ReasonCount struct {
	Key   string
	Count int64
}

// StageCoverage reports what the required-stage outcome set would cost once
// every metric occurrence produces one envelope per required stage (Phase 0
// item 4).
type StageCoverage struct {
	RequiredStages    []string
	MetricOccurrences int64
	// OutcomeEnvelopes is MetricOccurrences * len(RequiredStages).
	OutcomeEnvelopes int64
}

// CapacityModel is the Phase 0 item 4 storage and throughput projection.
type CapacityModel struct {
	EvidenceLinks        int64
	ClassDecisions       int64
	OutcomeEnvelopes     int64
	FindingsLowEstimate  int64
	FindingsHighEstimate int64
	AssertionsUpperBound int64
	EstimatedBytes       int64
}

// ClassFoundationCapacity projects the additive ADR 2026081701 rows from the
// current corpus without assuming that the foundation tables already exist.
// Until canonical convergence is proven, every metric may require its own
// provisional class, claim identity, observed profile, and observation row.
type ClassFoundationCapacity struct {
	LegacyAssertions            int64
	ProvisionalClassCandidates  int64
	ClaimIdentitiesUpperBound   int64
	ObservedProfilesUpperBound  int64
	ObservedProfileObservations int64
	EstimatedBytes              int64
}

// Row-size figures used by the capacity model, INCLUDING index overhead.
//
// bytesPerOutcome is measured, not guessed: the Phase 0 load test
// (TestIntegrationLoadTestCorpusScale) wrote 21,222 envelopes and reported
// pg_total_relation_size = 16.3 MiB, of which 9.0 MiB was indexes -- 806 bytes
// per envelope all-in. The four partial and base indexes on the table are why
// index size is comparable to heap size, which a heap-only estimate would have
// missed by roughly half.
//
// The others remain conservative estimates of the same shape; they are
// annotated so a reader can tell measurement from projection.
const (
	bytesPerOutcome            = 806  // measured, load test 2026-08-18
	bytesPerFinding            = 384  // estimated
	bytesPerEvidence           = 448  // estimated
	bytesPerDecision           = 384  // estimated
	bytesPerAssertion          = 1024 // estimated
	bytesPerClassHeader        = 640  // estimated
	bytesPerClaimIdentity      = 512  // estimated
	bytesPerObservedProfile    = 768  // estimated
	bytesPerProfileObservation = 640  // estimated
)

// requiredMetricStages is the metric adapter's declared required stage set as
// specified by ADR 2026081801 DR13 and DR5. Phase 0 reports against the
// declaration so the capacity model and the later completeness projection use
// one source of truth.
var requiredMetricStages = []string{
	"semantic:stage_normalize",
	"semantic:stage_class_resolution",
	"semantic:stage_associate",
}

// Collect runs every Phase 0 query. It is read-only by construction: the
// baseline must be runnable against production without side effects.
func Collect(ctx context.Context, db *sql.DB) (Baseline, error) {
	var b Baseline
	var err error
	if b.Corpus, err = collectCorpus(ctx, db); err != nil {
		return b, fmt.Errorf("corpus counts: %w", err)
	}
	if b.PerRecord, err = collectPerRecord(ctx, db); err != nil {
		return b, fmt.Errorf("per-record counts: %w", err)
	}
	if b.MappingStates, err = collectMappingStates(ctx, db); err != nil {
		return b, fmt.Errorf("mapping states: %w", err)
	}
	if b.DeferralReasons, err = collectDeferralReasons(ctx, db); err != nil {
		return b, fmt.Errorf("deferral reasons: %w", err)
	}
	if b.AssertionStates, err = collectAssertionStates(ctx, db); err != nil {
		return b, fmt.Errorf("assertion states: %w", err)
	}
	b.StageCoverage = StageCoverage{
		RequiredStages:    requiredMetricStages,
		MetricOccurrences: b.Corpus.MetricOccurrences,
		OutcomeEnvelopes:  b.Corpus.MetricOccurrences * int64(len(requiredMetricStages)),
	}
	b.Capacity = modelCapacity(b.Corpus, b.StageCoverage)
	b.ClassFoundation = modelClassFoundationCapacity(b.Corpus)
	return b, nil
}

func collectCorpus(ctx context.Context, db *sql.DB) (CorpusCounts, error) {
	var c CorpusCounts
	const stmt = `
SELECT
  (SELECT count(*) FROM kb.inputs),
  (SELECT count(DISTINCT input_record_id) FROM kb.metrics),
  (SELECT count(*) FROM kb.metrics),
  (SELECT count(*) FROM kb.metrics WHERE value_range_type IS NOT NULL AND btrim(value_range_type) <> ''),
  (SELECT count(*) FROM kb.metrics WHERE value_range_type IS NULL OR btrim(value_range_type) = ''),
  (SELECT count(*) FROM kb.metrics WHERE value_range_type_error IS NOT NULL AND btrim(value_range_type_error) <> ''),
  (SELECT count(*) FROM kb.semantic_decision_candidates WHERE source_artifact_type = 'metric'),
  (SELECT count(*) FROM kb.semantic_assertions),
  (SELECT count(*) FROM kb.assertion_evidence WHERE deleted = false),
  (SELECT count(*) FROM kb.assertion_evidence
     WHERE deleted = false AND artifact_type = 'metric' AND evidence_role = 'supports')`
	if err := db.QueryRowContext(ctx, stmt).Scan(
		&c.InputRecords, &c.RecordsWithMetrics, &c.MetricOccurrences,
		&c.MetricsWithRangeType, &c.MetricsWithoutRangeType, &c.MetricsWithRangeError,
		&c.DecisionCandidates, &c.Assertions, &c.ActiveEvidenceLinks, &c.MetricSupportLinks,
	); err != nil {
		return c, err
	}

	// Duplicate current support links block the Phase 3 partial unique index,
	// so they are counted here rather than discovered at migration time.
	const dupStmt = `
SELECT count(*) FROM (
  SELECT artifact_id, input_record_id
  FROM kb.assertion_evidence
  WHERE deleted = false AND artifact_type = 'metric' AND evidence_role = 'supports'
  GROUP BY artifact_id, input_record_id
  HAVING count(*) > 1
) d`
	if err := db.QueryRowContext(ctx, dupStmt).Scan(&c.DuplicateMetricSupport); err != nil {
		return c, err
	}

	const unreachableStmt = `
SELECT count(*) FROM kb.metrics m
WHERE NOT EXISTS (
  SELECT 1 FROM kb.assertion_evidence e
  WHERE e.deleted = false AND e.artifact_type = 'metric'
    AND e.evidence_role = 'supports'
    AND e.artifact_id = m.metric_id
    AND e.input_record_id = m.input_record_id)`
	if err := db.QueryRowContext(ctx, unreachableStmt).Scan(&c.UnreachableMetrics); err != nil {
		return c, err
	}
	return c, nil
}

func collectPerRecord(ctx context.Context, db *sql.DB) ([]RecordCounts, error) {
	// ReviewVisible mirrors what a consumer filtering on the current
	// accepted-only policy would see, which is the number the ADR's "Review
	// Document visibility" item asks for.
	const stmt = `
SELECT m.input_record_id,
       coalesce(i.file_name, ''),
       count(*) AS metrics,
       (SELECT count(*) FROM kb.semantic_decision_candidates dc
          WHERE dc.source_artifact_type = 'metric' AND dc.input_record_id = m.input_record_id),
       count(*) FILTER (WHERE EXISTS (
          SELECT 1 FROM kb.assertion_evidence e
          WHERE e.deleted = false AND e.artifact_type = 'metric' AND e.evidence_role = 'supports'
            AND e.artifact_id = m.metric_id AND e.input_record_id = m.input_record_id)),
       count(*) FILTER (WHERE NOT EXISTS (
          SELECT 1 FROM kb.assertion_evidence e
          WHERE e.deleted = false AND e.artifact_type = 'metric' AND e.evidence_role = 'supports'
            AND e.artifact_id = m.metric_id AND e.input_record_id = m.input_record_id)),
       count(*) FILTER (WHERE EXISTS (
          SELECT 1 FROM kb.assertion_evidence e
          JOIN kb.semantic_assertions a ON a.id = e.assertion_id AND a.status = 'accepted'
          WHERE e.deleted = false AND e.artifact_type = 'metric' AND e.evidence_role = 'supports'
            AND e.artifact_id = m.metric_id AND e.input_record_id = m.input_record_id))
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
GROUP BY m.input_record_id, i.file_name
ORDER BY m.input_record_id`
	rows, err := db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecordCounts
	for rows.Next() {
		var r RecordCounts
		if err := rows.Scan(&r.InputRecordID, &r.Filename, &r.Metrics, &r.DecisionCandidates,
			&r.SupportedMetrics, &r.UnreachableMetrics, &r.ReviewVisible); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func collectMappingStates(ctx context.Context, db *sql.DB) ([]MappingStateCount, error) {
	// The join key reproduces normalizeValueRangeTypeRaw (lowercase, trim,
	// '-'/' ' -> '_') so this report and the runtime lookup agree; a mismatch
	// here would understate the proposed/ambiguous population the ADR sizes.
	const stmt = `
WITH norm AS (
  SELECT m.id,
         CASE WHEN m.value_range_type IS NULL OR btrim(m.value_range_type) = '' THEN NULL
              ELSE replace(replace(lower(btrim(m.value_range_type)), '-', '_'), ' ', '_')
         END AS raw_key
  FROM kb.metrics m
)
SELECT CASE
         WHEN n.raw_key IS NULL THEN 'absent'
         WHEN vm.raw_value IS NULL THEN 'unmapped'
         ELSE vm.status
       END AS mapping_state,
       count(*),
       count(DISTINCT n.raw_key)
FROM norm n
LEFT JOIN kb.metric_value_range_type_map vm ON vm.raw_value = n.raw_key
GROUP BY 1
ORDER BY 2 DESC`
	rows, err := db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MappingStateCount
	for rows.Next() {
		var m MappingStateCount
		if err := rows.Scan(&m.MappingState, &m.Metrics, &m.DistinctRaw); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func collectDeferralReasons(ctx context.Context, db *sql.DB) ([]ReasonCount, error) {
	const stmt = `
SELECT coalesce(nullif(btrim(decision_reason), ''), '(none)') AS reason, count(*)
FROM kb.semantic_decision_candidates
WHERE status = 'deferred'
GROUP BY 1 ORDER BY 2 DESC`
	return scanReasons(ctx, db, stmt)
}

func collectAssertionStates(ctx context.Context, db *sql.DB) ([]ReasonCount, error) {
	const stmt = `SELECT status, count(*) FROM kb.semantic_assertions GROUP BY 1 ORDER BY 2 DESC`
	return scanReasons(ctx, db, stmt)
}

func scanReasons(ctx context.Context, db *sql.DB, stmt string) ([]ReasonCount, error) {
	rows, err := db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReasonCount
	for rows.Next() {
		var r ReasonCount
		if err := rows.Scan(&r.Key, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// modelCapacity projects the steady-state row and byte cost of lossless
// processing (Phase 0 item 4). The finding estimate is a band, not a point:
// the low end assumes only metrics whose mapping is unresolved or ambiguous
// produce a finding, the high end assumes every occurrence produces one
// finding per required stage.
func modelCapacity(c CorpusCounts, s StageCoverage) CapacityModel {
	stages := int64(len(s.RequiredStages))
	m := CapacityModel{
		EvidenceLinks:    c.MetricOccurrences,
		ClassDecisions:   c.MetricOccurrences,
		OutcomeEnvelopes: s.OutcomeEnvelopes,
		// Upper bound before canonical convergence: one assertion per
		// occurrence. Convergence can only reduce this.
		AssertionsUpperBound: c.MetricOccurrences,
	}
	m.FindingsHighEstimate = c.MetricOccurrences * stages
	m.EstimatedBytes = m.OutcomeEnvelopes*bytesPerOutcome +
		m.EvidenceLinks*bytesPerEvidence +
		m.ClassDecisions*bytesPerDecision +
		m.AssertionsUpperBound*bytesPerAssertion +
		m.FindingsHighEstimate*bytesPerFinding
	return m
}

func modelClassFoundationCapacity(c CorpusCounts) ClassFoundationCapacity {
	// Before canonical convergence, a metric occurrence is the conservative
	// upper bound for every new class/claim/profile cardinality. Later corpus
	// evidence can only reduce classes and claims through safe convergence.
	m := ClassFoundationCapacity{
		LegacyAssertions:            c.Assertions,
		ProvisionalClassCandidates:  c.MetricOccurrences,
		ClaimIdentitiesUpperBound:   c.MetricOccurrences,
		ObservedProfilesUpperBound:  c.MetricOccurrences,
		ObservedProfileObservations: c.MetricOccurrences,
	}
	m.EstimatedBytes =
		m.ProvisionalClassCandidates*bytesPerClassHeader +
			m.ClaimIdentitiesUpperBound*bytesPerClaimIdentity +
			m.ObservedProfilesUpperBound*bytesPerObservedProfile +
			m.ObservedProfileObservations*bytesPerProfileObservation
	return m
}

// SetFindingsLowEstimate fills the low end of the finding band from the
// measured mapping-state distribution: only unmapped, proposed, and ambiguous
// occurrences are guaranteed to produce at least one finding today.
func (b *Baseline) SetFindingsLowEstimate() {
	var low int64
	for _, m := range b.MappingStates {
		switch m.MappingState {
		case "unmapped", "proposed", "ambiguous":
			low += m.Metrics
		}
	}
	b.Capacity.FindingsLowEstimate = low
}
