package agentservicehandler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestMain(m *testing.M) {
	// Fold synchronously so tests can observe it without racing the fakes.
	runInBackground = func(f func()) { f() }
	os.Exit(m.Run())
}

func strPtr(s string) *string { return &s }

func turnMessages(attempt string, userSeq int, question, answer, answerStatus string) []Message {
	return []Message{
		{ID: attempt + "-q", AttemptID: strPtr(attempt), SequenceNo: userSeq, Role: "user", Content: question, Status: "complete"},
		{ID: attempt + "-a", AttemptID: strPtr(attempt), SequenceNo: userSeq + 1, Role: "assistant", Content: answer, Status: answerStatus},
	}
}

func TestEstimateTokensCountsCJKPerCharacter(t *testing.T) {
	if got := estimateTokens("abcdefgh"); got != 2 {
		t.Fatalf("ascii estimate = %d", got)
	}
	if got := estimateTokens("拉伸强度"); got != 4 {
		t.Fatalf("cjk estimate = %d", got)
	}
}

func TestShortenForHistoryCutsByUTF16Units(t *testing.T) {
	if got := shortenForHistory("short", 100); got != "short" {
		t.Fatalf("short content changed: %q", got)
	}
	got := shortenForHistory(strings.Repeat("字", 50), 20)
	if utf16Len(got) > 20 || !strings.HasSuffix(got, truncatedMark) {
		t.Fatalf("shortened = %q (%d units)", got, utf16Len(got))
	}
	emoji := shortenForHistory(strings.Repeat("😀", 20), 20)
	if utf16Len(emoji) > 20 {
		t.Fatalf("emoji shortened to %d units", utf16Len(emoji))
	}
}

func TestSourceFooterDedupesAndLimits(t *testing.T) {
	sources := []SourceRecord{
		{DocumentID: "1", DocumentTitle: "GB 1234-2020", PageStart: 5, LineStart: 10, LineEnd: 20},
		{DocumentID: "1", DocumentTitle: "GB 1234-2020", PageStart: 5, LineStart: 10, LineEnd: 20},
		{DocumentID: "2", DocumentTitle: "", LineStart: 3, LineEnd: 4},
	}
	footer := sourceFooter(sources)
	if !strings.Contains(footer, "[Sources cited in this answer]\n- GB 1234-2020 (p. 5, lines 10–20)\n- document 2 (lines 3–4)") || strings.Count(footer, "\n- ") != 2 {
		t.Fatalf("footer = %q", footer)
	}
	many := make([]SourceRecord, 0, 12)
	for i := 1; i <= 12; i++ {
		many = append(many, SourceRecord{DocumentID: "d", DocumentTitle: "Doc", LineStart: i, LineEnd: i})
	}
	if got := strings.Count(sourceFooter(many), "\n- "); got != maxFooterSources {
		t.Fatalf("footer lists %d sources", got)
	}
	if sourceFooter(nil) != "" {
		t.Fatal("footer for no sources")
	}
}

func TestCompleteTurnsDropsUnansweredQuestions(t *testing.T) {
	var messages []Message
	messages = append(messages, turnMessages("a1", 1, "Q1", "A1", "complete")...)
	messages = append(messages, turnMessages("a2", 3, "Q2", "partial", "incomplete")...)
	messages = append(messages, Message{ID: "a3-q", AttemptID: strPtr("a3"), SequenceNo: 5, Role: "user", Content: "Q3 (answer hidden)", Status: "complete"})
	messages = append(messages, turnMessages("a4", 6, "Q4", "A4", "complete")...)
	sources := map[string][]SourceRecord{"a4-a": {{DocumentID: "9", DocumentTitle: "Pump manual", PageStart: 2}}}

	turns := completeTurns(messages, sources, nil, 0, 0)
	if len(turns) != 2 || turns[0].User != "Q1" || turns[1].User != "Q4" {
		t.Fatalf("turns = %+v", turns)
	}
	if !strings.HasPrefix(turns[1].Assistant, "A4\n\n[Sources cited in this answer]\n- Pump manual (p. 2)") {
		t.Fatalf("answer with footer = %q", turns[1].Assistant)
	}
	if after := completeTurns(messages, sources, nil, 2, 0); len(after) != 1 || after[0].User != "Q4" {
		t.Fatalf("turns after summary = %+v", after)
	}
}

