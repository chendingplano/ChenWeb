package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Metric frame backfill (openspec change metric-frame, design D5): moves the legacy apparatus,
// medium, method and stage qualifiers of stored rows out of condition into frame with
// splitLegacyCondition, with no LLM calls.

// MetricFrameBackfillStats summarizes one backfill run over one table.
type MetricFrameBackfillStats struct {
	Table           string
	Rows            int            // rows with a non-empty condition scanned
	Changed         int            // rows whose condition / frame changed (or would change)
	Moved           map[string]int // label → qualifiers moved
	UnresolvedSpecs int            // 药物浓度 qualifiers kept in condition: no single agent
}

// MetricFrameBackup is the before-image of one changed row, for --revert.
type MetricFrameBackup struct {
	Table       string          `json:"table"`
	ID          int64           `json:"id"`
	Condition   string          `json:"condition"`
	ConditionEn string          `json:"condition_en"`
	Frame       json.RawMessage `json:"frame,omitempty"`
}

// MetricFrameBackfillChange describes one changed row.
type MetricFrameBackfillChange struct {
	Table, MetricID          string
	Before, After, FrameJSON string
}

// metricFrameBackfillQueries are the readers per table. kb.metrics skips resolved reference rows.
var metricFrameBackfillQueries = map[string]string{
	"kb.metrics": `SELECT id, COALESCE(metric_id, ''), condition, COALESCE(condition_en, ''), frame FROM kb.metrics
WHERE ref_mention_id IS NULL AND COALESCE(condition, '') <> '' AND ($1 = 0 OR input_record_id = $1) ORDER BY id`,
	"testbed.metrics": `SELECT id, COALESCE(metric_id, ''), condition, COALESCE(condition_en, ''), frame FROM testbed.metrics
WHERE COALESCE(condition, '') <> '' AND ($1 = 0 OR input_record_id = $1) ORDER BY id`,
}

// RunMetricFrameBackfill splits the legacy qualifiers of every row of table ("kb.metrics" or
// "testbed.metrics"); recordID 0 means every record. With dryRun nothing is written. It returns the
// before-image of every changed row; onChange (optional) sees each change.
func RunMetricFrameBackfill(ctx context.Context, db *sql.DB, table string, recordID int64, dryRun bool,
	onChange func(MetricFrameBackfillChange)) (MetricFrameBackfillStats, []MetricFrameBackup, error) {
	stats := MetricFrameBackfillStats{Table: table, Moved: map[string]int{}}
	q, ok := metricFrameBackfillQueries[table]
	if !ok {
		return stats, nil, fmt.Errorf("(MID_26101802) unknown table %q", table)
	}
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return stats, nil, fmt.Errorf("(MID_26101803) read %s: %w", table, err)
	}
	type update struct {
		id           int64
		cond, condEn string
		frame        any
		before       MetricFrameBackup
		metricID     string
	}
	var updates []update
	for rows.Next() {
		var (
			id                     int64
			metricID, cond, condEn string
			frameJSON              sql.NullString
		)
		if err := rows.Scan(&id, &metricID, &cond, &condEn, &frameJSON); err != nil {
			rows.Close()
			return stats, nil, err
		}
		stats.Rows++
		m := map[string]any{"condition": cond, "condition_en": condEn}
		if f, _ := metricFrameFromValue(frameJSON.String); f != nil {
			m["frame"] = f
		}
		res := splitLegacyCondition(m)
		stats.UnresolvedSpecs += res.UnresolvedSpecs
		if len(res.Moved) == 0 {
			continue
		}
		for label, n := range res.Moved {
			stats.Moved[label] += n
		}
		backup := MetricFrameBackup{Table: table, ID: id, Condition: cond, ConditionEn: condEn}
		if frameJSON.Valid {
			backup.Frame = json.RawMessage(frameJSON.String)
		}
		updates = append(updates, update{id: id, cond: asString(m["condition"]), condEn: asString(m["condition_en"]),
			frame: metricFrameSQLValue(m), before: backup, metricID: metricID})
	}
	if err := rows.Close(); err != nil {
		return stats, nil, err
	}
	if err := rows.Err(); err != nil {
		return stats, nil, err
	}

	backups := make([]MetricFrameBackup, 0, len(updates))
	for _, u := range updates {
		stats.Changed++
		backups = append(backups, u.before)
		if onChange != nil {
			onChange(MetricFrameBackfillChange{Table: table, MetricID: u.metricID, Before: u.before.Condition,
				After: u.cond, FrameJSON: asString(u.frame)})
		}
		if dryRun {
			continue
		}
		if _, err := db.ExecContext(ctx, `UPDATE `+table+` SET condition = $2, condition_en = NULLIF($3, ''), frame = $4::jsonb WHERE id = $1`,
			u.id, u.cond, u.condEn, u.frame); err != nil {
			return stats, backups, fmt.Errorf("(MID_26101804) update %s id=%d: %w", table, u.id, err)
		}
	}
	return stats, backups, nil
}

// RevertMetricFrameBackfill restores the before-images a backfill returned. It returns the number
// of rows restored.
func RevertMetricFrameBackfill(ctx context.Context, db *sql.DB, backups []MetricFrameBackup) (int, error) {
	n := 0
	for _, b := range backups {
		if _, ok := metricFrameBackfillQueries[b.Table]; !ok {
			return n, fmt.Errorf("(MID_26101805) unknown table %q in backup", b.Table)
		}
		var frame any
		if len(b.Frame) > 0 {
			frame = string(b.Frame)
		}
		if _, err := db.ExecContext(ctx, `UPDATE `+b.Table+` SET condition = $2, condition_en = NULLIF($3, ''), frame = $4::jsonb WHERE id = $1`,
			b.ID, b.Condition, b.ConditionEn, frame); err != nil {
			return n, fmt.Errorf("(MID_26101806) restore %s id=%d: %w", b.Table, b.ID, err)
		}
		n++
	}
	return n, nil
}
