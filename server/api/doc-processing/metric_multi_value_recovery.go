package docprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Retry only candidates with combined values, once and using their original evidence.
// Splitting mixed units or conditions needs document judgement, not string splitting.
// Keep the original rows if a retry fails; the existing multi_value_rows log remains
// the diagnostic for anything the model still cannot split.
func (p *MetricsProcessor) recoverMultiValueMetricRows(
	ctx context.Context, recordID int64, callID string, chunkIdx, totalBatches int,
	inputText string, candidates []metricCandidate, metrics []map[string]any,
) []map[string]any {
	flagged := make(map[string]bool)
	for _, m := range metrics {
		if metricValueHoldsSeveralValues(asString(m["metric_value"])) {
			flagged[asString(m["candidate_id"])] = true
		}
	}
	var pending []metricCandidate
	for _, c := range candidates {
		if flagged[c.CandidateID] {
			pending = append(pending, c)
		}
	}
	replacements := make(map[string][]map[string]any)
	for i, batch := range groupCandidatesByChunk(pending, 1) {
		if isCtxStopped(ctx) {
			break
		}
		allowed := make(map[string]bool)
		evidence := make(map[string][]string)
		for _, c := range batch.candidates {
			allowed[c.CandidateID] = true
			evidence[c.CandidateID] = candidateSourceLineSpans(c)
		}
		var combined []map[string]any
		for _, m := range metrics {
			id := asString(m["candidate_id"])
			if allowed[id] {
				evidence[id] = append(evidence[id], toStringSlice(m["source_line_spans"])...)
			}
			if allowed[asString(m["candidate_id"])] && metricValueHoldsSeveralValues(asString(m["metric_value"])) {
				combined = append(combined, m)
			}
		}
		data, _ := json.Marshal(map[string]any{"multi_value_rows_to_split": combined})
		start := p.Now()
		retryID := fmt.Sprintf("%s_split_%d", callID, i+1)
		p.Logger.Warn("pass2: retrying candidates with combined metric values", "record_id", recordID, "call_id", retryID, "num_rows", len(combined))
		payload, err := p.extractMetricPayload(ctx, inputText,
			p.RelationPromptText+"\n\n"+buildMetricRelationBatchPrompt(batch.candidates)+"\n\n"+string(data),
			p.RelationModelName, p.RelationModelCfg, p.RelationPromptRef, "enrich_metrics", "MID-26101602", true)
		p.logEnrichMetricsChunk(ctx, recordID, retryID, chunkIdx, totalBatches,
			[]string{p.RelationModelName}, p.RelationPromptRef, payload, err, start, p.Now(), "")
		if err != nil {
			p.Logger.Warn("pass2: combined value retry failed; keeping original rows", "record_id", recordID, "call_id", retryID, "error", err)
			continue
		}
		raw, _ := payload["metrics"].([]any)
		retry := normalizeMetricList(raw)
		backfillMetricResultCandidateIDs(retry, batch.candidates)
		byCandidate := make(map[string][]map[string]any)
		invalid := make(map[string]bool)
		for _, m := range retry {
			id := asString(m["candidate_id"])
			if !allowed[id] {
				continue
			}
			spans := toStringSlice(m["source_line_spans"])
			if strings.TrimSpace(asString(m["metric_name"])) == "" || strings.TrimSpace(asString(m["metric_value"])) == "" ||
				!metricSplitEvidenceWithin(spans, evidence[id]) || metricValueHoldsSeveralValues(asString(m["metric_value"])) || metricDropReasonTag(m) != "" {
				invalid[id] = true
			}
			byCandidate[id] = append(byCandidate[id], m)
		}
		for _, c := range batch.candidates {
			// A combined row must yield multiple rows, not disappear or become just
			// one of its values. Never accept only the valid half of a retry.
			rows := dedupeFinalMetricRows(byCandidate[c.CandidateID])
			if invalid[c.CandidateID] || len(rows) < 2 || !metricSplitKeepsNumbers(combined, rows, c.CandidateID) {
				p.Logger.Warn("pass2: combined value retry incomplete; keeping original rows", "record_id", recordID, "call_id", retryID, "candidate_id", c.CandidateID)
				continue
			}
			replacements[c.CandidateID] = rows
			p.Logger.Info("pass2: split combined metric values", "record_id", recordID, "call_id", retryID, "candidate_id", c.CandidateID, "num_rows", len(rows))
		}
	}
	var out []map[string]any
	for _, m := range metrics {
		id := asString(m["candidate_id"])
		if rows, ok := replacements[id]; ok && metricValueHoldsSeveralValues(asString(m["metric_value"])) {
			out = append(out, rows...)
			// Insert replacement rows once even when the candidate had multiple
			// combined rows. Non-combined siblings remain for final deduplication.
			replacements[id] = nil
			continue
		}
		out = append(out, m)
	}
	return out
}

// A retry may narrow a source span but must not introduce evidence outside the
// candidate and its original rows. Reject malformed spans as well as empty ones.
func metricSplitEvidenceWithin(spans, evidence []string) bool {
	if len(spans) == 0 {
		return false
	}
	for _, span := range spans {
		lo, hi, ok := parseMetricLineSpan(span)
		if !ok {
			return false
		}
		contained := false
		for _, source := range evidence {
			start, end, valid := parseMetricLineSpan(source)
			if valid && lo >= start && hi <= end {
				contained = true
				break
			}
		}
		if !contained {
			return false
		}
	}
	return true
}

// Conservatively require every numeric token of every combined row to survive
// in the distinct replacement values. This is a loss guard, not a semantic
// parser: equivalent rewrites it cannot prove are left for the warning log.
var metricSplitNumberRe = regexp.MustCompile(metricNumberPattern)

func metricSplitKeepsNumbers(original, replacement []map[string]any, candidateID string) bool {
	counts := make(map[float64]int)
	for _, m := range replacement {
		for _, token := range metricSplitNumberRe.FindAllString(compactMetricValue(asString(m["metric_value"])), -1) {
			if n, ok := toOptionalFloat(token).(float64); ok {
				counts[n]++
			}
		}
	}
	for _, m := range original {
		if asString(m["candidate_id"]) != candidateID {
			continue
		}
		for _, token := range metricSplitNumberRe.FindAllString(compactMetricValue(asString(m["metric_value"])), -1) {
			if n, ok := toOptionalFloat(token).(float64); ok {
				counts[n]--
				if counts[n] < 0 {
					return false
				}
			}
		}
	}
	return true
}
