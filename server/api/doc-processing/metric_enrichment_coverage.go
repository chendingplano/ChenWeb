package docprocessing

import (
	"context"
	"errors"
	"fmt"
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
	missing := unaccountedMetricCandidates(metrics, dropped, candidates)
	if len(missing) == 0 {
		return payload, nil
	}
	p.Logger.Warn("pass2: retrying candidates neither enriched nor listed in dropped_candidates",
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
		for _, m := range retryMetrics {
			if allowed[asString(m["candidate_id"])] {
				metrics = append(metrics, m)
			}
		}
		retryDropped, _ := retry["dropped_candidates"].([]any)
		for _, d := range retryDropped {
			if m, ok := d.(map[string]any); ok && allowed[asString(m["candidate_id"])] {
				dropped = append(dropped, d)
			}
		}
		retryUncertain, _ := retry["uncertain_metrics"].([]any)
		uncertain = append(uncertain, retryUncertain...)
	}
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
