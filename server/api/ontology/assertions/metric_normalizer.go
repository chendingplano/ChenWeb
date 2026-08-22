package assertions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

// MetricNormalizer is the seam-5 instantiation for the metric artifact
// family (ADR DR12: metrics is the pilot family). It reads kb.metrics rows
// for an input record and proposes one 'assertion' decision candidate per
// metric row -- never writing kb.semantic_assertions directly.
//
// extract_metrics today emits threshold_or_target as free text (the ADR's
// own "single highest-leverage change" note); rather than requiring that
// processor to change its output schema first, this normalizer performs the
// text-to-structured parsing itself (DR8's stated job for
// normalize_assertions). A value that cannot be parsed still produces a
// candidate with value_form='unparsed' and the untouched raw_text -- an
// artifact is never silently dropped for being hard to parse (spec §10.5:
// missing context produces deferred, not an invented target).
type MetricNormalizer struct{}

// FamilyName implements Normalizer.
func (MetricNormalizer) FamilyName() string { return "metric" }

// init registers the metric normalizer with the default registry (DR11 seam
// 5): adding this family required only this file, no edit to the registry
// mechanism or to normalize_assertions.go.
func init() {
	if err := RegisterNormalizer(MetricNormalizer{}); err != nil {
		panic(err)
	}
}

type metricRow struct {
	ID                     int64
	MetricID               string
	MetricDefinitionTermID sql.NullString
	MetricName             sql.NullString
	MetricUnit             sql.NullString
	MetricUnitEn           sql.NullString
	FormulaOrDefinition    sql.NullString
	ThresholdOrTarget      sql.NullString
	MeasurementFrequency   sql.NullString
	Confidence             sql.NullFloat64
	SourceLineSpans        json.RawMessage
	SubjectObjectID        sql.NullString
	ValueRangeType         sql.NullString
	ValueClass             sql.NullString
	MetricValue            sql.NullString
	ValueMin               sql.NullFloat64
	ValueMax               sql.NullFloat64
	Condition              sql.NullString
	EventID                sql.NullString
	ModelName              sql.NullString
	PromptName             sql.NullString
	// MetricDesc, KeywordConceptID, MetricSubject, ReasoningTags, ExtInfo,
	// MetricCategories, ValueDataType are forwarded into the candidate
	// payload for class synthesis (Definition, ConceptID) and the
	// [ontology_term_property_map]/[semantic_assertion_property_map] field
	// maps (bug 2026082101 findings 5, 6, 8) -- previously read only by
	// extract_metrics' own auto-promotion path, now needed here since class/
	// qualifier synthesis moved to associate_semantics.
	MetricDesc       sql.NullString
	KeywordConceptID sql.NullString
	MetricSubject    sql.NullString
	ReasoningTags    json.RawMessage
	ExtInfo          json.RawMessage
	MetricCategories sql.NullString
	ValueDataType    sql.NullString
}

