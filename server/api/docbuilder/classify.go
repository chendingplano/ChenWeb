package docbuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

// ClassifyNew sends every uncategorised entry of m to c, in batches, and stores the answers.
// Entries that already have a category are never reclassified (the operation is one-time per
// file); set force to clear them first. m is saved after each batch, so a failure keeps progress.
// It returns the number of entries newly categorised.
func ClassifyNew(ctx context.Context, dir string, m *Manifest, c Classifier, model string, force bool) (int, error) {
	if force {
		for i := range m.Entries {
			m.Entries[i].Category = ""
		}
	}
	m.Model = model
	var pending []int
	for i, e := range m.Entries {
		if e.Category == "" {
			pending = append(pending, i)
		}
	}
	done := 0
	for start := 0; start < len(pending); start += batchSize {
		end := min(start+batchSize, len(pending))
		var docs []Candidate
		for _, i := range pending[start:end] {
			e := m.Entries[i]
			docs = append(docs, Candidate{Path: e.Path, Title: e.Title, Excerpt: excerpt(dir, e)})
		}
		answers, err := c.Classify(ctx, categoriesOf(m), docs)
		if err != nil {
			_ = SaveManifest(dir, m)
			return done, err
		}
		for _, i := range pending[start:end] {
			if cat := cleanCategory(answers[m.Entries[i].Path]); cat != "" {
				m.Entries[i].Category = cat
				done++
			}
		}
		if err := SaveManifest(dir, m); err != nil {
			return done, err
		}
	}
	return done, nil
}

func categoriesOf(m *Manifest) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range m.Entries {
		if e.Category != "" && !seen[e.Category] {
			seen[e.Category] = true
			out = append(out, e.Category)
		}
	}
	return out
}

func cleanCategory(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxCategoryLen {
		s = string([]rune(s)[:maxCategoryLen])
	}
	return strings.TrimSpace(s)
}

// excerpt is the start of a Markdown file; a PDF has none (its title and path are what we have).
func excerpt(dir string, e Entry) string {
	if e.Kind != KindMarkdown {
		return ""
	}
	raw, err := os.ReadFile(safeJoin(dir, e.Path))
	if err != nil {
		return ""
	}
	r := []rune(string(raw))
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
