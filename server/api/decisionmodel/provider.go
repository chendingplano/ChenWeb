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

// Provider maps a .models.toml entry to the provider that runs its decisions.
//
// A "decision-model" entry is a decision model and must say which kind through
// decision_model_type: "native" (a real decision-model service such as Jev) runs
// through jev_compatible, "emulated" (a chat model answering through its logprobs)
// through jev_emulated. A missing or invalid decision_model_type is an error. An
// "llm" entry is an ordinary chat model and always runs emulated. Any other
// model type cannot run decisions (ok is false).
func Provider(cfg ApiTypes.LLMModelDef) (provider llmclients.ProviderID, ok bool, err error) {
	switch strings.TrimSpace(cfg.ModelType) {
	case "decision-model":
		if err := cfg.CheckDecisionModelType(); err != nil {
			return "", true, err
		}
		if strings.TrimSpace(cfg.DecisionModelType) == ApiTypes.DecisionModelNative {
			return llmclients.ProviderJevCompatible, true, nil
		}
		return llmclients.ProviderJevEmulated, true, nil
	case "llm":
		return llmclients.ProviderJevEmulated, true, nil
	default:
		return "", false, nil
	}
}

// RequireDecisionModel reports an error unless the entry is a decision model, that is
// model_type = "decision-model" with a valid decision_model_type. An "llm" entry does
// not qualify, although Provider can still run it for the Playground.
func RequireDecisionModel(key string, cfg ApiTypes.LLMModelDef) error {
	if t := strings.TrimSpace(cfg.ModelType); t != "decision-model" {
		return fmt.Errorf("model %q has model_type %q: a decision model must have model_type %q", key, t, "decision-model")
	}
	if err := cfg.CheckDecisionModelType(); err != nil {
		return fmt.Errorf("model %q: %w", key, err)
	}
	return nil
}

// ProviderConfig builds the client config for a model entry.
func ProviderConfig(key string, cfg ApiTypes.LLMModelDef) (llmclients.ProviderConfig, error) {
	provider, ok, err := Provider(cfg)
	if err != nil {
		return llmclients.ProviderConfig{}, fmt.Errorf("model %q: %w", key, err)
	}
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
