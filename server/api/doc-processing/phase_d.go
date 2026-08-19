// Package docprocessing: phase_d declares DR8's three Phase D stages --
// normalize_assertions, associate_semantics, project_semantics -- as real,
// routed processors (ADR 2026072901 §8.1/§8.2), rather than one hardcoded
// call site outside the processor registry. They run within the existing
// Phase C post-process-indexing tier, ordered after every extraction
// processor has finished (exactly where DR8 places "Phase D" conceptually:
// "three declared stages that run after Phase C"), sequenced via
// PostProcessDependsOn so normalize -> associate -> project always run in
// order for one record. Each stays gated by SEMANTIC_ASSOCIATION_ENABLED
// (default true as of 2026-08-13, superseding the ADR's original config
// table default of false) via the same self-gating pattern P4's routed
// processors (extract_metric_definitions et al.) already use, since the DR5
// Class/Cost/OnUndetermined declarations are metadata only until a runtime
// DAG enforcer exists to consume them. Because that enforcer doesn't exist
// yet and all three are unconditionally registered in runtime.go, this
// SEMANTIC_ASSOCIATION_ENABLED check is the only thing gating them -- so
// flipping its default is enough to turn Phase D on by default.
package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// SemanticAssociationEnabledFromEnv resolves the SEMANTIC_ASSOCIATION_ENABLED
// (default: true) setting. Unset (or any value that does not parse as a boolean) resolves to
// enabled, matching the default of 'true'. An explicit boolean-false value
// disables the Phase D stages.
func SemanticAssociationEnabledFromEnv() bool {
	raw := strings.TrimSpace(os.Getenv("SEMANTIC_ASSOCIATION_ENABLED"))
	if raw == "" {
		return true
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return enabled
}

// NormalizeAssertionsProcessor is DR8's normalize_assertions stage: every
// registered seam-5 normalizer (DR11) turns its artifact family's output
// into candidate qualified assertions.
type NormalizeAssertionsProcessor struct{}

func NewNormalizeAssertionsProcessor() *NormalizeAssertionsProcessor {
	return &NormalizeAssertionsProcessor{}
}
func (p *NormalizeAssertionsProcessor) Name() string                              { return "normalize_assertions" }
func (p *NormalizeAssertionsProcessor) HandleEvent(context.Context, []byte) error { return nil }

// PostProcessDependsOn (ADR 2026081401 DR6 ordering requirement): waits for
// extract_metrics's Phase C mapping check (when invoked this run) to finish
// before reading kb.metrics rows, so it never observes a row before its
// value_range_type_error flag or kb.metric_value_range_type_map proposal
// exists -- the two Phase C passes would otherwise race.
func (p *NormalizeAssertionsProcessor) PostProcessDependsOn() []string {
	return []string{"extract_metrics"}
}

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

func NewAssociateSemanticsProcessor() *AssociateSemanticsProcessor {
	return &AssociateSemanticsProcessor{}
}
func (p *AssociateSemanticsProcessor) Name() string                              { return "associate_semantics" }
func (p *AssociateSemanticsProcessor) HandleEvent(context.Context, []byte) error { return nil }
func (p *AssociateSemanticsProcessor) PostProcessDependsOn() []string {
	return []string{"normalize_assertions"}
}

func (p *AssociateSemanticsProcessor) PostProcessIndex(ctx context.Context, recordID int64) error {
	if !SemanticAssociationEnabledFromEnv() || ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	report, err := (assertions.AssociateSemantics{DB: ApiTypes.ProjectDBHandle}).Run(ctx, recordID)
	if err != nil && report.MappingMisses == 0 {
		// Candidate/persistence failures use the established generic summary
		// entry type. Keep this at the doc-processing boundary because the
		// assertions package cannot import docprocessing without a cycle.
		if logErr := logAssociateSemanticsFailure(ctx, ApiTypes.ProjectDBHandle, recordID, err); logErr != nil {
			return fmt.Errorf("%w (also failed to write associate_semantics error to kb.doc_proc_logs: %v)", err, logErr)
		}
	}
	// ADR 2026081401 DR3/DR2: the log write lives here, not inside
	// AssociateSemantics.Run itself -- assertions cannot import
	// docprocessing (docprocessing already imports assertions; a reverse
	// import would cycle), and this is the one call site that already has
	// both the report and a DocProcLogger.
	if report.MappingMisses > 0 {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		logErr := DocProcLogger{DB: ApiTypes.ProjectDBHandle}.LogAssertionMappingMiss(ctx, DocProcLogRecord{
			DocProcName: p.Name(),
			RecordID:    &recordID,
			Errors:      &errMsg,
		}, "phase_d.go_AssociateSemanticsProcessor.PostProcessIndex")
		if logErr != nil {
			return fmt.Errorf("%w (also failed to write associate_semantics mapping error to kb.doc_proc_logs: %v)", err, logErr)
		}
	}
	return err
}

func logAssociateSemanticsFailure(ctx context.Context, db *sql.DB, recordID int64, processingErr error) error {
	return logPhaseDError(ctx, db, recordID, "associate_semantics", processingErr)
}

func logPhaseDError(ctx context.Context, db *sql.DB, recordID int64, stage string, processingErr error) error {
	errText := ""
	if processingErr != nil {
		errText = processingErr.Error()
	}
	extraInfo, marshalErr := json.Marshal(map[string]any{
		"stage":     stage,
		"record_id": recordID,
		"error":     errText,
	})
	if marshalErr != nil {
		return fmt.Errorf("marshal %s failure details: %w", stage, marshalErr)
	}
	extraInfoText := string(extraInfo)
	return (DocProcLogger{DB: db}).LogSummary(ctx, EntryTypeError, DocProcLogRecord{
		DocProcName:   stage,
		RecordID:      &recordID,
		Errors:        &errText,
		ExtraInfoJSON: &extraInfoText,
	}, "MID-yyyymmdd-01")
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
		if logErr := logPhaseDError(ctx, db, recordID, p.Name(), err); logErr != nil && p.Logger != nil {
			p.Logger.Error("phase_d project semantics error log failed", "record_id", recordID, "error", logErr)
		}
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
	report, err := assertions.BuildAssociationRunReport(ctx, db, recordID)
	if err != nil {
		if logErr := logPhaseDError(ctx, db, recordID, p.Name(), err); logErr != nil && p.Logger != nil {
			p.Logger.Error("phase_d association-run error log failed", "record_id", recordID, "error", logErr)
		}
		if p.Logger != nil {
			p.Logger.Error("phase_d association-run report failed", "record_id", recordID, "error", err)
		}
		return
	}
	if p.Logger == nil {
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
		"deferred_by_reason", report.DeferredByReason,
		"new_assertions", report.NewAssertions,
		"deterministic_decisions", report.DeterministicDecisions,
		"human_decisions", report.HumanDecisions,
		"reconciles", report.Reconciles(),
	)
	if report.ArtifactsExamined > 0 && report.LifecycleCounts[assertions.StatusDeferred]*100 >= report.ArtifactsExamined*50 {
		p.Logger.Warn("phase_d high deferred candidate rate",
			"record_id", recordID,
			"deferred", report.LifecycleCounts[assertions.StatusDeferred],
			"artifacts_examined", report.ArtifactsExamined,
			"deferred_by_reason", report.DeferredByReason,
		)
		if err := logHighDeferredCandidateRate(ctx, db, recordID,
			report.LifecycleCounts[assertions.StatusDeferred],
			report.ArtifactsExamined,
			report.DeferredByReason,
		); err != nil {
			p.Logger.Error("phase_d high deferred candidate rate log failed", "record_id", recordID, "error", err)
		}
	}
}

func logHighDeferredCandidateRate(ctx context.Context, db *sql.DB, recordID int64, deferred, examined int, deferredByReason map[string]int) error {
	extraInfo, err := json.Marshal(map[string]any{
		"stage":              "associate_semantics",
		"record_id":          recordID,
		"deferred":           deferred,
		"artifacts_examined": examined,
		"deferred_by_reason": deferredByReason,
	})
	if err != nil {
		return fmt.Errorf("marshal high deferred candidate rate details: %w", err)
	}
	errText := fmt.Sprintf("deferred candidate rate exceeded 50%%: %d of %d candidates", deferred, examined)
	extraInfoText := string(extraInfo)
	return (DocProcLogger{DB: db}).LogSummary(ctx, EntryTypeError, DocProcLogRecord{
		DocProcName:   "associate_semantics",
		RecordID:      &recordID,
		Errors:        &errText,
		ExtraInfoJSON: &extraInfoText,
	}, "MID-yyyymmdd-01")
}
