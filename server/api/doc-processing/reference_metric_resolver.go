package docprocessing

// The metric family's reference resolver (design D9/D10, devdoc §6). It takes criterion and
// criteria_set references, matches them against the target record's metrics with the
// deterministic policy metric_reference_match/v1, and writes resolved rows <rec>_rfm_<n> into the
// referencing record's kb.metrics. extract_references calls it in step 5, extract_metrics in step 6.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	metricRefPolicyVersion = "metric_reference_match/v1"
	metricRefResolveMin    = 0.8  // top score needed to resolve
	metricRefLeadMin       = 0.1  // lead over the second candidate needed to resolve
	metricRefCandidateMin  = 0.3  // below this a target metric does not resemble the quantity
	metricRefSubjectBonus  = 0.15 // subject matches (Dice >= 0.5)
	metricRefLocatorBonus  = 0.1  // candidate sits in the cited table/section
	metricRefMaxCandidates = 5
)

// Values of kb.ref_mentions.metric_state.
const (
	metricRefResolved    = "resolved"
	metricRefNeedsReview = "needs_review"
	metricRefNoMatch     = "no_match"
	metricRefPending     = "pending_target"
)

type metricReferenceResolver struct{}

func init() { RegisterReferenceResolver(metricReferenceResolver{}) }

func (metricReferenceResolver) Family() string { return "metric" }

func (metricReferenceResolver) Takes(m RefMention) bool {
	return m.TargetContent == "criterion" || m.TargetContent == "criteria_set"
}

func (metricReferenceResolver) HasArtifacts(ctx context.Context, db *sql.DB, recordID int64) (bool, error) {
	var ok bool
	err := db.QueryRowContext(ctx, `
SELECT EXISTS (SELECT 1 FROM kb.metrics WHERE input_record_id = $1 AND ref_mention_id IS NULL)
    OR EXISTS (SELECT 1 FROM kb.input_proc_status WHERE record_id = $1 AND processor = 'extract_metrics' AND proc_status = 'success')`,
		recordID).Scan(&ok)
	return ok, err
}

