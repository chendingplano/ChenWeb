package docprocessing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrNoEffectiveRoutingClearance        = errors.New("no effective routing clearance")
	ErrMultipleEffectiveRoutingClearances = errors.New("multiple effective routing clearances")
)

type RoutingClearanceSubject struct {
	// PipelineName/PipelineVersion identify the resolved kb.pipelines
	// version this clearance subject was evaluated against (ADR 2026081001
	// DR3) -- replaces the retired system-wide PolicyID/PolicyVersion.
	PipelineName         string
	PipelineVersion      int
	SubjectKind          string
	SubjectID            int64
	SubjectChecksum      string
	DocumentKind         string
	NetPlanDeltaChecksum string
}

type EffectiveRoutingClearance struct {
	CoverageID  int64
	ClearanceID int64
	ApprovedBy  string
}

type RoutingClearanceStore struct{ DB *sql.DB }

type RoutingClearanceApproval struct {
	Subject                   RoutingClearanceSubject
	PolicyChecksum            string
	ManifestChecksum          string
	BaselineRunID             string
	RoutedRunID               string
	Repetitions               int
	PairedCaseCount           int
	GoldPositiveDenominator   int
	ProcessorFailures         int
	InfrastructureFailures    int
	ScorerFailures            int
	BaselineRecallNumerator   int
	BaselineRecallDenominator int
	RoutedRecallNumerator     int
	RoutedRecallDenominator   int
	BaselinePrecision         *float64
	RoutedPrecision           *float64
	CostYield                 map[string]float64
	DecisionTraces            []string
	Approved                  bool
	Actor                     string
	Rationale                 string
}

const routingClearanceEffectiveQuery = `
SELECT cv.id, cv.clearance_id, c.approved_by
FROM kb.pipeline_routing_clearance_coverage cv
JOIN kb.pipeline_routing_clearances c ON c.id = cv.clearance_id
WHERE cv.pipeline_name = $1
  AND cv.pipeline_version = $2
  AND cv.subject_kind = $3
  AND cv.subject_id = $4
  AND cv.document_kind = $5
  AND cv.subject_checksum = $6
  AND cv.net_plan_delta_checksum = $7
  AND NOT EXISTS (
      SELECT 1 FROM kb.pipeline_routing_clearance_revocations r
      WHERE r.clearance_id = cv.clearance_id
  )
ORDER BY cv.id
LIMIT 2`

func (s RoutingClearanceStore) ResolveEffective(ctx context.Context, subject RoutingClearanceSubject) (EffectiveRoutingClearance, error) {
	if s.DB == nil {
		return EffectiveRoutingClearance{}, errors.New("db is nil")
	}
	rows, err := s.DB.QueryContext(ctx, routingClearanceEffectiveQuery, strings.TrimSpace(subject.PipelineName), subject.PipelineVersion, strings.TrimSpace(subject.SubjectKind), subject.SubjectID, strings.TrimSpace(subject.DocumentKind), strings.TrimSpace(subject.SubjectChecksum), strings.TrimSpace(subject.NetPlanDeltaChecksum))
	if err != nil {
		return EffectiveRoutingClearance{}, err
	}
	defer rows.Close()
	var matches []EffectiveRoutingClearance
	for rows.Next() {
		var match EffectiveRoutingClearance
		if err := rows.Scan(&match.CoverageID, &match.ClearanceID, &match.ApprovedBy); err != nil {
			return EffectiveRoutingClearance{}, err
		}
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return EffectiveRoutingClearance{}, err
	}
	if len(matches) == 0 {
		return EffectiveRoutingClearance{}, ErrNoEffectiveRoutingClearance
	}
	if len(matches) != 1 {
		return EffectiveRoutingClearance{}, ErrMultipleEffectiveRoutingClearances
	}
	return matches[0], nil
}

func (s RoutingClearanceStore) Replace(ctx context.Context, approval RoutingClearanceApproval) (int64, error) {
	return s.writeApproval(ctx, approval, true)
}

func (s RoutingClearanceStore) Approve(ctx context.Context, approval RoutingClearanceApproval) (int64, error) {
	return s.writeApproval(ctx, approval, false)
}

