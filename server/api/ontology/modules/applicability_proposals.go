package modules

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// Proposal status closed vocabulary (spec 2026080102 section 8).
const (
	ProposalStatusDraft             = "draft"
	ProposalStatusInReview          = "in_review"
	ProposalStatusApproved          = "approved"
	ProposalStatusIncludedInRelease = "included_in_release"
	ProposalStatusRejected          = "rejected"
)

// ApplicabilityProposal is one governed routing proposal carried by an
// ontology module release. The predicate is validated through semrules
// before the proposal can transition to approved.
type ApplicabilityProposal struct {
	ID                    int64           `json:"id"`
	ModuleID              string          `json:"module_id"`
	ReleaseID             int64           `json:"release_id"`
	ProposalKind          string          `json:"proposal_kind"`
	Predicate             json.RawMessage `json:"predicate"`
	PredicateChecksum     string          `json:"predicate_checksum"`
	Status                string          `json:"status"`
	SourceReleaseChecksum string          `json:"source_release_checksum,omitempty"`
	ApprovedBy            string          `json:"approved_by,omitempty"`
	ApprovedAt            *time.Time      `json:"approved_at,omitempty"`
	IncludedInReleaseID   *int64          `json:"included_in_release_id,omitempty"`
	CreatedBy             string          `json:"created_by,omitempty"`
	CreateTime            time.Time       `json:"create_time"`
}

// ProposalStore persists applicability proposals and their lifecycle
// transitions.
type ProposalStore struct {
	DB *sql.DB
}

// CreateProposal inserts a new draft proposal. The predicate is validated
// through semrules before insertion.
func (s ProposalStore) CreateProposal(ctx context.Context, moduleID string, releaseID int64, predicate json.RawMessage, createdBy, sourceReleaseChecksum string) (ApplicabilityProposal, error) {
	if s.DB == nil {
		return ApplicabilityProposal{}, errors.New("db is nil")
	}
	if strings.TrimSpace(moduleID) == "" {
		return ApplicabilityProposal{}, errors.New("module_id is required")
	}
	if releaseID <= 0 {
		return ApplicabilityProposal{}, errors.New("release_id is required")
	}
	if len(predicate) == 0 {
		return ApplicabilityProposal{}, errors.New("predicate is required")
	}

	doc, err := parsePredicateDocument(predicate)
	if err != nil {
		return ApplicabilityProposal{}, fmt.Errorf("invalid predicate: %w", err)
	}
	if err := semrules.Validate(doc); err != nil {
		return ApplicabilityProposal{}, fmt.Errorf("predicate validation: %w", err)
	}
	// Store the canonical predicate bytes and their canonical checksum so the
	// checksum is exactly what policy_compile.go recomputes and verifies at
	// promotion time (P5 review 2026080302 finding P5-3). The previous scheme
	// hashed the client's raw bytes with a "sha256:" prefix, so every promoted
	// binding failed the compiler's canonical-checksum check.
	canonical, checksum, err := semrules.Canonicalize(doc)
	if err != nil {
		return ApplicabilityProposal{}, fmt.Errorf("predicate canonicalization: %w", err)
	}

	var proposal ApplicabilityProposal
	err = s.DB.QueryRowContext(ctx, `
INSERT INTO kb.ontology_applicability_proposals
    (module_id, release_id, proposal_kind, predicate, predicate_checksum, status, source_release_checksum, created_by)
VALUES ($1, $2, 'routing', $3::jsonb, $4, 'draft', $5, $6)
RETURNING id, module_id, release_id, proposal_kind, predicate, predicate_checksum, status,
          COALESCE(source_release_checksum, ''), COALESCE(created_by, ''), create_time`,
		strings.TrimSpace(moduleID), releaseID, string(canonical), checksum,
		nullIfEmptyStr(sourceReleaseChecksum), nullIfEmptyStr(createdBy),
	).Scan(
		&proposal.ID, &proposal.ModuleID, &proposal.ReleaseID, &proposal.ProposalKind,
		&proposal.Predicate, &proposal.PredicateChecksum, &proposal.Status,
		&proposal.SourceReleaseChecksum, &proposal.CreatedBy, &proposal.CreateTime,
	)
	return proposal, err
}

