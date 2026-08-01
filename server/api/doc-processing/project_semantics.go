// Package docprocessing: project_semantics is the third DR8 Phase D stage
// (spec §10.8). It builds derived projections from accepted assertions --
// this slice's only projection is kb.object_nodes.primary_class_term_id
// (DR10). Gated by the same SEMANTIC_ASSOCIATION_ENABLED flag as stages 1-2.
package docprocessing

import (
	"context"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// runProjectSemantics executes Phase D stage 3 for one input record.
func (s *ControlService) runProjectSemantics(ctx context.Context, recordID int64) {
	if !SemanticAssociationEnabledFromEnv() {
		return
	}
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return
	}

	report, err := (assertions.ProjectSemantics{DB: db}).Run(ctx, recordID)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("project_semantics failed", "record_id", recordID, "error", err)
		}
		return
	}
	if s.Logger != nil && report.TargetsExamined > 0 {
		s.Logger.Info("project_semantics complete",
			"record_id", recordID, "targets_examined", report.TargetsExamined,
			"built", report.Built, "errors", report.Errors)
	}
}
