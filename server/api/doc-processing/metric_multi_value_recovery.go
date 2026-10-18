package docprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Review unclassified combined values once against their original evidence.
// A multi-value assertion may legitimately remain one structured row.
// Keep the original rows if a retry fails; the existing multi_value_rows log remains
// the diagnostic for anything the model still cannot split.
func (p *MetricsProcessor) recoverMultiValueMetricRows(
	ctx context.Context, recordID int64, callID string, chunkIdx, totalBatches int,
	inputText string, candidates []metricCandidate, metrics []map[string]any,
) []map[string]any {
	flagged := make(map[string]bool)
	for _, m := range metrics {
		if metricRowNeedsValueReview(m) {
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
			if allowed[asString(m["candidate_id"])] && metricRowNeedsValueReview(m) {
				combined = append(combined, m)
			}
		}
		data, _ := json.Marshal(map[string]any{"multi_value_rows_to_review": combined})
		start := p.Now()
		retryID := fmt.Sprintf("%s_split_%d", callID, i+1)
		p.Logger.Info("pass2: reviewing combined metric value semantics", "record_id", recordID, "call_id", retryID, "num_rows", len(combined))
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
		decisions := metricValueReviewDecisions(payload["value_review_decisions"])
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
				!metricSplitEvidenceWithin(spans, evidence[id]) || metricRowNeedsValueReview(m) || metricDropReasonTag(m) != "" {
				invalid[id] = true
			}
			byCandidate[id] = append(byCandidate[id], m)
		}
		for _, c := range batch.candidates {
			rows := dedupeFinalMetricRows(byCandidate[c.CandidateID])
			action := decisions[c.CandidateID]
			valid := !invalid[c.CandidateID] && metricSplitKeepsNumbers(combined, rows, c.CandidateID)
			switch action {
			case "split":
				valid = valid && len(rows) >= 2
			case "keep":
				valid = valid && len(rows) == 1 && normalizeMetricValueStructure(rows[0]["value_structure"]) != nil && !metricRowNeedsValueReview(rows[0])
				// Keep does not reinterpret a value's unit or introduce new numeric
				// text. With multiple original combined rows, preserve them instead.
				var originals []map[string]any
				for _, m := range combined {
					if asString(m["candidate_id"]) == c.CandidateID {
						originals = append(originals, m)
					}
				}
				valid = valid && len(originals) == 1
				if valid {
					valid = compactMetricValue(asString(rows[0]["metric_value"])) == compactMetricValue(asString(originals[0]["metric_value"])) && metricRowUnit(rows[0]) == metricRowUnit(originals[0])
					if valid {
						// Keep classifies the existing assertion; it does not authorize
						// rewriting its subject, applicability, frame or provenance.
						kept := cloneMetricMap(originals[0])
						kept["value_structure"] = rows[0]["value_structure"]
						kept["value_min"], kept["value_max"] = nil, nil
						valid = !metricRowNeedsValueReview(kept)
						var tags []string
						for _, tag := range toStringSlice(kept["reasoning_tags"]) {
							if tag != "value_review_unresolved" {
								tags = append(tags, tag)
							}
						}
						kept["reasoning_tags"] = tags
						rows = []map[string]any{kept}
					}
				}
			default:
				valid = false
			}
			if !valid {
				p.Logger.Warn("pass2: value semantics unresolved; keeping original rows", "record_id", recordID, "call_id", retryID, "candidate_id", c.CandidateID)
				continue
			}
			replacements[c.CandidateID] = rows
			p.Logger.Info("pass2: resolved combined metric values", "record_id", recordID, "call_id", retryID, "candidate_id", c.CandidateID, "action", action, "num_rows", len(rows))
		}
	}
	var out []map[string]any
	for _, m := range metrics {
		id := asString(m["candidate_id"])
		if rows, ok := replacements[id]; ok && metricRowNeedsValueReview(m) {
			out = append(out, rows...)
			// Insert replacement rows once even when the candidate had multiple
			// combined rows. Non-combined siblings remain for final deduplication.
			replacements[id] = nil
			continue
		}
		if metricRowNeedsValueReview(m) {
			markMetricValueUnresolved(m)
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

// Requiring an explicit decision avoids interpreting a model's arbitrary row
// count as proof that the source can be split without losing its relationships.
func metricValueReviewDecisions(raw any) map[string]string {
	result := make(map[string]string)
	seen := make(map[string]bool)
	rows, _ := raw.([]any)
	for _, item := range rows {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := strings.TrimSpace(asString(row["candidate_id"]))
		if id == "" {
			continue
		}
		if seen[id] {
			result[id] = ""
			continue
		}
		seen[id] = true
		if strings.TrimSpace(asString(row["reason"])) == "" {
			continue
		}
		action := strings.TrimSpace(asString(row["action"]))
		if action == "split" || action == "keep" || action == "unresolved" {
			result[id] = action
		}
	}
	return result
}