// Normalize implements Normalizer.
func (n MetricNormalizer) Normalize(ctx context.Context, db *sql.DB, inputRecordID int64) (NormalizeReport, error) {
	report := NormalizeReport{ArtifactFamily: n.FamilyName()}
	if db == nil {
		return report, fmt.Errorf("db is nil")
	}

	const stmt = `
SELECT m.id, m.metric_id, m.metric_name, m.metric_unit, m.metric_unit_en, m.formula_or_definition,
       m.metric_definition_term_id, m.threshold_or_target, m.measurement_frequency, m.confidence, m.source_line_spans,
       ao.object_id,
       m.value_range_type, m.value_class, m.metric_value, m.value_min, m.value_max, m.condition,
       m.event_id, m.model_name, m.prompt_name,
       m.metric_desc, m.keyword_concept_id, m.metric_subject, m.reasoning_tags, m.ext_info,
       m.metric_categories, m.value_data_type
FROM kb.metrics m
LEFT JOIN kb.artifact_objects ao
  ON ao.artifact_type = 'metric' AND ao.artifact_id = m.metric_id AND ao.input_record_id = m.input_record_id
WHERE m.input_record_id = $1`
	rows, err := db.QueryContext(ctx, stmt, inputRecordID)
	if err != nil {
		return report, err
	}
	defer rows.Close()

	dcStore := DecisionCandidateStore{DB: db}
	mapper := NewValueRangeTypeMapper(db)
	propResolver := NewGovernedPropertyResolver(db)
	classPropertyEntries := appconfig.GetOntologyTermPropertyMap()["metric"]
	qualifierMappings := ParsePropertyMap(appconfig.GetSemanticAssertionPropertyMap())["metric"]
	for rows.Next() {
		var r metricRow
		if err := rows.Scan(&r.ID, &r.MetricID, &r.MetricName, &r.MetricUnit, &r.MetricUnitEn, &r.FormulaOrDefinition, &r.MetricDefinitionTermID,
			&r.ThresholdOrTarget, &r.MeasurementFrequency, &r.Confidence, &r.SourceLineSpans, &r.SubjectObjectID,
			&r.ValueRangeType, &r.ValueClass, &r.MetricValue, &r.ValueMin, &r.ValueMax, &r.Condition,
			&r.EventID, &r.ModelName, &r.PromptName,
			&r.MetricDesc, &r.KeywordConceptID, &r.MetricSubject, &r.ReasoningTags, &r.ExtInfo,
			&r.MetricCategories, &r.ValueDataType); err != nil {
			return report, err
		}
		report.Examined++

		metricID := strings.TrimSpace(r.MetricID)
		if metricID == "" {
			report.Skipped++
			continue
		}
		canonicalBucket, lookupStatus, err := mapper.Lookup(ctx, r.ValueRangeType.String, inputRecordID)
		if err != nil {
			return report, fmt.Errorf("value_range_type lookup for metric %s: %w", metricID, err)
		}
		payload, err := metricCandidatePayloadForRow(ctx, propResolver, r, canonicalBucket, lookupStatus, classPropertyEntries, qualifierMappings, inputRecordID)
		if err != nil {
			return report, fmt.Errorf("build class properties for metric %s: %w", metricID, err)
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return report, err
		}

		recordID := inputRecordID
		candidate := DecisionCandidate{
			LogicalIdentityKey: fmt.Sprintf("metric:%d:%s", inputRecordID, metricID),
			CandidateKind:      "assertion",
			ProposedPayload:    payloadJSON,
			Method:             "explicit_structured",
			SourceArtifactType: "metric",
			SourceArtifactID:   metricID,
			InputRecordID:      &recordID,
			SourceLineSpans:    r.SourceLineSpans,
		}
		if r.Confidence.Valid {
			v := r.Confidence.Float64
			candidate.Confidence = &v
		}

		result, err := dcStore.Propose(ctx, candidate)
		if err != nil {
			return report, fmt.Errorf("propose candidate for metric %s: %w", metricID, err)
		}
		if result.Reused {
			report.Reused++
		} else {
			report.Proposed++
		}
	}
	return report, rows.Err()
}

