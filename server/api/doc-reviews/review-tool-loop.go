package docreviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/llm"
)

// defaultP3MaxToolTokens is the code-level token budget for a P3 tool-use
// reviewer's conversation loop (per window) when the config leaves
// max_tool_tokens unset (DR10c).
const defaultP3MaxToolTokens = 60000

const docReviewToolUseSystemPrompt = "You are a document review engine. Return strict JSON findings only unless you need to call an available tool."

var ErrToolUseFinalizeUnparseable = errors.New("tool-use finalize response was not parseable findings JSON")

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
	_ string,
	userContext string,
	tools []ReviewTool,
	recordID int64,
	logger ApiTypes.JimoLogger,
) ([]ReviewFinding, *LLMUsage, error) {
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
		{Role: LLMRoleSystem, Content: docReviewToolUseSystemPrompt},
		{Role: LLMRoleUser, Content: userContext},
	}

	tokensUsed := 0
	aggregateUsage := &LLMUsage{}
	for turn := 0; turn < maxTurns; turn++ {
		if isCtxStopped(ctx) {
			return nil, aggregateUsage, ErrPipelineStopped
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
			return nil, aggregateUsage, fmt.Errorf("(MID_26062595) tool-use LLM call failed: %w", err)
		}
		logLoopUsage(logger, modelName, turn, resp.Usage)
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
			for _, tc := range resp.ToolCalls {
				// logger.Info("tool-use call",
				// 	"record_id", recordID,
				// 	"turn", turn,
				// 	"tool_call_id", tc.ID,
				// 	"tool_name", tc.Name,
				// 	"arguments", previewToolLogText(tc.Arguments),
				// )
				result := executeToolCall(ctx, tc, toolByName, recordID)
				// logger.Info("tool-use result",
				// 	"record_id", recordID,
				// 	"turn", turn,
				// 	"tool_call_id", tc.ID,
				// 	"tool_name", tc.Name,
				// 	"result", previewToolLogText(result),
				// )
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
			logToolUseFindingsStatus(logger, "response", recordID, turn, findings)
			return findings, aggregateUsage, nil
		}

		// Degenerate response (neither tool calls nor parseable findings):
		// one repair attempt asking for strict JSON, then give up for this unit.
		messages = append(messages, LLMMessage{Role: LLMRoleAssistant, Content: resp.Content})
		findings, usage, err := finalizeFindings(ctx, client, modelName, recordID, messages, toolByName, logger)
		addUsage(aggregateUsage, usage)
		return findings, aggregateUsage, err
	}

	// Budget exhausted (turns or tokens): force the model to produce findings
	// from the evidence collected so far, without tools (DR10b force-produce).
	findings, usage, err := finalizeFindings(ctx, client, modelName, recordID, messages, toolByName, logger)
	addUsage(aggregateUsage, usage)
	return findings, aggregateUsage, err
}

// finalizeFindings appends the force-produce instruction and makes one final
// model call without tools, returning the parsed findings. A parseable
// `{"findings":[]}` is a normal "no issues found" outcome; an unparseable
// response is treated as an error because the model failed its output contract.
func finalizeFindings(
	ctx context.Context,
	client LLMChatClient,
	modelName string,
	recordID int64,
	messages []LLMMessage,
	toolByName map[string]ReviewTool,
	logger ApiTypes.JimoLogger,
) ([]ReviewFinding, *LLMUsage, error) {
	if isCtxStopped(ctx) {
		return nil, nil, ErrPipelineStopped
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
		return nil, nil, fmt.Errorf("(MID_26062596) tool-use finalize call failed: %w", err)
	}
	totalUsage := &LLMUsage{}
	addUsage(totalUsage, resp.Usage)
	logLoopUsage(logger, modelName, -1, resp.Usage)
	if findings, ok, _ := parseFindingsContentDetailed(resp.Content); ok {
		logToolUseFindingsStatus(logger, "finalize", recordID, -1, findings)
		return findings, totalUsage, nil
	}
	_, _, parseReason := parseFindingsContentDetailed(resp.Content)
	if shouldRepairFindingsJSON(parseReason) {
		repairFindings, repairUsage, repairErr := repairFinalFindingsJSON(
			ctx, client, modelName, recordID, messages, toolByName, resp.Content, parseReason, logger,
		)
		addUsage(totalUsage, repairUsage)
		if repairErr == nil {
			return repairFindings, totalUsage, nil
		}
		logger.Error("tool-use finalize repair failed",
			"record_id", recordID,
			"parse_failure_reason", repairErr.Error(),
			"response_preview", previewToolLogText(resp.Content),
		)
		return nil, totalUsage, repairErr
	}
	logger.Error("tool-use finalize returned invalid findings format",
		"record_id", recordID,
		"parse_failure_reason", parseReason,
		"response_preview", previewToolLogText(resp.Content),
	)
	return nil, totalUsage, fmt.Errorf("%w: record_id=%d reason=%s response=%q",
		ErrToolUseFinalizeUnparseable, recordID, parseReason, previewToolLogText(resp.Content))
}

