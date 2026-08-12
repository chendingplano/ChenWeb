package docprocessing

import "testing"

func TestMetricsIdentical_SameFieldsMatch(t *testing.T) {
	a := map[string]any{
		"metric_name": "Latency", "metric_subject": "gateway", "metric_unit": "ms",
		"metric_value": "200", "source_line_spans": []any{float64(2)},
	}
	b := map[string]any{
		"metric_name": "Latency", "metric_subject": "gateway", "metric_unit": "ms",
		"metric_value": "200", "source_line_spans": []any{float64(2)},
	}
	if !metricsIdentical(a, b) {
		t.Fatalf("expected identical")
	}
}

func TestMetricsIdentical_DifferentValueNotMatch(t *testing.T) {
	a := map[string]any{
		"metric_name": "Latency", "metric_subject": "gateway", "metric_unit": "ms",
		"metric_value": "200", "source_line_spans": []any{float64(2)},
	}
	b := map[string]any{
		"metric_name": "Latency", "metric_subject": "gateway", "metric_unit": "ms",
		"metric_value": "300", "source_line_spans": []any{float64(2)},
	}
	if metricsIdentical(a, b) {
		t.Fatalf("expected not identical")
	}
}

func TestMetricLineSpansOverlap_True(t *testing.T) {
	a := map[string]any{"source_line_spans": []any{float64(2), float64(3)}}
	b := map[string]any{"source_line_spans": []any{"3:4"}}
	if !metricLineSpansOverlap(a, b) {
		t.Fatalf("expected overlap (both cover line 3)")
	}
}

func TestMetricLineSpansOverlap_False(t *testing.T) {
	a := map[string]any{"source_line_spans": []any{float64(2)}}
	b := map[string]any{"source_line_spans": []any{float64(9)}}
	if metricLineSpansOverlap(a, b) {
		t.Fatalf("expected no overlap")
	}
}

// TestComputeMetricGroups_TransitiveChain regression-tests the single-hop bug
// the ADR calls out: A overlaps B, B overlaps C, but A and C do not directly
// overlap. All three must land in one group.
func TestComputeMetricGroups_TransitiveChain(t *testing.T) {
	metrics := []map[string]any{
		{"source_line_spans": []any{float64(1), float64(2)}}, // A: lines 1-2
		{"source_line_spans": []any{float64(2), float64(3)}}, // B: lines 2-3 (overlaps A)
		{"source_line_spans": []any{float64(3), float64(4)}}, // C: lines 3-4 (overlaps B, not A)
		{"source_line_spans": []any{float64(100)}},           // D: unrelated
	}
	groups := computeMetricGroups(metrics)
	if len(groups) != 2 {
		t.Fatalf("groups=%d, want 2 (one of size 3, one of size 1); got %+v", len(groups), groups)
	}
	var sizes []int
	for _, g := range groups {
		sizes = append(sizes, len(g))
	}
	found3, found1 := false, false
	for _, s := range sizes {
		if s == 3 {
			found3 = true
		}
		if s == 1 {
			found1 = true
		}
	}
	if !found3 || !found1 {
		t.Fatalf("expected group sizes [3,1], got %v", sizes)
	}
}

func TestMetricSeqnoCounter_StartsAfterMax(t *testing.T) {
	existing := []map[string]any{
		{"metric_id": "173_mtc_2"},
		{"metric_id": "173_mtc_5"},
		{"metric_id": "173_mtc_1"},
	}
	c := newMetricSeqnoCounter(existing)
	if got := c.Assign(173); got != "173_mtc_6" {
		t.Fatalf("first assign = %q, want 173_mtc_6", got)
	}
	if got := c.Assign(173); got != "173_mtc_7" {
		t.Fatalf("second assign = %q, want 173_mtc_7", got)
	}
}

func TestMetricSeqnoCounter_EmptyExistingStartsAtOne(t *testing.T) {
	c := newMetricSeqnoCounter(nil)
	if got := c.Assign(9); got != "9_mtc_1" {
		t.Fatalf("assign = %q, want 9_mtc_1", got)
	}
}

