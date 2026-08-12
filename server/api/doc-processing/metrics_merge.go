package docprocessing

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// metricsIdentical implements ADR 2026071002 DR2 Rule-1. A resolved keyword
// concept is the authoritative metric identity: two non-empty equal concept
// IDs are the same metric even when extraction spelling, values, or spans
// differ. Unresolved metrics fall back to the legacy content-and-span key.
func metricsIdentical(a, b map[string]any) bool {
	conceptA := strings.TrimSpace(asString(a["keyword_concept_id"]))
	conceptB := strings.TrimSpace(asString(b["keyword_concept_id"]))
	if conceptA != "" && conceptA == conceptB {
		return true
	}

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

// metricStaticMatch reports whether candidate is a sufficiently specific
// deterministic match for existing. The metric name is required. At least
// one additional identifying field must be present on the candidate, and
// every populated identifying field must agree. metric_value is intentionally
// excluded: a changed value is a normal reason to reprocess an existing
// metric, not evidence that it is a different metric. If more than one
// existing row satisfies this predicate, the caller must retain the group for
// LLM adjudication.
func metricStaticMatch(candidate, existing map[string]any) bool {
	candidateConcept := strings.TrimSpace(asString(candidate["keyword_concept_id"]))
	if candidateConcept != "" && candidateConcept == strings.TrimSpace(asString(existing["keyword_concept_id"])) {
		return true
	}
	candidateName := strings.TrimSpace(asString(candidate["metric_name"]))
	existingName := strings.TrimSpace(asString(existing["metric_name"]))
	if candidateName == "" {
		return false
	}

	nameExact := candidateName == existingName
	if !nameExact && !metricFuzzyLexicalMatch(candidateName, existingName, 0.80) {
		return false
	}

	discriminators := []string{
		"metric_subject", "metric_unit", "value_data_type", "value_range_type",
		"value_class", "threshold_or_target",
	}
	hasDiscriminator := false
	for _, field := range discriminators {
		candidateValue := strings.TrimSpace(asString(candidate[field]))
		if candidateValue == "" {
			continue
		}
		hasDiscriminator = true
		existingValue := strings.TrimSpace(asString(existing[field]))
		if field == "metric_subject" && !nameExact {
			if !metricFuzzyLexicalMatch(candidateValue, existingValue, 0.55) {
				return false
			}
			continue
		}
		if candidateValue != existingValue {
			return false
		}
	}
	if !nameExact && strings.TrimSpace(asString(candidate["metric_unit"])) != strings.TrimSpace(asString(existing["metric_unit"])) {
		return false
	}
	if !nameExact && strings.TrimSpace(asString(candidate["metric_value"])) != strings.TrimSpace(asString(existing["metric_value"])) {
		return false
	}
	return hasDiscriminator
}

// metricCloseEnough is the second-stage gate between source-span overlap and
// static matching. It keeps semantically plausible alternatives for LLM
// adjudication, while preventing unrelated metrics that happen to be printed
// on the same source line from entering the same pending group.
func metricCloseEnough(candidate, existing map[string]any) bool {
	if candidateConcept := strings.TrimSpace(asString(candidate["keyword_concept_id"])); candidateConcept != "" && candidateConcept == strings.TrimSpace(asString(existing["keyword_concept_id"])) {
		return true
	}

	name := strings.TrimSpace(asString(candidate["metric_name"]))
	existingName := strings.TrimSpace(asString(existing["metric_name"]))
	if name == "" || existingName == "" {
		return false
	}
	if metricLexicalMatch(name, existingName, 0.45, 0.15) {
		return true
	}

	subject := strings.TrimSpace(asString(candidate["metric_subject"]))
	existingSubject := strings.TrimSpace(asString(existing["metric_subject"]))
	if subject == "" || existingSubject == "" || !metricLexicalMatch(subject, existingSubject, 0.45, 0.15) {
		return false
	}
	// A shared subject is useful evidence only when the measurement shape is
	// compatible. This avoids grouping unrelated quantities under a broad
	// subject such as "environment".
	for _, field := range []string{"value_data_type", "value_range_type"} {
		candidateValue := strings.TrimSpace(asString(candidate[field]))
		existingValue := strings.TrimSpace(asString(existing[field]))
		if candidateValue != "" && existingValue != "" && candidateValue != existingValue {
			return false
		}
	}
	return true
}

// metricFuzzyLexicalMatch mirrors the keyword module's tier-1/2 normalization
// and tier-5 trigram-blocked/edit-distance scoring for the already bounded
// in-memory merge candidates. It is deliberately stricter than a raw
// similarity check: short strings do not fuzzy-match, and both lexical
// signals must clear their floors.
func metricFuzzyLexicalMatch(candidate, existing string, minimum float64) bool {
	return metricLexicalMatch(candidate, existing, minimum, 0.30)
}

func metricLexicalMatch(candidate, existing string, minimum, trigramMinimum float64) bool {
	normalizer := semid.Normalizer{Version: semid.CurrentNormalizerVersion}
	a := normalizer.Normalize(candidate)
	b := normalizer.Normalize(existing)
	if a.Norm == "" || b.Norm == "" {
		return false
	}
	if a.Norm == b.Norm || a.Alnum == b.Alnum || a.Sorted == b.Sorted {
		return true
	}
	if utf8.RuneCountInString(a.Norm) <= 4 || utf8.RuneCountInString(b.Norm) <= 4 {
		return false
	}
	if firstRune(a.Norm) != firstRune(b.Norm) {
		return false
	}
	return metricTrigramSimilarity(a.Norm, b.Norm) >= trigramMinimum &&
		metricNormalizedSimilarity(a.Norm, b.Norm) >= minimum
}

func metricTrigramSimilarity(a, b string) float64 {
	aSet := metricTrigrams(a)
	bSet := metricTrigrams(b)
	if len(aSet) == 0 || len(bSet) == 0 {
		return 0
	}
	intersection := 0
	for trigram := range aSet {
		if bSet[trigram] {
			intersection++
		}
	}
	return float64(intersection) / float64(len(aSet)+len(bSet)-intersection)
}

func metricTrigrams(s string) map[string]bool {
	runes := []rune(s)
	trigrams := make(map[string]bool)
	for i := 0; i+3 <= len(runes); i++ {
		trigrams[string(runes[i:i+3])] = true
	}
	return trigrams
}

func metricNormalizedSimilarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min(min(prev[j]+1, curr[j-1]+1), prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	longer := len(ra)
	if len(rb) > longer {
		longer = len(rb)
	}
	if longer == 0 {
		return 1
	}
	return 1 - float64(prev[len(rb)])/float64(longer)
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
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

type mergeMetricsResult struct {
	Added         []map[string]any
	StaticMerges  []map[string]any
	PendingGroups [][]map[string]any
	Decisions     []mergeDecision
}

type mergeDecision struct {
	MetricName           string
	CandidateMetricID    string
	Decision             string
	Reason               string
	OverlappingMetricIDs []string
	StaticMatchIDs       []string
	CandidateFields      map[string]any
	Comparisons          []map[string]any
}

// metricMergeComparedFields contains every field used by the merge decision:
// identity fields, static discriminators, and source spans. Keep this explicit
// and stable so diagnostic logs can be compared directly with kb.metrics.
func metricMergeComparedFields(metric map[string]any) map[string]any {
	fields := []string{
		"metric_id", "keyword_concept_id", "metric_definition_term_id", "metric_name", "metric_subject", "metric_unit", "metric_value",
		"value_data_type", "value_range_type", "value_class", "threshold_or_target",
		"source_line_spans",
	}
	compared := make(map[string]any, len(fields))
	for _, field := range fields {
		if value, ok := metric[field]; ok {
			compared[field] = value
		} else {
			compared[field] = ""
		}
	}
	return compared
}

func metricStaticComparisons(candidate map[string]any, overlapping []map[string]any) []map[string]any {
	comparisons := make([]map[string]any, 0, len(overlapping))
	for _, existing := range overlapping {
		comparisons = append(comparisons, map[string]any{
			"existing_metric_id": asString(existing["metric_id"]),
			"static_match":       metricStaticMatch(candidate, existing),
			"existing_fields":    metricMergeComparedFields(existing),
		})
	}
	return comparisons
}

// mergeMetrics implements ADR 2026071002 DR2 Rule-2/3/4. existing is read
// from kb.metrics (unmodified maps); newCandidates are this run's enriched
// Pass-2 output (no metric_id yet). seqno must be initialized from existing
// (DR3) before calling.
func mergeMetrics(existing, newCandidates []map[string]any, seqno *metricSeqnoCounter, recordID int64) mergeMetricsResult {
	var result mergeMetricsResult
	remainingNew := make([]map[string]any, 0, len(newCandidates))

	// Rule-2: discard any new candidate that's an exact duplicate of an existing
	// metric, including candidates with the same resolved keyword concept ID.
	for _, cand := range newCandidates {
		duplicate := false
		duplicateID := ""
		for _, ex := range existing {
			if metricsIdentical(cand, ex) {
				duplicate = true
				duplicateID = asString(ex["metric_id"])
				break
			}
		}
		if !duplicate {
			remainingNew = append(remainingNew, cand)
		} else {
			result.Decisions = append(result.Decisions, mergeDecision{
				MetricName:      asString(cand["metric_name"]),
				Decision:        "duplicate_discarded",
				Reason:          "exact_match",
				StaticMatchIDs:  []string{duplicateID},
				CandidateFields: metricMergeComparedFields(cand),
			})
		}
	}

	// Rule-3: any remaining candidate with zero line overlap against existing
	// metrics is unambiguously new. Same-span existing metrics first pass the
	// close-enough gate; only those candidates can be statically matched or
	// sent onward as ambiguous alternatives.
	stillPending := make([]map[string]any, 0, len(remainingNew))
	closeExisting := make([]map[string]any, 0, len(existing))
	for _, cand := range remainingNew {
		overlapping := make([]map[string]any, 0)
		for _, ex := range existing {
			if metricLineSpansOverlap(cand, ex) {
				overlapping = append(overlapping, ex)
			}
		}
		close := make([]map[string]any, 0, len(overlapping))
		for _, ex := range overlapping {
			if metricCloseEnough(cand, ex) {
				close = append(close, ex)
				closeExisting = append(closeExisting, ex)
			}
		}
		if len(close) == 0 {
			added := cloneMetricMap(cand)
			added["metric_id"] = seqno.Assign(recordID)
			result.Added = append(result.Added, added)
			result.Decisions = append(result.Decisions, mergeDecision{
				MetricName:           asString(cand["metric_name"]),
				CandidateMetricID:    asString(added["metric_id"]),
				Decision:             "added",
				Reason:               "no_close_metric_match",
				OverlappingMetricIDs: metricIDs(overlapping),
				CandidateFields:      metricMergeComparedFields(cand),
			})
			continue
		}

		staticMatches := make([]map[string]any, 0, len(close))
		for _, ex := range close {
			if metricStaticMatch(cand, ex) {
				staticMatches = append(staticMatches, ex)
			}
		}
		if len(staticMatches) == 1 {
			merged := mergeStaticMetric(staticMatches[0], cand)
			result.StaticMerges = append(result.StaticMerges, merged)
			result.Decisions = append(result.Decisions, mergeDecision{
				MetricName:           asString(cand["metric_name"]),
				CandidateMetricID:    asString(merged["metric_id"]),
				Decision:             "static_merge",
				Reason:               "one_unique_static_match",
				OverlappingMetricIDs: metricIDs(close),
				StaticMatchIDs:       metricIDs(staticMatches),
				CandidateFields:      metricMergeComparedFields(cand),
				Comparisons:          metricStaticComparisons(cand, close),
			})
			continue
		}
		pendingID := seqno.Assign(recordID)
		cand["metric_id"] = pendingID
		stillPending = append(stillPending, cand)
		result.Decisions = append(result.Decisions, mergeDecision{
			MetricName:           asString(cand["metric_name"]),
			CandidateMetricID:    pendingID,
			Decision:             "llm_pending",
			Reason:               "no_unique_static_match",
			OverlappingMetricIDs: metricIDs(close),
			StaticMatchIDs:       metricIDs(staticMatches),
			CandidateFields:      metricMergeComparedFields(cand),
			Comparisons:          metricStaticComparisons(cand, close),
		})
	}

	// Rule-4: everything left overlaps at least one existing metric. Assign a
	// metric_id to each (DR4 — every pending-list entry needs one before the
	// Merge Resolution LLM call, per DR2's updated call contract), tag its
	// source, then group by Metric Groups transitive closure over the union
	// of existing + pending candidates.
	tagged := make([]map[string]any, 0, len(existing)+len(stillPending))
	closeExistingByID := make(map[string]map[string]any, len(closeExisting))
	for _, ex := range closeExisting {
		closeExistingByID[asString(ex["metric_id"])] = ex
	}
	for _, ex := range existing {
		if closeEx, ok := closeExistingByID[asString(ex["metric_id"])]; ok {
			e := cloneMetricMap(closeEx)
			e["_merge_source"] = "existing"
			tagged = append(tagged, e)
		}
	}
	for _, cand := range stillPending {
		c := cloneMetricMap(cand)
		c["_merge_source"] = "new"
		if asString(c["metric_id"]) == "" {
			c["metric_id"] = seqno.Assign(recordID)
		}
		tagged = append(tagged, c)
	}

	if len(stillPending) == 0 {
		return result
	}

	groups := computeMetricGroups(tagged)
	for _, idxs := range groups {
		hasPendingNew := false
		for _, idx := range idxs {
			if tagged[idx]["_merge_source"] == "new" {
				hasPendingNew = true
				break
			}
		}
		if !hasPendingNew {
			continue // an existing-only group with no new candidate touching it: untouched.
		}
		group := make([]map[string]any, 0, len(idxs))
		for _, idx := range idxs {
			group = append(group, tagged[idx])
		}
		result.PendingGroups = append(result.PendingGroups, group)
	}
	return result
}

func metricIDs(metrics []map[string]any) []string {
	ids := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		if id := strings.TrimSpace(asString(metric["metric_id"])); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// mergeStaticMetric preserves fields that are not present in the new
// enrichment result, then overlays the candidate's extracted fields. This
// mirrors the merge winner reconstruction used after the LLM path.
func mergeStaticMetric(existing, candidate map[string]any) map[string]any {
	merged := cloneMetricMap(existing)
	for key, value := range candidate {
		if key == "metric_id" || strings.HasPrefix(key, "_") {
			continue
		}
		merged[key] = value
	}
	merged["metric_id"] = existing["metric_id"]
	return merged
}

func cloneMetricMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
