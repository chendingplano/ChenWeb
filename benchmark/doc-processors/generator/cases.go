package generator

// Case is the structured source of truth for one generated benchmark case.
type Case struct {
	ID         string
	Lines      []string
	Processors []string
	Tags       []string
	Expected   map[string]any
}

// Cases returns the deterministic synthetic-v1 corpus definition.
func Cases() []Case {
	chunk := func(lines []int) map[string]any {
		return map[string]any{"sequence": 1, "overlap_lines": []int{}, "normal_lines": lines}
	}
	return []Case{
		{ID: "toc-boundary-001", Lines: []string{"Contents", "1. Overview", "2. Findings", "Overview", "Findings"}, Processors: []string{"chunking"}, Tags: []string{"toc", "boundary"}, Expected: map[string]any{"schema_version": 1, "chunking": map[string]any{"protected_groups": []any{}, "chunks": []any{chunk([]int{1, 2, 3, 4, 5})}}}},
		{ID: "long-list-overlap-001", Lines: []string{"Items", "Alpha", "Beta", "Gamma", "Delta", "Epsilon"}, Processors: []string{"chunking"}, Tags: []string{"long-list", "overlap", "final-small-chunk"}, Expected: map[string]any{"schema_version": 1, "chunking": map[string]any{"protected_groups": []any{map[string]any{"group_id": "list-1", "kind": "non_numeric_list", "split_policy": "never", "lines": []int{2, 3, 4, 5}}}, "chunks": []any{chunk([]int{1, 2, 3, 4, 5, 6})}}}},
		{ID: "reordered-lines-001", Lines: []string{"Summary", "First point", "Second point", "Third point"}, Processors: []string{"chunking"}, Tags: []string{"reordered-lines"}, Expected: map[string]any{"schema_version": 1, "chunking": map[string]any{"protected_groups": []any{}, "chunks": []any{chunk([]int{1, 2, 3, 4})}}}},
		{ID: "metric-multiple-units-001", Lines: []string{"Revenue was 12.5 million USD in 2024.", "Margin reached 35%."}, Processors: []string{"extract_metrics"}, Tags: []string{"multiple-metrics", "multiple-units", "negative-metric"}, Expected: map[string]any{"schema_version": 1, "extract_metrics": map[string]any{"metrics": []any{
			map[string]any{"gold_id": "m1", "metric_name": "Revenue", "metric_value": "12.5", "metric_unit": "million USD", "is_explicit_metric": true, "source_lines": []int{1}},
			map[string]any{"gold_id": "m2", "metric_name": "Margin", "metric_value": "35", "metric_unit": "%", "is_explicit_metric": true, "source_lines": []int{2}},
		}}}},
		{ID: "metric-implicit-multilingual-001", Lines: []string{"增长率约为 8 percent.", "La satisfacción fue 0.80."}, Processors: []string{"extract_metrics"}, Tags: []string{"implicit-metric", "multilingual", "multiple-units"}, Expected: map[string]any{"schema_version": 1, "extract_metrics": map[string]any{"metrics": []any{
			map[string]any{"gold_id": "m1", "metric_name": "增长率", "metric_value": "8", "metric_unit": "percent", "is_explicit_metric": false, "source_lines": []int{1}},
			map[string]any{"gold_id": "m2", "metric_name": "Satisfaction", "metric_value": "0.80", "is_explicit_metric": false, "source_lines": []int{2}},
		}}}},
		{ID: "metric-no-metric-001", Lines: []string{"This paragraph contains no quantitative measurement.", "Qualitative evidence only."}, Processors: []string{"extract_metrics"}, Tags: []string{"no-metric"}, Expected: map[string]any{"schema_version": 1, "extract_metrics": map[string]any{"metrics": []any{}}}},
		{ID: "pipeline-unicode-001", Lines: []string{"Café output: 3.50 kg", "温度达到 20 °C"}, Processors: []string{"chunking", "extract_metrics"}, Tags: []string{"multilingual", "multiple-units", "boundary"}, Expected: map[string]any{"schema_version": 1, "chunking": map[string]any{"protected_groups": []any{}, "chunks": []any{chunk([]int{1, 2})}}, "extract_metrics": map[string]any{"metrics": []any{
			map[string]any{"gold_id": "m1", "metric_name": "output", "metric_value": "3.50", "metric_unit": "kg", "source_lines": []int{1}},
			map[string]any{"gold_id": "m2", "metric_name": "温度", "metric_value": "20", "metric_unit": "°C", "source_lines": []int{2}},
		}}}},
	}
}
