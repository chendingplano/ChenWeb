package main

import (
	"fmt"
	"strings"
	"time"
)

// RenderMarkdown renders the baseline as the committed Phase 0 report. The
// layout follows the ADR's Phase 0 item order so a reviewer can check the gate
// item by item.
func RenderMarkdown(b Baseline, now time.Time) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Lossless Semantic Processing — Phase 0 Corpus Baseline\n\n")
	fmt.Fprintf(&sb, "Generated %s by `semantic-baseline` (ADR 2026081801 Phase 0 items 3–4).\n\n", now.Format("2006-01-02 15:04:05 MST"))

	c := b.Corpus
	fmt.Fprintf(&sb, "## 1. Corpus summary\n\n| Measure | Value |\n|---|---:|\n")
	fmt.Fprintf(&sb, "| Input records | %d |\n", c.InputRecords)
	fmt.Fprintf(&sb, "| Records with metrics | %d |\n", c.RecordsWithMetrics)
	fmt.Fprintf(&sb, "| Metric occurrences | %d |\n", c.MetricOccurrences)
	fmt.Fprintf(&sb, "| Metrics with a `value_range_type` | %d |\n", c.MetricsWithRangeType)
	fmt.Fprintf(&sb, "| Metrics without a `value_range_type` | %d |\n", c.MetricsWithoutRangeType)
	fmt.Fprintf(&sb, "| Metrics carrying `value_range_type_error` | %d |\n", c.MetricsWithRangeError)
	fmt.Fprintf(&sb, "| Metric decision candidates | %d |\n", c.DecisionCandidates)
	fmt.Fprintf(&sb, "| Semantic assertions | %d |\n", c.Assertions)
	fmt.Fprintf(&sb, "| Active evidence links (all families) | %d |\n", c.ActiveEvidenceLinks)
	fmt.Fprintf(&sb, "| Active metric supporting links | %d |\n", c.MetricSupportLinks)
	fmt.Fprintf(&sb, "| Metric occurrences with duplicate current support | %d |\n", c.DuplicateMetricSupport)
	fmt.Fprintf(&sb, "| **Metric occurrences with no current supporting link** | **%d** |\n", c.UnreachableMetrics)
	if c.MetricOccurrences > 0 {
		fmt.Fprintf(&sb, "\nLosslessness gap: %.2f%% of metric occurrences are semantically unreachable today.\n",
			100*float64(c.UnreachableMetrics)/float64(c.MetricOccurrences))
	}

	fmt.Fprintf(&sb, "\n## 2. Range-type mapping states\n\n| Mapping state | Metric occurrences | Distinct raw values |\n|---|---:|---:|\n")
	for _, m := range b.MappingStates {
		fmt.Fprintf(&sb, "| `%s` | %d | %d |\n", m.MappingState, m.Metrics, m.DistinctRaw)
	}

	fmt.Fprintf(&sb, "\n## 3. Deferral reasons (current decision candidates)\n\n| Reason | Count |\n|---|---:|\n")
	if len(b.DeferralReasons) == 0 {
		fmt.Fprintf(&sb, "| _(none)_ | 0 |\n")
	}
	for _, r := range b.DeferralReasons {
		fmt.Fprintf(&sb, "| `%s` | %d |\n", r.Key, r.Count)
	}

	fmt.Fprintf(&sb, "\n## 4. Assertion lifecycle states\n\n| Status | Count |\n|---|---:|\n")
	for _, r := range b.AssertionStates {
		fmt.Fprintf(&sb, "| `%s` | %d |\n", r.Key, r.Count)
	}

	fmt.Fprintf(&sb, "\n## 5. Required stage coverage\n\nMetric adapter required stages: ")
	for i, s := range b.StageCoverage.RequiredStages {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "`%s`", s)
	}
	fmt.Fprintf(&sb, "\n\nOne outcome envelope per occurrence per required stage: %d × %d = **%d envelopes**.\n",
		b.StageCoverage.MetricOccurrences, len(b.StageCoverage.RequiredStages), b.StageCoverage.OutcomeEnvelopes)

	m := b.Capacity
	fmt.Fprintf(&sb, "\n## 6. Capacity model\n\n| Projected row set | Rows |\n|---|---:|\n")
	fmt.Fprintf(&sb, "| Evidence links | %d |\n", m.EvidenceLinks)
	fmt.Fprintf(&sb, "| Class-resolution decisions | %d |\n", m.ClassDecisions)
	fmt.Fprintf(&sb, "| Outcome envelopes | %d |\n", m.OutcomeEnvelopes)
	fmt.Fprintf(&sb, "| Findings (low estimate) | %d |\n", m.FindingsLowEstimate)
	fmt.Fprintf(&sb, "| Findings (high estimate) | %d |\n", m.FindingsHighEstimate)
	fmt.Fprintf(&sb, "| Assertions before canonical convergence | %d |\n", m.AssertionsUpperBound)
	fmt.Fprintf(&sb, "| **Estimated bytes (worst case)** | **%s** |\n", humanBytes(m.EstimatedBytes))

	fmt.Fprintf(&sb, "\n## 7. Per-record detail\n\n| Record | File | Metrics | Candidates | Supported | Unreachable | Review-visible |\n|---:|---|---:|---:|---:|---:|---:|\n")
	for _, r := range b.PerRecord {
		fmt.Fprintf(&sb, "| %d | %s | %d | %d | %d | %d | %d |\n",
			r.InputRecordID, shortFile(r.Filename), r.Metrics, r.DecisionCandidates,
			r.SupportedMetrics, r.UnreachableMetrics, r.ReviewVisible)
	}
	return sb.String()
}

func shortFile(name string) string {
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if len(name) > 48 {
		return name[:45] + "..."
	}
	return name
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
