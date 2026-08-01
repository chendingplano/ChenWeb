package comparison

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// ComparisonScope is the immutable selection record for a reproducible
// comparison. It pins governing releases separately from live activation.
type ComparisonScope struct {
	ComparisonScopeID string          `json:"comparison_scope_id"`
	TargetObjectIDs   json.RawMessage `json:"target_object_ids"`
	MetricKeys        json.RawMessage `json:"metric_keys"`
	AsOfDate          string          `json:"as_of_date"`
	ModuleReleases    json.RawMessage `json:"module_releases"`
	ProfileReleases   json.RawMessage `json:"profile_releases"`
	PrecedencePolicy  json.RawMessage `json:"precedence_policy"`
	ClosedDimensions  json.RawMessage `json:"closed_dimensions"`
	SelectedBy        string          `json:"selected_by"`
	SelectionReason   string          `json:"selection_reason"`
	CreateTime        time.Time       `json:"create_time"`
}

// ComparisonRun records a concrete evaluation of one frozen scope at a known
// assertion watermark and comparator implementation version.
type ComparisonRun struct {
	ID                 int64     `json:"id"`
	ComparisonScopeID  string    `json:"comparison_scope_id"`
	InputRecordID      int64     `json:"input_record_id"`
	AssertionWatermark string    `json:"assertion_watermark"`
	ComparatorVersion  string    `json:"comparator_version"`
	CreateTime         time.Time `json:"create_time"`
}

// ComparisonCell retains all assertion/citation evidence beside the
// precedence-selected representatives used for its directional verdict.
type ComparisonCell struct {
	RunID                              int64           `json:"comparison_run_id"`
	TargetObjectID                     string          `json:"target_object_id"`
	MetricKey                          string          `json:"metric_key"`
	SubjectFamily                      string          `json:"subject_family"`
	SubjectRepresentativeAssertionID   int64           `json:"subject_representative_assertion_id"`
	SubjectAssertions                  json.RawMessage `json:"subject_assertions"`
	SubjectRemainderCount              int64           `json:"subject_remainder_count"`
	AuthorityFamily                    string          `json:"authority_family"`
	AuthorityRepresentativeAssertionID int64           `json:"authority_representative_assertion_id"`
	AuthorityAssertions                json.RawMessage `json:"authority_assertions"`
	AuthorityRemainderCount            int64           `json:"authority_remainder_count"`
	Verdict                            Verdict         `json:"verdict"`
	Direction                          string          `json:"direction"`
	Rationale                          string          `json:"rationale"`
}

type ComparisonStore struct{ DB terms.DBX }

func (s ComparisonStore) CreateScope(ctx context.Context, scope ComparisonScope) (ComparisonScope, error) {
	if s.DB == nil {
		return ComparisonScope{}, errors.New("db is nil")
	}
	if strings.TrimSpace(scope.ComparisonScopeID) == "" || scope.AsOfDate == "" || len(scope.TargetObjectIDs) == 0 || len(scope.MetricKeys) == 0 || len(scope.ModuleReleases) == 0 || len(scope.ProfileReleases) == 0 {
		return ComparisonScope{}, errors.New("comparison scope identity, targets, metrics, as_of_date, module_releases, and profile_releases are required")
	}
	if len(scope.PrecedencePolicy) == 0 {
		scope.PrecedencePolicy = json.RawMessage(`{}`)
	}
	if len(scope.ClosedDimensions) == 0 {
		scope.ClosedDimensions = json.RawMessage(`{}`)
	}
	const stmt = `INSERT INTO kb.ontology_comparison_scopes (comparison_scope_id, target_object_ids, metric_keys, as_of_date, module_releases, profile_releases, precedence_policy, closed_dimensions, selected_by, selection_reason) VALUES ($1, $2::jsonb, $3::jsonb, $4::date, $5::jsonb, $6::jsonb, $7::jsonb, $8::jsonb, $9, $10) RETURNING comparison_scope_id, target_object_ids, metric_keys, as_of_date::text, module_releases, profile_releases, precedence_policy, closed_dimensions, COALESCE(selected_by, ''), COALESCE(selection_reason, ''), create_time`
	row := s.DB.QueryRowContext(ctx, stmt, scope.ComparisonScopeID, string(scope.TargetObjectIDs), string(scope.MetricKeys), scope.AsOfDate, string(scope.ModuleReleases), string(scope.ProfileReleases), string(scope.PrecedencePolicy), string(scope.ClosedDimensions), nullableText(scope.SelectedBy), nullableText(scope.SelectionReason))
	var out ComparisonScope
	err := row.Scan(&out.ComparisonScopeID, &out.TargetObjectIDs, &out.MetricKeys, &out.AsOfDate, &out.ModuleReleases, &out.ProfileReleases, &out.PrecedencePolicy, &out.ClosedDimensions, &out.SelectedBy, &out.SelectionReason, &out.CreateTime)
	return out, err
}

