// Package decisionmodel maps a .models.toml entry to the client config that runs
// decision requests against it (devdoc 2026100502). Shared by the Decision Model
// Playground and extract_metrics' open-value decision (openspec change
// metric-row-soft-drop-decision-model).
package decisionmodel

import (
	"fmt"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// Provider maps a .models.toml entry to the decision provider used to run it:
// real Jev for decision-model entries, the logprob emulation for ordinary chat
// models.
func Provider(cfg ApiTypes.LLMModelDef) (llmclients.ProviderID, bool) {
	switch strings.TrimSpace(cfg.ModelType) {
	case "decision-model":
		return llmclients.ProviderJevCompatible, true
	case "llm":
		return llmclients.ProviderJevEmulated, true
	default:
		return "", false
	}
}

// ProviderConfig builds the client config for a model entry.
func ProviderConfig(key string, cfg ApiTypes.LLMModelDef) (llmclients.ProviderConfig, error) {
	provider, ok := Provider(cfg)
	if !ok {
		return llmclients.ProviderConfig{}, fmt.Errorf("model %q (type %q) cannot run decisions", key, cfg.ModelType)
	}
	pc := llmclients.ProviderConfig{
		ID:          provider,
		BaseURL:     strings.TrimSpace(cfg.BaseURL),
		APIKey:      strings.TrimSpace(cfg.APIKey),
		ProfileName: key,
	}
	if provider == llmclients.ProviderJevEmulated {
		pc.Extra = map[string]string{}
		base := strings.ToLower(pc.BaseURL)
		// DashScope returns at most 5 alternatives per token.
		if strings.Contains(base, "dashscope") {
			pc.Extra["top_logprobs"] = "5"
		}
		// DeepSeek V4 thinks by default, which uses up the single output
		// token and returns no logprobs. Its logprobs are also taken after
		// temperature: at 0 every non-chosen option is -9999, so use 1 to get
		// the model's real distribution (the sampled token itself is unused).
		if strings.Contains(base, "deepseek") {
			pc.Extra["thinking"] = "disabled"
			pc.Extra["temperature"] = "1"
		}
		if cfg.OmitTemperature {
			pc.Extra["temperature"] = "omit"
		}
	}
	return pc, nil
}
