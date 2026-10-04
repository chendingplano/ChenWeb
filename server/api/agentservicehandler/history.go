package agentservicehandler

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// historySummaryPromptFile is the system prompt for folding earlier turns
// into a conversation's rolling summary.
const historySummaryPromptFile = "prompt-agent-history-summary-v1.md"

const (
	// maxHistoryTurns keeps the history within the gateway's 100-message limit.
	maxHistoryTurns = 50
	// maxHistoryContentUnits stays under the gateway's 16,000 UTF-16-unit
	// per-message limit.
	maxHistoryContentUnits = 15000
	maxFooterSources       = 8
	historyBudgetFloor     = 2000
	// historyBudgetFallback applies when the gateway cannot report the
	// model's context window.
	historyBudgetFallback = 16000
	// maxFoldBytes keeps a summary request well under the gateway's 512 KiB limit.
	maxFoldBytes  = 400 * 1024
	maxTitleRunes = 60
	truncatedMark = "…[truncated]"
)

func loadHistorySummaryPrompt(promptDir string) (string, error) {
	path := filepath.Join(promptDir, historySummaryPromptFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt %q: %w", path, err)
	}
	prompt := strings.TrimSpace(string(raw))
	if prompt == "" {
		return "", fmt.Errorf("prompt %q is empty", path)
	}
	return prompt, nil
}

func (r *ProfileRegistry) HistorySummaryPrompt() string {
	if r == nil {
		return ""
	}
	return r.historySummaryPrompt
}

// historyTurn is one complete question and answer, ready to send.
type historyTurn struct {
	UserSeq      int
	AssistantSeq int
	User         string
	Assistant    string
	Tokens       int
}