func TestMergeMetrics_Rule2_ExactDuplicateDiscarded(t *testing.T) {
	existing := []map[string]any{
		{"metric_id": "173_mtc_1", "metric_name": "Latency", "metric_subject": "gw",
			"metric_unit": "ms", "metric_value": "200", "source_line_spans": []any{float64(2)}},
	}
	newCandidates := []map[string]any{
		{"metric_name": "Latency", "metric_subject": "gw",
			"metric_unit": "ms", "metric_value": "200", "source_line_spans": []any{float64(2)}},
	}
	res := mergeMetrics(existing, newCandidates, newMetricSeqnoCounter(existing), 173)
	if len(res.Added) != 0 {
		t.Fatalf("Added=%d, want 0 (exact duplicate should be discarded)", len(res.Added))
	}
	if len(res.PendingGroups) != 0 {
		t.Fatalf("PendingGroups=%d, want 0", len(res.PendingGroups))
	}
}

func TestMergeMetrics_Rule3_NoOverlapAdded(t *testing.T) {
	existing := []map[string]any{
		{"metric_id": "173_mtc_1", "metric_name": "Latency", "source_line_spans": []any{float64(2)}},
	}
	newCandidates := []map[string]any{
		{"metric_name": "Throughput", "source_line_spans": []any{float64(50)}},
	}
	res := mergeMetrics(existing, newCandidates, newMetricSeqnoCounter(existing), 173)
	if len(res.Added) != 1 {
		t.Fatalf("Added=%d, want 1", len(res.Added))
	}
	if res.Added[0]["metric_id"] != "173_mtc_2" {
		t.Fatalf("new metric_id=%v, want 173_mtc_2", res.Added[0]["metric_id"])
	}
	if len(res.PendingGroups) != 0 {
		t.Fatalf("PendingGroups=%d, want 0", len(res.PendingGroups))
	}
}

func TestMergeMetrics_Rule4_OverlapNotIdenticalGoesToPending(t *testing.T) {
	existing := []map[string]any{
		{"metric_id": "173_mtc_1", "metric_name": "Latency", "metric_subject": "gw",
			"metric_unit": "ms", "metric_value": "200", "source_line_spans": []any{float64(2)}},
		{"metric_id": "173_mtc_2", "metric_name": "Latency", "metric_subject": "gw",
			"metric_unit": "ms", "metric_value": "300", "source_line_spans": []any{float64(2)}},
	}
	newCandidates := []map[string]any{
		// Same line, different value -> not Rule-1 identical, but overlaps -> Rule-4.
		{"metric_name": "Latency", "metric_subject": "gw",
			"metric_unit": "ms", "metric_value": "250", "source_line_spans": []any{float64(2)}},
	}
	res := mergeMetrics(existing, newCandidates, newMetricSeqnoCounter(existing), 173)
	if len(res.Added) != 0 {
		t.Fatalf("Added=%d, want 0", len(res.Added))
	}
	if len(res.PendingGroups) != 1 || len(res.PendingGroups[0]) != 3 {
		t.Fatalf("PendingGroups=%+v, want 1 group of 3", res.PendingGroups)
	}
	var sawExisting, sawNew bool
	for _, m := range res.PendingGroups[0] {
		switch m["_merge_source"] {
		case "existing":
			sawExisting = true
		case "new":
			sawNew = true
			if m["metric_id"] == nil || m["metric_id"] == "" {
				t.Fatalf("new candidate must have a pre-assigned metric_id (DR4)")
			}
		}
	}
	if !sawExisting || !sawNew {
		t.Fatalf("expected existing and new tagged candidates in pending group")
	}
}

