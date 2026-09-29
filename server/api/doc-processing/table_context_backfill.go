package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// TableContextBackfillStats summarizes a table-context backfill run.
type TableContextBackfillStats struct {
	Records      int            // records scanned
	RecordErrors int            // records whose line file could not be read
	ErrorRecords []int64        // their IDs
	TableMetrics int            // metrics whose spans cover a table line
	Changed      int            // metrics whose context or row refs changed
	Outcomes     map[string]int // row-selection outcome counts
}

// TableContextBackfillChange describes one metric the backfill changed (or would
// change, in a dry run).
type TableContextBackfillChange struct {
	RecordID int64
	MetricID string
	Outcome  string
	Before   string
	After    string
}

// RunTableContextBackfill rebuilds metric_context, source_table_rows and
// search_document for existing table-sourced metrics from stored lines, with no LLM
// calls. recordID 0 means every record. metric_context_en is never touched. With
// dryRun nothing is written; onChange (optional) sees every change either way.
func RunTableContextBackfill(ctx context.Context, db *sql.DB, recordID int64, dryRun bool, onChange func(TableContextBackfillChange)) (TableContextBackfillStats, error) {
	stats := TableContextBackfillStats{Outcomes: map[string]int{}}
	ids, err := tableBackfillRecordIDs(ctx, db, recordID)
	if err != nil {
		return stats, err
	}
	for _, id := range ids {
		stats.Records++
		lines, err := LoadRecordLinesByID(ctx, db, id)
		if err != nil {
			stats.RecordErrors++
			stats.ErrorRecords = append(stats.ErrorRecords, id)
			continue
		}
		if !hasTableLine(lines) {
			continue
		}
		if err := backfillRecordTableContexts(ctx, db, id, lines, dryRun, &stats, onChange); err != nil {
			return stats, fmt.Errorf("record %d: %w", id, err)
		}
	}
	return stats, nil
}

func tableBackfillRecordIDs(ctx context.Context, db *sql.DB, recordID int64) ([]int64, error) {
	q := `SELECT DISTINCT input_record_id FROM kb.metrics ORDER BY 1`
	args := []any{}
	if recordID > 0 {
		q = `SELECT DISTINCT input_record_id FROM kb.metrics WHERE input_record_id = $1 ORDER BY 1`
		args = append(args, recordID)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list records: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func hasTableLine(lines []Line) bool {
	for _, l := range lines {
		if l.LineType == "table" {
			return true
		}
	}
	return false
}

func backfillRecordTableContexts(ctx context.Context, db *sql.DB, recordID int64, lines []Line, dryRun bool, stats *TableContextBackfillStats, onChange func(TableContextBackfillChange)) error {
	rows, err := db.QueryContext(ctx, `
SELECT id, COALESCE(metric_id, ''), COALESCE(metric_name, ''), COALESCE(metric_value, ''),
       COALESCE(threshold_or_target, ''), COALESCE(condition, ''),
       COALESCE(source_line_spans::text, '[]'), COALESCE(metric_context, ''),
       COALESCE(source_table_rows::text, ''), COALESCE(search_document, '')
FROM kb.metrics
WHERE input_record_id = $1
ORDER BY id`, recordID)
	if err != nil {
		return fmt.Errorf("load metrics: %w", err)
	}
	type pending struct {
		id                              int64
		metricID, oldCtx, oldRows, sDoc string
		metric                          map[string]any
	}
	var list []pending
	for rows.Next() {
		var p pending
		var name, value, threshold, condition, spans string
		if err := rows.Scan(&p.id, &p.metricID, &name, &value, &threshold, &condition, &spans, &p.oldCtx, &p.oldRows, &p.sDoc); err != nil {
			_ = rows.Close()
			return err
		}
		var spanVal any
		_ = json.Unmarshal([]byte(spans), &spanVal)
		p.metric = map[string]any{
			"metric_id": p.metricID, "metric_name": name, "metric_value": value,
			"threshold_or_target": threshold, "condition": condition,
			"source_line_spans": spanVal, "context": p.oldCtx,
			"source_table_rows": parseTableRowRefs(p.oldRows),
		}
		list = append(list, p)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	idx := newTableLineIndex(lines)
	for _, p := range list {
		res := buildTableMetricContext(idx, p.metric)
		if !res.HasTable {
			continue
		}
		stats.TableMetrics++
		stats.Outcomes[res.Outcome]++
		if res.Context == "" {
			continue
		}
		newRows, _ := json.Marshal(res.Refs)
		if res.Context == p.oldCtx && jsonEqual(p.oldRows, string(newRows)) {
			continue
		}
		stats.Changed++
		if onChange != nil {
			onChange(TableContextBackfillChange{RecordID: recordID, MetricID: p.metricID, Outcome: res.Outcome, Before: p.oldCtx, After: res.Context})
		}
		if dryRun {
			continue
		}
		if _, err := db.ExecContext(ctx,
			`UPDATE kb.metrics SET metric_context = $1, source_table_rows = $2::jsonb, search_document = $3 WHERE id = $4`,
			res.Context, string(newRows), replaceSearchDocumentContext(p.sDoc, p.oldCtx, res.Context), p.id); err != nil {
			return fmt.Errorf("update metric %s: %w", p.metricID, err)
		}
	}
	return nil
}

// replaceSearchDocumentContext swaps the old context text inside a stored
// search_document for the new one, keeping every other part (objects, categories,
// English fields) as it was. When the old text is not found the new context is
// appended.
func replaceSearchDocumentContext(doc, oldCtx, newCtx string) string {
	if oldCtx != "" && strings.Contains(doc, oldCtx) {
		return strings.Replace(doc, oldCtx, newCtx, 1)
	}
	if strings.TrimSpace(doc) == "" {
		return newCtx
	}
	return doc + " " + newCtx
}

func jsonEqual(a, b string) bool {
	var va, vb any
	if json.Unmarshal([]byte(a), &va) != nil || json.Unmarshal([]byte(b), &vb) != nil {
		return false
	}
	ja, _ := json.Marshal(va)
	jb, _ := json.Marshal(vb)
	return string(ja) == string(jb)
}
