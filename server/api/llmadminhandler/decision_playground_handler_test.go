package llmadminhandler

import (
	"encoding/json"
	"reflect"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

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