func TestMergeMetrics_StaticOverlapMatchAvoidsPendingGroup(t *testing.T) {
	existing := []map[string]any{
		{"metric_id": "173_mtc_1", "metric_name": "Latency", "metric_subject": "gw",
			"metric_unit": "ms", "metric_value": "200", "value_data_type": "number",
			"source_line_spans": []any{float64(2)}},
	}
	newCandidates := []map[string]any{
		{"metric_name": "Latency", "metric_subject": "gw", "metric_unit": "ms",
			"metric_value": "250", "value_data_type": "number",
			"source_line_spans": []any{float64(2)}},
	}
	res := mergeMetrics(existing, newCandidates, newMetricSeqnoCounter(existing), 173)
	if len(res.PendingGroups) != 0 {
		t.Fatalf("PendingGroups=%d, want 0 for unique static match", len(res.PendingGroups))
	}
	if len(res.StaticMerges) != 1 || res.StaticMerges[0]["metric_id"] != "173_mtc_1" {
		t.Fatalf("StaticMerges=%+v, want existing metric id", res.StaticMerges)
	}
}

func TestMergeMetrics_StaticOverlapFuzzyChineseIdentityAvoidsPendingGroup(t *testing.T) {
	existing := []map[string]any{
		{"metric_id": "416_mtc_1", "metric_name": "农村生活垃圾分类类别数量",
			"metric_subject": "农村生活垃圾分类体系", "metric_unit": "类", "metric_value": "4",
			"source_line_spans": []any{float64(50)}},
	}
	newCandidates := []map[string]any{
		{"metric_name": "农村生活垃圾分类类别数", "metric_subject": "农村生活垃圾",
			"metric_unit": "类", "metric_value": "4", "source_line_spans": []any{float64(50)}},
	}
	res := mergeMetrics(existing, newCandidates, newMetricSeqnoCounter(existing), 416)
	if len(res.StaticMerges) != 1 || len(res.PendingGroups) != 0 {
		t.Fatalf("StaticMerges=%d PendingGroups=%d, want 1 and 0", len(res.StaticMerges), len(res.PendingGroups))
	}
}

func TestMergeMetrics_IdenticalKeywordConceptIsAuthoritative(t *testing.T) {
	existing := []map[string]any{
		{"metric_id": "416_mtc_1", "metric_name": "旧名称", "metric_subject": "旧主体",
			"metric_unit": "类", "metric_value": "4", "keyword_concept_id": "kwc:shared",
			"source_line_spans": []any{float64(50)}},
	}
	newCandidates := []map[string]any{
		{"metric_name": "完全不同的名称", "metric_subject": "完全不同的主体",
			"metric_unit": "个", "metric_value": "99", "keyword_concept_id": "kwc:shared",
			"source_line_spans": []any{float64(50)}},
	}
	res := mergeMetrics(existing, newCandidates, newMetricSeqnoCounter(existing), 416)
	if len(res.Decisions) != 1 || res.Decisions[0].Decision != "duplicate_discarded" {
		t.Fatalf("Decisions=%#v, want one duplicate_discarded decision", res.Decisions)
	}
	if len(res.StaticMerges) != 0 || len(res.PendingGroups) != 0 {
		t.Fatalf("StaticMerges=%d PendingGroups=%d, want 0 and 0", len(res.StaticMerges), len(res.PendingGroups))
	}
}

func TestMergeMetrics_StaticOverlapMultipleMatchesRemainPending(t *testing.T) {
	existing := []map[string]any{
		{"metric_id": "173_mtc_1", "metric_name": "Latency", "metric_subject": "gw",
			"metric_unit": "ms", "metric_value": "200", "source_line_spans": []any{float64(2)}},
		{"metric_id": "173_mtc_2", "metric_name": "Latency", "metric_subject": "gw",
			"metric_unit": "ms", "metric_value": "300", "source_line_spans": []any{float64(2)}},
	}
	newCandidates := []map[string]any{
		{"metric_name": "Latency", "metric_subject": "gw", "metric_unit": "ms",
			"metric_value": "250", "source_line_spans": []any{float64(2)}},
	}
	res := mergeMetrics(existing, newCandidates, newMetricSeqnoCounter(existing), 173)
	if len(res.StaticMerges) != 0 {
		t.Fatalf("StaticMerges=%d, want 0 for ambiguous matches", len(res.StaticMerges))
	}
	if len(res.PendingGroups) != 1 || len(res.PendingGroups[0]) != 3 {
		t.Fatalf("PendingGroups=%+v, want one unresolved group of 3", res.PendingGroups)
	}
}

