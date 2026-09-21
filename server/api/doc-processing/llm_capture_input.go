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

type llmRunIDKey struct{}

func withLLMRunID(ctx context.Context, runID int64) context.Context {
	if runID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, llmRunIDKey{}, runID)
}

func llmRunIDFromContext(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	runID, _ := ctx.Value(llmRunIDKey{}).(int64)
	return runID
}

type llmUserIDKey struct{}

// withLLMUserID tags ctx with the user_id carried on the triggering event
// (LineFileGeneratedEvent.UserID / StartDocProcessingEvent.UserID), so every
// processor's LLM calls made through newLLMJSONInput stamp
// llm_usage_event.user_id -- mirroring withLLMRunID above. The event
// generator (jetstreamhandler.PublishEvent, etc.) is responsible for
// supplying the user_id in the first place; this only extracts it.
func withLLMUserID(ctx context.Context, userID string) context.Context {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, llmUserIDKey{}, userID)
}

func llmUserIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	userID, _ := ctx.Value(llmUserIDKey{}).(string)
	return userID
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
		UserID:     llmUserIDFromContext(ctx),
		RecordID:   llmRecordIDFromContext(ctx),
		RunID:      llmRunIDFromContext(ctx),
		CallReason: callReason,
		CallLoc:    callLoc,
		// Document-first layout puts the repeated chunk/block input ahead of the
		// per-call task instructions so it forms a stable, cacheable prefix for
		// DeepSeek prompt caching. See ADR 2026062501 (DeepSeek prompt cache).
		DocumentFirst: true,
	}
}
