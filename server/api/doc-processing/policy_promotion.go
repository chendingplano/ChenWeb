package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
)

// DraftPolicyPromoter is the minimal transaction interface required by the
// module release flow to ensure a draft pipeline-policy version exists for
// the source release's approved proposals. Defined in docprocessing to avoid
// import cycles (modules -> profiles -> docprocessing -> modules). The
// ontology-compiler command wires the modules.ProposalStore to this interface.
type DraftPolicyPromoter interface {
	EnsureDraftFromModuleRelease(ctx context.Context, releaseID int64, releaseChecksum string, proposals []PromotedProposal) (int64, error)
}

// PromotedProposal is the minimal proposal data the promoter needs to
// materialize draft policy content.
type PromotedProposal struct {
	ProposalID        int64
	Predicate         json.RawMessage
	PredicateChecksum string
}

// PolicyPromotionStore is the production DraftPolicyPromoter. It creates
// draft pipeline-policy versions from approved module proposals inside the
// release transaction. It never activates routing (spec 2026080102 section 8:
// "It never activates routing").
type PolicyPromotionStore struct {
	DB    *sql.DB
	Audit policyaudit.Writer
}

// EnsureDraftFromModuleRelease creates a new draft pipeline-policy version
// carrying the source release's approved proposals as conditional bindings.
// It is idempotent per release: if a draft already exists for this release,
// it returns the existing policy id without creating a duplicate.
//
// The draft policy is invisible to resolution until separately activated
// through the existing authenticated endpoint (E2).
func (s PolicyPromotionStore) EnsureDraftFromModuleRelease(ctx context.Context, releaseID int64, releaseChecksum string, proposals []PromotedProposal) (int64, error) {
	if s.DB == nil {
		return 0, errors.New("db is nil")
	}
	if releaseID <= 0 {
		return 0, errors.New("release_id is required")
	}
	if strings.TrimSpace(releaseChecksum) == "" {
		return 0, errors.New("release_checksum is required")
	}

	// Idempotency: check if a draft already exists for this release.
	var existingID int64
	err := s.DB.QueryRowContext(ctx, `
SELECT id FROM kb.pipeline_policies
WHERE source_ref = $1 AND status = 'draft'
LIMIT 1`, fmt.Sprintf("module_release:%d", releaseID)).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	// Determine the next version number.
	var maxVersion int
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM kb.pipeline_policies`).Scan(&maxVersion)
	nextVersion := maxVersion + 1

	// Create the draft policy.
	var policyID int64
	err = s.DB.QueryRowContext(ctx, `
INSERT INTO kb.pipeline_policies (version, status, source_ref, checksum)
VALUES ($1, 'draft', $2, $3)
RETURNING id`, nextVersion, fmt.Sprintf("module_release:%d", releaseID), releaseChecksum).Scan(&policyID)
	if err != nil {
		return 0, fmt.Errorf("create draft policy: %w", err)
	}

	// Look up the default pipeline ID for conditional bindings.
	var defaultPipelineID int64
	err = s.DB.QueryRowContext(ctx, `
SELECT id FROM kb.pipelines WHERE name = $1 LIMIT 1`, DefaultProductionPipelineName).Scan(&defaultPipelineID)
	if err != nil {
		return 0, fmt.Errorf("lookup default pipeline: %w", err)
	}

	// Materialize approved proposals as conditional bindings under the draft.
	for _, proposal := range proposals {
		_, err := s.DB.ExecContext(ctx, `
INSERT INTO kb.pipeline_bindings
    (ks_store_id, pipeline_id, policy_id, name, priority, active, tenant_id, user_id, binding_kind, predicate, predicate_checksum)
VALUES (NULL, $1, $2, $3, 0, true, '-', '', 'conditional', $4::jsonb, $5)`,
			defaultPipelineID, policyID,
			fmt.Sprintf("module_proposal:%d", proposal.ProposalID),
			string(proposal.Predicate), proposal.PredicateChecksum)
		if err != nil {
			return 0, fmt.Errorf("materialize proposal %d: %w", proposal.ProposalID, err)
		}
	}

	// Emit audit event.
	if s.Audit != nil {
		_ = s.Audit.WriteEvent(ctx, policyaudit.Event{
			Kind:          "proposal_promoted",
			PolicyID:      policyID,
			PolicyVersion: nextVersion,
			SubjectKind:   "module_release",
			SubjectID:     releaseID,
			Detail: map[string]any{
				"release_checksum": releaseChecksum,
				"proposal_count":   len(proposals),
				"draft_policy_id":  policyID,
			},
		})
	}

	return policyID, nil
}

// PromoteModuleReleaseProposals is the convenience entry point that loads
// approved proposals from the module's proposal store and promotes them
// through the DraftPolicyPromoter. It is called by the ontology-compiler
// command during release creation/activation.
func PromoteModuleReleaseProposals(ctx context.Context, promoter DraftPolicyPromoter, proposalLister interface {
	ListApprovedProposals(context.Context, int64) ([]proposalRecord, error)
}, releaseID int64, releaseChecksum string) (int64, error) {
	if promoter == nil {
		return 0, nil // no promoter configured; promotion is optional
	}
	records, err := proposalLister.ListApprovedProposals(ctx, releaseID)
	if err != nil {
		return 0, fmt.Errorf("list approved proposals: %w", err)
	}
	if len(records) == 0 {
		return 0, nil // no proposals to promote
	}
	proposals := make([]PromotedProposal, len(records))
	for i, r := range records {
		proposals[i] = PromotedProposal{
			ProposalID:        r.ID,
			Predicate:         r.Predicate,
			PredicateChecksum: r.PredicateChecksum,
		}
	}
	return promoter.EnsureDraftFromModuleRelease(ctx, releaseID, releaseChecksum, proposals)
}

// proposalRecord mirrors modules.ApplicabilityProposal's fields needed for
// promotion, avoiding an import cycle.
type proposalRecord struct {
	ID                int64
	Predicate         json.RawMessage
	PredicateChecksum string
}

// Ensure PolicyPromotionStore satisfies DraftPolicyPromoter at compile time.
var _ DraftPolicyPromoter = PolicyPromotionStore{}
