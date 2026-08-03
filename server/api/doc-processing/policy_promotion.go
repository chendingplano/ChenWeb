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
// the source release's approved proposals. It joins the caller's release
// transaction via the *sql.Tx it is handed, so promotion is atomic with
// release creation/activation and rolls back with it on failure (P5 review
// 2026080302 finding P5-4). Defined in docprocessing to avoid import cycles;
// the ontology-compiler command wires the modules.ProposalStore to this
// interface.
type DraftPolicyPromoter interface {
	EnsureDraftFromModuleRelease(ctx context.Context, tx *sql.Tx, releaseID int64, releaseChecksum string, proposals []PromotedProposal) (int64, error)
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
// "It never activates routing"). All promotion queries run on the supplied
// *sql.Tx; Audit (may be nil) receives the best-effort proposal_promoted
// event on the store's own connection.
type PolicyPromotionStore struct {
	Audit policyaudit.Writer
}

// EnsureDraftFromModuleRelease creates a new draft pipeline-policy version
// carrying the source release's approved proposals as conditional bindings.
// All queries run on the supplied *sql.Tx so promotion joins the caller's
// release transaction and rolls back with it on failure (P5 review 2026080302
// finding P5-4). It is idempotent per source release across draft AND
// activated states: any policy -- not just a draft -- with the same source_ref
// is returned without creating a duplicate, so an activated release is never
// re-promoted (finding P5-10's wrong idempotency key).
//
// The draft policy is invisible to resolution until separately activated
// through the existing authenticated endpoint (E2).
func (s PolicyPromotionStore) EnsureDraftFromModuleRelease(ctx context.Context, tx *sql.Tx, releaseID int64, releaseChecksum string, proposals []PromotedProposal) (int64, error) {
	if tx == nil {
		return 0, errors.New("tx is required")
	}
	if releaseID <= 0 {
		return 0, errors.New("release_id is required")
	}
	if strings.TrimSpace(releaseChecksum) == "" {
		return 0, errors.New("release_checksum is required")
	}

	// Idempotency: any policy carrying this release's source_ref (draft,
	// active, or archived) already materialized it; return it as-is.
	var existingID int64
	err := tx.QueryRowContext(ctx, `
SELECT id FROM kb.pipeline_policies
WHERE source_ref = $1
LIMIT 1`, fmt.Sprintf("module_release:%d", releaseID)).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	// Determine the next version number. The scan error is surfaced (it was
	// silently discarded), and kb.pipeline_policies(version) carries a unique
	// constraint (migration 20260801000023) as the real guard against two
	// concurrent promotions minting the same version.
	var maxVersion int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM kb.pipeline_policies`).Scan(&maxVersion); err != nil {
		return 0, fmt.Errorf("next policy version: %w", err)
	}
	nextVersion := maxVersion + 1

	// Create the draft policy.
	var policyID int64
	err = tx.QueryRowContext(ctx, `
INSERT INTO kb.pipeline_policies (version, status, source_ref, checksum)
VALUES ($1, 'draft', $2, $3)
RETURNING id`, nextVersion, fmt.Sprintf("module_release:%d", releaseID), releaseChecksum).Scan(&policyID)
	if err != nil {
		return 0, fmt.Errorf("create draft policy: %w", err)
	}

	// Look up the default pipeline ID for conditional bindings.
	var defaultPipelineID int64
	err = tx.QueryRowContext(ctx, `
SELECT id FROM kb.pipelines WHERE name = $1 LIMIT 1`, DefaultProductionPipelineName).Scan(&defaultPipelineID)
	if err != nil {
		return 0, fmt.Errorf("lookup default pipeline: %w", err)
	}

	// Materialize approved proposals as conditional bindings under the draft.
	for _, proposal := range proposals {
		_, err := tx.ExecContext(ctx, `
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

// ApprovedProposalLister is the interface the ontology-compiler adapter must
// satisfy to feed approved proposals into PromoteModuleReleaseProposals.
// Returning []PromotedProposal directly avoids the import cycle
// (modules → profiles → docprocessing → modules): the adapter in the
// ontology-compiler command converts modules.ApplicabilityProposal values.
type ApprovedProposalLister interface {
	ListApprovedProposals(ctx context.Context, releaseID int64) ([]PromotedProposal, error)
}

// PromoteModuleReleaseProposals is the convenience entry point that loads
// approved proposals from the module's proposal store and promotes them
// through the DraftPolicyPromoter, joining the caller's release transaction.
// It is called by the ontology-compiler command inside the release flow.
// A nil promoter or nil lister means "no promotion configured" (optional),
// never a panic (P5 review 2026080302 finding P5-10).
func PromoteModuleReleaseProposals(ctx context.Context, tx *sql.Tx, promoter DraftPolicyPromoter, lister ApprovedProposalLister, releaseID int64, releaseChecksum string) (int64, error) {
	if promoter == nil || lister == nil {
		return 0, nil // no promotion configured; promotion is optional
	}
	proposals, err := lister.ListApprovedProposals(ctx, releaseID)
	if err != nil {
		return 0, fmt.Errorf("list approved proposals: %w", err)
	}
	if len(proposals) == 0 {
		return 0, nil // no proposals to promote
	}
	return promoter.EnsureDraftFromModuleRelease(ctx, tx, releaseID, releaseChecksum, proposals)
}

// Ensure PolicyPromotionStore satisfies DraftPolicyPromoter at compile time.
var _ DraftPolicyPromoter = PolicyPromotionStore{}
