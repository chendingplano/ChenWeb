package docprocessing

import (
	"fmt"
	"strconv"
	"strings"
)

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

type metricSeqnoCounter struct {
	next int
}

// newMetricSeqnoCounter scans existing metric_ids of the form "<id>_mtc_<seqno>"
// and initializes the counter to one past the current max (DR3).
func newMetricSeqnoCounter(existing []map[string]any) *metricSeqnoCounter {
	max := 0
	for _, m := range existing {
		id := asString(m["metric_id"])
		parts := strings.Split(id, "_mtc_")
		if len(parts) != 2 {
			continue
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return &metricSeqnoCounter{next: max + 1}
}

// Assign returns the next metric_id for recordID and advances the counter.
func (c *metricSeqnoCounter) Assign(recordID int64) string {
	id := fmt.Sprintf("%d_mtc_%d", recordID, c.next)
	c.next++
	return id
}
