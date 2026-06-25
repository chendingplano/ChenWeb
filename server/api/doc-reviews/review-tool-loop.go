package docreviews

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// defaultP3MaxToolTokens is the code-level token budget for a P3 tool-use
// reviewer's conversation loop (per window) when the config leaves
// max_tool_tokens unset (DR10c).
const defaultP3MaxToolTokens = 60000

// runToolUseReview runs the bounded tool-use conversation loop (DR10b) for one
// review unit (a window for StrategyChunk reviewers). It places the canonical
// document input first (DR8a prefix-cache layout), calls the tool-capable model
// with the bound tool definitions, executes any requested tool calls against the
// record-scoped registry, and repeats until the model returns findings or the
// turn/token budget is exhausted (then force-finalizes). A single model response
// is treated as either findings or tool calls, never both.
func runToolUseReview(
	ctx context.Context,
	client LLMChatClient,
	modelName string,
	cfg ReviewerConfig,
	systemPrompt string,
	userContext string,
	tools []ReviewTool,
	recordID int64,
	logger ApiTypes.JimoLogger,
) ([]ReviewFinding, error) {
	toolByName := make(map[string]ReviewTool, len(tools))
	for _, t := range tools {
		toolByName[t.Name] = t
	}
	toolDefs := toolDefsFor(tools)

	maxTurns := cfg.MaxToolTurns
	if maxTurns <= 0 {
		maxTurns = 1
	}
	maxTokens := cfg.MaxToolTokens
	if maxTokens <= 0 {
		maxTokens = defaultP3MaxToolTokens
	}

	messages := []LLMMessage{
		{Role: LLMRoleSystem, Content: systemPrompt},
		{Role: LLMRoleUser, Content: userContext},
	}

	tokensUsed := 0
	for turn := 0; turn < maxTurns; turn++ {
		if isCtxStopped(ctx) {
			return nil, ErrPipelineStopped
		}

		resp, err := client.Complete(ctx, LLMRequest{
			Model:      modelName,
			Messages:   messages,
			Tools:      toolDefs,
			ToolChoice: "auto",
			RecordID:   recordID,
			CallReason: "review_tool_use",
			CallLoc:    "MID-CWB-REVIEW-TOOL-LOOP",
		})
		if err != nil {
			return nil, fmt.Errorf("(MID_26062595) tool-use LLM call failed: %w", err)
		}
		logLoopUsage(logger, modelName, turn, resp.Usage)
		tokensUsed += usageTotalTokens(resp.Usage)

		// Tool calls take precedence: a response that requests tools is never
		// also treated as findings (the "never both" contract).
		if len(resp.ToolCalls) > 0 {
			messages = append(messages, LLMMessage{
				Role:      LLMRoleAssistant,
				Content:   resp.Content,
				ToolCalls: resp.ToolCalls,
			})
			for _, tc := range resp.ToolCalls {
				result := executeToolCall(ctx, tc, toolByName, recordID)
				messages = append(messages, LLMMessage{
					Role:       LLMRoleTool,
					ToolCallID: tc.ID,
					Content:    result,
				})
			}
			if maxTokens > 0 && tokensUsed >= maxTokens {
				logger.Info("tool-use token budget exhausted; finalizing",
					"record_id", recordID, "tokens_used", tokensUsed, "max_tokens", maxTokens)
				break
			}
			continue
		}

		// No tool calls: the model is producing its final findings.
		if findings, ok := parseFindingsContent(resp.Content); ok {
			return findings, nil
		}

		// Degenerate response (neither tool calls nor parseable findings):
		// one repair attempt asking for strict JSON, then give up for this unit.
		messages = append(messages, LLMMessage{Role: LLMRoleAssistant, Content: resp.Content})
		return finalizeFindings(ctx, client, modelName, recordID, messages, logger)
	}

	// Budget exhausted (turns or tokens): force the model to produce findings
	// from the evidence collected so far, without tools (DR10b force-produce).
	return finalizeFindings(ctx, client, modelName, recordID, messages, logger)
}

