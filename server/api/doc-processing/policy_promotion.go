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

// EnsureDraftFromModuleRelease materializes the source release's approved
// proposals as inactive ("draft") conditional kb.pipeline_bindings rows
// against the default pipeline. All queries run on the supplied *sql.Tx so
// promotion joins the caller's release transaction and rolls back with it on
// failure (P5 review 2026080302 finding P5-4). It is idempotent per source
// release: a binding already carrying this release's name is returned
// without creating a duplicate, so an activated release is never re-promoted
// (finding P5-10's wrong idempotency key).
//
// ADR 2026081001 DR3 retired kb.pipeline_policies (the "draft version,
// separately activated" envelope this used to group bindings under); the
// per-row kb.pipeline_bindings.active flag is the only "is this live" signal
// left, so it now plays the draft/activate role directly: bindings are
// inserted with active=false and stay invisible to resolution
// (ResolveProductionPipelineBinding only reads active=true rows) until
// separately flipped active through the existing binding-update endpoint --
// same reviewed-before-live property as before, expressed with one fewer
// concept.
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

	sourceRef := fmt.Sprintf("module_release:%d", releaseID)

	// Idempotency: a binding already named for this release means it was
	// already materialized; return it as-is rather than duplicating.
	var existingID int64
	err := tx.QueryRowContext(ctx, `
SELECT id FROM kb.pipeline_bindings
WHERE name = $1
ORDER BY id
LIMIT 1`, sourceRef).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	// Look up the current version of the default pipeline for conditional
	// bindings (ADR 2026081001 DR1: only the active version is a valid
	// binding target).
	var defaultPipelineID, defaultPipelineVersion int64
	err = tx.QueryRowContext(ctx, `
SELECT id, version FROM kb.pipelines WHERE name = $1 AND status = 'active' LIMIT 1`, DefaultProductionPipelineName).Scan(&defaultPipelineID, &defaultPipelineVersion)
	if err != nil {
		return 0, fmt.Errorf("lookup default pipeline: %w", err)
	}

	// Materialize approved proposals as inactive conditional bindings.
	var firstBindingID int64
	for i, proposal := range proposals {
		name := sourceRef
		if len(proposals) > 1 {
			name = fmt.Sprintf("%s:%d", sourceRef, proposal.ProposalID)
		}
		var bindingID int64
		err := tx.QueryRowContext(ctx, `
INSERT INTO kb.pipeline_bindings
    (ks_store_id, pipeline_id, name, priority, active, tenant_id, user_id, binding_kind, predicate, predicate_checksum)
VALUES (NULL, $1, $2, 0, false, '-', '', 'conditional', $3::jsonb, $4)
RETURNING id`,
			defaultPipelineID, name, string(proposal.Predicate), proposal.PredicateChecksum).Scan(&bindingID)
		if err != nil {
			return 0, fmt.Errorf("materialize proposal %d: %w", proposal.ProposalID, err)
		}
		if i == 0 {
			firstBindingID = bindingID
		}
	}

	// Emit audit event.
	if s.Audit != nil {
		_ = s.Audit.WriteEvent(ctx, policyaudit.Event{
			Kind:            "proposal_promoted",
			PipelineName:    DefaultProductionPipelineName,
			PipelineVersion: int(defaultPipelineVersion),
			SubjectKind:     "module_release",
			SubjectID:       releaseID,
			Detail: map[string]any{
				"release_checksum":  releaseChecksum,
				"proposal_count":    len(proposals),
				"first_binding_id":  firstBindingID,
			},
		})
	}

	return firstBindingID, nil
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