func shouldRepairFindingsJSON(parseReason string) bool {
	return strings.HasPrefix(parseReason, "json_unmarshal_failed:") || parseReason == "tool_calls_in_final_text"
}

func repairFinalFindingsJSON(
	ctx context.Context,
	client LLMChatClient,
	modelName string,
	recordID int64,
	messages []LLMMessage,
	toolByName map[string]ReviewTool,
	badContent string,
	parseReason string,
	logger ApiTypes.JimoLogger,
) ([]ReviewFinding, *LLMUsage, error) {
	if isCtxStopped(ctx) {
		return nil, nil, ErrPipelineStopped
	}
	repairMessages := append([]LLMMessage(nil), messages...)
	if parseReason == "tool_calls_in_final_text" {
		textCalls := parseTextToolCalls(badContent)
		if len(textCalls) > 0 {
			// The model emitted its tool calls as raw DSML text rather than
			// structured tool_calls, so resp.ToolCalls was empty. Attach the
			// re-parsed calls to the assistant message as structured ToolCalls so
			// their IDs correlate with the role:"tool" results appended below.
			// Without this, the OpenAI-compatible API rejects the repair request
			// ("Messages with role 'tool' must be a response to a preceding message
			// with 'tool_calls'", HTTP 400). The raw DSML text is dropped from the
			// content to avoid the model re-emitting the same pseudo tool call.
			repairMessages = append(repairMessages, LLMMessage{Role: LLMRoleAssistant, ToolCalls: textCalls})
			for _, tc := range textCalls {
				result := executeToolCall(ctx, tc, toolByName, recordID)
				logger.Info("tool-use final text call executed",
					"record_id", recordID,
					"tool_call_id", tc.ID,
					"tool_name", tc.Name,
					"arguments", previewToolLogText(tc.Arguments),
					"result", previewToolLogText(result),
				)
				repairMessages = append(repairMessages, LLMMessage{
					Role:       LLMRoleTool,
					ToolCallID: tc.ID,
					Content:    result,
				})
			}
			repairMessages = append(repairMessages, LLMMessage{
				Role: LLMRoleUser,
				Content: "Use the tool results above as the final additional evidence. " +
					"Do not call tools again. Return strict JSON only with the exact shape " +
					`{"findings":[...]}. Return {"findings":[]} if the evidence does not support any finding.`,
			})
			return callFinalFindingsRepair(ctx, client, modelName, recordID, repairMessages, logger)
		}
	}
	repairMessages = append(repairMessages,
		LLMMessage{Role: LLMRoleAssistant, Content: badContent},
		LLMMessage{
			Role:    LLMRoleUser,
			Content: finalFindingsRepairPrompt(parseReason),
		},
	)
	return callFinalFindingsRepair(ctx, client, modelName, recordID, repairMessages, logger)
}