func TestCompleteTurnsShortensLongAnswerWithFooter(t *testing.T) {
	messages := turnMessages("a1", 1, "Q", strings.Repeat("x", 20000), "complete")
	turns := completeTurns(messages, map[string][]SourceRecord{"a1-a": {{DocumentID: "1", DocumentTitle: "Doc"}}}, nil, 0, 0)
	if len(turns) != 1 || utf16Len(turns[0].Assistant) > maxHistoryContentUnits || !strings.Contains(turns[0].Assistant, truncatedMark+"\n\n[Sources cited") {
		t.Fatalf("long answer = %d units", utf16Len(turns[0].Assistant))
	}
}

func TestHistoryBudget(t *testing.T) {
	for _, tc := range []struct{ window, maxTokens, want int }{
		{200000, 32000, 32000},
		{1000000, 0, 32000},
		{200000, 4000, 4000},
		{16000, 32000, 4000},
		{0, 32000, 16000},
		{0, 8000, 8000},
		{4000, 32000, historyBudgetFloor},
	} {
		if got := historyBudget(tc.window, tc.maxTokens); got != tc.want {
			t.Errorf("historyBudget(%d, %d) = %d, want %d", tc.window, tc.maxTokens, got, tc.want)
		}
	}
}

func TestSelectHistoryKeepsNewestThatFit(t *testing.T) {
	turns := []historyTurn{{UserSeq: 1, Tokens: 50}, {UserSeq: 3, Tokens: 40}, {UserSeq: 5, Tokens: 30}}
	got := selectHistory(turns, 100, 20)
	if len(got) != 2 || got[0].UserSeq != 3 || got[1].UserSeq != 5 {
		t.Fatalf("selected = %+v", got)
	}
	many := make([]historyTurn, 60)
	if got := selectHistory(many, 1000, 0); len(got) != maxHistoryTurns {
		t.Fatalf("selected %d turns", len(got))
	}
}

func TestPlanFoldFoldsOldestUntilHalfBudget(t *testing.T) {
	turns := []historyTurn{{UserSeq: 1, Tokens: 40}, {UserSeq: 3, Tokens: 40}, {UserSeq: 5, Tokens: 40}}
	if fold := planFold(turns, 200); fold != nil {
		t.Fatalf("fold under budget = %+v", fold)
	}
	fold := planFold(turns, 100)
	if len(fold) != 2 || fold[1].UserSeq != 3 {
		t.Fatalf("fold = %+v", fold)
	}
}

func TestCompleteTurnsAttachToolCallsAndShortenLongResults(t *testing.T) {
	messages := turnMessages("a1", 1, "Q1", "A1", "complete")
	long := `{"items":[{"content":"` + strings.Repeat("x", 300) + `"}]}`
	results := map[string][]ToolResultRecord{"a1": {
		{ToolCallID: "t1", ToolName: "search_knowledge", Arguments: json.RawMessage(`{"query":"pump"}`), Result: `{"items":[]}`},
		{ToolCallID: "t2", ToolName: "read_source_passages", Arguments: json.RawMessage(`{"document_id":"4"}`), Result: long},
		{ToolCallID: "t3", ToolName: "get_artifact_details", Result: `{"error":"knowledge access denied"}`, IsError: true},
	}}
	turns := completeTurns(messages, nil, results, 0, 100)
	if len(turns) != 1 || len(turns[0].ToolCalls) != 3 {
		t.Fatalf("turns = %+v", turns)
	}
	calls := turns[0].ToolCalls
	if calls[0].Result != `{"untrusted_evidence":true,"evidence":{"items":[]}}` || string(calls[0].Arguments) != `{"query":"pump"}` {
		t.Fatalf("short result = %+v", calls[0])
	}
	var truncated map[string]any
	if err := json.Unmarshal([]byte(calls[1].Result), &truncated); err != nil {
		t.Fatalf("truncated result is not JSON: %v", err)
	}
	if truncated["truncated"] != true || truncated["tool_call_id"] != "t2" || truncated["retrieve_with"] != savedToolResultTool ||
		int(truncated["full_bytes"].(float64)) != len(long) || len(truncated["evidence_prefix"].(string)) != 100 {
		t.Fatalf("truncated = %+v", truncated)
	}
	if !calls[2].IsError || string(calls[2].Arguments) != `{}` {
		t.Fatalf("error call = %+v", calls[2])
	}
	if withoutCalls := completeTurns(messages, nil, nil, 0, 100); turns[0].Tokens <= withoutCalls[0].Tokens {
		t.Fatal("tool calls not counted toward the budget")
	}
}