// finalizeFindings appends the force-produce instruction and makes one final
// model call without tools, returning the parsed findings (empty if still
// unparseable — no error, so the window simply contributes no findings).
func finalizeFindings(
	ctx context.Context,
	client LLMChatClient,
	modelName string,
	recordID int64,
	messages []LLMMessage,
	logger ApiTypes.JimoLogger,
) ([]ReviewFinding, error) {
	if isCtxStopped(ctx) {
		return nil, ErrPipelineStopped
	}
	messages = append(messages, LLMMessage{
		Role: LLMRoleUser,
		Content: "You have reached the maximum investigation budget. Based on the " +
			"evidence collected so far, return your final findings now as strict JSON " +
			`of the form {"findings": [...]}. Return {"findings": []} if there are no issues.`,
	})
	resp, err := client.Complete(ctx, LLMRequest{
		Model:      modelName,
		Messages:   messages,
		RecordID:   recordID,
		CallReason: "review_tool_use_finalize",
		CallLoc:    "MID-CWB-REVIEW-TOOL-LOOP-FINAL",
	})
	if err != nil {
		return nil, fmt.Errorf("(MID_26062596) tool-use finalize call failed: %w", err)
	}
	logLoopUsage(logger, modelName, -1, resp.Usage)
	if findings, ok := parseFindingsContent(resp.Content); ok {
		return findings, nil
	}
	logger.Warn("tool-use finalize produced no parseable findings", "record_id", recordID)
	return nil, nil
}

// executeToolCall validates a tool call's arguments against the tool's schema
// and runs it, returning a JSON string to feed back as the tool-role result. On
// any error (unknown tool, bad args, execution failure) it returns an error
// payload the model can read and retry from — it never aborts the loop.
func executeToolCall(
	ctx context.Context,
	tc LLMToolCall,
	toolByName map[string]ReviewTool,
	recordID int64,
) string {
	tool, ok := toolByName[tc.Name]
	if !ok {
		return toolErrorResult(fmt.Sprintf("unknown tool %q", tc.Name))
	}
	var args map[string]any
	if strings.TrimSpace(tc.Arguments) != "" {
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			return toolErrorResult(fmt.Sprintf("arguments are not valid JSON: %v", err))
		}
	}
	if missing := missingRequiredArgs(tool.Parameters, args); len(missing) > 0 {
		return toolErrorResult(fmt.Sprintf("missing required argument(s): %s", strings.Join(missing, ", ")))
	}
	result, err := tool.Execute(ctx, recordID, args)
	if err != nil {
		return toolErrorResult(err.Error())
	}
	b, err := json.Marshal(result)
	if err != nil {
		return toolErrorResult(fmt.Sprintf("failed to encode result: %v", err))
	}
	return string(b)
}

func toolErrorResult(msg string) string {
	b, _ := json.Marshal(map[string]any{"error": msg})
	return string(b)
}

// missingRequiredArgs returns the schema-required argument names absent from args.
func missingRequiredArgs(schema json.RawMessage, args map[string]any) []string {
	if len(schema) == 0 {
		return nil
	}
	var s struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return nil
	}
	var missing []string
	for _, r := range s.Required {
		if _, ok := args[r]; !ok {
			missing = append(missing, r)
		}
	}
	return missing
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
	obj := extractJSONObject(content)
	if obj == "" {
		return nil, false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(obj), &payload); err != nil {
		return nil, false
	}
	if _, ok := payload["findings"]; !ok {
		return nil, false
	}
	return normalizeFindingsJSON(payload), true
}

// extractJSONObject returns the outermost {...} span in s, stripping ```json
// fences and leading/trailing prose. Returns "" when no balanced object is found.
func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		rest = strings.TrimPrefix(rest, "JSON")
		if j := strings.Index(rest, "```"); j >= 0 {
			rest = rest[:j]
		}
		s = strings.TrimSpace(rest)
	}
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
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
				return s[start : i+1]
			}
		}
	}
	return ""
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

// logLoopUsage records per-call token usage including DeepSeek prompt-cache
// hit/miss counts (DR8a measurement). turn == -1 denotes the finalize call.
func logLoopUsage(logger ApiTypes.JimoLogger, modelName string, turn int, u *LLMUsage) {
	if u == nil {
		return
	}
	logger.Info("tool-use llm call usage",
		"model", modelName,
		"turn", turn,
		"input_tokens", u.InputTokens,
		"output_tokens", u.OutputTokens,
		"total_tokens", u.TotalTokens,
		"prompt_cache_hit_tokens", u.PromptCacheHitTokens,
		"prompt_cache_miss_tokens", u.PromptCacheMissTokens,
	)
}
