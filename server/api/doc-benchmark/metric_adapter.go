package docbenchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
)

const MetricRowsQuery = `SELECT to_jsonb(m) AS row
FROM kb.metrics AS m
WHERE m.input_record_id = $1
ORDER BY m.metric_id COLLATE "C" ASC NULLS LAST, m.id ASC`

var metricCoreFields = []string{"metric_id", "metric_name", "metric_subject", "metric_value", "metric_unit", "is_explicit_metric", "source_line_spans"}

type MetricCapture struct {
	Rows     []json.RawMessage
	File     []byte
	FileName string
	Diff     map[string]any
}
type MetricActual struct{ Rows []map[string]any }
type MetricAdapter struct {
	DB           *sql.DB
	ArtifactPath func(int64) string
}

func (a MetricAdapter) Processor() Processor { return ProcessorExtractMetrics }
func (a MetricAdapter) AllowedOverrides() map[string]any {
	return map[string]any{"extract_metrics": map[string]any{"prompt": true, "model": true}}
}
func (a MetricAdapter) Applicable(e ExpectedOutput) bool { return e.ExtractMetrics != nil }
func (a MetricAdapter) Capture(ctx context.Context, id int64) (any, error) {
	if a.DB == nil {
		return nil, fmt.Errorf("nil database")
	}
	rs, err := a.DB.QueryContext(ctx, MetricRowsQuery, id)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	c := MetricCapture{}
	for rs.Next() {
		var b []byte
		if err := rs.Scan(&b); err != nil {
			return nil, err
		}
		var x any
		if err := json.Unmarshal(b, &x); err != nil {
			return nil, err
		}
		cb, _ := canonicalValue(x)
		c.Rows = append(c.Rows, cb)
	}
	if err := rs.Err(); err != nil {
		return nil, err
	}
	if a.ArtifactPath == nil {
		return nil, fmt.Errorf("%w: missing metrics artifact", ErrInvalidOutput)
	}
	p := a.ArtifactPath(id)
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidOutput, err)
	}
	c.File = b
	c.FileName = p
	return c, nil
}
func (a MetricAdapter) Reconcile(v any) (any, error) {
	c, ok := v.(MetricCapture)
	if !ok {
		return nil, fmt.Errorf("%w: invalid capture", ErrInvalidOutput)
	}
	var file []map[string]any
	if err := json.Unmarshal(c.File, &file); err != nil {
		return nil, fmt.Errorf("%w: metrics file: %v", ErrInvalidOutput, err)
	}
	if len(file) != len(c.Rows) {
		return nil, fmt.Errorf("%w: row count mismatch", ErrInvalidOutput)
	}
	out := MetricActual{Rows: make([]map[string]any, len(c.Rows))}
	for i, b := range c.Rows {
		var dbrow, maprow map[string]any
		if err := json.Unmarshal(b, &dbrow); err != nil {
			return nil, err
		}
		maprow = file[i]
		for _, k := range metricCoreFields {
			d, _ := canonicalValue(dbrow[k])
			f, _ := canonicalValue(maprow[k])
			if string(d) != string(f) {
				c.Diff = map[string]any{"row": i, "field": k, "db": dbrow[k], "file": maprow[k]}
				return c, fmt.Errorf("%w: core field %s disagrees", ErrInvalidOutput, k)
			}
		}
		out.Rows[i] = dbrow
	}
	return out, nil
}
func (a MetricAdapter) Cleanup(ctx context.Context, id int64) error {
	if a.DB == nil {
		return fmt.Errorf("nil database")
	}
	_, err := a.DB.ExecContext(ctx, "DELETE FROM kb.metrics WHERE input_record_id = $1", id)
	return err
}
