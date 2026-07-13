package docprocessing

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

func fileHash(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func promptConfig(path, ref, text string) map[string]any {
	v := map[string]any{"ref": ref, "path": path}
	if h := fileHash(path); h != "" {
		v["sha256"] = h
	}
	if text != "" {
		h := sha256.Sum256([]byte(text))
		v["content_sha256"] = hex.EncodeToString(h[:])
	}
	return v
}

// RuntimeConfig returns the effective non-secret chunking settings.
func (s *FixedSizeChunkingService) RuntimeConfig() map[string]any {
	return map[string]any{"chunk_size": s.ChunkSize, "overlap_percent": s.OverlapPercent,
		"model":          map[string]any{"ref": s.ModelRef, "path": s.ModelCfgPath, "name": s.ModelName},
		"summary_model":  map[string]any{"ref": s.SummaryModelRef, "path": s.SummaryModelCfgPath, "name": s.SummaryModelName},
		"prompt":         promptConfig(s.PromptPath, s.PromptRef, s.PromptText),
		"summary_prompt": promptConfig(s.SummaryPromptPath, s.SummaryPromptRef, s.SummaryPromptText),
		"concurrency":    map[string]any{"summary_max_tasks": s.GenerateSummaryMaxTasks, "topics_max_tasks": s.ExtractTopicsMaxTasks}}
}

// RuntimeConfig returns effective metric prompts and model references while
// deliberately excluding API keys and provider credentials.
func (p *MetricsProcessor) RuntimeConfig() map[string]any {
	return map[string]any{"mention_prompt": promptConfig(p.MentionPromptPath, p.MentionPromptRef, p.MentionPromptText),
		"relation_prompt":      promptConfig(p.RelationPromptPath, p.RelationPromptRef, p.RelationPromptText),
		"merge_resolve_prompt": promptConfig(p.MergeResolvePromptPath, p.MergeResolvePromptRef, p.MergeResolvePromptText),
		"model":                map[string]any{"ref": p.ModelRef, "path": p.ModelCfgPath, "name": p.ModelName},
		"mention_model":        map[string]any{"ref": p.MentionModelRef, "path": p.MentionModelCfgPath, "name": p.MentionModelName},
		"relation_model":       map[string]any{"ref": p.RelationModelRef, "path": p.RelationModelCfgPath, "name": p.RelationModelName},
		"merge_resolve_model":  map[string]any{"ref": p.MergeResolveModelRef, "path": p.MergeResolveModelCfgPath, "name": p.MergeResolveModelName},
		"enrich_group_size":    p.MetricEnrichGroupSize, "max_tasks": p.MaxTasks}
}
