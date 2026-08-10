package kbhandler

import (
	"context"
	"database/sql"
	"fmt"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// reloadProductionPipelineState reloads the in-process canonical binding and
// gate sets after a pipeline/binding/rule write, so a running process picks
// up the change immediately. ADR 2026081001 DR3 retired the separate
// "activate a policy" step that used to be the one place this reload
// happened (P5 review 2026080302 finding P5-12) -- every mutating endpoint
// now plays that role directly, since kb.pipeline_bindings.active/
// kb.pipeline_rules.active are the only "is this live" signal left.
// Multi-process deployments still require an out-of-band restart/reload
// signal per process; that remains a documented carry-forward, not solved
// here.
var reloadProductionPipelineState = func(ctx context.Context, db *sql.DB) error {
	if err := docprocessing.LoadProductionPipelineRegistry(ctx, docprocessing.PipelineRegistrySQLStore{DB: db}); err != nil {
		return fmt.Errorf("reload pipeline registry: %w", err)
	}
	if err := docprocessing.LoadProductionPipelineBindings(ctx, docprocessing.PipelineBindingSQLStore{DB: db}); err != nil {
		return fmt.Errorf("reload pipeline bindings: %w", err)
	}
	if err := docprocessing.LoadProductionPipelineGates(ctx, docprocessing.PipelineGateSQLStore{DB: db}); err != nil {
		return fmt.Errorf("reload pipeline gates: %w", err)
	}
	return nil
}

// writeRoutingPolicyAlarm raises an operator alarm on the shared
// alarms_errors table. A reload failure after a successful write commit uses
// this instead of failing the request -- the write itself already
// committed and must not be misrepresented as rolled back.
var writeRoutingPolicyAlarm = func(ctx context.Context, db *sql.DB, alarm docprocessing.RoutingAlarm) error {
	return (docprocessing.RoutingAlarmSQLWriter{DB: db}).WriteAlarm(ctx, alarm)
}

// reloadAfterPipelineWrite is the shared best-effort post-commit hook: log +
// alarm on failure, never fail the request that already committed.
func reloadAfterPipelineWrite(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, opDescription string) (warning string) {
	if reloadErr := reloadProductionPipelineState(ctx, db); reloadErr != nil {
		if logger != nil {
			logger.Error("in-process pipeline state reload failed after write", "op", opDescription, "err", reloadErr)
		}
		alarm := docprocessing.RoutingAlarm{
			Kind: docprocessing.RoutingAlarmKindPolicyIntegrity, Severity: docprocessing.RoutingAlarmSeverityError,
			Message: fmt.Sprintf("%s committed but in-process reload failed: %s; this process requires a restart to apply it", opDescription, reloadErr),
		}
		if err := writeRoutingPolicyAlarm(ctx, db, alarm); err != nil && logger != nil {
			logger.Error("failed to write pipeline reload alarm", "op", opDescription, "err", err)
		}
		return "committed, but in-process reload failed; this process requires a restart to apply it"
	}
	return ""
}