func metricCandidatePayloadForRow(ctx context.Context, propResolver GovernedPropertyResolver, r metricRow, canonicalBucket, lookupStatus string, classPropertyEntries []appconfig.OntologyTermPropertyMapEntry, qualifierMappings []PropertyMapping, inputRecordID int64) (map[string]any, error) {
	unit := r.MetricUnit.String
	if unit == "" {
		unit = r.MetricUnitEn.String
	}
	parsed := resolveMetricValue(r, canonicalBucket)
	payload := map[string]any{
		"metric_id":               strings.TrimSpace(r.MetricID),
		"metric_name":             r.MetricName.String,
		"unit":                    unit,
		"raw_text":                r.ThresholdOrTarget.String,
		"value_form":              parsed.ValueForm,
		"comparator":              parsed.Comparator,
		"assertion_kind":          parsed.AssertionKind,
		"value_range_type_lookup": lookupStatus,
		"value_range_type_raw":    normalizeValueRangeTypeRaw(r.ValueRangeType.String),
	}
	if r.MetricDefinitionTermID.Valid && strings.TrimSpace(r.MetricDefinitionTermID.String) != "" {
		payload["metric_definition_term_id"] = strings.TrimSpace(r.MetricDefinitionTermID.String)
	}
	if r.Condition.Valid && r.Condition.String != "" {
		payload["condition"] = r.Condition.String
	}
	if r.SubjectObjectID.Valid && r.SubjectObjectID.String != "" {
		payload["subject_object_id"] = r.SubjectObjectID.String
	}
	if r.EventID.Valid && r.EventID.String != "" {
		payload["extraction_run"] = r.EventID.String
	}
	if r.ModelName.Valid && r.ModelName.String != "" {
		payload["model"] = r.ModelName.String
	}
	if r.PromptName.Valid && r.PromptName.String != "" {
		payload["prompt_version"] = r.PromptName.String
	}
	if parsed.NumericValue != nil {
		payload["numeric_value"] = *parsed.NumericValue
	}
	if parsed.LowerValue != nil {
		payload["lower_value"] = *parsed.LowerValue
	}
	if parsed.UpperValue != nil {
		payload["upper_value"] = *parsed.UpperValue
	}
	if r.KeywordConceptID.Valid && r.KeywordConceptID.String != "" {
		payload["keyword_concept_id"] = r.KeywordConceptID.String
	}
	// Definition sourcing mirrors extract_metrics' pre-finding-5 auto-promotion
	// logic exactly (bug 2026082101 finding 1): metric_desc is the descriptive
	// sentence every other definition writer uses; formula_or_definition is
	// legitimately empty for bare-threshold sources, so it's only a fallback.
	definition := strings.TrimSpace(r.MetricDesc.String)
	if definition == "" {
		definition = strings.TrimSpace(r.FormulaOrDefinition.String)
	}
	if definition != "" {
		payload["definition"] = definition
	}
	if r.ValueDataType.Valid && r.ValueDataType.String != "" {
		payload["value_data_type"] = r.ValueDataType.String
	}
	if r.ValueRangeType.Valid && r.ValueRangeType.String != "" {
		payload["value_range_type_text"] = r.ValueRangeType.String
	}

	fieldMap := metricFieldMap(r, unit)
	classProperties, err := BuildSignatureProperties(ctx, propResolver, classPropertyEntries, fieldMap, inputRecordID)
	if err != nil {
		return nil, err
	}
	if classProperties != nil {
		payload["class_properties"] = classProperties
	}
	if qualifiers := BuildMappedProperties(qualifierMappings, fieldMap); qualifiers != nil {
		payload["qualifiers"] = qualifiers
	}
	return payload, nil
}

// metricFieldMap builds the dotted-path-capable field map both
// [ontology_term_property_map] (class-level) and
// [semantic_assertion_property_map] (instance-level) resolve their
// configured field names against (bug 2026082101 findings 6/8). Keys match
// kb.metrics' canonical column names, the same convention
// extract_metrics' own property-map application used.
func metricFieldMap(r metricRow, unit string) map[string]any {
	m := map[string]any{
		"metric_name":           r.MetricName.String,
		"metric_subject":        r.MetricSubject.String,
		"metric_unit":           unit,
		"formula_or_definition": r.FormulaOrDefinition.String,
		"threshold_or_target":   r.ThresholdOrTarget.String,
		"measurement_frequency": r.MeasurementFrequency.String,
		"metric_value":          r.MetricValue.String,
		"value_data_type":       r.ValueDataType.String,
		"value_range_type":      r.ValueRangeType.String,
		"value_class":           r.ValueClass.String,
		"metric_categories":     r.MetricCategories.String,
		"condition":             r.Condition.String,
	}
	if r.ValueMin.Valid {
		m["value_min"] = r.ValueMin.Float64
	}
	if r.ValueMax.Valid {
		m["value_max"] = r.ValueMax.Float64
	}
	if len(r.ReasoningTags) > 0 {
		var tags any
		if err := json.Unmarshal(r.ReasoningTags, &tags); err == nil {
			m["reasoning_tags"] = tags
		}
	}
	if len(r.ExtInfo) > 0 {
		var extInfo any
		if err := json.Unmarshal(r.ExtInfo, &extInfo); err == nil {
			m["ext_info"] = extInfo
		}
	}
	return m
}

