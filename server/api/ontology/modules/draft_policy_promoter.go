package modules

import (
	"context"
	"encoding/json"
)

// DraftPolicyPromoter is the minimal transaction interface required by the
// module release flow to ensure a draft pipeline-policy version exists for
// the source release's approved proposals. The modules package defines this
// interface; docprocessing provides the production implementation
// (PolicyPromotionStore). Dependency direction: command -> modules -> docprocessing.
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
