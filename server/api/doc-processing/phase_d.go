// Package docprocessing: phase_d is the DR8 Phase D orchestrator. It runs
// all three stages (spec §10.1) for one input record --
// assertions.RunPhaseD normalizes artifacts into candidates, resolves and
// adjudicates them, and builds derived projections -- and logs the spec
// §10.9 association-run report as one consolidated line. Gated by
// SEMANTIC_ASSOCIATION_ENABLED (default false, matching the ADR config
// table) so Phase D stays inert until explicitly turned on.
package docprocessing

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// SemanticAssociationEnabledFromEnv resolves the SEMANTIC_ASSOCIATION_ENABLED
// setting. Unset (or any value that does not parse as a boolean true)
// resolves to disabled, matching the ADR's stated default of 'false'.
func SemanticAssociationEnabledFromEnv() bool {
	raw := strings.TrimSpace(os.Getenv("SEMANTIC_ASSOCIATION_ENABLED"))
	if raw == "" {
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	return err == nil && enabled
}

// runPhaseD executes all three Phase D stages for one input record and logs
// the resulting association-run report (spec §10.9): artifacts examined,
// candidates by method, resolution outcomes, lifecycle counts, new
// assertions, deterministic-vs-human decision counts, and per-stage timing
// and errors.
func (s *ControlService) runPhaseD(ctx context.Context, recordID int64) {
	if !SemanticAssociationEnabledFromEnv() {
		return
	}
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return
	}

	report, err := assertions.RunPhaseD(ctx, db, recordID)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("phase_d run failed", "record_id", recordID, "error", err)
		}
		return
	}
	if len(report.StageErrors) > 0 && s.Logger != nil {
		s.Logger.Error("phase_d stage errors", "record_id", recordID, "stage_errors", report.StageErrors)
	}
	if report.ArtifactsExamined == 0 {
		return
	}
	if s.Logger != nil {
		s.Logger.Info("phase_d run complete",
			"record_id", recordID,
			"artifacts_examined", report.ArtifactsExamined,
			"candidates_by_method", report.CandidatesByMethod,
			"resolution_outcomes", report.ResolutionOutcomes,
			"lifecycle_counts", report.LifecycleCounts,
			"new_assertions", report.NewAssertions,
			"deterministic_decisions", report.DeterministicDecisions,
			"human_decisions", report.HumanDecisions,
			"stage_timings", report.StageTimings,
			"reconciles", report.Reconciles(),
		)
	}
}
