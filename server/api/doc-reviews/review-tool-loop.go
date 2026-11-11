package docreviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/llm"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// defaultP3MaxToolTokens is the code-level token budget for a P3 tool-use
// reviewer's conversation loop (per window) when the config leaves
// max_tool_tokens unset (DR10c).
const defaultP3MaxToolTokens = 60000

const docReviewToolUseSystemPrompt = "You are a document review engine. Return strict JSON findings only unless you need to call an available tool."

var (
	ErrToolUseResponseUnparseable = errors.New("tool-use response was not parseable findings JSON")
	ErrToolUseBudgetExhausted     = errors.New("tool-use review investigation budget exhausted")
)

// docReviewCallInfo builds the callInfo JSON string passed into runToolUseReview.
// It merges the run_id from ctx (when present) with reviewer-supplied identifying
// fields (e.g. which window/block/artifact was under review) so that a row in
// llm_usage_event.metadata_json is enough to diagnose what a given call was for
// (ADR 2026070501).
func docReviewCallInfo(ctx context.Context, fields map[string]any) string {
	info := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		info[k] = v
	}
	if runID := llmRunIDFromContext(ctx); runID > 0 {
		info["run_id"] = runID
	}
	b, err := json.Marshal(info)
	if err != nil {
		return ""
	}
	return string(b)
}

// runToolUseReview runs the bounded tool-use conversation loop (DR10b) for one
// review unit (a window for StrategyChunk reviewers). It places the canonical
// document input first (DR8a prefix-cache layout), calls the tool-capable model
// with the bound tool definitions, executes any requested tool calls against the
// record-scoped registry, and repeats until the model returns findings or the
// turn/token budget is exhausted (an error). A single model response
// is treated as either findings or tool calls, never both.
func runToolUseReview(
	ctx context.Context,
	client LLMChatClient,
	modelName string,
	cfg ReviewerConfig,
	promptText string,
	userContext string,
	tools []ReviewTool,
	recordID int64,
	logger ApiTypes.JimoLogger,
	callReason string,
	callInfo string,
	callLoc string,
) ([]ReviewFinding, *LLMUsage, error) {
	findings, _, usage, err := runToolUseReviewWithPayload(
		ctx, client, modelName, cfg, promptText, userContext, tools, recordID, logger, callReason, callInfo, callLoc,
	)
	return findings, usage, err
}