func TestTitleFromMessage(t *testing.T) {
	if got := titleFromMessage("\n  Why is   flow low?\nmore detail"); got != "Why is flow low?" {
		t.Fatalf("title = %q", got)
	}
	long := titleFromMessage("What does GB 1234 require for tensile strength of the outer sheath at low temperatures?")
	if len([]rune(long)) > maxTitleRunes+1 || !strings.HasPrefix(long, "What does GB 1234 require") || !strings.HasSuffix(long, "…") {
		t.Fatalf("long title = %q", long)
	}
	if got := titleFromMessage(strings.Repeat("泵", 70)); got != strings.Repeat("泵", 60)+"…" {
		t.Fatalf("cjk title = %q", got)
	}
}

// runHistoryTurn sends one message through the run endpoint with the given
// conversation state and gateway, and returns them for inspection.
func runHistoryTurn(t *testing.T, store *fakeRunStore, gateway *fakeGatewayBridge) {
	t.Helper()
	withAgentUser(t, "user-1")
	if gateway.stream == "" {
		gateway.stream = `{"type":"answer_delta","text":"Answer."}` + "\n" + `{"type":"completion","status":"completed"}` + "\n"
	}
	signer, err := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	registry := testAgentProfileRegistry()
	registry.historySummaryPrompt = "Summarize."
	e := echo.New()
	RegisterRunRoutes(e.Group("/api/v1/agent-services"), NewRunHandler(store, registry, allowRunSources{}, gateway, signer))
	rec := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs", `{"message":"New question","idempotency_key":"key-1","permission_mode":"auto"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("run status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func historyConversation(title string, messages ...[]Message) ResumeState {
	state := ResumeState{Conversation: Conversation{ID: "conversation-1", OwnerUserID: "user-1", ServiceSlug: "knowledge-guide", ProfileSlug: "knowledge-guide", ProfileVersion: "v1", ModelName: "model-1", Title: title}}
	for _, turn := range messages {
		state.Messages = append(state.Messages, turn...)
	}
	return state
}

func TestRunSendsOnlyCompleteTurnsAndSetsTitleOnce(t *testing.T) {
	store := &fakeRunStore{created: true, state: historyConversation("",
		turnMessages("a1", 1, "Q1", "A1", "complete"),
		turnMessages("a2", 3, "Q2", "failed", "incomplete"))}
	gateway := &fakeGatewayBridge{window: 200000}
	runHistoryTurn(t, store, gateway)
	history := gateway.request.History
	if len(history) != 2 || history[0].Content != "Q1" || history[1].Content != "A1" {
		t.Fatalf("history = %+v", history)
	}
	if store.title != "New question" || store.titleSets != 1 {
		t.Fatalf("title = %q sets = %d", store.title, store.titleSets)
	}

	titled := &fakeRunStore{created: true, state: historyConversation("Existing")}
	runHistoryTurn(t, titled, &fakeGatewayBridge{window: 200000})
	if titled.titleSets != 0 {
		t.Fatalf("existing title was rewritten %d times", titled.titleSets)
	}
}

func TestRunSendsStoredSummaryWithLaterTurnsOnly(t *testing.T) {
	store := &fakeRunStore{created: true, summary: HistorySummary{Text: "Earlier: pumps.", ThroughSeq: 2}, state: historyConversation("t",
		turnMessages("a1", 1, "Q1", "A1", "complete"),
		turnMessages("a2", 3, "Q2", "A2", "complete"))}
	gateway := &fakeGatewayBridge{window: 200000}
	runHistoryTurn(t, store, gateway)
	if gateway.request.HistorySummary != "Earlier: pumps." || len(gateway.request.History) != 2 || gateway.request.History[0].Content != "Q2" {
		t.Fatalf("summary=%q history=%+v", gateway.request.HistorySummary, gateway.request.History)
	}
	if len(gateway.summarized) != 0 {
		t.Fatalf("summarized=%d", len(gateway.summarized))
	}
}

type revokedDocumentSources struct{}

func (revokedDocumentSources) CheckSourceWithGroups(context.Context, string, []string, []string, SourceRecord) error {
	return nil
}
func (revokedDocumentSources) CheckDocumentAccess(_ context.Context, _ string, _ []string, _ []string, documentID string) error {
	if documentID == "revoked" {
		return ErrKnowledgeAccessDenied
	}
	return nil
}

func TestRunKeepsSummaryAndAnswersAfterOldRevocation(t *testing.T) {
	withAgentUser(t, "user-1")
	store := &fakeRunStore{created: true, summary: HistorySummary{Text: "Mentions the revoked document.", ThroughSeq: 2}, state: historyConversation("t",
		turnMessages("a1", 1, "Q1", "A1", "complete"),
		turnMessages("a2", 3, "Q2", "A2", "complete"))}
	// Zero CreatedAt makes every record older than the snapshot period.
	store.sourceDeps = map[string][]SourceRecord{"a2-a": {{MessageID: "a2-a", DocumentID: "revoked", DocumentTitle: "Revoked doc"}, {MessageID: "a2-a", DocumentID: "ok", DocumentTitle: "Open doc"}}}
	store.toolResults = map[string][]ToolResultRecord{"a2": {
		{AttemptID: "a2", ToolCallID: "t1", ToolName: "search_knowledge", Result: `{"items":[]}`, DocumentIDs: []string{"revoked"}},
		{AttemptID: "a2", ToolCallID: "t2", ToolName: "search_knowledge", Result: `{"items":[]}`, DocumentIDs: []string{"ok"}},
	}}
	gateway := &fakeGatewayBridge{window: 200000, stream: `{"type":"answer_delta","text":"Answer."}` + "\n" + `{"type":"completion","status":"completed"}` + "\n"}
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	e := echo.New()
	RegisterRunRoutes(e.Group("/api/v1/agent-services"), NewRunHandler(store, testAgentProfileRegistry(), revokedDocumentSources{}, gateway, signer))
	rec := callAgentHandler(t, e, http.MethodPost, "/api/v1/agent-services/conversations/conversation-1/runs", `{"message":"New question","idempotency_key":"key-1","permission_mode":"auto"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("run status=%d", rec.Code)
	}
	history := gateway.request.History
	if gateway.request.HistorySummary != "Mentions the revoked document." || len(history) != 2 || history[1].Content == "" {
		t.Fatalf("summary=%q history=%+v", gateway.request.HistorySummary, history)
	}
	if !strings.HasPrefix(history[1].Content, "A2") || strings.Contains(history[1].Content, "Revoked doc") || !strings.Contains(history[1].Content, "Open doc") {
		t.Fatalf("answer = %q", history[1].Content)
	}
	if len(history[1].ToolCalls) != 1 || history[1].ToolCalls[0].ID != "t2" {
		t.Fatalf("tool calls = %+v", history[1].ToolCalls)
	}
	if !stringIn(gateway.request.Profile.AllowedTools, savedToolResultTool) {
		t.Fatalf("allowed tools = %v", gateway.request.Profile.AllowedTools)
	}
}

func TestCompletedRunFoldsOverflowingHistory(t *testing.T) {
	big := strings.Repeat("word ", 2000) // about 2,500 estimated tokens per message
	store := &fakeRunStore{created: true, state: historyConversation("t",
		turnMessages("a1", 1, big, big, "complete"),
		turnMessages("a2", 3, big, big, "complete"))}
	// A 4,000-token window gives the 2,000-token floor budget, so both turns overflow it.
	gateway := &fakeGatewayBridge{window: 4000, summaryText: "Folded summary."}
	runHistoryTurn(t, store, gateway)
	if len(gateway.request.History) != 0 {
		t.Fatalf("oversized turns were sent: %d messages", len(gateway.request.History))
	}
	if len(gateway.summarized) != 1 || gateway.summarized[0].SystemPrompt != "Summarize." || gateway.summarized[0].MaxTokens != 500 {
		t.Fatalf("summarize calls = %+v", gateway.summarized)
	}
	if store.savedSummary == nil || store.savedSummary.Text != "Folded summary." || store.savedSummary.ThroughSeq == 0 {
		t.Fatalf("saved summary = %+v", store.savedSummary)
	}
}

func TestFailedSummaryLeavesTurnCompleted(t *testing.T) {
	big := strings.Repeat("word ", 2000)
	store := &fakeRunStore{created: true, state: historyConversation("t", turnMessages("a1", 1, big, big, "complete"))}
	gateway := &fakeGatewayBridge{window: 4000}
	runHistoryTurn(t, store, gateway)
	if len(gateway.summarized) != 1 || store.savedSummary != nil || len(store.settled) != 1 || store.settled[0].Status != "completed" {
		t.Fatalf("summarized=%d saved=%+v settled=%+v", len(gateway.summarized), store.savedSummary, store.settled)
	}
}
