package agentservicehandler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

type fakeToolResultStore struct {
	saved   []ToolResultRecord
	owners  []string
	stored  map[string]ToolResultRecord
	lookups []string
}

func (f *fakeToolResultStore) SaveToolResult(_ context.Context, owner string, record ToolResultRecord) error {
	f.saved, f.owners = append(f.saved, record), append(f.owners, owner)
	return nil
}
func (f *fakeToolResultStore) LoadSavedToolResult(_ context.Context, owner, attemptID, toolCallID string) (ToolResultRecord, string, error) {
	f.lookups = append(f.lookups, owner+"/"+attemptID+"/"+toolCallID)
	record, ok := f.stored[toolCallID]
	if !ok || owner != "u" {
		return ToolResultRecord{}, "", sql.ErrNoRows
	}
	return record, "conversation-1", nil
}

type fakeDocumentAccess struct{ denied map[string]bool }

func (f fakeDocumentAccess) CheckDocumentAccess(_ context.Context, _ string, _ []string, _ []string, documentID string) error {
	if f.denied[documentID] {
		return ErrKnowledgeAccessDenied
	}
	return nil
}

func savedResultTestHandler(t *testing.T, results *fakeToolResultStore, documents fakeDocumentAccess, now time.Time) (*echo.Echo, string) {
	t.Helper()
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), func() time.Time { return now })
	claims := RunCapabilityClaims{UserID: "u", ProfileSlug: "knowledge-guide", ProfileVersion: "v1", RunID: "r",
		AllowedTools: append(append([]string(nil), knowledgeToolNames...), savedToolResultTool), KnowledgeStoreIDs: []string{"store-1"}, MaxEvidenceBytes: 65536}
	token, _ := signer.Mint(claims, time.Minute)
	registry := testAgentProfileRegistry()
	registry.snapshotWindow = 48 * time.Hour
	h := NewInternalToolHandler("gateway-secret", signer, NewKnowledgeToolService(&recordingBackend{}, &mutableAccessChecker{allowed: true})).
		WithToolResults(results, registry, documents)
	h.now = func() time.Time { return now }
	e := echo.New()
	RegisterInternalToolRoutes(e, h)
	return e, token
}

func callInternalTool(e *echo.Echo, token, tool, toolCallID string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/agent-tools/"+tool, bytes.NewReader(raw))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, "Bearer gateway-secret")
	req.Header.Set(HeaderRunID, "r")
	req.Header.Set(HeaderRunCapability, token)
	if toolCallID != "" {
		req.Header.Set(HeaderToolCallID, toolCallID)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestInternalToolStoresArgumentsAndExactResult(t *testing.T) {
	results := &fakeToolResultStore{}
	e, token := savedResultTestHandler(t, results, fakeDocumentAccess{}, time.Now())
	rec := callInternalTool(e, token, "read_source_passages", "toolu_1", ToolInput{KnowledgeStoreID: "store-1", DocumentID: "doc-9", Ranges: []LineRange{{Start: 1, End: 2}}})
	if rec.Code != http.StatusOK || len(results.saved) != 1 {
		t.Fatalf("status=%d saved=%d", rec.Code, len(results.saved))
	}
	record := results.saved[0]
	if record.AttemptID != "r" || record.ToolCallID != "toolu_1" || record.ToolName != "read_source_passages" || record.IsError || results.owners[0] != "u" {
		t.Fatalf("record = %+v", record)
	}
	if record.Result != rec.Body.String() {
		t.Fatalf("stored result differs from response:\n%s\n%s", record.Result, rec.Body.String())
	}
	if string(record.Arguments) != `{"knowledge_store_id":"store-1","document_id":"doc-9","ranges":[{"start":1,"end":2}]}` {
		t.Fatalf("arguments = %s", record.Arguments)
	}
	if len(record.DocumentIDs) != 2 || record.DocumentIDs[0] != "doc-9" || record.DocumentIDs[1] != "doc-1" {
		t.Fatalf("document ids = %v", record.DocumentIDs)
	}
}

func TestInternalToolStoresRejectedCallAsError(t *testing.T) {
	results := &fakeToolResultStore{}
	e, token := savedResultTestHandler(t, results, fakeDocumentAccess{}, time.Now())
	rec := callInternalTool(e, token, "search_knowledge", "toolu_2", ToolInput{Query: "pump", KnowledgeStoreID: "other-store"})
	if rec.Code != http.StatusForbidden || len(results.saved) != 1 || !results.saved[0].IsError || results.saved[0].Result != rec.Body.String() {
		t.Fatalf("status=%d saved=%+v", rec.Code, results.saved)
	}
	if rec := callInternalTool(e, token, "search_knowledge", "", ToolInput{Query: "pump", KnowledgeStoreID: "store-1"}); rec.Code != http.StatusOK || len(results.saved) != 1 {
		t.Fatalf("call without a tool-call id was stored: status=%d saved=%d", rec.Code, len(results.saved))
	}
}

func TestSavedToolResultReturnsStoredBodyUnderSnapshotRule(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	results := &fakeToolResultStore{stored: map[string]ToolResultRecord{
		"recent": {ToolCallID: "recent", Result: `{"items":[{"document_id":"revoked"}]}`, DocumentIDs: []string{"revoked"}, CreatedAt: now.Add(-time.Hour)},
		"old-ok": {ToolCallID: "old-ok", Result: `{"items":[{"document_id":"open"}]}`, DocumentIDs: []string{"open"}, CreatedAt: now.Add(-72 * time.Hour)},
		"old-no": {ToolCallID: "old-no", Result: `{"items":[{"document_id":"revoked"}]}`, DocumentIDs: []string{"revoked"}, CreatedAt: now.Add(-72 * time.Hour)},
	}}
	e, token := savedResultTestHandler(t, results, fakeDocumentAccess{denied: map[string]bool{"revoked": true}}, now)
	for _, tc := range []struct {
		id     string
		status int
	}{{"recent", http.StatusOK}, {"old-ok", http.StatusOK}, {"old-no", http.StatusNotFound}, {"other-conversation", http.StatusNotFound}} {
		rec := callInternalTool(e, token, savedToolResultTool, "toolu_"+tc.id, map[string]string{"tool_call_id": tc.id})
		if rec.Code != tc.status {
			t.Fatalf("%s: status=%d body=%s", tc.id, rec.Code, rec.Body.String())
		}
		if tc.status == http.StatusOK && rec.Body.String() != results.stored[tc.id].Result {
			t.Fatalf("%s: body=%s", tc.id, rec.Body.String())
		}
	}
	if results.lookups[0] != "u/r/recent" {
		t.Fatalf("lookup = %s", results.lookups[0])
	}
	// The retrieval itself is stored, carrying the original documents so the
	// snapshot rule applies to it too.
	if len(results.saved) != 4 || results.saved[0].ToolName != savedToolResultTool || results.saved[0].DocumentIDs[0] != "revoked" || !results.saved[2].IsError {
		t.Fatalf("saved = %+v", results.saved)
	}
	if rec := callInternalTool(e, token, savedToolResultTool, "toolu_bad", map[string]string{}); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing id status = %d", rec.Code)
	}
}