func TestMergeMetrics_FarSameSpanMetricIsExcludedFromPendingGroup(t *testing.T) {
	existing := []map[string]any{
		{"metric_id": "416_mtc_1", "metric_name": "最大速度", "metric_subject": "车辆",
			"metric_unit": "km/h", "metric_value": "80", "source_line_spans": []any{float64(50)}},
		{"metric_id": "416_mtc_2", "metric_name": "土壤pH值", "metric_subject": "土壤酸碱度",
			"metric_unit": "pH", "metric_value": "7", "source_line_spans": []any{float64(50)}},
	}
	newCandidates := []map[string]any{
		{"metric_name": "车辆运行速度", "metric_subject": "车辆",
			"metric_unit": "km/h", "metric_value": "85", "source_line_spans": []any{float64(50)}},
	}

	res := mergeMetrics(existing, newCandidates, newMetricSeqnoCounter(existing), 416)
	if len(res.PendingGroups) != 1 {
		t.Fatalf("PendingGroups=%d, want 1", len(res.PendingGroups))
	}
	if len(res.PendingGroups[0]) != 2 {
		t.Fatalf("pending group=%+v, want candidate plus one close existing metric", res.PendingGroups[0])
	}
	for _, metric := range res.PendingGroups[0] {
		if asString(metric["metric_id"]) == "416_mtc_2" {
			t.Fatalf("far pH metric was retained in pending group: %+v", metric)
		}
	}
}

func TestMergeMetrics_FarOnlySameSpanMetricIsIgnoredAndCandidateAdded(t *testing.T) {
	existing := []map[string]any{
		{"metric_id": "416_mtc_2", "metric_name": "土壤pH值", "metric_subject": "土壤酸碱度",
			"metric_unit": "pH", "metric_value": "7", "source_line_spans": []any{float64(50)}},
	}
	newCandidates := []map[string]any{
		{"metric_name": "车辆运行速度", "metric_subject": "车辆",
			"metric_unit": "km/h", "metric_value": "85", "source_line_spans": []any{float64(50)}},
	}

	res := mergeMetrics(existing, newCandidates, newMetricSeqnoCounter(existing), 416)
	if len(res.Added) != 1 || res.Added[0]["metric_id"] != "416_mtc_3" {
		t.Fatalf("Added=%+v, want the candidate as a new metric", res.Added)
	}
	if len(res.PendingGroups) != 0 {
		t.Fatalf("PendingGroups=%d, want 0", len(res.PendingGroups))
	}
}

func TestMergeMetrics_StaticOverlapDistinctIdentitiesResolveIndependently(t *testing.T) {
	existing := []map[string]any{
		{"metric_id": "173_mtc_1", "metric_name": "Temperature", "metric_subject": "inlet",
			"metric_unit": "C", "metric_value": "20", "source_line_spans": []any{float64(2)}},
		{"metric_id": "173_mtc_2", "metric_name": "Temperature", "metric_subject": "outlet",
			"metric_unit": "C", "metric_value": "25", "source_line_spans": []any{float64(2)}},
	}
	newCandidates := []map[string]any{
		{"metric_name": "Temperature", "metric_subject": "inlet", "metric_unit": "C",
			"metric_value": "21", "source_line_spans": []any{float64(2)}},
		{"metric_name": "Temperature", "metric_subject": "outlet", "metric_unit": "C",
			"metric_value": "26", "source_line_spans": []any{float64(2)}},
	}
	res := mergeMetrics(existing, newCandidates, newMetricSeqnoCounter(existing), 173)
	if len(res.StaticMerges) != 2 || len(res.PendingGroups) != 0 {
		t.Fatalf("StaticMerges=%d PendingGroups=%d, want 2 and 0", len(res.StaticMerges), len(res.PendingGroups))
	}
}
