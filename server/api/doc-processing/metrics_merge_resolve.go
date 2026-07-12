package docprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// resolveMergeAmbiguities sends one pending Metric Group (DR2 Rule-4) to the
// Merge Resolution LLM call and returns the winning metrics, the candidates
// payload sent to the LLM (for caller-side logging), and the model name that
// produced the returned winners. Every entry in group must already carry
// "metric_id" and "_merge_source" ("existing"|"new") (DR4). Retries once with
// the fallback model on failure or an invalid partition (ADR 2026071002 DR2/DR4).
func (p *MetricsProcessor) resolveMergeAmbiguities(ctx context.Context, recordID int64, group []map[string]any) (winners []map[string]any, candidates []map[string]any, modelUsed string, err error) {
	inputIDs := make(map[string]bool, len(group))
	candidates = make([]map[string]any, 0, len(group))
	for _, m := range group {
		id := asString(m["metric_id"])
		inputIDs[id] = true
		candidates = append(candidates, map[string]any{
			"metric_id":           id,
			"source":              m["_merge_source"],
			"metric_name":         m["metric_name"],
			"metric_subject":      m["metric_subject"],
			"metric_unit":         m["metric_unit"],
			"metric_value":        m["metric_value"],
			"value_data_type":     m["value_data_type"],
			"value_range_type":    m["value_range_type"],
			"value_class":         m["value_class"],
			"threshold_or_target": m["threshold_or_target"],
			"metric_categories":   m["metric_categories"],
			"source_line_spans":   m["source_line_spans"],
		})
	}

	winners, err = p.callMergeResolve(ctx, recordID, candidates, p.MergeResolveModelName, p.MergeResolveModelCfg)
	if err == nil {
		if valErr := validateMergeResolveWinners(winners, inputIDs); valErr == nil {
			return winners, candidates, p.MergeResolveModelName, nil
		} else {
			err = valErr
		}
	}

	fallbackModelName := strings.TrimSpace(p.FallbackMergeResolveModelName)
	if fallbackModelName == "" {
		return nil, candidates, p.MergeResolveModelName, fmt.Errorf("(MID_26071101) merge resolve failed and no fallback model configured: %w", err)
	}
	if p.FallbackMergeResolveModelErr != nil {
		return nil, candidates, p.MergeResolveModelName, fmt.Errorf("(MID_26071102) merge resolve failed and fallback model %q unavailable: %w", p.FallbackMergeResolveModelRef, err)
	}
	p.Logger.Warn("merge resolve failed on primary model; retrying fallback",
		"record_id", recordID, "primary_model", p.MergeResolveModelName,
		"fallback_model", fallbackModelName, "error", err)

	winners, fbErr := p.callMergeResolve(ctx, recordID, candidates, fallbackModelName, p.FallbackMergeResolveModelCfg)
	if fbErr != nil {
		return nil, candidates, fallbackModelName, fmt.Errorf("(MID_26071103) primary merge resolve failed: %w; fallback failed: %v", err, fbErr)
	}
	if valErr := validateMergeResolveWinners(winners, inputIDs); valErr != nil {
		return nil, candidates, fallbackModelName, fmt.Errorf("(MID_26071104) fallback merge resolve returned invalid partition: %w", valErr)
	}
	return winners, candidates, fallbackModelName, nil
}

func (p *MetricsProcessor) callMergeResolve(ctx context.Context, recordID int64, candidates []map[string]any, modelName string, cfg structureModelConfig) ([]map[string]any, error) {
	applyStructureModelConfigToExtractor(p.Extractor, cfg)
	taskPrompt := mergeResolveTask(p.MergeResolvePromptText, recordID, candidates)
	in := newLLMJSONInput(ctx, p.MergeResolvePromptRef, p.MergeResolvePromptText, modelName, taskPrompt,
		"merge_resolve_metrics", "MID-26071105")
	payload, err := p.Extractor.ExtractJSON(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("(MID_26071106) merge resolve LLM call failed: %w", err)
	}
	raw, ok := payload["winning_metrics"].([]any)
	if !ok {
		return nil, fmt.Errorf("(MID_26071107) merge resolve output missing winning_metrics")
	}
	winners := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			winners = append(winners, m)
		}
	}
	return winners, nil
}

// validateMergeResolveWinners checks every input metric_id appears exactly
// once, either as a winner's own metric_id or in its absorbed_metric_ids.
func validateMergeResolveWinners(winners []map[string]any, inputIDs map[string]bool) error {
	seen := map[string]int{}
	for _, w := range winners {
		id := asString(w["metric_id"])
		seen[id]++
		if absorbed, ok := w["absorbed_metric_ids"].([]any); ok {
			for _, a := range absorbed {
				seen[asString(a)]++
			}
		}
	}
	for id := range inputIDs {
		if seen[id] != 1 {
			return fmt.Errorf("(MID_26071108) metric_id %q appeared %d times in output, want 1", id, seen[id])
		}
	}
	for id := range seen {
		if !inputIDs[id] {
			return fmt.Errorf("(MID_26071109) output metric_id %q was not in the input", id)
		}
	}
	return nil
}

// mergeResolveTask fills the Merge Resolution prompt template's
// {{input_record_id}}/{{candidates_json}} placeholders. Mirrors
// metricCandidateTask's shape (extract-metrics.go).
func mergeResolveTask(basePrompt string, recordID int64, candidates []map[string]any) string {
	candidatesJSON, _ := json.Marshal(candidates)
	task := strings.ReplaceAll(basePrompt, "{{input_record_id}}", fmt.Sprintf("%d", recordID))
	return strings.ReplaceAll(task, "{{candidates_json}}", string(candidatesJSON))
}