// runToolUseReviewWithPayload is the 'Harness Loop', the same conversation loop as
// runToolUseReview but also returns the raw JSON payload of whichever model
// response ultimately produced the findings, so callers whose prompt asks
// for sibling top-level keys (e.g. the provisions reviewer's mandatory
// per-match `analyses`) can recover them without a second LLM call.
func runToolUseReviewWithPayload(
	ctx context.Context,
	client LLMChatClient,
	modelName string,
	cfg ReviewerConfig,
	_ string,
	userContext string,
	tools []ReviewTool,
	recordID int64,
	logger ApiTypes.JimoLogger,
	callReason string,
	callInfo string,
	callLoc string,
) ([]ReviewFinding, map[string]any, *LLMUsage, error) {
	toolByName := make(map[string]ReviewTool, len(tools))
	for _, t := range tools {
		toolByName[t.Name] = t
	}
	toolDefs := toolDefsFor(tools)

	var callMetadata map[string]any
	if strings.TrimSpace(callInfo) != "" {
		if err := json.Unmarshal([]byte(callInfo), &callMetadata); err != nil {
			logger.Error("tool-use call metadata is invalid", "record_id", recordID, "error", err)
			return nil, nil, nil, fmt.Errorf("invalid tool-use call metadata: %w", err)
		}
	}

	maxTurns := cfg.MaxToolTurns
	if maxTurns <= 0 {
		maxTurns = 1
	}
	maxTokens := cfg.MaxToolTokens
	if maxTokens <= 0 {
		maxTokens = defaultP3MaxToolTokens
	}

	messages := []LLMMessage{
		{Role: LLMRoleSystem, Content: docReviewToolUseSystemPrompt},
		{Role: LLMRoleUser, Content: userContext},
	}

	tokensUsed := 0
	aggregateUsage := &LLMUsage{}
	for turn := 0; turn < maxTurns; turn++ {
		if isCtxStopped(ctx) {
			logger.Error("tool-use review stopped", "record_id", recordID, "error", ErrPipelineStopped)
			return nil, nil, aggregateUsage, ErrPipelineStopped
		}
		if err := ctx.Err(); err != nil {
			logger.Error("tool-use review cancelled", "record_id", recordID, "error", err)
			return nil, nil, aggregateUsage, err
		}

		turnMetadata := maps.Clone(callMetadata)
		if turnMetadata == nil {
			turnMetadata = map[string]any{}
		}
		turnMetadata["turn_count"] = turn + 1
		if cfg.PromptRef != "" {
			turnMetadata["prompt_ref"] = cfg.PromptRef
		}
		turnMetadata["prompt_dir_env_var"] = "PROMPT_DIR"
		if promptDir := strings.TrimSpace(os.Getenv("PROMPT_DIR")); promptDir != "" {
			turnMetadata["prompt_dir"] = promptDir
		}

		resp, err := client.Complete(ctx, LLMRequest{
			Model:      modelName,
			UserID:     llmUserIDFromContext(ctx),
			PromptName: cfg.PromptRef,
			Messages:   messages,
			Tools:      toolDefs,
			ToolChoice: "auto",
			RecordID:   recordID,
			RunID:      llmRunIDFromContext(ctx),
			CallReason: callReason,
			CallLoc:    callLoc,
			Metadata:   turnMetadata,
		})
		if err != nil {
			logger.Error("tool-use LLM call failed", "record_id", recordID, "turn", turn, "error", err)
			return nil, nil, aggregateUsage, fmt.Errorf("(MID_26062595) tool-use LLM call failed: %w", err)
		}
		if resp == nil {
			err := errors.New("tool-use LLM returned no response")
			logger.Error("tool-use LLM call failed", "record_id", recordID, "turn", turn, "error", err)
			return nil, nil, aggregateUsage, err
		}
		logLoopUsage(callReason, logger, modelName, turn, resp.Usage)
		addUsage(aggregateUsage, resp.Usage)
		tokensUsed += usageTotalTokens(resp.Usage)

		// Tool calls take precedence: a response that requests tools is never
		// also treated as findings (the "never both" contract).
		if len(resp.ToolCalls) > 0 {
			messages = append(messages, LLMMessage{
				Role:      LLMRoleAssistant,
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			})
			if turn+1 >= maxTurns || tokensUsed >= maxTokens {
				logger.Error("tool-use review budget exhausted", "record_id", recordID, "turns_used", turn+1, "max_turns", maxTurns, "tokens_used", tokensUsed, "max_tokens", maxTokens)
				return nil, nil, aggregateUsage, fmt.Errorf("%w: record_id=%d turns=%d/%d tokens=%d/%d", ErrToolUseBudgetExhausted, recordID, turn+1, maxTurns, tokensUsed, maxTokens)
			}
			for _, tc := range resp.ToolCalls {
				if err := ctx.Err(); err != nil {
					logger.Error("tool-use review cancelled", "record_id", recordID, "error", err)
					return nil, nil, aggregateUsage, err
				}
				result, err := executeToolCall(ctx, tc, toolByName, recordID)
				if err != nil {
					logger.Error("tool-use tool call failed", "record_id", recordID, "turn", turn, "tool_name", tc.Name, "tool_call_id", tc.ID, "error", err)
					return nil, nil, aggregateUsage, fmt.Errorf("tool-use tool %s failed: %w", tc.Name, err)
				}
				messages = append(messages, LLMMessage{Role: LLMRoleTool, ToolCallID: tc.ID, Content: result})
			}
			continue
		}

		// No tool calls: the model is producing its final findings.
		findings, payload, ok, parseReason := parseFindingsContentDetailedWithPayload(resp.Content, modelName)
		if ok {
			logToolUseFindingsStatus(logger, "response", recordID, int(llmRunIDFromContext(ctx)), turn, findings)
			return findings, payload, aggregateUsage, nil
		}
		logger.Error("tool-use response returned invalid findings format", "record_id", recordID, "turn", turn, "parse_failure_reason", parseReason, "response_preview", previewToolLogText(resp.Content))
		return nil, nil, aggregateUsage, fmt.Errorf("%w: record_id=%d reason=%s", ErrToolUseResponseUnparseable, recordID, parseReason)
	}
	return nil, nil, aggregateUsage, ErrToolUseBudgetExhausted
}