// The pilot corpus (ADR OD1: 呼吸机/医疗器械) is predominantly Chinese-language
// standard text, so the comparator vocabulary covers both languages rather
// than only English -- 不低于/不小于/不得低于/至少 ("not less than"/"at least")
// and 不超过/不大于/不高于/不应超过/不得超过 ("not more than"/"not exceeding")
// are the common forms observed in the gold corpus's threshold_or_target
// values.
var (
	reLowerBound = regexp.MustCompile(`(?i)(at least|no less than|not less than|minimum of|>=|≥|不低于|不小于|不得低于|至少)`)
	reUpperBound = regexp.MustCompile(`(?i)(at most|no more than|not more than|maximum of|<=|≤|不超过|不大于|不高于|不应超过|不得超过)`)
	// reRangeKeyword matches an explicit range keyword/dash between two
	// numbers, with optional surrounding whitespace.
	reRangeKeyword = regexp.MustCompile(`(?i)(-?\d+(?:\.\d+)?)\s*(?:to|~|～|至|―|–|—)\s*(-?\d+(?:\.\d+)?)`)
	// reRangeHyphen matches a plain ASCII hyphen as a range separator only
	// when whitespace surrounds it on both sides -- an unspaced hyphen
	// between two numbers is at least as likely to be part of a standard/
	// document identifier ("GB 9706.1-2020") as a genuine range, and
	// reRangeKeyword already covers "10-20"-style ranges via its keyword
	// forms once a space or Chinese range marker is present.
	reRangeHyphen = regexp.MustCompile(`(-?\d+(?:\.\d+)?)\s+-\s+(-?\d+(?:\.\d+)?)`)
	// reWholeRange accepts only a complete, two-endpoint range. Structured
	// range rows may contain schedules or tiered classifications; extracting a
	// substring from those compound values would fabricate an interval.
	reWholeRange = regexp.MustCompile(`^\s*(-?\d+(?:\.\d+)?)\s*(?:to|~|～|至|―|–|—|\s-\s)\s*(-?\d+(?:\.\d+)?)\s*$`)
	reNumber     = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
	// reRatioDenominatorOne matches the "N:1" contrast/ratio notation.
	reRatioDenominatorOne = regexp.MustCompile(`(\d+(?:\.\d+)?):1\b`)
)

type parsedThreshold struct {
	ValueForm     string
	Comparator    string
	AssertionKind string
	NumericValue  *float64
	LowerValue    *float64
	UpperValue    *float64
}

// structuredValueAssertionKind maps a kb.metrics value_range_type + value_class
// to the governed assertion kind the normalizer emits (design D1).
var structuredValueAssertionKind = map[string]string{
	"lower_bound": "lower_bound_requirement",
	"upper_bound": "upper_bound_requirement",
	"exact":       "exact_value",
	"range":       "interval_requirement",
}

// CanonicalMetricValueRangeType was the extractor/normalizer range contract
// through commit 56b6. As of ADR 2026081401 DR1, production classification no
// longer calls this function -- it goes through the governed, DB-backed
// ValueRangeTypeMapper (kb.metric_value_range_type_map) below, which an
// operator can extend without a code change. This function is retained only
// as (a) the source-of-truth this ADR's DR5 migration seeded the governed
// table from, and (b) a canonicalization oracle for tests that don't need to
// exercise the DB-backed lookup directly (see resolveMetricValueLegacy in
// metric_normalizer_test.go).
func CanonicalMetricValueRangeType(raw string) string {
	vrt := strings.ToLower(strings.TrimSpace(raw))
	vrt = strings.ReplaceAll(vrt, "-", "_")
	vrt = strings.ReplaceAll(vrt, " ", "_")
	switch vrt {
	// Additions below were found by surveying production kb.metrics.value_range_type
	// (2026-08-14): 309 distinct strings across 7,036 rows, confirming extract_metrics
	// treats this as free text rather than a fixed enum. Each group here covers only
	// synonyms whose direction is unambiguous from the string itself; genuinely
	// direction-ambiguous strings (e.g. "threshold", "target", "tolerance",
	// "categorical") are intentionally left unmapped -- they fall through to
	// value_form='unparsed' as before, which is the honest, conservative outcome for a
	// bound that can't be inferred rather than guessed.
	case "min", "minimum", "min_threshold", "lower_limit", "lower_threshold",
		"greater_than_or_equal", "greater_than_or_equal_to", "greater_or_equal", "greater_than",
		"at_least", ">=", ">", "≥", "大于等于", "下限", "minimumormore", "threshold_min":
		return "lower_bound"
	case "max", "maximum", "max_threshold", "upper_limit", "upper_threshold",
		"less_than_or_equal", "less_than_or_equal_to", "less_than",
		"<=", "<", "≤", "不大于", "上限":
		return "upper_bound"
	case "exact", "exact_count", "exact_duration", "exact_ratio", "exact_specification",
		"single", "single_value", "integer", "numeric", "exact_value", "fixed",
		"固定值", "单值", "单一值", "点值", "精确值":
		return "exact"
	case "range", "interval", "between", "value_range", "min_max", "closed_interval",
		"区间", "范围":
		return "range"
	default:
		return vrt
	}
}

