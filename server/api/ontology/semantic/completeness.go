package semantic

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// CompletenessReport answers the question the partial unique indexes cannot.
//
// DR4, DR5, and DR13 all say the same thing in different words: a unique index
// enforces AT MOST ONE row, never that a row exists. "Exactly one" is the
// conjunction of database uniqueness, transactional creation, and this
// projection. Anything reported here blocks activation or cutover.
type CompletenessReport struct {
	ArtifactType string
	// RequiredStages is the adapter's declaration this report was built from.
	RequiredStages []string

	CurrentArtifacts int64
	// MissingStageOutcomes counts (artifact, stage) pairs with no active
	// outcome envelope.
	MissingStageOutcomes int64
	// ArtifactsMissingAnyStage counts artifacts missing at least one stage.
	ArtifactsMissingAnyStage int64
	// ArtifactsWithNeitherPath counts identifiable artifacts that have neither
	// a compliant assertion path nor an active unresolved occurrence -- the
	// failed losslessness invariant (DR13).
	ArtifactsWithNeitherPath int64
	// SummaryDrift counts active outcomes whose denormalized finding_count or
	// highest_severity disagrees with their current child set (DR4).
	SummaryDrift int64
	// OrphanActiveFindings counts active findings under an inactive outcome.
	// The deferred constraint trigger makes this unreachable in normal
	// operation; a non-zero value means the trigger was dropped or bypassed.
	OrphanActiveFindings int64
	// AssertionsMissingValueState counts assertions with no governed value
	// state, which the value-state-aware constraint permits for legacy rows
	// but the lossless writer must never produce.
	AssertionsMissingValueState int64
	// ArtifactsWithMissingValue counts source-backed artifacts that were
	// represented and supported, but whose governed value state is explicitly
	// missing. These are present artifacts, not absent-artifact gaps.
	ArtifactsWithMissingValue int64
}

// Complete reports whether the projection found nothing that blocks cutover.
func (r CompletenessReport) Complete() bool {
	return r.MissingStageOutcomes == 0 &&
		r.ArtifactsWithNeitherPath == 0 &&
		r.SummaryDrift == 0 &&
		r.OrphanActiveFindings == 0
}

// BlockingReasons lists why cutover is blocked, for an operator-facing report.
func (r CompletenessReport) BlockingReasons() []string {
	var out []string
	if r.MissingStageOutcomes > 0 {
		out = append(out, fmt.Sprintf("%d (artifact, stage) pair(s) have no active outcome envelope", r.MissingStageOutcomes))
	}
	if r.ArtifactsWithNeitherPath > 0 {
		out = append(out, fmt.Sprintf("%d artifact(s) have neither a semantic instance nor an active unresolved occurrence", r.ArtifactsWithNeitherPath))
	}
	if r.SummaryDrift > 0 {
		out = append(out, fmt.Sprintf("%d outcome(s) have a finding summary that disagrees with their child findings", r.SummaryDrift))
	}
	if r.OrphanActiveFindings > 0 {
		out = append(out, fmt.Sprintf("%d active finding(s) reference an inactive outcome", r.OrphanActiveFindings))
	}
	return out
}

// CompletenessChecker builds the projection for one family.
type CompletenessChecker struct {
	DB *sql.DB
	// ArtifactSourceSQL is the family's current-occurrence query. It must
	// select (artifact_id, input_record_id) for every CURRENT artifact of the
	// family. It is supplied by the family rather than inferred because only
	// the adapter knows what "one atomic current occurrence" means for it.
	ArtifactSourceSQL string
}

