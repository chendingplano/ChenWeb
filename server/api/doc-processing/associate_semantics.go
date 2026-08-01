// Package docprocessing: associate_semantics is the second DR8 Phase D
// stage (spec §10.3-§10.7). It resolves, validates, and deterministically
// adjudicates the decision candidates normalize_assertions proposed,
// persisting accepted ones to kb.semantic_assertions. Gated by the same
// SEMANTIC_ASSOCIATION_ENABLED flag as stage 1.
package docprocessing

import (
	"context"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// runAssociateSemantics executes Phase D stage 2 for one input record.
func (s *ControlService) runAssociateSemantics(ctx context.Context, recordID int64) {
	if !SemanticAssociationEnabledFromEnv() {
		return
	}
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return
	}

	report, err := (assertions.AssociateSemantics{DB: db}).Run(ctx, recordID)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("associate_semantics failed", "record_id", recordID, "error", err)
		}
		return
	}
	if s.Logger != nil && report.Examined > 0 {
		s.Logger.Info("associate_semantics complete",
			"record_id", recordID, "examined", report.Examined,
			"accepted", report.Accepted, "deferred", report.Deferred, "rejected", report.Rejected)
	}
}