// valueRangeTypeMapCacheTTL bounds how stale the in-process
// kb.metric_value_range_type_map cache can be. The table is read on every
// metric row normalized (design D1: a per-call DB round trip is not
// acceptable), so the whole table is cached; a short TTL plus
// invalidate-on-write (see ValueRangeTypeMapper.Lookup) keeps a just-approved
// mapping visible within one TTL window even without an explicit signal.
const valueRangeTypeMapCacheTTL = 30 * time.Second

// valueRangeTypeMapEntry is one kb.metric_value_range_type_map row as held in
// the in-process cache.
type valueRangeTypeMapEntry struct {
	CanonicalBucket string
	Status          string
}

// valueRangeTypeMapCache holds the full kb.metric_value_range_type_map table
// in memory, keyed by raw_value. Not exported: production code shares
// defaultValueRangeTypeMapCache via NewValueRangeTypeMapper; tests construct
// their own so cached state never leaks between test functions.
type valueRangeTypeMapCache struct {
	mu       sync.RWMutex
	entries  map[string]valueRangeTypeMapEntry
	loadedAt time.Time
}

var defaultValueRangeTypeMapCache = &valueRangeTypeMapCache{}

func (c *valueRangeTypeMapCache) fresh(ctx context.Context, db *sql.DB) (map[string]valueRangeTypeMapEntry, error) {
	c.mu.RLock()
	if c.entries != nil && time.Since(c.loadedAt) < valueRangeTypeMapCacheTTL {
		entries := c.entries
		c.mu.RUnlock()
		return entries, nil
	}
	c.mu.RUnlock()
	return c.reload(ctx, db)
}