func (s ComparisonStore) CreateRun(ctx context.Context, run ComparisonRun) (ComparisonRun, error) {
	if s.DB == nil {
		return ComparisonRun{}, errors.New("db is nil")
	}
	if strings.TrimSpace(run.ComparisonScopeID) == "" || run.InputRecordID == 0 || strings.TrimSpace(run.AssertionWatermark) == "" || strings.TrimSpace(run.ComparatorVersion) == "" {
		return ComparisonRun{}, errors.New("comparison run provenance is required")
	}
	const stmt = `INSERT INTO kb.ontology_comparison_runs (comparison_scope_id, input_record_id, assertion_watermark, comparator_version) VALUES ($1, $2, $3, $4) RETURNING id, comparison_scope_id, input_record_id, assertion_watermark, comparator_version, create_time`
	var out ComparisonRun
	err := s.DB.QueryRowContext(ctx, stmt, run.ComparisonScopeID, run.InputRecordID, run.AssertionWatermark, run.ComparatorVersion).Scan(&out.ID, &out.ComparisonScopeID, &out.InputRecordID, &out.AssertionWatermark, &out.ComparatorVersion, &out.CreateTime)
	return out, err
}

func (s ComparisonStore) PersistCell(ctx context.Context, cell ComparisonCell) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	if cell.RunID == 0 || strings.TrimSpace(cell.TargetObjectID) == "" || strings.TrimSpace(cell.MetricKey) == "" || strings.TrimSpace(cell.SubjectFamily) == "" || strings.TrimSpace(cell.AuthorityFamily) == "" || cell.Verdict == "" || cell.Direction != "subject_to_authority" || strings.TrimSpace(cell.Rationale) == "" {
		return errors.New("comparison cell provenance and directional verdict are required")
	}
	if len(cell.SubjectAssertions) == 0 {
		cell.SubjectAssertions = json.RawMessage(`[]`)
	}
	if len(cell.AuthorityAssertions) == 0 {
		cell.AuthorityAssertions = json.RawMessage(`[]`)
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO kb.ontology_comparison_cells (comparison_run_id, target_object_id, metric_key, subject_family, subject_representative_assertion_id, subject_assertions, subject_remainder_count, authority_family, authority_representative_assertion_id, authority_assertions, authority_remainder_count, verdict, direction, rationale) VALUES ($1, $2, $3, $4, NULLIF($5, 0), $6::jsonb, $7, $8, NULLIF($9, 0), $10::jsonb, $11, $12, $13, $14)`, cell.RunID, cell.TargetObjectID, cell.MetricKey, cell.SubjectFamily, cell.SubjectRepresentativeAssertionID, string(cell.SubjectAssertions), cell.SubjectRemainderCount, cell.AuthorityFamily, cell.AuthorityRepresentativeAssertionID, string(cell.AuthorityAssertions), cell.AuthorityRemainderCount, string(cell.Verdict), cell.Direction, cell.Rationale)
	return err
}

func nullableText(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
