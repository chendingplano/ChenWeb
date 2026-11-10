package docbuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

const (
	batchSize      = 25
	excerptRunes   = 500
	maxCategoryLen = 40
)

// Classifier assigns a category to each document. It is called only for documents that have none.
type Classifier interface {
	Classify(ctx context.Context, existing []string, docs []Candidate) (map[string]string, error)
}

// Candidate is what the classifier sees of one document.
type Candidate struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"`
}

// ClassifyBatches sends items to c in batches of batchSize and passes each batch's cleaned
// answers (key → category, empty answers dropped) to save before the next batch, so a failure
// keeps the progress already saved. existing lists the categories in use; it grows as batches
// are answered. It returns how many items were categorised.
func ClassifyBatches(ctx context.Context, existing []string, items []Candidate, c Classifier,
	save func(answers map[string]string) error) (int, error) {
	done := 0
	for start := 0; start < len(items); start += batchSize {
		end := min(start+batchSize, len(items))
		answers, err := c.Classify(ctx, existing, items[start:end])
		if err != nil {
			return done, err
		}
		clean := map[string]string{}
		for _, it := range items[start:end] {
			if cat := cleanCategory(answers[it.Path]); cat != "" {
				clean[it.Path] = cat
				if !contains(existing, cat) {
					existing = append(existing, cat)
				}
			}
		}
		if err := save(clean); err != nil {
			return done, err
		}
		done += len(clean)
	}
	return done, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func cleanCategory(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxCategoryLen {
		s = string([]rune(s)[:maxCategoryLen])
	}
	return strings.TrimSpace(s)
}

// Excerpt returns the first characters of a document's text for the classifier.
func Excerpt(text string) string {
	r := []rune(text)
	if len(r) > excerptRunes {
		r = r[:excerptRunes]
	}
	return string(r)
}

// LLMClassifier classifies through a chat model, with the prompt text as the system message.
type LLMClassifier struct {
	Client llmclients.Client
	Model  string
	Prompt string
}

func (l *LLMClassifier) Classify(ctx context.Context, existing []string, docs []Candidate) (map[string]string, error) {
	state, err := json.Marshal(map[string]any{"existing_categories": existing, "documents": docs})
	if err != nil {
		return nil, err
	}
	resp, err := l.Client.Complete(ctx, llmclients.Request{
		Model:      l.Model,
		UserID:     "docbuilder",
		PromptName: "docbuilder_classify",
		CallReason: "docbuilder_classify",
		CallLoc:    "MID-26110901",
		Messages: []llmclients.Message{
			{Role: llmclients.RoleSystem, Content: l.Prompt},
			{Role: llmclients.RoleUser, Content: string(state)},
		},
	})
	if err != nil {
		return nil, err
	}
	return parseAnswers(resp.Content)
}

// parseAnswers reads the model's {"path": "category"} object, tolerating a code fence around it.
func parseAnswers(s string) (map[string]string, error) {
	s = strings.TrimSpace(s)
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		s = s[i : j+1]
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("classifier answer is not a path→category object: %w", err)
	}
	return out, nil
}