func (c *valueRangeTypeMapCache) reload(ctx context.Context, db *sql.DB) (map[string]valueRangeTypeMapEntry, error) {
	rows, err := db.QueryContext(ctx, `SELECT raw_value, canonical_bucket, status FROM kb.metric_value_range_type_map`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make(map[string]valueRangeTypeMapEntry)
	for rows.Next() {
		var rawValue, status string
		var bucket sql.NullString
		if err := rows.Scan(&rawValue, &bucket, &status); err != nil {
			return nil, err
		}
		entries[rawValue] = valueRangeTypeMapEntry{CanonicalBucket: bucket.String, Status: status}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.entries = entries
	c.loadedAt = time.Now()
	c.mu.Unlock()
	return entries, nil
}

// invalidate drops the cached table so the next Lookup reloads it -- called
// after any write so a row this process just inserted is visible to this
// process immediately, without waiting for the TTL.
func (c *valueRangeTypeMapCache) invalidate() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}

// InvalidateValueRangeTypeMapCache clears the in-process
// kb.metric_value_range_type_map cache so the next Lookup re-reads the table
// instead of serving a stale entry for up to the 30s TTL. Callers outside this
// package that write to the table directly (e.g. an admin correction handler)
// must call this after committing.
func InvalidateValueRangeTypeMapCache() {
	defaultValueRangeTypeMapCache.invalidate()
}

// NormalizeValueRangeTypeRaw exposes normalizeValueRangeTypeRaw so callers
// outside this package can key kb.metric_value_range_type_map.raw_value
// identically to the runtime lookup (see ValueRangeTypeMapper.Lookup).
func NormalizeValueRangeTypeRaw(raw string) string {
	return normalizeValueRangeTypeRaw(raw)
}

// ValueRangeTypeMapper is the DB-backed replacement for
// CanonicalMetricValueRangeType (ADR 2026081401 DR1). Construct via
// NewValueRangeTypeMapper in production; the zero-value (nil cache) is only
// valid in tests that supply their own cache field.
type ValueRangeTypeMapper struct {
	DB    *sql.DB
	cache *valueRangeTypeMapCache
}

// NewValueRangeTypeMapper returns a mapper backed by the shared,
// process-wide table cache.
func NewValueRangeTypeMapper(db *sql.DB) ValueRangeTypeMapper {
	return ValueRangeTypeMapper{DB: db, cache: defaultValueRangeTypeMapCache}
}

// Lookup classifies a raw kb.metrics.value_range_type string against
// kb.metric_value_range_type_map. canonicalBucket is non-empty and
// authoritative ONLY when status == "approved" -- a 'proposed' row's
// best-effort bucket guess is persisted for human review (see
// guessCanonicalBucketFromCues) but is never returned as usable here, since
// only human approval makes a bucket authoritative for classification (DR1).
//
// status is one of:
//   - "absent"   -- raw was empty/NULL to begin with; not a mapping problem.
//   - "approved" -- a human-confirmed mapping; canonicalBucket is usable.
//   - "ambiguous" -- a human decided no bound direction can be inferred.
//   - "proposed" -- never seen before (auto-inserted by this call) or seen
//     but not yet triaged; recordID is recorded as first/last-seen.
func (m ValueRangeTypeMapper) Lookup(ctx context.Context, raw string, recordID int64) (canonicalBucket string, status string, err error) {
	normalized := normalizeValueRangeTypeRaw(raw)
	if normalized == "" {
		return "", "absent", nil
	}
	if m.DB == nil {
		return "", "", fmt.Errorf("db is nil")
	}
	entries, err := m.cache.fresh(ctx, m.DB)
	if err != nil {
		return "", "", err
	}
	if entry, ok := entries[normalized]; ok {
		if entry.Status == "proposed" || entry.Status == "ambiguous" {
			if err := m.touchOccurrence(ctx, normalized, recordID); err != nil {
				return "", "", err
			}
		}
		if entry.Status != "approved" {
			return "", entry.Status, nil
		}
		return entry.CanonicalBucket, entry.Status, nil
	}
	guess := guessCanonicalBucketFromCues(normalized)
	if err := m.insertProposed(ctx, normalized, guess, recordID); err != nil {
		return "", "", err
	}
	m.cache.invalidate()
	return "", "proposed", nil
}

func (m ValueRangeTypeMapper) touchOccurrence(ctx context.Context, rawValue string, recordID int64) error {
	_, err := m.DB.ExecContext(ctx, `
UPDATE kb.metric_value_range_type_map
   SET occurrence_count = occurrence_count + 1, last_seen_record_id = $2, modify_time = NOW()
 WHERE raw_value = $1`, rawValue, recordID)
	return err
}

func (m ValueRangeTypeMapper) insertProposed(ctx context.Context, rawValue, bucketGuess string, recordID int64) error {
	var bucket sql.NullString
	if bucketGuess != "" {
		bucket = sql.NullString{String: bucketGuess, Valid: true}
	}
	_, err := m.DB.ExecContext(ctx, `
INSERT INTO kb.metric_value_range_type_map (raw_value, canonical_bucket, status, occurrence_count, first_seen_record_id, last_seen_record_id)
VALUES ($1, $2, 'proposed', 1, $3, $3)
ON CONFLICT (raw_value) DO NOTHING`, rawValue, bucket, recordID)
	return err
}

// normalizeValueRangeTypeRaw matches CanonicalMetricValueRangeType's existing
// normalization exactly, so kb.metric_value_range_type_map.raw_value and the
// runtime lookup key always agree.
func normalizeValueRangeTypeRaw(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.ReplaceAll(v, "-", "_")
	v = strings.ReplaceAll(v, " ", "_")
	return v
}

// guessCanonicalBucketFromCues is DR6's deliberately lightweight best-effort
// suggestion for a newly-proposed raw_value, reusing the same direction-cue
// regexes parseThresholdOrTarget already recognizes. It only ever seeds a
// 'proposed' row's suggestion for a human to confirm or correct -- never an
// authoritative classification -- so a wrong guess has no consequence (ADR
// 2026081401 §7).
func guessCanonicalBucketFromCues(normalized string) string {
	text := strings.ReplaceAll(normalized, "_", " ")
	switch {
	case reLowerBound.MatchString(text):
		return "lower_bound"
	case reUpperBound.MatchString(text):
		return "upper_bound"
	default:
		return ""
	}
}

// valueClassAssertionKind refines the assertion kind by value_class, mirroring
// the parser's requirement-vs-observation distinction: an observed test result
// is an observed_value even when the source phrasing is a bound.
var valueClassAssertionKind = map[string]string{
	"observation":       "observed_value",
	"requirement":       "",
	"target":            "target",
	"reference":         "reference",
	"design_capability": "capability",
	"definition":        "reference",
}

// resolveMetricValue is the structured-first path (design D1). canonicalBucket
// is the already-resolved governed bucket for r.ValueRangeType (ADR 2026081401
// DR1 -- callers get it from ValueRangeTypeMapper.Lookup, not by canonicalizing
// here) and is only ever non-empty when the lookup was 'approved'. When
// r.ValueRangeType itself is empty (pre-structured-schema rows, or extraction
// that left it unset) this falls back to parseThresholdOrTarget on
// threshold_or_target -- distinct from a populated-but-ungoverned
// value_range_type (canonicalBucket == "" but r.ValueRangeType != ""), which
// must NOT fall back to free-text parsing: the row explicitly claims a
// structured value, so an unrecognized/unapproved one honestly falls through
// to unparsed below instead. It never fabricates: a structured row with no
// parseable number produces an honest unparsed, never a value invented from
// unrelated text.
func resolveMetricValue(r metricRow, canonicalBucket string) parsedThreshold {
	if strings.TrimSpace(r.ValueRangeType.String) == "" {
		// Legacy row (pre-structured-schema, or extraction left the enum
		// unset): fall back to the free-text parser unchanged.
		return parseThresholdOrTarget(r.ThresholdOrTarget.String)
	}
	vrt := canonicalBucket
	// kind starts from the enum's base assertion kind (empty for
	// qualitative/limit_absent, which have no numeric-kind term) and is
	// refined by value_class.
	kind := structuredValueAssertionKind[vrt]
	if vc, ok := valueClassAssertionKind[strings.TrimSpace(r.ValueClass.String)]; ok && vc != "" {
		kind = vc
	}

	switch vrt {
	case "range":
		if r.ValueMin.Valid && r.ValueMax.Valid {
			lo, hi := r.ValueMin.Float64, r.ValueMax.Float64
			return parsedThreshold{
				ValueForm: "range", Comparator: "between", AssertionKind: kind,
				LowerValue: &lo, UpperValue: &hi,
			}
		}
		// The current extractor records production ranges in metric_value but
		// has not populated value_min/value_max. Parsing only the known range
		// shape retains a structured interval without inventing endpoints.
		rawRange := strings.NewReplacer("℃", "", "°C", "", "°", "").Replace(r.MetricValue.String)
		if match := reWholeRange.FindStringSubmatch(rawRange); match != nil {
			lo, errLo := strconv.ParseFloat(match[1], 64)
			hi, errHi := strconv.ParseFloat(match[2], 64)
			if errLo == nil && errHi == nil {
				return parsedThreshold{ValueForm: "range", Comparator: "between", AssertionKind: kind, LowerValue: &lo, UpperValue: &hi}
			}
		}
	case "lower_bound", "upper_bound":
		if v, ok := firstParseableNumber(r.MetricValue.String); ok {
			comparator := ">="
			if vrt == "upper_bound" {
				comparator = "<="
			}
			return parsedThreshold{
				ValueForm: "single", Comparator: comparator, AssertionKind: kind,
				NumericValue: &v,
			}
		}
	case "exact":
		// Ratios such as 1:10 are not scalar exact values. Their representation
		// is not yet modeled as a governed literal, so preserve them as
		// unparsed rather than silently accepting the numerator as a value.
		if strings.Contains(strings.TrimSpace(r.MetricValue.String), ":") {
			break
		}
		if v, ok := firstParseableNumber(r.MetricValue.String); ok {
			return parsedThreshold{
				ValueForm: "single", Comparator: "=", AssertionKind: kind,
				NumericValue: &v,
			}
		}
	case "qualitative", "limit_absent":
		// No numeric fields by design; the kind follows value_class (a
		// qualitative observation is an observed_value with no number).
		return parsedThreshold{ValueForm: vrt, AssertionKind: kind}
	}
	// The row claims a structured enum but either it is unrecognized or the
	// value cannot be grounded in the structured columns. Honest unparsed --
	// never a value invented from unrelated text, and never a free-text parse
	// of a row that declared structured values.
	return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
}

// firstParseableNumber returns the first number in s ("" -> no number), which
// is what a single numeric metric_value carries. For structured rows this is
// the trusted source; unlike parseThresholdOrTarget there is no comparator to
// anchor against, so a non-numeric metric_value is honest unparsed.
func firstParseableNumber(s string) (float64, bool) {
	m := reNumber.FindString(strings.TrimSpace(s))
	if m == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseThresholdOrTarget best-effort parses extract_metrics' free-text
// threshold_or_target into a value form, comparator, and assertion kind. It
// never fabricates a value: a string with no recognizable number produces
// value_form='unparsed' with every numeric field left nil, and the original
// text is preserved unchanged by the caller regardless of parse outcome.
func parseThresholdOrTarget(raw string) parsedThreshold {
	text := strings.TrimSpace(raw)
	if text == "" {
		return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
	}
	// Contrast-ratio notation ("500:1") is common in the pilot corpus's
	// display metrics; without this, the ":1" denominator is mistaken for a
	// second range endpoint (matching "1" instead of "500"). Collapsing it
	// to the numerator is the parseable reading -- raw_text still preserves
	// the untouched "500:1" for anyone who needs the literal ratio.
	text = reRatioDenominatorOne.ReplaceAllString(text, "$1")

	m := reRangeKeyword.FindStringSubmatch(text)
	if m == nil {
		m = reRangeHyphen.FindStringSubmatch(text)
	}
	if m != nil {
		lo, errLo := strconv.ParseFloat(m[1], 64)
		hi, errHi := strconv.ParseFloat(m[2], 64)
		if errLo == nil && errHi == nil {
			return parsedThreshold{
				ValueForm: "range", Comparator: "between", AssertionKind: "interval_requirement",
				LowerValue: &lo, UpperValue: &hi,
			}
		}
	}

	// Anchor the numeric match to the position right after the matched
	// comparator keyword rather than the first number anywhere in the
	// string -- otherwise a leading distance, date, or standard-number
	// token ("在 1 m 距离处不低于 250", "GB 9706.1-2020 规定不低于 250") is
	// mistaken for the threshold value itself. A comparator keyword with no
	// number after it is unparsed, never fabricated from an earlier number.
	if loc := reLowerBound.FindStringIndex(text); loc != nil {
		if value, ok := firstNumberAfter(text, loc[1]); ok {
			return parsedThreshold{ValueForm: "single", Comparator: ">=", AssertionKind: "lower_bound_requirement", NumericValue: &value}
		}
		return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
	}
	if loc := reUpperBound.FindStringIndex(text); loc != nil {
		if value, ok := firstNumberAfter(text, loc[1]); ok {
			return parsedThreshold{ValueForm: "single", Comparator: "<=", AssertionKind: "upper_bound_requirement", NumericValue: &value}
		}
		return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
	}

	// No comparator keyword: a bare number is recorded as an observed value,
	// not a requirement -- spec §16.3 item 8's distinction between
	// requirement and observation assertion kinds.
	numMatch := reNumber.FindString(text)
	if numMatch == "" {
		return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
	}
	value, err := strconv.ParseFloat(numMatch, 64)
	if err != nil {
		return parsedThreshold{ValueForm: "unparsed", AssertionKind: "unparsed"}
	}
	return parsedThreshold{ValueForm: "single", Comparator: "=", AssertionKind: "observed_value", NumericValue: &value}
}

// firstNumberAfter returns the first parseable number in text at or after
// byte offset from, and whether one was found.
func firstNumberAfter(text string, from int) (float64, bool) {
	if from > len(text) {
		return 0, false
	}
	numMatch := reNumber.FindString(text[from:])
	if numMatch == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(numMatch, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