func (metricReferenceResolver) Pending(ctx context.Context, db *sql.DB, m RefMention) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := clearMetricResolution(ctx, tx, m.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE kb.ref_mentions SET metric_state = $2 WHERE id = $1`, m.ID, metricRefPending); err != nil {
		return err
	}
	return tx.Commit()
}

func clearMetricResolution(ctx context.Context, tx *sql.Tx, mentionID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.metrics WHERE ref_mention_id = $1`, mentionID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM kb.reference_artifact_links WHERE ref_mention_id = $1 AND artifact_family = 'metric'`, mentionID)
	return err
}

// metricRefTarget is one target metric (own row or dropped definition) as seen by the matcher.
type metricRefTarget struct {
	Table, ArtifactID                               string
	RowID                                           int64
	Name, NameEn, Subject, SubjectEn, Section, Cond string
}

func (r metricReferenceResolver) Resolve(ctx context.Context, db *sql.DB, m RefMention, target int64) (ResolveOutcome, error) {
	ts, err := loadMetricRefTargets(ctx, db, target)
	if err != nil {
		return ResolveOutcome{}, err
	}
	return scoreMetricReference(m, target, ts), nil
}

func loadMetricRefTargets(ctx context.Context, db *sql.DB, target int64) ([]metricRefTarget, error) {
	rows, err := db.QueryContext(ctx, `
SELECT 'metrics', id, COALESCE(metric_id, ''), COALESCE(metric_name, ''), COALESCE(metric_name_en, ''),
       COALESCE(metric_subject, ''), COALESCE(metric_subject_en, ''), COALESCE(table_name_or_section, ''), COALESCE(condition, '')
FROM kb.metrics WHERE input_record_id = $1 AND ref_mention_id IS NULL
UNION ALL
SELECT 'metrics_dropped', id, drop_id, COALESCE(row_data->>'metric_name', ''), COALESCE(row_data->>'metric_name_en', ''),
       COALESCE(row_data->>'subject', row_data->>'metric_subject', ''), COALESCE(row_data->>'subject_en', row_data->>'metric_subject_en', ''),
       COALESCE(row_data->>'table_name_or_section', ''), COALESCE(row_data->>'condition', '')
FROM kb.metrics_dropped WHERE input_record_id = $1 AND drop_reason IN ('term_definition', 'no_value')`, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metricRefTarget
	for rows.Next() {
		var t metricRefTarget
		if err := rows.Scan(&t.Table, &t.RowID, &t.ArtifactID, &t.Name, &t.NameEn, &t.Subject, &t.SubjectEn, &t.Section, &t.Cond); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// scoreMetricReference applies policy metric_reference_match/v1 (design D9).
func scoreMetricReference(m RefMention, target int64, ts []metricRefTarget) ResolveOutcome {
	located := func(t metricRefTarget) bool {
		loc := refCompact(m.TargetLocator)
		return loc != "" && strings.Contains(refCompact(t.Section), loc)
	}
	var cands []ArtifactCandidate
	for _, t := range ts {
		score := 0.0
		for _, a := range []string{m.MetricName, m.MetricNameEn} {
			for _, b := range []string{t.Name, t.NameEn} {
				if s := refDice(a, b); s > score {
					score = s
				}
			}
		}
		subj := 0.0
		for _, a := range []string{m.MetricSubject, m.MetricSubjectEn} {
			for _, b := range []string{t.Subject, t.SubjectEn} {
				if s := refDice(a, b); s > subj {
					subj = s
				}
			}
		}
		if subj >= 0.5 {
			score += metricRefSubjectBonus
		}
		if located(t) {
			score += metricRefLocatorBonus
			if m.TargetContent == "criteria_set" && t.Table == "metrics" && score < metricRefResolveMin {
				score = metricRefResolveMin
			}
		}
		if score > 1 {
			score = 1
		}
		cands = append(cands, ArtifactCandidate{Table: t.Table, ArtifactID: t.ArtifactID, RowID: t.RowID, Score: score,
			ArtifactKey: metricArtifactKey(target, t.Section, firstNonEmptyTrimmed(t.Name, t.NameEn), t.Subject, t.Cond),
			Resolved:    m.TargetContent == "criteria_set" && t.Table == "metrics" && located(t)})
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })

	if m.TargetContent == "criteria_set" {
		var set []ArtifactCandidate
		for _, c := range cands {
			if c.Resolved {
				set = append(set, c)
			}
		}
		if len(set) > 0 {
			return ResolveOutcome{State: metricRefResolved, Candidates: set}
		}
		for i := range cands {
			cands[i].Resolved = false
		}
		return ResolveOutcome{State: metricRefNeedsReview, Notes: []string{"no_located_set"}, Candidates: topRefCandidates(cands, metricRefCandidateMin)}
	}

	top := topRefCandidates(cands, metricRefCandidateMin)
	if len(top) == 0 {
		return ResolveOutcome{State: metricRefNoMatch, Notes: []string{"type_doubtful"}}
	}
	lead := top[0].Score
	if len(top) > 1 {
		lead -= top[1].Score
	}
	if top[0].Score >= metricRefResolveMin && lead >= metricRefLeadMin {
		top[0].Resolved = true
		return ResolveOutcome{State: metricRefResolved, Candidates: top}
	}
	return ResolveOutcome{State: metricRefNeedsReview, Candidates: top}
}

func topRefCandidates(cs []ArtifactCandidate, min float64) []ArtifactCandidate {
	var out []ArtifactCandidate
	for _, c := range cs {
		if c.Score >= min && len(out) < metricRefMaxCandidates {
			out = append(out, c)
		}
	}
	return out
}

// Apply replaces the mention's metric links and resolved rows with the outcome (design D8/D10).
func (metricReferenceResolver) Apply(ctx context.Context, db *sql.DB, m RefMention, out ResolveOutcome) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Serialize writers of this record's <rec>_rfm_<n> numbering.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('kb.metrics.rfm:' || $1::text, 0))`, m.InputRecordID); err != nil {
		return err
	}
	if err := clearMetricResolution(ctx, tx, m.ID); err != nil {
		return err
	}
	var next int
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(NULLIF(split_part(metric_id, '_rfm_', 2), '')::int), 0)
FROM kb.metrics WHERE input_record_id = $1 AND ref_mention_id IS NOT NULL`, m.InputRecordID).Scan(&next); err != nil {
		return err
	}
	for i, c := range out.Candidates {
		status := "candidate"
		if c.Resolved {
			status = "resolved"
		}
		target := c.RowID
		var linkID int64
		err := tx.QueryRowContext(ctx, `
INSERT INTO kb.reference_artifact_links (ref_mention_id, artifact_family, citing_input_id, target_input_id,
  target_artifact_table, target_artifact_id, target_artifact_key, rank, score, match_method, status, policy_version)
SELECT $1, 'metric', $2, input_record_id, $3, $4, $5, $6, $7, 'name', $8, $9
FROM kb.`+refTargetTable(c.Table)+` WHERE id = $10
ON CONFLICT (ref_mention_id, artifact_family, target_artifact_key) DO NOTHING
RETURNING id`, m.ID, m.InputRecordID, c.Table, c.ArtifactID, c.ArtifactKey, i+1, c.Score, status, metricRefPolicyVersion, target).Scan(&linkID)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		if !c.Resolved || c.Table != "metrics" {
			continue // a dropped definition is linked, never copied: it has no value
		}
		next++
		rowID := fmt.Sprintf("%d_rfm_%d", m.InputRecordID, next)
		if err := insertResolvedMetricRow(ctx, tx, m, c, rowID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE kb.reference_artifact_links SET resolved_artifact_id = $2 WHERE id = $1`, linkID, rowID); err != nil {
			return err
		}
	}
	ext, _ := json.Marshal(map[string]any{"policy": metricRefPolicyVersion, "notes": out.Notes})
	if _, err := tx.ExecContext(ctx, `
UPDATE kb.ref_mentions SET metric_state = $2, ext_info = ext_info || jsonb_build_object('metric_resolution', $3::jsonb)
WHERE id = $1`, m.ID, out.State, string(ext)); err != nil {
		return err
	}
	return tx.Commit()
}

