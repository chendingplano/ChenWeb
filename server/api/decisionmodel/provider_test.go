package decisionmodel

import (
	"reflect"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestProviderConfig(t *testing.T) {
	pc, err := ProviderConfig("jev-latest", ApiTypes.LLMModelDef{ModelType: "decision-model", BaseURL: "https://jev-ai.pro/api"})
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

	if _, err := ProviderConfig("emb", ApiTypes.LLMModelDef{ModelType: "embedding"}); err == nil {
		t.Fatal("embedding model must be rejected")
	}
}