// executeToolCall validates arguments and executes one tool. Every error aborts
// the affected reviewer rather than becoming tool content the model can retry.
func executeToolCall(ctx context.Context, tc LLMToolCall, toolByName map[string]ReviewTool, recordID int64) (string, error) {
	tool, ok := toolByName[tc.Name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", tc.Name)
	}
	args := map[string]any{}
	if strings.TrimSpace(tc.Arguments) != "" {
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			return "", fmt.Errorf("arguments are not valid JSON: %w", err)
		}
		if args == nil {
			return "", errors.New("arguments must be a JSON object")
		}
	}
	if len(tool.Parameters) > 0 {
		var schemaDoc any
		if err := json.Unmarshal(tool.Parameters, &schemaDoc); err != nil {
			return "", fmt.Errorf("invalid tool schema: %w", err)
		}
		compiler := jsonschema.NewCompiler()
		const schemaURL = "mem://review-tool.schema.json"
		if err := compiler.AddResource(schemaURL, schemaDoc); err != nil {
			return "", fmt.Errorf("invalid tool schema: %w", err)
		}
		schema, err := compiler.Compile(schemaURL)
		if err != nil {
			return "", fmt.Errorf("compile tool schema: %w", err)
		}
		if err := schema.Validate(args); err != nil {
			return "", fmt.Errorf("tool arguments fail schema validation: %w", err)
		}
	}
	if tool.Execute == nil {
		return "", errors.New("tool execution is not configured")
	}
	result, err := tool.Execute(ctx, recordID, args)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to encode tool result: %w", err)
	}
	return string(b), nil
}

func previewToolLogText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	const maxLen = 1000
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}

func logToolUseFindingsStatus(
	logger ApiTypes.JimoLogger,
	phase string,
	recordID int64,
	runID int,
	turn int,
	findings []ReviewFinding) {
	if len(findings) == 0 {
		logger.Info("tool-use returned no findings",
			"record_id", recordID,
			"run_id", runID,
			"phase", phase,
			"turn", turn,
		)
		return
	}
	logger.Info("tool-use returned findings",
		"record_id", recordID,
		"run_id", runID,
		"phase", phase,
		"turn", turn,
		"findings", len(findings),
	)
}

