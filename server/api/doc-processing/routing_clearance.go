package docprocessing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

var (
	ErrNoEffectiveRoutingClearance        = errors.New("no effective routing clearance")
	ErrMultipleEffectiveRoutingClearances = errors.New("multiple effective routing clearances")
)

type RoutingClearanceSubject struct {
	PolicyID             int64
	PolicyVersion        int
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

const routingClearanceEffectiveQuery = `
SELECT cv.id, cv.clearance_id, c.approved_by
FROM kb.pipeline_routing_clearance_coverage cv
JOIN kb.pipeline_routing_clearances c ON c.id = cv.clearance_id
WHERE cv.policy_id = $1
  AND cv.policy_version = $2
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
	rows, err := s.DB.QueryContext(ctx, routingClearanceEffectiveQuery, subject.PolicyID, subject.PolicyVersion, strings.TrimSpace(subject.SubjectKind), subject.SubjectID, strings.TrimSpace(subject.DocumentKind), strings.TrimSpace(subject.SubjectChecksum), strings.TrimSpace(subject.NetPlanDeltaChecksum))
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
