package semantic

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/classfoundation"
)

// MetricAdapterName and MetricAdapterVersion identify the metric family
// adapter in the runtime compliance registry.
const (
	MetricAdapterName    = "metric_lossless_adapter"
	MetricAdapterVersion = "0.1.0-shadow"
	// MetricArtifactType matches kb.assertion_evidence.artifact_type and
	// kb.artifact_objects.artifact_type for metrics.
	MetricArtifactType = "metric"
	// MetricOccurrenceScope is the family-declared source scope that
	// participates in OccurrenceKey. It is versioned because changing what
	// counts as one atomic metric occurrence changes occurrence identity.
	MetricOccurrenceScope = "metric_occurrence:v1"
)

// MetricAdapter is the metric family's declaration under DR13. It is the first
// vertical slice: ADR 2026081801 section 2.5 makes metrics first because the
// loss is confirmed there and ADR 2026081701 requires this decision before its
// metric-instance pipeline.
//
// In this phase the adapter DECLARES; it does not write. The lossless metric
// writer is Phase 3 and is blocked on ADR 2026081701's claim registry,
// canonical keys, and class resolution.
type MetricAdapter struct{}

func (MetricAdapter) ArtifactType() string    { return MetricArtifactType }
func (MetricAdapter) AdapterName() string     { return MetricAdapterName }
func (MetricAdapter) AdapterVersion() string  { return MetricAdapterVersion }
func (MetricAdapter) OccurrenceScope() string { return MetricOccurrenceScope }

// SupportsInstances is true, which under DR1 means metrics MUST produce a
// raw-preserved instance and may never fall back to an unresolved occurrence.
func (MetricAdapter) SupportsInstances() bool { return true }

// RawIdentityFields are the kb.metrics columns that constitute the raw
// occurrence identity and payload (DR2). value_range_type and
// threshold_or_target are listed explicitly because they are the two fields
// whose normalization failures cause today's loss.
func (MetricAdapter) RawIdentityFields() []string {
	return []string{
		"metric_id", "input_record_id", "metric_name", "metric_unit", "metric_unit_en",
		"value_range_type", "value_class", "metric_value", "value_min", "value_max",
		"threshold_or_target", "condition", "source_line_spans",
	}
}

// RequiredStages is the metric adapter's declared required stage set (DR5,
// DR13). Every current metric occurrence must have exactly one active outcome
// envelope for each of these, and the completeness projection reports against
// this list.
func (MetricAdapter) RequiredStages() []StageContract {
	return []StageContract{
		{
			StageTermID: StageNormalize,
			// Decision scopes are what make several simultaneous findings
			// possible without colliding: a mapping finding and a value
			// finding on the same stage have different finding keys because
			// they are keyed within different scopes (DR4).
			DecisionScopes:      []string{"range_type_mapping", "value_literal", "unit_resolution"},
			Dimensions:          []string{DimensionMapping, DimensionValue},
			AllowedDispositions: []string{DispositionNormalized, DispositionRawPreserved, DispositionNoResult},
		},
		{
			StageTermID:         StageClassResolution,
			DecisionScopes:      []string{"class_identity"},
			Dimensions:          []string{DimensionClass, DimensionIdentity},
			AllowedDispositions: []string{DispositionNormalized, DispositionRawPreserved, DispositionNoResult},
		},
		{
			StageTermID:         StageAssociate,
			DecisionScopes:      []string{"assertion_conformance", "source_conflict"},
			Dimensions:          []string{DimensionConformance, DimensionConflict},
			AllowedDispositions: []string{DispositionNormalized, DispositionRawPreserved, DispositionNotApplicable},
		},
	}
}

// ValueStates is every value state a metric instance may carry (DR9).
func (MetricAdapter) ValueStates() []string {
	return []string{ValuePresent, ValueMissing, ValueUnparsed, ValueDatatypeMismatch, ValueUnknown, ValueNotApplicable}
}

// ConformanceStates is every conformance state a metric instance may carry.
func (MetricAdapter) ConformanceStates() []string {
	return []string{Conforms, ConformanceContractViolation, ConformanceNotEvaluated}
}

// DependencyAxes names the axes a metric outcome's dependency fingerprint
// covers, so a retry trigger can be validated against the family declaration.
func (MetricAdapter) DependencyAxes() []string {
	return []string{
		"mapping_revision", "parser_version", "class_identity_decision",
		"class_contract_revision", "validator_version", "unit_vocabulary_release",
		"model_version", "prompt_version",
	}
}