// estimateTokens is a conservative estimate without a tokenizer: about four
// ASCII bytes per token, and one token per non-ASCII character (CJK text is
// roughly one token per character).
func estimateTokens(s string) int {
	ascii, other := 0, 0
	for _, r := range s {
		if r < utf8.RuneSelf {
			ascii++
		} else {
			other++
		}
	}
	return (ascii+3)/4 + other
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// shortenForHistory cuts content so it is at most limit UTF-16 units long,
// marking the cut.
func shortenForHistory(content string, limit int) string {
	if utf16Len(content) <= limit {
		return content
	}
	keep := limit - utf16Len(truncatedMark)
	n := 0
	for i, r := range content {
		if n+utf16.RuneLen(r) > keep {
			return content[:i] + truncatedMark
		}
		n += utf16.RuneLen(r)
	}
	return content
}

// sourceFooter lists up to maxFooterSources distinct sources an answer cited,
// so a later turn knows what was cited without the retrieved passages.
func sourceFooter(sources []SourceRecord) string {
	lines := make([]string, 0, maxFooterSources)
	seen := make(map[string]bool)
	for _, source := range sources {
		key := fmt.Sprintf("%s|%d|%d|%d", source.DocumentID, source.PageStart, source.LineStart, source.LineEnd)
		if seen[key] {
			continue
		}
		seen[key] = true
		title := strings.TrimSpace(source.DocumentTitle)
		if title == "" {
			title = "document " + source.DocumentID
		}
		var where []string
		if source.PageStart > 0 {
			where = append(where, fmt.Sprintf("p. %d", source.PageStart))
		}
		if source.LineStart > 0 && source.LineEnd >= source.LineStart {
			where = append(where, fmt.Sprintf("lines %d–%d", source.LineStart, source.LineEnd))
		}
		if len(where) > 0 {
			title += " (" + strings.Join(where, ", ") + ")"
		}
		lines = append(lines, "- "+title)
		if len(lines) == maxFooterSources {
			break
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "\n\n[Sources cited in this answer]\n" + strings.Join(lines, "\n")
}

// completeTurns pairs each visible complete answer with the question of the
// same attempt. A question without such an answer is left out, so the model
// never sees two user messages in a row. Only turns after throughSeq (already
// summarized) are returned, oldest first.
func completeTurns(messages []Message, sourcesByMessage map[string][]SourceRecord, throughSeq int) []historyTurn {
	questions := make(map[string]Message)
	for _, message := range messages {
		if message.Role == "user" && message.Status == "complete" && message.AttemptID != nil {
			questions[*message.AttemptID] = message
		}
	}
	turns := make([]historyTurn, 0)
	for _, answer := range messages {
		if answer.Role != "assistant" || answer.Status != "complete" || answer.AttemptID == nil {
			continue
		}
		question, ok := questions[*answer.AttemptID]
		if !ok || question.SequenceNo <= throughSeq {
			continue
		}
		footer := sourceFooter(sourcesByMessage[answer.ID])
		turn := historyTurn{
			UserSeq: question.SequenceNo, AssistantSeq: answer.SequenceNo,
			User:      shortenForHistory(question.Content, maxHistoryContentUnits),
			Assistant: shortenForHistory(answer.Content, maxHistoryContentUnits-utf16Len(footer)) + footer,
		}
		turn.Tokens = estimateTokens(turn.User) + estimateTokens(turn.Assistant)
		turns = append(turns, turn)
	}
	sort.Slice(turns, func(i, j int) bool { return turns[i].UserSeq < turns[j].UserSeq })
	return turns
}

// historyBudget is the estimated-token budget for earlier turns: a quarter of
// the model's context window, never above the guide's cap.
func historyBudget(contextWindow, maxHistoryTokens int) int {
	if maxHistoryTokens <= 0 {
		maxHistoryTokens = defaultMaxHistoryTokens
	}
	limit := maxHistoryTokens
	if contextWindow > 0 {
		limit = min(limit, contextWindow/4)
	} else {
		limit = min(limit, historyBudgetFallback)
	}
	return max(historyBudgetFloor, limit)
}

// selectHistory keeps the newest turns that fit in the budget left after the
// summary, and returns them oldest first.
func selectHistory(turns []historyTurn, budget, summaryTokens int) []historyTurn {
	remaining := budget - summaryTokens
	start := len(turns)
	for start > 0 && len(turns)-start < maxHistoryTurns && turns[start-1].Tokens <= remaining {
		remaining -= turns[start-1].Tokens
		start--
	}
	return turns[start:]
}

func historyMessages(turns []historyTurn) []GatewayHistoryMessage {
	out := make([]GatewayHistoryMessage, 0, 2*len(turns))
	for _, turn := range turns {
		out = append(out,
			GatewayHistoryMessage{Role: "user", Content: turn.User},
			GatewayHistoryMessage{Role: "assistant", Content: turn.Assistant})
	}
	return out
}

// planFold returns the oldest unsummarized turns to fold into the summary
// when the unsummarized turns no longer fit the budget, folding until the
// rest fit in half of it. It returns nil when no fold is needed.
func planFold(turns []historyTurn, budget int) []historyTurn {
	total := 0
	for _, turn := range turns {
		total += turn.Tokens
	}
	if total <= budget {
		return nil
	}
	count, bytes := 0, 0
	for count < len(turns) && count < maxHistoryTurns && total > budget/2 {
		size := len(turns[count].User) + len(turns[count].Assistant)
		if count > 0 && bytes+size > maxFoldBytes {
			break
		}
		total -= turns[count].Tokens
		bytes += size
		count++
	}
	return turns[:count]
}

// summaryCoversHidden reports whether a hidden answer was folded into the
// summary, in which case the summary must not be used.
func summaryCoversHidden(messages []Message, hiddenIDs []string, throughSeq int) bool {
	if throughSeq == 0 || len(hiddenIDs) == 0 {
		return false
	}
	hidden := make(map[string]bool, len(hiddenIDs))
	for _, id := range hiddenIDs {
		hidden[id] = true
	}
	for _, message := range messages {
		if hidden[message.ID] && message.SequenceNo <= throughSeq {
			return true
		}
	}
	return false
}

// titleFromMessage is the first non-empty line of the first question, with
// whitespace collapsed, cut at maxTitleRunes.
func titleFromMessage(message string) string {
	for _, line := range strings.Split(message, "\n") {
		title := strings.Join(strings.Fields(line), " ")
		if title == "" {
			continue
		}
		if utf8.RuneCountInString(title) <= maxTitleRunes {
			return title
		}
		return strings.TrimSpace(string([]rune(title)[:maxTitleRunes])) + "…"
	}
	return ""
}