// toolDefsFor adapts the review tools to the shared llm tool-definition shape.
func toolDefsFor(tools []ReviewTool) []LLMToolDef {
	if len(tools) == 0 {
		return nil
	}
	defs := make([]LLMToolDef, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, LLMToolDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
	return defs
}

// parseFindingsContent extracts a {"findings":[...]} payload from model content,
// tolerating code fences and surrounding prose. The bool is true when a JSON
// object was successfully parsed (an empty findings array is a valid result).
func parseFindingsContent(content string) ([]ReviewFinding, bool) {
	findings, ok, _ := parseFindingsContentDetailed(content)
	return findings, ok
}

func parseFindingsContentDetailed(content string) ([]ReviewFinding, bool, string) {
	findings, _, ok, reason := parseFindingsContentDetailedWithPayload(content, "")
	return findings, ok, reason
}

// parseFindingsContentDetailedWithPayload is parseFindingsContentDetailed but
// also returns the unmarshaled payload map, so callers whose prompt requires
// sibling top-level keys alongside `findings` (e.g. the provisions reviewer's
// mandatory per-match `analyses`) can recover them.
func parseFindingsContentDetailedWithPayload(content string, modelName string) ([]ReviewFinding, map[string]any, bool, string) {
	obj, reason := extractJSONObjectDetailed(content)
	if obj == "" {
		return nil, nil, false, reason
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(obj), &payload); err != nil {
		// Common LLM JSON failures — unescaped inner quotes in Chinese text,
		// trailing commas, code fences — are fixed programmatically so we
		// avoid the expensive LLM repair round-trip for these trivial issues.
		if repaired, ok := repairMalformedFindingsJSON(content, obj); ok {
			if err2 := json.Unmarshal([]byte(repaired), &payload); err2 == nil {
				goto checkFindingsKey
			}
		}
		return nil, nil, false, "json_unmarshal_failed: " + err.Error()
	}
checkFindingsKey:
	rawFindings, ok := payload["findings"]
	if !ok {
		return nil, nil, false, "missing_findings_key"
	}
	findingList, ok := rawFindings.([]any)
	if !ok {
		return nil, nil, false, "findings_must_be_array"
	}
	for _, rawFinding := range findingList {
		finding, ok := rawFinding.(map[string]any)
		if !ok {
			return nil, nil, false, "finding_must_be_object"
		}
		title, ok := finding["title"].(string)
		if !ok || strings.TrimSpace(title) == "" {
			return nil, nil, false, "finding_missing_title"
		}
	}
	return normalizeFindingsJSON(payload, modelName), payload, true, ""
}

// repairMalformedFindingsJSON tries the shared library's JSON-repair heuristics
// (unescaped inner quotes, trailing commas, code fences) on the raw content and
// the already-extracted object, returning the repaired JSON and true on success.
func repairMalformedFindingsJSON(rawContent, extractedObj string) (string, bool) {
	// Try repairing the extracted object first (likely already the right span).
	if repaired, ok := llm.RepairLLMJSON(extractedObj); ok {
		return repaired, true
	}
	// Fall back to repairing the full raw content (e.g. the extraction may
	// have been fooled by a nested brace inside a string).
	if repaired, ok := llm.RepairLLMJSON(rawContent); ok {
		return repaired, true
	}
	return "", false
}

// extractJSONObject returns the outermost {...} span in s, stripping ```json
// fences and leading/trailing prose. Returns "" when no balanced object is found.
func extractJSONObject(s string) string {
	obj, _ := extractJSONObjectDetailed(s)
	return obj
}

func extractJSONObjectDetailed(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "empty_response"
	}
	if looksLikeTextToolCalls(s) {
		return "", "tool_calls_in_final_text"
	}
	if strings.HasPrefix(s, "```") {
		rest := s[3:]
		rest = strings.TrimPrefix(rest, "json")
		rest = strings.TrimPrefix(rest, "JSON")
		if j := strings.Index(rest, "```"); j >= 0 {
			rest = rest[:j]
		}
		s = strings.TrimSpace(rest)
	}
	s = strings.TrimSpace(strings.TrimSuffix(s, "```"))
	start := strings.Index(s, "{")
	if start < 0 {
		return "", "no_json_object_found"
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case c == '\\':
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
			// inside string literal; ignore braces
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1], ""
			}
		}
	}
	return "", "unbalanced_json_object"
}

func looksLikeTextToolCalls(s string) bool {
	return strings.Contains(s, "tool_calls") &&
		(strings.Contains(s, "<｜｜DSML｜｜") || strings.Contains(s, "<tool_call") || strings.Contains(s, "<tool_calls"))
}

// usageTotalTokens returns the total tokens for a call (0 when usage is absent).
func usageTotalTokens(u *LLMUsage) int {
	if u == nil {
		return 0
	}
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.InputTokens + u.OutputTokens
}

func addUsage(total *LLMUsage, u *LLMUsage) {
	if total == nil || u == nil {
		return
	}
	total.InputTokens += u.InputTokens
	total.OutputTokens += u.OutputTokens
	total.TotalTokens += usageTotalTokens(u)
	total.PromptCacheHitTokens += u.PromptCacheHitTokens
	total.PromptCacheMissTokens += u.PromptCacheMissTokens
	if u.EventID != "" {
		total.EventIDs = append(total.EventIDs, u.EventID)
	}
}

// logLoopUsage records per-call token usage including DeepSeek prompt-cache
// hit/miss counts (DR8a measurement). turn == -1 denotes the finalize call.
func logLoopUsage(
	callReason string,
	logger ApiTypes.JimoLogger,
	modelName string,
	turn int,
	u *LLMUsage) {
	if u == nil {
		return
	}
	logger.Info("tool-use llm call usage",
		"model", modelName,
		"turn", turn,
		"input_tokens", u.InputTokens,
		"output_tokens", u.OutputTokens,
		"total_tokens", u.TotalTokens,
		"callReason", callReason,
		"prompt_cache_hit_tokens", u.PromptCacheHitTokens,
		"prompt_cache_miss_tokens", u.PromptCacheMissTokens,
		"llm_usage_event_id", u.EventID,
	)
}
