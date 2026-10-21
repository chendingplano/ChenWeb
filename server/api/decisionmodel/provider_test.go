package decisionmodel

import (
	"reflect"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestProviderConfig(t *testing.T) {
	pc, err := ProviderConfig("jev-latest", ApiTypes.LLMModelDef{ModelType: "decision-model", DecisionModelType: "native", BaseURL: "https://jev-ai.pro/api"})
	if err != nil || pc.ID != llmclients.ProviderJevCompatible || pc.Extra != nil {
		t.Fatalf("decision-model: got %+v, %v", pc, err)
	}

	pc, err = ProviderConfig("qwen-plus", ApiTypes.LLMModelDef{ModelType: "llm", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", OmitTemperature: true})
	if err != nil || pc.ID != llmclients.ProviderJevEmulated {
		t.Fatalf("llm: got %+v, %v", pc, err)
	}
	want := map[string]string{"top_logprobs": "5", "temperature": "omit"}
	if !reflect.DeepEqual(pc.Extra, want) {
		t.Fatalf("extra = %v, want %v", pc.Extra, want)
	}
	if pc.ProfileName != "qwen-plus" {
		t.Fatalf("profile name = %q", pc.ProfileName)
	}

	pc, err = ProviderConfig("deepseek-flash-4-1", ApiTypes.LLMModelDef{ModelType: "llm", BaseURL: "https://api.deepseek.com"})
	if err != nil || !reflect.DeepEqual(pc.Extra, map[string]string{"thinking": "disabled", "temperature": "1"}) {
		t.Fatalf("deepseek: got %+v, %v", pc, err)
	}

	pc, err = ProviderConfig("deepseek-flash-decision-model", ApiTypes.LLMModelDef{ModelType: "decision-model", DecisionModelType: "emulated", BaseURL: "https://api.deepseek.com"})
	if err != nil || pc.ID != llmclients.ProviderJevEmulated || !reflect.DeepEqual(pc.Extra, map[string]string{"thinking": "disabled", "temperature": "1"}) {
		t.Fatalf("emulated decision-model: got %+v, %v", pc, err)
	}

	for _, dmt := range []string{"", "jev", "Native"} {
		if _, err := ProviderConfig("bad", ApiTypes.LLMModelDef{ModelType: "decision-model", DecisionModelType: dmt}); err == nil {
			t.Fatalf("decision_model_type %q must be rejected", dmt)
		}
	}

	if _, err := ProviderConfig("emb", ApiTypes.LLMModelDef{ModelType: "embedding"}); err == nil {
		t.Fatal("embedding model must be rejected")
	}
}

func TestRequireDecisionModel(t *testing.T) {
	ok := ApiTypes.LLMModelDef{ModelType: "decision-model", DecisionModelType: "emulated"}
	if err := RequireDecisionModel("a", ok); err != nil {
		t.Fatalf("decision-model: %v", err)
	}
	for name, cfg := range map[string]ApiTypes.LLMModelDef{
		"llm":          {ModelType: "llm"},
		"embedding":    {ModelType: "embedding"},
		"missing type": {ModelType: "decision-model"},
		"bad type":     {ModelType: "decision-model", DecisionModelType: "jev"},
	} {
		if err := RequireDecisionModel(name, cfg); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}
