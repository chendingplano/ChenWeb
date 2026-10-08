package docprocessing

// Soft drop of extract_metrics rows (openspec change metric-row-soft-drop-decision-model,
// spec metric-row-soft-drop). A row the pipeline sets aside is saved to kb.metrics_dropped
// instead of being discarded; kb.metrics keeps only live rows, so its readers never see it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Drop stages, in pipeline order (design D2).
const (
	metricDropStageLLMTag        = "llm_tag"
	metricDropStageStatementKind = "statement_kind"
	metricDropStageDecision      = "decision_model"
)

// droppedMetricRow is one enriched row the pipeline set aside.
type droppedMetricRow struct {
	Row      map[string]any
	Stage    string
	Reason   string
	Kind     string         // statement kind when set aside
	Decision map[string]any // open-value decision; nil for the other stages
	DropID   string         // assigned by SaveDroppedMetrics
}

func newDroppedMetricRow(row map[string]any, stage, reason string) droppedMetricRow {
	return droppedMetricRow{Row: row, Stage: stage, Reason: reason, Kind: metricStatementKind(row)}
}

// DroppedMetricsStore persists set-aside rows. MetricsSQLStore implements it; a store
// that does not is skipped (the drops are still logged).
type DroppedMetricsStore interface {
	// SaveDroppedMetrics numbers rows <record>_drp_<n>, continuing after the record's
	// highest existing drop id, sets each row's DropID and saves them.
	SaveDroppedMetrics(ctx context.Context, inputRecordID int64, eventID string, rows []droppedMetricRow) error
}

// SaveDroppedMetrics delegates to Inner when it stores dropped rows.
func (s *ResolvingMetricsStore) SaveDroppedMetrics(ctx context.Context, inputRecordID int64, eventID string, rows []droppedMetricRow) error {
	inner, ok := s.Inner.(DroppedMetricsStore)
	if !ok {
		return nil
	}
	return inner.SaveDroppedMetrics(ctx, inputRecordID, eventID, rows)
}

func (s MetricsSQLStore) SaveDroppedMetrics(ctx context.Context, inputRecordID int64, eventID string, rows []droppedMetricRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("(MID_26100801) begin dropped-metrics tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var maxSeq int
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(split_part(drop_id, '_drp_', 2)::int), 0)
FROM kb.metrics_dropped WHERE input_record_id = $1`, inputRecordID).Scan(&maxSeq); err != nil {
		return fmt.Errorf("(MID_26100802) read max drop id for record_id=%d: %w", inputRecordID, err)
	}
	for i := range rows {
		rows[i].DropID = fmt.Sprintf("%d_drp_%d", inputRecordID, maxSeq+i+1)
		rowJSON, err := json.Marshal(rows[i].Row)
		if err != nil {
			return fmt.Errorf("(MID_26100803) marshal dropped row %s: %w", rows[i].DropID, err)
		}
		var decision any
		if rows[i].Decision != nil {
			b, _ := json.Marshal(rows[i].Decision)
			decision = string(b)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO kb.metrics_dropped
	(input_record_id, drop_id, candidate_id, drop_stage, drop_reason, decision, row_data, event_id)
VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6::jsonb, $7::jsonb, NULLIF($8, ''))`,
			inputRecordID, rows[i].DropID, asString(rows[i].Row["candidate_id"]), rows[i].Stage,
			rows[i].Reason, decision, string(rowJSON), eventID); err != nil {
			return fmt.Errorf("(MID_26100804) insert dropped row %s: %w", rows[i].DropID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("(MID_26100805) commit dropped rows for record_id=%d: %w", inputRecordID, err)
	}
	return nil
}

// saveDroppedMetricRows saves the run's set-aside rows and writes the drop_metric_rows log.
// A failed save is logged, not returned: the live rows are already saved and the drop is
// still recorded in the log.
func (p *MetricsProcessor) saveDroppedMetricRows(ctx context.Context, recordID int64, rows []droppedMetricRow) {
	if len(rows) == 0 {
		return
	}
	if store, ok := p.Store.(DroppedMetricsStore); ok {
		if err := store.SaveDroppedMetrics(ctx, recordID, eventIDFromContext(ctx), rows); err != nil {
			p.Logger.Warn("failed to save dropped metric rows", "record_id", recordID, "num_dropped", len(rows), "error", err)
		}
	} else {
		p.Logger.Warn("metrics store does not keep dropped rows; logging them only", "record_id", recordID, "num_dropped", len(rows))
	}
	p.logDroppedMetricRows(ctx, recordID, rows)
}

// logDroppedMetricRows writes one extract_metrics log entry (activity drop_metric_rows)
// listing every set-aside row of the run.
func (p *MetricsProcessor) logDroppedMetricRows(ctx context.Context, recordID int64, rows []droppedMetricRow) {
	byStage := map[string]int{}
	byReason := map[string]int{}
	dropped := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		byStage[r.Stage]++
		byReason[r.Reason]++
		dropped = append(dropped, map[string]any{
			"drop_id":             r.DropID,
			"candidate_id":        r.Row["candidate_id"],
			"drop_stage":          r.Stage,
			"drop_reason":         r.Reason,
			"kind":                r.Kind,
			"metric_name":         r.Row["metric_name"],
			"subject":             r.Row["subject"],
			"threshold_or_target": r.Row["threshold_or_target"],
			"context":             r.Row["context"],
			"source_line_spans":   r.Row["source_line_spans"],
			"decision":            r.Decision,
		})
	}
	p.Logger.Info("dropped metric rows",
		"record_id", recordID, "num_dropped", len(rows), "by_stage", byStage, "by_reason", byReason)
	artifactBytes, _ := json.Marshal(map[string]any{"dropped": dropped})
	artifactStr := string(artifactBytes)
	extraBytes, _ := json.Marshal(map[string]any{"num_dropped": len(rows), "by_stage": byStage, "by_reason": byReason})
	extraStr := string(extraBytes)
	activityName := "drop_metric_rows"
	rec := DocProcLogRecord{
		CallReason:    p.Name(),
		DocProcName:   p.Name(),
		ModelNames:    compactNonEmptyStrings([]string{p.RelationModelName}),
		PromptName:    p.RelationPromptRef,
		RecordID:      int64Ptr(recordID),
		ActivityName:  &activityName,
		ArtifactJSON:  &artifactStr,
		ExtraInfoJSON: &extraStr,
	}
	if err := p.ProcLogger.LogExtractMetrics(ctx, rec, "MID-26100806"); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			p.Logger.Info("drop_metric_rows log skipped: doc processor stopped by user request", "record_id", recordID)
		} else {
			p.Logger.Warn("failed to write drop_metric_rows log", "record_id", recordID, "error", err)
		}
	}
}