// insertResolvedMetricRow copies the target metric's value into the referencing record (design
// D10): value columns from the target, name/subject/context/section/lines from the referencing text.
func insertResolvedMetricRow(ctx context.Context, tx *sql.Tx, m RefMention, c ArtifactCandidate, rowID string) error {
	spans, _ := json.Marshal(m.SourceLineSpans)
	ext, _ := json.Marshal(map[string]any{"reference": map[string]any{"ref_id": m.RefID, "cited_raw": m.CitedRaw, "score": c.Score, "policy": metricRefPolicyVersion}})
	_, err := tx.ExecContext(ctx, `
INSERT INTO kb.metrics (
  input_record_id, metric_id, metric_name, metric_name_en, metric_subject, metric_subject_en, metric_desc, metric_desc_en,
  metric_context, source_line_spans, location_type, metric_keywords, metric_keywords_en, metric_unit, metric_unit_en,
  formula_or_definition, threshold_or_target, measurement_frequency, confidence, is_explicit_metric, reasoning_tags,
  ext_info, table_name_or_section, metric_value, value_data_type, value_range_type, value_class, value_class_en,
  category_paths, category_paths_en, metric_categories, metric_categories_en, value_min, value_max, condition,
  keyword_concept_id, metric_definition_term_id, ref_mention_id, ref_input_record_id, ref_metric_id, ref_metric_key, source_table_rows)
SELECT $1, $2, COALESCE(NULLIF($3, ''), t.metric_name), COALESCE(NULLIF($4, ''), t.metric_name_en),
  COALESCE(NULLIF($5, ''), t.metric_subject), COALESCE(NULLIF($6, ''), t.metric_subject_en), t.metric_desc, t.metric_desc_en,
  NULLIF($7, ''), $8::jsonb, t.location_type, t.metric_keywords, t.metric_keywords_en, t.metric_unit, t.metric_unit_en,
  t.formula_or_definition, t.threshold_or_target, t.measurement_frequency, t.confidence, FALSE, '["reference_resolved"]'::jsonb,
  $9::jsonb, NULLIF($10, ''), t.metric_value, t.value_data_type, t.value_range_type, t.value_class, t.value_class_en,
  t.category_paths, t.category_paths_en, t.metric_categories, t.metric_categories_en, t.value_min, t.value_max,
  COALESCE(NULLIF($11, ''), t.condition), t.keyword_concept_id, t.metric_definition_term_id,
  $12, t.input_record_id, t.metric_id, $13, $15::jsonb
FROM kb.metrics t WHERE t.id = $14`,
		m.InputRecordID, rowID, m.MetricName, m.MetricNameEn, m.MetricSubject, m.MetricSubjectEn,
		m.Evidence, string(spans), string(ext), m.Section, m.Condition, m.ID, c.ArtifactKey, c.RowID, sourceTableRowsSQLValue(m.SourceTableRows))
	return err
}

// refTargetTable allows only the two candidate tables into SQL text.
func refTargetTable(table string) string {
	if table == "metrics_dropped" {
		return "metrics_dropped"
	}
	return "metrics"
}

// refDice is the Dice similarity of two names: character bigrams when either contains CJK text,
// word tokens otherwise. Equal non-empty strings score 1.
func refDice(a, b string) float64 {
	a, b = refNormKey(a), refNormKey(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	var x, y []string
	if refHasCJK(a) || refHasCJK(b) {
		x, y = refBigrams(a), refBigrams(b)
	} else {
		x, y = refTokens(a), refTokens(b)
	}
	if len(x) == 0 || len(y) == 0 {
		return 0
	}
	count := map[string]int{}
	for _, g := range x {
		count[g]++
	}
	inter := 0
	for _, g := range y {
		if count[g] > 0 {
			count[g]--
			inter++
		}
	}
	return 2 * float64(inter) / float64(len(x)+len(y))
}

func refHasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func refBigrams(s string) []string {
	var rs []rune
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			rs = append(rs, r)
		}
	}
	if len(rs) == 1 {
		return []string{string(rs)}
	}
	var out []string
	for i := 0; i+1 < len(rs); i++ {
		out = append(out, string(rs[i:i+2]))
	}
	return out
}

func refTokens(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// refCompact lowercases and drops spaces and punctuation, so "Table 1" ~ "table1" and "表 1" ~ "表1".
func refCompact(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