func callFinalFindingsRepair(
	ctx context.Context,
	client LLMChatClient,
	modelName string,
	recordID int64,
	repairMessages []LLMMessage,
	logger ApiTypes.JimoLogger,
) ([]ReviewFinding, *LLMUsage, error) {
	resp, err := client.Complete(ctx, LLMRequest{
		Model:      modelName,
		Messages:   repairMessages,
		RecordID:   recordID,
		CallReason: "review_tool_use_finalize_repair",
		CallLoc:    "MID-CWB-REVIEW-TOOL-LOOP-FINAL-REPAIR",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("(MID_26062597) tool-use finalize repair call failed: %w", err)
	}
	logLoopUsage(logger, modelName, -2, resp.Usage)
	if findings, ok, _ := parseFindingsContentDetailed(resp.Content); ok {
		logToolUseFindingsStatus(logger, "finalize_repair", recordID, -2, findings)
		return findings, resp.Usage, nil
	}
	_, _, repairReason := parseFindingsContentDetailed(resp.Content)
	return nil, resp.Usage, fmt.Errorf("%w: record_id=%d reason=%s response=%q",
		ErrToolUseFinalizeUnparseable, recordID, repairReason, previewToolLogText(resp.Content))
}

func parseTextToolCalls(content string) []LLMToolCall {
	invokeRe := regexp.MustCompile(`(?s)<｜｜DSML｜｜invoke[[:space:]]+name="([^"]+)">(.*?)</｜｜DSML｜｜invoke>`)
	paramRe := regexp.MustCompile(`(?s)<｜｜DSML｜｜parameter[[:space:]]+name="([^"]+)"[[:space:]]+string="([^"]+)">(.*?)</｜｜DSML｜｜parameter>`)
	matches := invokeRe.FindAllStringSubmatch(content, -1)
	calls := make([]LLMToolCall, 0, len(matches))
	for i, m := range matches {
		args := make(map[string]any)
		for _, pm := range paramRe.FindAllStringSubmatch(m[2], -1) {
			name := strings.TrimSpace(pm[1])
			isString := strings.EqualFold(strings.TrimSpace(pm[2]), "true")
			value := strings.TrimSpace(pm[3])
			if isString {
				args[name] = value
				continue
			}
			if n, err := strconv.Atoi(value); err == nil {
				args[name] = n
				continue
			}
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				args[name] = f
				continue
			}
			args[name] = value
		}
		b, _ := json.Marshal(args)
		calls = append(calls, LLMToolCall{
			ID:        fmt.Sprintf("final_text_tool_%d", i+1),
			Name:      strings.TrimSpace(m[1]),
			Arguments: string(b),
		})
	}
	return calls
}

func finalFindingsRepairPrompt(parseReason string) string {
	if parseReason == "tool_calls_in_final_text" {
		return "The previous response attempted to call tools, but tool use is now closed. " +
			"Do not request or mention tools. Based only on the evidence already available in this conversation, " +
			"return strict JSON only with the exact shape " + `{"findings":[...]}. ` +
			`Return {"findings":[]} if the available evidence does not support any finding.`
	}
	return "The previous response was not valid JSON: " + parseReason + ". " +
		"Rewrite the same findings as strict JSON only, with the exact shape " +
		`{"findings":[...]}. Preserve the finding text, escape quotes and newlines inside string values, ` +
		"and return no markdown fences or prose."
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

func logToolUseFindingsStatus(logger ApiTypes.JimoLogger, phase string, recordID int64, turn int, findings []ReviewFinding) {
	if len(findings) == 0 {
		logger.Info("tool-use returned no findings",
			"record_id", recordID,
			"phase", phase,
			"turn", turn,
		)
		return
	}
	logger.Info("tool-use returned findings",
		"record_id", recordID,
		"phase", phase,
		"turn", turn,
		"findings", len(findings),
	)
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
	findings, ok, _ := parseFindingsContentDetailed(content)
	return findings, ok
}

func parseFindingsContentDetailed(content string) ([]ReviewFinding, bool, string) {
	obj, reason := extractJSONObjectDetailed(content)
	if obj == "" {
		return nil, false, reason
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
		return nil, false, "json_unmarshal_failed: " + err.Error()
	}
checkFindingsKey:
	if _, ok := payload["findings"]; !ok {
		return nil, false, "missing_findings_key"
	}
	return normalizeFindingsJSON(payload), true, ""
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