func (s RoutingClearanceStore) Revoke(ctx context.Context, clearanceID int64, actor, reason string) (err error) {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	if clearanceID <= 0 || strings.TrimSpace(actor) == "" || strings.TrimSpace(reason) == "" {
		return errors.New("clearance id, actor, and reason are required")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", fmt.Sprintf("clearance/%d", clearanceID)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO kb.pipeline_routing_clearance_revocations (clearance_id, revoked_by, reason) VALUES ($1, $2, $3)", clearanceID, strings.TrimSpace(actor), strings.TrimSpace(reason)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s RoutingClearanceStore) writeApproval(ctx context.Context, approval RoutingClearanceApproval, replace bool) (clearanceID int64, err error) {
	if s.DB == nil {
		return 0, errors.New("db is nil")
	}
	if !approval.Approved {
		return 0, errors.New("routing evidence is not approved")
	}
	if strings.TrimSpace(approval.Actor) == "" || strings.TrimSpace(approval.Rationale) == "" {
		return 0, errors.New("actor and rationale are required")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	lockKey := strings.Join([]string{approval.Subject.PipelineName, fmt.Sprint(approval.Subject.PipelineVersion), approval.Subject.SubjectKind, fmt.Sprint(approval.Subject.SubjectID), approval.Subject.DocumentKind}, "/")
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return 0, err
	}
	var superseded any
	if replace {
		rows, queryErr := tx.QueryContext(ctx, `SELECT cv.clearance_id
FROM kb.pipeline_routing_clearance_coverage cv
WHERE cv.pipeline_name=$1 AND cv.pipeline_version=$2 AND cv.subject_kind=$3 AND cv.subject_id=$4 AND cv.document_kind=$5
  AND NOT EXISTS (SELECT 1 FROM kb.pipeline_routing_clearance_revocations r WHERE r.clearance_id=cv.clearance_id)
ORDER BY cv.clearance_id`, approval.Subject.PipelineName, approval.Subject.PipelineVersion, approval.Subject.SubjectKind, approval.Subject.SubjectID, approval.Subject.DocumentKind)
		if queryErr != nil {
			return 0, queryErr
		}
		var ids []int64
		for rows.Next() {
			var id int64
			if scanErr := rows.Scan(&id); scanErr != nil {
				_ = rows.Close()
				return 0, scanErr
			}
			ids = append(ids, id)
		}
		if closeErr := rows.Close(); closeErr != nil {
			return 0, closeErr
		}
		if len(ids) > 1 {
			return 0, ErrMultipleEffectiveRoutingClearances
		}
		if len(ids) == 1 {
			superseded = ids[0]
			if _, err = tx.ExecContext(ctx, "INSERT INTO kb.pipeline_routing_clearance_revocations (clearance_id, revoked_by, reason) VALUES ($1, $2, $3)", ids[0], strings.TrimSpace(approval.Actor), "replaced: "+strings.TrimSpace(approval.Rationale)); err != nil {
				return 0, err
			}
		}
	}
	costRaw, err := json.Marshal(approval.CostYield)
	if err != nil {
		return 0, err
	}
	traceRaw, err := json.Marshal(approval.DecisionTraces)
	if err != nil {
		return 0, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO kb.pipeline_routing_clearances (
pipeline_name, pipeline_version, document_kind, manifest_checksum, baseline_run_id, routed_run_id,
repetitions, paired_case_count, gold_positive_denominator, processor_failure_count,
infrastructure_failure_count, scorer_failure_count, baseline_recall_numerator,
baseline_recall_denominator, routed_recall_numerator, routed_recall_denominator,
baseline_precision, routed_precision, cost_yield, decision_traces, approved_by, rationale
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19::jsonb,$20::jsonb,$21,$22)
RETURNING id`, approval.Subject.PipelineName, approval.Subject.PipelineVersion, approval.Subject.DocumentKind, approval.ManifestChecksum, approval.BaselineRunID, approval.RoutedRunID, approval.Repetitions, approval.PairedCaseCount, approval.GoldPositiveDenominator, approval.ProcessorFailures, approval.InfrastructureFailures, approval.ScorerFailures, approval.BaselineRecallNumerator, approval.BaselineRecallDenominator, approval.RoutedRecallNumerator, approval.RoutedRecallDenominator, approval.BaselinePrecision, approval.RoutedPrecision, string(costRaw), string(traceRaw), strings.TrimSpace(approval.Actor), strings.TrimSpace(approval.Rationale)).Scan(&clearanceID)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO kb.pipeline_routing_clearance_coverage
(clearance_id, pipeline_name, pipeline_version, subject_kind, subject_id, subject_checksum, document_kind, net_plan_delta_checksum, superseded_clearance_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, clearanceID, approval.Subject.PipelineName, approval.Subject.PipelineVersion, approval.Subject.SubjectKind, approval.Subject.SubjectID, approval.Subject.SubjectChecksum, approval.Subject.DocumentKind, approval.Subject.NetPlanDeltaChecksum, superseded); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return clearanceID, nil
}

type NetPlanDelta struct {
	RemovedProcessors []string `json:"removed_processors"`
	Suppressive       bool     `json:"suppressive"`
}

func BuildNetPlanDelta(baseline, selected []string) (NetPlanDelta, string, error) {
	selectedSet := make(map[string]struct{}, len(selected))
	for _, processor := range selected {
		if normalized := normalizeRuntimeName(processor); normalized != "" {
			selectedSet[normalized] = struct{}{}
		}
	}
	removedSet := map[string]struct{}{}
	for _, processor := range baseline {
		normalized := normalizeRuntimeName(processor)
		if normalized == "" {
			continue
		}
		if _, present := selectedSet[normalized]; !present {
			removedSet[normalized] = struct{}{}
		}
	}
	removed := make([]string, 0, len(removedSet))
	for processor := range removedSet {
		removed = append(removed, processor)
	}
	sort.Strings(removed)
	delta := NetPlanDelta{RemovedProcessors: removed, Suppressive: len(removed) > 0}
	raw, err := json.Marshal(delta)
	if err != nil {
		return NetPlanDelta{}, "", err
	}
	digest := sha256.Sum256(raw)
	return delta, "sha256:" + strings.ToLower(hex.EncodeToString(digest[:])), nil
}