// MetricArtifactSourceSQL is the family's current-occurrence query for the
// completeness projection. One row per atomic current kb.metrics occurrence.
const MetricArtifactSourceSQL = `
SELECT m.metric_id AS artifact_id, m.input_record_id AS input_record_id
FROM kb.metrics m
WHERE m.metric_id IS NOT NULL AND btrim(m.metric_id) <> ''`

// ShadowComparison is what a shadow-mode run reports: what the lossless writer
// WOULD have produced versus what the existing path actually produced.
//
// Phase 1 item 7 requires shadow mode to perform no consumer-visible semantic
// writes, so this type is the entire output. The number that matters is
// WouldBecomeRawPreserved: the metrics that are unreachable today and would
// become raw-preserved instances after cutover.
type ShadowComparison struct {
	InputRecordID int64

	MetricsExamined int
	// ExistingSupportedMetrics have a current supporting evidence link today.
	ExistingSupportedMetrics int
	// ExistingUnreachableMetrics have none -- the loss this ADR removes.
	ExistingUnreachableMetrics int

	// WouldBeNormalized and WouldBecomeRawPreserved are the intended
	// dispositions under DR12's value-range table.
	WouldBeNormalized       int
	WouldBecomeRawPreserved int

	// IntendedFindingsByTerm counts the findings the lossless writer would
	// record, by governed term.
	IntendedFindingsByTerm map[string]int
	// IntendedOutcomeEnvelopes is metrics examined times required stages.
	IntendedOutcomeEnvelopes int
	// DuplicateCurrentSupport counts occurrences that already violate the
	// Phase 3 uq_assertion_evidence_current_metric_support invariant and must
	// be resolved by auditable backfill before that index can be created.
	DuplicateCurrentSupport int
	// FoundationClassResolved counts source-backed class candidates after
	// read-only redirect resolution. Unavailable candidates remain explicit;
	// shadow mode never creates provisional classes.
	FoundationClassResolved     int
	FoundationClassUnavailable  int
	FoundationClaimCandidates   int
	FoundationProfileCandidates int
	// FoundationReport retains bounded, per-occurrence diagnostics behind the
	// aggregate counters. It is shadow-only and never persists evidence.
	FoundationReport *FoundationShadowReport
}

type termRedirectLookup struct{ db *sql.DB }

func (l termRedirectLookup) ActiveRedirect(ctx context.Context, source string) (string, bool, error) {
	var target string
	err := l.db.QueryRowContext(ctx, `
SELECT target_term_id
FROM kb.ontology_term_redirects
WHERE source_term_id = $1 AND active`, source).Scan(&target)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return target, true, nil
}

