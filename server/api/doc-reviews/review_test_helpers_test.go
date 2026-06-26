package docreviews

import (
	"context"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// fakeJSONExtractor is a minimal LLMJSONExtractor stub for reviewer unit tests.
// It records the prompt/model/input it was called with and returns a canned
// response.
type fakeJSONExtractor struct {
	out           map[string]any
	seq           []map[string]any
	err           error
	promptNames   []string
	modelNames    []string
	inputTexts    []string
	documentFirst []bool
	lastUsage     *llmclients.Usage
	calledCount   int
}

func (f *fakeJSONExtractor) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	f.calledCount++
	f.promptNames = append(f.promptNames, in.PromptName)
	f.modelNames = append(f.modelNames, in.ModelName)
	f.inputTexts = append(f.inputTexts, in.InputText)
	f.documentFirst = append(f.documentFirst, in.DocumentFirst)
	if len(f.seq) > 0 {
		next := f.seq[0]
		f.seq = f.seq[1:]
		return next, f.err
	}
	return f.out, f.err
}

func (f *fakeJSONExtractor) LastJSONUsage() *llmclients.Usage {
	if f.lastUsage == nil {
		return nil
	}
	usage := *f.lastUsage
	return &usage
}