// Run builds the completeness report for one adapter.
func (c CompletenessChecker) Run(ctx context.Context, a FamilyAdapter) (CompletenessReport, error) {
	if c.DB == nil {
		return CompletenessReport{}, fmt.Errorf("db is nil")
	}
	if strings.TrimSpace(c.ArtifactSourceSQL) == "" {
		return CompletenessReport{}, fmt.Errorf("artifact source SQL is required: only the family knows what a current occurrence is")
	}
	stages := make([]string, 0, len(a.RequiredStages()))
	for _, st := range a.RequiredStages() {
		stages = append(stages, st.StageTermID)
	}
	rep := CompletenessReport{ArtifactType: a.ArtifactType(), RequiredStages: stages}

	src := "WITH current_artifacts AS (" + c.ArtifactSourceSQL + ")"

	if err := c.DB.QueryRowContext(ctx,
		src+` SELECT count(*) FROM current_artifacts`).Scan(&rep.CurrentArtifacts); err != nil {
		return rep, fmt.Errorf("count current artifacts: %w", err)
	}

	// Cross-join every current artifact with every required stage, then
	// anti-join the active outcomes. This is what makes "missing" detectable
	// at all -- an absent row cannot be found by scanning the outcome table.
	if err := c.DB.QueryRowContext(ctx, src+`
SELECT count(*)
FROM current_artifacts ca
CROSS JOIN unnest($1::text[]) AS s(stage_term_id)
WHERE NOT EXISTS (
  SELECT 1 FROM kb.semantic_processing_outcomes o
  WHERE o.active = true
    AND o.artifact_type = $2
    AND o.artifact_id = ca.artifact_id
    AND o.input_record_id IS NOT DISTINCT FROM ca.input_record_id
    AND o.stage_term_id = s.stage_term_id)`,
		pq.Array(stages), a.ArtifactType()).Scan(&rep.MissingStageOutcomes); err != nil {
		return rep, fmt.Errorf("count missing stage outcomes: %w", err)
	}

	if err := c.DB.QueryRowContext(ctx, src+`
SELECT count(*) FROM current_artifacts ca
WHERE EXISTS (
  SELECT 1 FROM unnest($1::text[]) AS s(stage_term_id)
  WHERE NOT EXISTS (
    SELECT 1 FROM kb.semantic_processing_outcomes o
    WHERE o.active = true AND o.artifact_type = $2
      AND o.artifact_id = ca.artifact_id
      AND o.input_record_id IS NOT DISTINCT FROM ca.input_record_id
      AND o.stage_term_id = s.stage_term_id))`,
		pq.Array(stages), a.ArtifactType()).Scan(&rep.ArtifactsMissingAnyStage); err != nil {
		return rep, fmt.Errorf("count artifacts missing a stage: %w", err)
	}

	// DR13's losslessness invariant: every committed identifiable current
	// artifact has either a compliant current assertion path (a current
	// supporting evidence link) or exactly one active unresolved occurrence.
	if err := c.DB.QueryRowContext(ctx, src+`
SELECT count(*) FROM current_artifacts ca
WHERE NOT EXISTS (
    SELECT 1 FROM kb.assertion_evidence e
    WHERE e.deleted = false AND e.evidence_role = 'supports'
      AND e.artifact_type = $1 AND e.artifact_id = ca.artifact_id
      AND e.input_record_id IS NOT DISTINCT FROM ca.input_record_id)
  AND NOT EXISTS (
    SELECT 1 FROM kb.unresolved_semantic_occurrences u
    WHERE u.active = true AND u.artifact_type = $1
      AND u.artifact_id = ca.artifact_id
      AND u.input_record_id IS NOT DISTINCT FROM ca.input_record_id)`,
		a.ArtifactType()).Scan(&rep.ArtifactsWithNeitherPath); err != nil {
		return rep, fmt.Errorf("count artifacts with neither path: %w", err)
	}

	if err := c.DB.QueryRowContext(ctx, `
SELECT count(*) FROM kb.semantic_processing_outcomes o
WHERE o.active = true AND o.artifact_type = $1
  AND (o.finding_count <> (
        SELECT count(*) FROM kb.semantic_processing_findings f
        WHERE f.outcome_id = o.id AND f.active = true))`,
		a.ArtifactType()).Scan(&rep.SummaryDrift); err != nil {
		return rep, fmt.Errorf("count summary drift: %w", err)
	}

	if err := c.DB.QueryRowContext(ctx, `
SELECT count(*) FROM kb.semantic_processing_findings f
JOIN kb.semantic_processing_outcomes o ON o.id = f.outcome_id
WHERE f.active = true AND o.active = false`).Scan(&rep.OrphanActiveFindings); err != nil {
		return rep, fmt.Errorf("count orphan active findings: %w", err)
	}

	if err := c.DB.QueryRowContext(ctx, `
SELECT count(*) FROM kb.semantic_assertions WHERE value_state_term_id IS NULL`).
		Scan(&rep.AssertionsMissingValueState); err != nil {
		return rep, fmt.Errorf("count assertions missing value state: %w", err)
	}

	if err := c.DB.QueryRowContext(ctx, `
SELECT count(DISTINCT (e.artifact_type, e.artifact_id, e.input_record_id))
FROM kb.assertion_evidence e
JOIN kb.semantic_assertions a ON a.id = e.assertion_id
WHERE e.deleted = false
  AND e.evidence_role = 'supports'
  AND a.value_state_term_id = 'semantic:value_state_missing'`).
		Scan(&rep.ArtifactsWithMissingValue); err != nil {
		return rep, fmt.Errorf("count artifacts with missing value: %w", err)
	}
	return rep, nil
}
