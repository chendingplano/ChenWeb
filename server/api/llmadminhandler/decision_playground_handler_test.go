package llmadminhandler

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestPlaygroundProviderConfig(t *testing.T) {
	pc, err := playgroundProviderConfig("jev-latest", ApiTypes.LLMModelDef{ModelType: "decision-model", BaseURL: "https://jev-ai.pro/api"})
	if err != nil || pc.ID != llmclients.ProviderJevCompatible || pc.Extra != nil {
		t.Fatalf("decision-model: got %+v, %v", pc, err)
	}

	pc, err = playgroundProviderConfig("qwen-plus", ApiTypes.LLMModelDef{ModelType: "llm", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", OmitTemperature: true})
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

	pc, err = playgroundProviderConfig("deepseek-flash-4-1", ApiTypes.LLMModelDef{ModelType: "llm", BaseURL: "https://api.deepseek.com"})
	if err != nil || !reflect.DeepEqual(pc.Extra, map[string]string{"thinking": "disabled", "temperature": "1"}) {
		t.Fatalf("deepseek: got %+v, %v", pc, err)
	}

	if _, err := playgroundProviderConfig("emb", ApiTypes.LLMModelDef{ModelType: "embedding"}); err == nil {
		t.Fatal("embedding model must be rejected")
	}
}

func TestPlaygroundJevQuestions(t *testing.T) {
	got, err := playgroundJevQuestions([]playgroundQuestion{
		{Type: "noul", Instructions: " Is it urgent? "},
		{ID: "team", Type: "choice", Instructions: "Which team", Criteria: json.RawMessage(`{"billing":"Payments","tech":"Bugs"}`)},
		{Type: "score", Instructions: "Frustration", Criteria: json.RawMessage(`["Calm","Angry"]`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := llmclients.JevQuestions{
		"q1":   {Type: "noul", Instructions: "Is it urgent?"},
		"team": {Type: "choice", Instructions: "Which team", Criteria: map[string]string{"billing": "Payments", "tech": "Bugs"}},
		"q3":   {Type: "score", Instructions: "Frustration", Criteria: []string{"Calm", "Angry"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}

	bad := map[string][]playgroundQuestion{
		"none":           nil,
		"no instruction": {{Type: "noul", Instructions: " "}},
		"one option":     {{Type: "choice", Instructions: "x", Criteria: json.RawMessage(`{"a":"A"}`)}},
		"one level":      {{Type: "score", Instructions: "x", Criteria: json.RawMessage(`["low"]`)}},
		"unknown type":   {{Type: "yesno", Instructions: "x"}},
		"duplicate id":   {{ID: "a", Type: "noul", Instructions: "x"}, {ID: "a", Type: "noul", Instructions: "y"}},
	}
	for name, qs := range bad {
		if _, err := playgroundJevQuestions(qs); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestPlaygroundState(t *testing.T) {
	got, err := playgroundState("hello", "be strict")
	if err != nil || got != `{"policy":"be strict","text":"hello"}` {
		t.Fatalf("got %q, %v", got, err)
	}
	got, err = playgroundState("hello", "  ")
	if err != nil || got != `{"text":"hello"}` {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := playgroundState(" ", ""); err == nil {
		t.Fatal("empty state must be rejected")
	}
}
