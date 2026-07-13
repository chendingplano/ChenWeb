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
func modelConfig(ref, path, name string, c structureModelConfig) map[string]any {
	return map[string]any{"ref": ref, "path": path, "name": name, "definition_sha256": fileHash(path), "profile": c.ProfileName, "provider": c.BaseURL, "model": c.ModelName, "timeout_sec": c.TimeoutSec, "thinking_type": c.ThinkingType}
}

// RuntimeConfig returns the effective non-secret chunking settings.
func (s *FixedSizeChunkingService) RuntimeConfig() map[string]any {
	return map[string]any{"chunk_size": s.ChunkSize, "overlap_percent": s.OverlapPercent,
		"model":          map[string]any{"ref": s.ModelRef, "path": s.ModelCfgPath, "name": s.ModelName, "definition_sha256": fileHash(s.ModelCfgPath)},
		"summary_model":  map[string]any{"ref": s.SummaryModelRef, "path": s.SummaryModelCfgPath, "name": s.SummaryModelName, "definition_sha256": fileHash(s.SummaryModelCfgPath)},
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
		"model":                modelConfig(p.ModelRef, p.ModelCfgPath, p.ModelName, p.RelationModelCfg),
		"mention_model":        modelConfig(p.MentionModelRef, p.MentionModelCfgPath, p.MentionModelName, p.MentionModelCfg),
		"relation_model":       modelConfig(p.RelationModelRef, p.RelationModelCfgPath, p.RelationModelName, p.RelationModelCfg),
		"merge_resolve_model":  modelConfig(p.MergeResolveModelRef, p.MergeResolveModelCfgPath, p.MergeResolveModelName, p.MergeResolveModelCfg),
		"enrich_group_size":    p.MetricEnrichGroupSize, "max_tasks": p.MaxTasks}
}
