package docprocessing

import "strings"

// metricsIdentical implements ADR 2026071002 DR2 Rule-1: two metrics are the
// same if these five fields match exactly (mirrors normalizedMetricCandidateKey's
// field set in dedupeFinalMetricRows, extract-metrics.go).
func metricsIdentical(a, b map[string]any) bool {
	fields := []string{"metric_name", "metric_subject", "metric_unit", "metric_value"}
	for _, f := range fields {
		if strings.TrimSpace(asString(a[f])) != strings.TrimSpace(asString(b[f])) {
			return false
		}
	}
	spansA := strings.Join(normalizeSourceLineSpans(a["source_line_spans"]), ",")
	spansB := strings.Join(normalizeSourceLineSpans(b["source_line_spans"]), ",")
	return spansA == spansB
}

// metricLineSpansOverlap reports whether two metrics share at least one source line.
func metricLineSpansOverlap(a, b map[string]any) bool {
	spansA := parseMetricSpanRanges(a["source_line_spans"])
	spansB := parseMetricSpanRanges(b["source_line_spans"])
	for _, ra := range spansA {
		for _, rb := range spansB {
			if ra.start <= rb.end && rb.start <= ra.end {
				return true
			}
		}
	}
	return false
}

type metricSpanRange struct{ start, end int }

// parseMetricSpanRanges reuses normalizeSourceLineSpans's canonical string
// output ("N" or "N:M") and reparses it into comparable [start,end] ranges.
func parseMetricSpanRanges(value any) []metricSpanRange {
	canonical := normalizeSourceLineSpans(value)
	out := make([]metricSpanRange, 0, len(canonical))
	for _, s := range canonical {
		start, end, ok := parseMetricLineSpan(s)
		if ok {
			out = append(out, metricSpanRange{start, end})
		}
	}
	return out
}

// computeMetricGroups partitions metrics into connected components (DR2
// "Metric Groups"): two metrics are in the same group if they directly share
// a line, or transitively via a chain of shared-line metrics. Returns groups
// as slices of indices into metrics. Union-find with path compression.
func computeMetricGroups(metrics []map[string]any) [][]int {
	n := len(metrics)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(x, y int) {
		rx, ry := find(x), find(y)
		if rx != ry {
			parent[rx] = ry
		}
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if metricLineSpansOverlap(metrics[i], metrics[j]) {
				union(i, j)
			}
		}
	}

	groupsByRoot := map[int][]int{}
	order := make([]int, 0, n)
	for i := 0; i < n; i++ {
		root := find(i)
		if _, ok := groupsByRoot[root]; !ok {
			order = append(order, root)
		}
		groupsByRoot[root] = append(groupsByRoot[root], i)
	}
	out := make([][]int, 0, len(order))
	for _, root := range order {
		out = append(out, groupsByRoot[root])
	}
	return out
}
