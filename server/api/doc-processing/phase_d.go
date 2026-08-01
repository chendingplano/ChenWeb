// Package docprocessing: phase_d declares DR8's three Phase D stages --
// normalize_assertions, associate_semantics, project_semantics -- as real,
// routed processors (ADR 2026072901 §8.1/§8.2), rather than one hardcoded
// call site outside the processor registry. They run within the existing
// Phase C post-process-indexing tier, ordered after every extraction
// processor has finished (exactly where DR8 places "Phase D" conceptually:
// "three declared stages that run after Phase C"), sequenced via
// PostProcessDependsOn so normalize -> associate -> project always run in
// order for one record. Each stays gated by SEMANTIC_ASSOCIATION_ENABLED
// (default false, matching the ADR config table) so registering them as
// selectable/routable processors does not change default production
// behavior -- the same self-gating pattern P4's routed processors
// (extract_metric_definitions et al.) already use, since the DR5
// Class/Cost/OnUndetermined declarations are metadata only until a runtime
// DAG enforcer exists to consume them.
package docprocessing

import (
	"context"
	"database/sql"
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

// NormalizeAssertionsProcessor is DR8's normalize_assertions stage: every
// registered seam-5 normalizer (DR11) turns its artifact family's output
// into candidate qualified assertions.
type NormalizeAssertionsProcessor struct{}

func NewNormalizeAssertionsProcessor() *NormalizeAssertionsProcessor { return &NormalizeAssertionsProcessor{} }
func (p *NormalizeAssertionsProcessor) Name() string                              { return "normalize_assertions" }
func (p *NormalizeAssertionsProcessor) HandleEvent(context.Context, []byte) error { return nil }

func (p *NormalizeAssertionsProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	if !SemanticAssociationEnabledFromEnv() || ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	return assertions.NormalizeAllFamilies(ctx, ApiTypes.ProjectDBHandle, recordID)
}

// AssociateSemanticsProcessor is DR8's associate_semantics stage: resolve,
// validate, adjudicate, and persist normalize_assertions' candidates. It
// waits for normalize_assertions to finish this run before starting.
type AssociateSemanticsProcessor struct{}

func NewAssociateSemanticsProcessor() *AssociateSemanticsProcessor { return &AssociateSemanticsProcessor{} }
func (p *AssociateSemanticsProcessor) Name() string                              { return "associate_semantics" }
func (p *AssociateSemanticsProcessor) HandleEvent(context.Context, []byte) error { return nil }
func (p *AssociateSemanticsProcessor) PostProcessDependsOn() []string {
	return []string{"normalize_assertions"}
}

func (p *AssociateSemanticsProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	if !SemanticAssociationEnabledFromEnv() || ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	_, err := (assertions.AssociateSemantics{DB: ApiTypes.ProjectDBHandle}).Run(ctx, recordID)
	return err
}

// ProjectSemanticsProcessor is DR8's project_semantics stage: build derived
// projections from assertions accepted this run, then log the spec §10.9
// association-run report. It waits for associate_semantics to finish this
// run before starting.
type ProjectSemanticsProcessor struct {
	Logger ApiTypes.JimoLogger
}

func NewProjectSemanticsProcessor(logger ApiTypes.JimoLogger) *ProjectSemanticsProcessor {
	return &ProjectSemanticsProcessor{Logger: logger}
}
func (p *ProjectSemanticsProcessor) Name() string                              { return "project_semantics" }
func (p *ProjectSemanticsProcessor) HandleEvent(context.Context, []byte) error { return nil }
func (p *ProjectSemanticsProcessor) PostProcessDependsOn() []string {
	return []string{"associate_semantics"}
}

func (p *ProjectSemanticsProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	if !SemanticAssociationEnabledFromEnv() || ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	db := ApiTypes.ProjectDBHandle
	if _, err := (assertions.ProjectSemantics{DB: db}).Run(ctx, recordID); err != nil {
		return err
	}
	p.logAssociationRunReport(ctx, db, recordID)
	return nil
}

// logAssociationRunReport builds and logs the spec §10.9 reconciliation
// report for this record now that all three Phase D stages have run. A
// report build failure is logged, not returned -- the Phase D stages
// themselves already succeeded, so a telemetry-query error must not be
// reported as a post-process indexing failure.
func (p *ProjectSemanticsProcessor) logAssociationRunReport(ctx context.Context, db *sql.DB, recordID int64) {
	if p.Logger == nil {
		return
	}
	report, err := assertions.BuildAssociationRunReport(ctx, db, recordID)
	if err != nil {
		p.Logger.Error("phase_d association-run report failed", "record_id", recordID, "error", err)
		return
	}
	if report.ArtifactsExamined == 0 {
		return
	}
	p.Logger.Info("phase_d run complete",
		"record_id", recordID,
		"artifacts_examined", report.ArtifactsExamined,
		"candidates_by_method", report.CandidatesByMethod,
		"resolution_outcomes", report.ResolutionOutcomes,
		"lifecycle_counts", report.LifecycleCounts,
		"new_assertions", report.NewAssertions,
		"deterministic_decisions", report.DeterministicDecisions,
		"human_decisions", report.HumanDecisions,
		"reconciles", report.Reconciles(),
	)
}
