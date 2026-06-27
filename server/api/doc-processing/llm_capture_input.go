package docprocessing

import (
	"context"
	"strings"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type llmRecordIDKey struct{}

func withLLMRecordID(ctx context.Context, recordID int64) context.Context {
	if recordID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, llmRecordIDKey{}, recordID)
}

func llmRecordIDFromContext(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	recordID, _ := ctx.Value(llmRecordIDKey{}).(int64)
	return recordID
}

func newLLMJSONInput(
	ctx context.Context,
	promptName string,
	promptText string,
	modelName string,
	inputText string,
	callReason string,
	callLoc string,
) llmclients.JSONExtractionInput {
	callReason = strings.TrimSpace(callReason)
	callLoc = strings.TrimSpace(callLoc)
	modelName = strings.TrimSpace(modelName)
	return llmclients.JSONExtractionInput{
		PromptName: llmclients.EnsurePromptName(strings.TrimSpace(promptName), callReason, callLoc, modelName),
		PromptText: promptText,
		ModelName:  modelName,
		InputText:  inputText,
		RecordID:   llmRecordIDFromContext(ctx),
		CallReason: callReason,
		CallLoc:    callLoc,
		// Document-first layout puts the repeated chunk/block input ahead of the
		// per-call task instructions so it forms a stable, cacheable prefix for
		// DeepSeek prompt caching. See ADR 2026062501 (DeepSeek prompt cache).
		DocumentFirst: true,
	}
}
