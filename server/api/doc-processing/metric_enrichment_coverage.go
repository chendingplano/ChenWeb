package docprocessing

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var errMetricCandidateCoverage = errors.New("metric enrichment left candidates unaccounted for")

// Recover omitted candidates once, in smaller batches. Keep candidates quoting the
// same evidence together so that sibling table cells do not become duplicate rows.
// A valid JSON response is not necessarily a complete enrichment response.
func (p *MetricsProcessor) recoverMetricCandidateCoverage(
	ctx context.Context, recordID int64, callID string, chunkIdx, totalBatches int,
	inputText string, candidates []metricCandidate, payload map[string]any,
) (map[string]any, error) {
	raw, _ := payload["metrics"].([]any)
	metrics := normalizeMetricList(raw)
	backfillMetricResultCandidateIDs(metrics, candidates)
	dropped, _ := payload["dropped_candidates"].([]any)
	// A row whose range type promises a number but whose metric_value is empty is a malformed
	// answer, not an enrichment: retry its candidate like an omitted one (record 364, run 233).
	missing := unaccountedMetricCandidates(rowsWithStatedValue(metrics), dropped, candidates)
	if len(missing) == 0 {
		return payload, nil
	}
	p.Logger.Warn("pass2: retrying candidates neither enriched nor listed in dropped_candidates, or enriched without their value",
		"record_id", recordID, "call_id", callID, "candidate_ids", missing)
	missingSet := make(map[string]bool, len(missing))
	for _, id := range missing {
		missingSet[id] = true
	}
	var pending []metricCandidate
	for _, c := range candidates {
		if missingSet[c.CandidateID] {
			pending = append(pending, c)
		}
	}
	// Copy before merging so the original response remains unchanged for diagnostics.
	merged := make(map[string]any, len(payload))
	for k, v := range payload {
		merged[k] = v
	}
	uncertain, _ := payload["uncertain_metrics"].([]any)
	var retryErrors error
	for i, batch := range groupCandidatesByChunk(pending, 1) {
		if isCtxStopped(ctx) {
			return merged, ErrPipelineStopped
		}
		start := p.Now()
		retryID := fmt.Sprintf("%s_recovery_%d", callID, i+1)
		retry, err := p.extractMetricPayload(ctx, inputText,
			p.RelationPromptText+"\n\n"+buildMetricRelationBatchPrompt(batch.candidates),
			p.RelationModelName, p.RelationModelCfg, p.RelationPromptRef, "enrich_metrics", "MID-26101601", true)
		p.logEnrichMetricsChunk(ctx, recordID, retryID, chunkIdx, totalBatches,
			[]string{p.RelationModelName}, p.RelationPromptRef, retry, err, start, p.Now(), "")
		if err != nil {
			retryErrors = errors.Join(retryErrors, err)
			continue
		}
		retryRaw, _ := retry["metrics"].([]any)
		retryMetrics := normalizeMetricList(retryRaw)
		backfillMetricResultCandidateIDs(retryMetrics, batch.candidates)
		allowed := make(map[string]bool, len(batch.candidates))
		for _, c := range batch.candidates {
			allowed[c.CandidateID] = true
		}
		var recovered []map[string]any
		for _, m := range retryMetrics {
			if allowed[asString(m["candidate_id"])] {
				recovered = append(recovered, m)
			}
		}
		// Replace a candidate's value-less rows only when the retry stated a value for it.
		replaced := make(map[string]bool)
		for _, m := range rowsWithStatedValue(recovered) {
			replaced[asString(m["candidate_id"])] = true
		}
		kept := metrics[:0]
		for _, m := range metrics {
			if !replaced[asString(m["candidate_id"])] || !metricRowLostItsValue(m) {
				kept = append(kept, m)
			}
		}
		metrics = append(kept, recovered...)
		retryDropped, _ := retry["dropped_candidates"].([]any)
		for _, d := range retryDropped {
			if m, ok := d.(map[string]any); ok && allowed[asString(m["candidate_id"])] {
				dropped = append(dropped, d)
			}
		}
		retryUncertain, _ := retry["uncertain_metrics"].([]any)
		uncertain = append(uncertain, retryUncertain...)
	}
	// Value-less rows left after the retry stay as they are; the no_value stage sets them aside.
	// Downstream parsing uses []any, including for already normalized rows.
	mergedRaw := make([]any, len(metrics))
	for i, m := range metrics {
		mergedRaw[i] = m
	}
	merged["metrics"], merged["dropped_candidates"], merged["uncertain_metrics"] = mergedRaw, dropped, uncertain
	if missing = unaccountedMetricCandidates(metrics, dropped, candidates); len(missing) > 0 {
		err := fmt.Errorf("%w: record_id=%d call_id=%s candidate_ids=%v", errMetricCandidateCoverage, recordID, callID, missing)
		p.Logger.Error("pass2: candidate recovery incomplete", "record_id", recordID, "call_id", callID, "candidate_ids", missing, "error", retryErrors)
		return merged, errors.Join(err, retryErrors)
	}
	p.Logger.Info("pass2: candidate recovery complete", "record_id", recordID, "call_id", callID, "recovered_candidates", len(pending))
	return merged, nil
}

// metricRowLostItsValue reports a row whose range type promises a stated number while its
// metric_value is empty: the model dropped the field, the source did not leave the value open.
func metricRowLostItsValue(m map[string]any) bool {
	switch strings.TrimSpace(asString(m["value_range_type"])) {
	case "exact", "lower_bound", "upper_bound", "range":
		return !metricHasValue(m)
	}
	return false
}

func rowsWithStatedValue(metrics []map[string]any) []map[string]any {
	var out []map[string]any
	for _, m := range metrics {
		if !metricRowLostItsValue(m) {
			out = append(out, m)
		}
	}
	return out
}