// TransitionProposal moves a proposal to the next status. Valid transitions:
// draft -> in_review, in_review -> approved, in_review -> rejected.
// approved -> included_in_release is NOT a manual transition: a proposal is
// included only as a consequence of the module release transaction that
// carries it (P5 review 2026080302 finding P5-17), so a caller-supplied
// release id can never claim inclusion in a release that does not contain it.
//
// The update is guarded by the expected current status, so a concurrent double
// transition cannot both succeed: only the transaction whose UPDATE matches
// the row at that status wins, and the loser surfaces a conflict instead of
// silently overwriting the winner's status.
func (s ProposalStore) TransitionProposal(ctx context.Context, id int64, targetStatus, actor string) (ApplicabilityProposal, error) {
	if s.DB == nil {
		return ApplicabilityProposal{}, errors.New("db is nil")
	}
	if id <= 0 {
		return ApplicabilityProposal{}, errors.New("id is required")
	}

	current, err := s.GetProposal(ctx, id)
	if err != nil {
		return ApplicabilityProposal{}, err
	}
	if !validProposalTransition(current.Status, targetStatus) {
		return ApplicabilityProposal{}, fmt.Errorf("invalid transition from %q to %q", current.Status, targetStatus)
	}

	var approvedBy any
	var approvedAt any
	if targetStatus == ProposalStatusApproved {
		if strings.TrimSpace(actor) == "" {
			return ApplicabilityProposal{}, errors.New("actor is required for approval")
		}
		approvedBy = actor
		approvedAt = time.Now()
	}

	var proposal ApplicabilityProposal
	var approvedAtScan sql.NullTime
	var includedReleaseScan sql.NullInt64
	err = s.DB.QueryRowContext(ctx, `
UPDATE kb.ontology_applicability_proposals
SET status = $2, approved_by = COALESCE($3, approved_by), approved_at = COALESCE($4, approved_at),
    modify_time = NOW()
WHERE id = $1 AND status = $5
RETURNING id, module_id, release_id, proposal_kind, predicate, predicate_checksum, status,
          COALESCE(source_release_checksum, ''), COALESCE(approved_by, ''), approved_at,
          included_in_release_id, COALESCE(created_by, ''), create_time`,
		id, targetStatus, approvedBy, approvedAt, current.Status,
	).Scan(
		&proposal.ID, &proposal.ModuleID, &proposal.ReleaseID, &proposal.ProposalKind,
		&proposal.Predicate, &proposal.PredicateChecksum, &proposal.Status,
		&proposal.SourceReleaseChecksum, &proposal.ApprovedBy, &approvedAtScan,
		&includedReleaseScan, &proposal.CreatedBy, &proposal.CreateTime,
	)
	if errors.Is(err, sql.ErrNoRows) {
		// The row no longer matched `status = $expected`: a concurrent
		// transition already moved it. Treat that as a conflict rather than
		// overwriting the winner's status (P5 review 2026080302 finding P5-17).
		return ApplicabilityProposal{}, fmt.Errorf("proposal %d is not in status %q (concurrent transition?)", id, current.Status)
	}
	if err != nil {
		return ApplicabilityProposal{}, err
	}
	if approvedAtScan.Valid {
		t := approvedAtScan.Time
		proposal.ApprovedAt = &t
	}
	if includedReleaseScan.Valid {
		v := includedReleaseScan.Int64
		proposal.IncludedInReleaseID = &v
	}
	return proposal, nil
}

// GetProposal reads one proposal by id.
func (s ProposalStore) GetProposal(ctx context.Context, id int64) (ApplicabilityProposal, error) {
	if s.DB == nil {
		return ApplicabilityProposal{}, errors.New("db is nil")
	}
	var proposal ApplicabilityProposal
	var approvedAt sql.NullTime
	var includedRelease sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `
SELECT id, module_id, release_id, proposal_kind, predicate, predicate_checksum, status,
       COALESCE(source_release_checksum, ''), COALESCE(approved_by, ''), approved_at,
       included_in_release_id, COALESCE(created_by, ''), create_time
FROM kb.ontology_applicability_proposals WHERE id = $1`, id,
	).Scan(
		&proposal.ID, &proposal.ModuleID, &proposal.ReleaseID, &proposal.ProposalKind,
		&proposal.Predicate, &proposal.PredicateChecksum, &proposal.Status,
		&proposal.SourceReleaseChecksum, &proposal.ApprovedBy, &approvedAt,
		&includedRelease, &proposal.CreatedBy, &proposal.CreateTime,
	)
	if err != nil {
		return ApplicabilityProposal{}, err
	}
	if approvedAt.Valid {
		t := approvedAt.Time
		proposal.ApprovedAt = &t
	}
	if includedRelease.Valid {
		v := includedRelease.Int64
		proposal.IncludedInReleaseID = &v
	}
	return proposal, nil
}