// RunShadow computes the shadow comparison for one input record.
//
// It is strictly read-only. Nothing in this function writes a row, which is
// what makes Phase 1's "shadow mode performs no consumer-visible semantic
// writes" verifiable rather than merely intended.
func (a MetricAdapter) RunShadow(ctx context.Context, db *sql.DB, inputRecordID int64) (ShadowComparison, error) {
	if db == nil {
		return ShadowComparison{}, fmt.Errorf("db is nil")
	}
	cmp := ShadowComparison{
		InputRecordID:          inputRecordID,
		IntendedFindingsByTerm: map[string]int{},
		FoundationReport:       NewFoundationShadowReport(0),
	}

	// The mapping-state join reproduces normalizeValueRangeTypeRaw's key
	// (lowercase, trim, '-'/' ' -> '_') so the shadow projection and the
	// runtime lookup agree about which metrics are unmapped.
	const stmt = `
WITH norm AS (
  SELECT m.metric_id,
         m.input_record_id,
	         coalesce(m.metric_definition_term_id, '') AS metric_definition_term_id,
	         coalesce(m.metric_name, '') AS metric_name,
	         coalesce(m.metric_value, '') AS metric_value,
	         coalesce(m.threshold_or_target, '') AS threshold_or_target,
	         coalesce(m.metric_unit, '') AS metric_unit,
	         CASE WHEN m.value_range_type IS NULL OR btrim(m.value_range_type) = '' THEN NULL
              ELSE replace(replace(lower(btrim(m.value_range_type)), '-', '_'), ' ', '_')
         END AS raw_key,
         nullif(btrim(coalesce(m.threshold_or_target, '')), '') AS literal
  FROM kb.metrics m
  WHERE m.input_record_id = $1 AND m.metric_id IS NOT NULL AND btrim(m.metric_id) <> ''
)
SELECT n.metric_id,
	   n.metric_definition_term_id,
	   n.metric_name,
	   n.metric_value,
	   n.threshold_or_target,
	   n.metric_unit,
	   coalesce(n.raw_key, ''),
       CASE WHEN n.raw_key IS NULL THEN 'absent'
            WHEN vm.raw_value IS NULL THEN 'unmapped'
            ELSE vm.status END AS mapping_state,
       (n.literal IS NOT NULL) AS has_literal,
       (SELECT count(*) FROM kb.assertion_evidence e
         WHERE e.deleted = false AND e.artifact_type = 'metric'
           AND e.evidence_role = 'supports'
           AND e.artifact_id = n.metric_id
           AND e.input_record_id = n.input_record_id) AS support_links
FROM norm n
LEFT JOIN kb.metric_value_range_type_map vm ON vm.raw_value = n.raw_key`

	rows, err := db.QueryContext(ctx, stmt, inputRecordID)
	if err != nil {
		return cmp, err
	}
	defer rows.Close()
	for rows.Next() {
		var metricID, metricDefinitionTerm, metricName, metricValue, thresholdOrTarget, metricUnit, rawRangeType, mappingState string
		var hasLiteral bool
		var supportLinks int
		if err := rows.Scan(&metricID, &metricDefinitionTerm, &metricName, &metricValue, &thresholdOrTarget, &metricUnit, &rawRangeType, &mappingState, &hasLiteral, &supportLinks); err != nil {
			return cmp, err
		}
		foundation, err := a.FoundationShadow(ctx, MetricFoundationShadowInput{
			MetricID:             metricID,
			MetricDefinitionTerm: metricDefinitionTerm,
			MetricName:           metricName,
			MetricValue:          metricValue,
			ThresholdOrTarget:    thresholdOrTarget,
			MetricUnit:           metricUnit,
			RangeType:            rawRangeType,
			Redirects:            termRedirectLookup{db: db},
		})
		if err != nil {
			return cmp, fmt.Errorf("compute metric foundation shadow for %s: %w", metricID, err)
		}
		if foundation.ClassState == classfoundation.ResolutionResolvedExisting {
			cmp.FoundationClassResolved++
		}
		if foundation.ClassState == MetricFoundationClassUnavailable {
			cmp.FoundationClassUnavailable++
		}
		if strings.TrimSpace(foundation.CanonicalClaimKey) != "" {
			cmp.FoundationClaimCandidates++
		}
		if foundation.ProfileObservation != nil {
			cmp.FoundationProfileCandidates++
		}
		cmp.FoundationReport.Observe(metricID, inputRecordID, foundation, supportLinks)
		cmp.MetricsExamined++
		switch {
		case supportLinks == 0:
			cmp.ExistingUnreachableMetrics++
		default:
			cmp.ExistingSupportedMetrics++
			if supportLinks > 1 {
				cmp.DuplicateCurrentSupport++
			}
		}

		// DR12's disposition table, evaluated without writing anything.
		// Mapping resolution and value parsing are independent axes, so a
		// metric can contribute findings on both.
		disposition := DispositionNormalized
		switch mappingState {
		case "proposed", "unmapped":
			disposition = DispositionRawPreserved
			cmp.IntendedFindingsByTerm[FindingMappingUnresolved]++
		case "ambiguous":
			disposition = DispositionRawPreserved
			cmp.IntendedFindingsByTerm[FindingMappingAmbiguous]++
		case "absent":
			// Absent range type is not itself a finding: DR12 records one only
			// when the class expects the field, which class resolution decides.
		}
		if !hasLiteral {
			cmp.IntendedFindingsByTerm[FindingValueMissing]++
			disposition = DispositionRawPreserved
		}
		if disposition == DispositionNormalized {
			cmp.WouldBeNormalized++
		} else {
			cmp.WouldBecomeRawPreserved++
		}
	}
	if err := rows.Err(); err != nil {
		return cmp, err
	}
	cmp.FoundationReport.Finalize()
	cmp.IntendedOutcomeEnvelopes = cmp.MetricsExamined * len(a.RequiredStages())
	return cmp, nil
}

// The metric adapter registers itself so LookupAdapter, the completeness
// projection, and AuthorizeWriterActivation can find it. Registration is not
// activation: the writer gate defaults off and DR13's conformance check runs
// independently, so a registered adapter still writes nothing until Phase 3.
func init() { RegisterAdapter(MetricAdapter{}) }