// ListApprovedProposals returns all approved proposals for a given release,
// used by H2's draft-policy promotion.
func (s ProposalStore) ListApprovedProposals(ctx context.Context, releaseID int64) ([]ApplicabilityProposal, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, module_id, release_id, proposal_kind, predicate, predicate_checksum, status,
       COALESCE(source_release_checksum, ''), COALESCE(approved_by, ''), approved_at,
       included_in_release_id, COALESCE(created_by, ''), create_time
FROM kb.ontology_applicability_proposals
WHERE release_id = $1 AND status IN ($2, $3)
ORDER BY id`, releaseID, ProposalStatusApproved, ProposalStatusIncludedInRelease)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var proposals []ApplicabilityProposal
	for rows.Next() {
		var p ApplicabilityProposal
		var approvedAt sql.NullTime
		var includedRelease sql.NullInt64
		if err := rows.Scan(
			&p.ID, &p.ModuleID, &p.ReleaseID, &p.ProposalKind,
			&p.Predicate, &p.PredicateChecksum, &p.Status,
			&p.SourceReleaseChecksum, &p.ApprovedBy, &approvedAt,
			&includedRelease, &p.CreatedBy, &p.CreateTime,
		); err != nil {
			return nil, err
		}
		if approvedAt.Valid {
			t := approvedAt.Time
			p.ApprovedAt = &t
		}
		if includedRelease.Valid {
			v := includedRelease.Int64
			p.IncludedInReleaseID = &v
		}
		proposals = append(proposals, p)
	}
	return proposals, rows.Err()
}

// ListProposals returns proposals filtered by release_id and/or status.
// Pass releaseID <= 0 to skip the release filter; pass empty status to
// skip the status filter.
func (s ProposalStore) ListProposals(ctx context.Context, releaseID int64, status string) ([]ApplicabilityProposal, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	query := `
SELECT id, module_id, release_id, proposal_kind, predicate, predicate_checksum, status,
       COALESCE(source_release_checksum, ''), COALESCE(approved_by, ''), approved_at,
       included_in_release_id, COALESCE(created_by, ''), create_time
FROM kb.ontology_applicability_proposals WHERE 1=1`
	var args []any
	argN := 1
	if releaseID > 0 {
		query += fmt.Sprintf(" AND release_id = $%d", argN)
		args = append(args, releaseID)
		argN++
	}
	if strings.TrimSpace(status) != "" {
		query += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, strings.TrimSpace(status))
		argN++
	}
	query += " ORDER BY id"
	_ = argN

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var proposals []ApplicabilityProposal
	for rows.Next() {
		var p ApplicabilityProposal
		var approvedAt sql.NullTime
		var includedRelease sql.NullInt64
		if err := rows.Scan(
			&p.ID, &p.ModuleID, &p.ReleaseID, &p.ProposalKind,
			&p.Predicate, &p.PredicateChecksum, &p.Status,
			&p.SourceReleaseChecksum, &p.ApprovedBy, &approvedAt,
			&includedRelease, &p.CreatedBy, &p.CreateTime,
		); err != nil {
			return nil, err
		}
		if approvedAt.Valid {
			t := approvedAt.Time
			p.ApprovedAt = &t
		}
		if includedRelease.Valid {
			v := includedRelease.Int64
			p.IncludedInReleaseID = &v
		}
		proposals = append(proposals, p)
	}
	return proposals, rows.Err()
}

func validProposalTransition(from, to string) bool {
	switch from {
	case ProposalStatusDraft:
		return to == ProposalStatusInReview
	case ProposalStatusInReview:
		return to == ProposalStatusApproved || to == ProposalStatusRejected
	case ProposalStatusApproved:
		// No manual transition out of approved: inclusion in a release is a
		// consequence of the release transaction, not an HTTP call (P5 review
		// 2026080302 finding P5-17).
		return false
	}
	return false
}

func parsePredicateDocument(raw json.RawMessage) (semrules.Document, error) {
	var doc semrules.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return doc, err
	}
	return doc, nil
}

func nullIfEmptyStr(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}
