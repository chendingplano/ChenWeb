package agentservicehandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
)

type mutableAccessChecker struct{ allowed bool }

func (m *mutableAccessChecker) CheckKnowledgeAccess(context.Context, KnowledgeAccessRequest) error {
	if !m.allowed {
		return ErrKnowledgeAccessDenied
	}
	return nil
}

type captureAccessChecker struct {
	request KnowledgeAccessRequest
}

func (c *captureAccessChecker) CheckKnowledgeAccess(_ context.Context, req KnowledgeAccessRequest) error {
	c.request = req
	return nil
}

func TestInternalToolEndpointsRequireGatewayAndRunCapability(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), func() time.Time { return now })
	claims := RunCapabilityClaims{UserID: "u", ProfileSlug: "p", ProfileVersion: "v1", RunID: "r", AllowedTools: append([]string(nil), knowledgeToolNames...), KnowledgeStoreIDs: []string{"store-1"}, MaxEvidenceBytes: 65536}
	token, _ := signer.Mint(claims, time.Minute)
	backend := &recordingBackend{}
	h := NewInternalToolHandler("gateway-secret", signer, NewKnowledgeToolService(backend, &mutableAccessChecker{allowed: true}))
	e := echo.New()
	RegisterInternalToolRoutes(e, h)

	for _, tool := range knowledgeToolNames {
		input := ToolInput{Query: "test", KnowledgeStoreID: "store-1", DocumentID: "doc-1", ArtifactID: "artifact-1", Ranges: []LineRange{{Start: 1, End: 2}}}
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/internal/agent-tools/"+tool, bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s without auth status = %d", tool, rec.Code)
		}

		req = httptest.NewRequest(http.MethodPost, "/api/internal/agent-tools/"+tool, bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set(echo.HeaderAuthorization, "Bearer gateway-secret")
		req.Header.Set(HeaderRunID, "r")
		req.Header.Set(HeaderRunCapability, token)
		rec = httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s authenticated status = %d body=%s", tool, rec.Code, rec.Body.String())
		}
	}
}

type recordingBackend struct{ calls int }

func (b *recordingBackend) Execute(_ context.Context, _ string, _ ToolInput) (ToolOutput, error) {
	b.calls++
	return ToolOutput{Items: []EvidenceItem{{DocumentID: "doc-1", KnowledgeStoreID: "store-1", UntrustedEvidence: true}}}, nil
}

func TestToolServiceRechecksAccessAndRejectsRevokedSource(t *testing.T) {
	checker := &mutableAccessChecker{allowed: true}
	backend := &recordingBackend{}
	service := NewKnowledgeToolService(backend, checker)
	claims := RunCapabilityClaims{UserID: "u", ProfileSlug: "p", ProfileVersion: "v1", RunID: "r", AllowedTools: []string{"get_document_context"}, KnowledgeStoreIDs: []string{"store-1"}, DocumentIDs: []string{"doc-1"}, MaxEvidenceBytes: 65536}
	input := ToolInput{KnowledgeStoreID: "store-1", DocumentID: "doc-1"}
	if _, err := service.Execute(context.Background(), claims, "get_document_context", input); err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	checker.allowed = false
	if _, err := service.Execute(context.Background(), claims, "get_document_context", input); !errors.Is(err, ErrKnowledgeAccessDenied) {
		t.Fatalf("revoked Execute() error = %v", err)
	}
	if backend.calls != 1 {
		t.Fatalf("backend calls = %d, want 1", backend.calls)
	}
}

func TestToolServiceEnforcesCapabilityScopeBeforeBackend(t *testing.T) {
	checker := &mutableAccessChecker{allowed: true}
	backend := &recordingBackend{}
	service := NewKnowledgeToolService(backend, checker)
	claims := RunCapabilityClaims{UserID: "u", ProfileSlug: "p", ProfileVersion: "v1", RunID: "r", AllowedTools: []string{"search_knowledge"}, KnowledgeStoreIDs: []string{"store-1"}, DocumentIDs: []string{"doc-1"}, MaxEvidenceBytes: 65536}
	_, err := service.Execute(context.Background(), claims, "search_knowledge", ToolInput{Query: "q", KnowledgeStoreID: "store-2"})
	if !errors.Is(err, ErrKnowledgeAccessDenied) || backend.calls != 0 {
		t.Fatalf("Execute() = err %v, backend calls %d", err, backend.calls)
	}
}

func TestToolServiceRejectsOutOfScopeAndOversizedBackendOutput(t *testing.T) {
	checker := &mutableAccessChecker{allowed: true}
	claims := RunCapabilityClaims{UserID: "u", ProfileSlug: "p", ProfileVersion: "v1", RunID: "r", AllowedTools: []string{"search_knowledge"}, KnowledgeStoreIDs: []string{"store-1"}, DocumentIDs: []string{"doc-1"}, MaxEvidenceBytes: 65536}

	backendOutput := ToolOutput{Items: []EvidenceItem{{KnowledgeStoreID: "store-1", DocumentID: "doc-2"}}}
	service := NewKnowledgeToolService(backendFunc(func(context.Context, string, ToolInput) (ToolOutput, error) { return backendOutput, nil }), checker)
	if _, err := service.Execute(context.Background(), claims, "search_knowledge", ToolInput{Query: "q", KnowledgeStoreID: "store-1", DocumentID: "doc-1"}); !errors.Is(err, ErrKnowledgeAccessDenied) {
		t.Fatalf("out-of-scope output error = %v", err)
	}

	large := json.RawMessage(`{"text":"` + strings.Repeat("x", MaxToolResponseBytes) + `"}`)
	service = NewKnowledgeToolService(backendFunc(func(context.Context, string, ToolInput) (ToolOutput, error) {
		return ToolOutput{Items: []EvidenceItem{{KnowledgeStoreID: "store-1", DocumentID: "doc-1", Content: large}}}, nil
	}), checker)
	if _, err := service.Execute(context.Background(), claims, "search_knowledge", ToolInput{Query: "q", KnowledgeStoreID: "store-1", DocumentID: "doc-1"}); !errors.Is(err, ErrToolResponseTooLarge) {
		t.Fatalf("oversized output error = %v", err)
	}
}

func TestToolServiceEnforcesProfileEvidenceLimit(t *testing.T) {
	claims := RunCapabilityClaims{UserID: "u", ProfileSlug: "p", ProfileVersion: "v1", RunID: "r", AllowedTools: []string{"search_knowledge"}, KnowledgeStoreIDs: []string{"7"}, MaxEvidenceBytes: 1024}
	large := json.RawMessage(`{"text":"` + strings.Repeat("x", 1500) + `"}`)
	service := NewKnowledgeToolService(backendFunc(func(context.Context, string, ToolInput) (ToolOutput, error) {
		return ToolOutput{Items: []EvidenceItem{{KnowledgeStoreID: "7", DocumentID: "42", Content: large}}}, nil
	}), &mutableAccessChecker{allowed: true})
	if _, err := service.Execute(context.Background(), claims, "search_knowledge", ToolInput{Query: "q", KnowledgeStoreID: "7"}); !errors.Is(err, ErrToolResponseTooLarge) {
		t.Fatalf("profile evidence limit error = %v", err)
	}
}

type backendFunc func(context.Context, string, ToolInput) (ToolOutput, error)

func (f backendFunc) Execute(ctx context.Context, tool string, input ToolInput) (ToolOutput, error) {
	return f(ctx, tool, input)
}

func TestInternalToolEndpointRejectsMalformedBearerAndTrailingJSON(t *testing.T) {
	signer, _ := NewCapabilitySigner([]byte("0123456789abcdef0123456789abcdef"), time.Now)
	claims := RunCapabilityClaims{UserID: "u", ProfileSlug: "p", ProfileVersion: "v1", RunID: "r", AllowedTools: []string{"search_knowledge"}, KnowledgeStoreIDs: []string{"store-1"}, MaxEvidenceBytes: 65536}
	token, _ := signer.Mint(claims, time.Minute)
	h := NewInternalToolHandler("gateway-secret", signer, NewKnowledgeToolService(&recordingBackend{}, &mutableAccessChecker{allowed: true}))
	e := echo.New()
	RegisterInternalToolRoutes(e, h)
	for _, tc := range []struct {
		auth, body string
		want       int
	}{
		{"gateway-secret", `{"query":"q","knowledge_store_id":"store-1"}`, http.StatusUnauthorized},
		{"Bearer  gateway-secret", `{"query":"q","knowledge_store_id":"store-1"}`, http.StatusUnauthorized},
		{"Bearer gateway-secret", `{"query":"q","knowledge_store_id":"store-1"}{}`, http.StatusBadRequest},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/internal/agent-tools/search_knowledge", strings.NewReader(tc.body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set(echo.HeaderAuthorization, tc.auth)
		req.Header.Set(HeaderRunID, "r")
		req.Header.Set(HeaderRunCapability, token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("auth=%q status=%d want=%d body=%s", tc.auth, rec.Code, tc.want, rec.Body.String())
		}
	}
}

func TestProfileAccessCheckerRevalidatesVersionAndScope(t *testing.T) {
	registry := &ProfileRegistry{versions: map[string]map[string]PiProfile{"p": {"v1": {Slug: "p", Version: "v1", Enabled: true, AllowedTools: []string{"search_knowledge"}, AllowedKnowledgeStores: []string{"store-1"}, AllowedDocumentGroups: []string{"published"}}}}, active: map[string]string{"p": "v1"}}
	next := &captureAccessChecker{}
	checker := ProfileAccessChecker{Registry: registry, Next: next}
	req := KnowledgeAccessRequest{UserID: "u", ProfileSlug: "p", ProfileVersion: "v1", ToolName: "search_knowledge", KnowledgeStoreID: "7", DocumentGroup: "published"}
	if err := checker.CheckKnowledgeAccess(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(next.request.AllowedKnowledgeStoreNames) != 1 || next.request.AllowedKnowledgeStoreNames[0] != "store-1" {
		t.Fatalf("profile store names not forwarded: %+v", next.request)
	}
	p := registry.versions["p"]["v1"]
	p.Enabled = false
	registry.versions["p"]["v1"] = p
	if err := checker.CheckKnowledgeAccess(context.Background(), req); !errors.Is(err, ErrKnowledgeAccessDenied) {
		t.Fatalf("disabled profile error=%v", err)
	}
}

func TestActiveKnowledgeAccessCheckerRequiresCurrentUserGrant(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	req := KnowledgeAccessRequest{UserID: "user-1", KnowledgeStoreID: "7", DocumentID: "42", DocumentGroup: "manual", AllowedKnowledgeStoreNames: []string{"Research"}}
	mock.ExpectQuery(`(?s)FROM kb\.agentic_knowledge_grants g.*g\.user_id=\$1.*ks\.id::text=\$2.*g\.document_id=i\.id`).
		WithArgs("user-1", "7", "42", "manual", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	if err := (ActiveKnowledgeAccessChecker{DB: db}).CheckKnowledgeAccess(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLKnowledgeBackendSearchShapesBoundedEvidence(t *testing.T) {
	t.Setenv("SEARCH_LEXICAL_BACKEND", "postgres")
	t.Setenv("SEARCH_SEMANTIC_ENABLED", "false")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	searchColumns := []string{"artifact_type", "artifact_id", "input_record_id", "primary_label", "secondary_label", "source_title", "source_filename", "source_line_spans", "semantic_payload", "keywords", "score", "snippet"}
	mock.ExpectQuery(`(?s)FROM kb\.search_artifacts sa.*agent_store\.id::text=\$2.*agent_input\.type=\$3.*LIMIT \$4 OFFSET \$5`).
		WithArgs("pump", "7", "manual", 2, 0).
		WillReturnRows(sqlmock.NewRows(searchColumns).AddRow("metric", "metric-7", int64(42), "Flow", "Nominal", "Pump guide", "pump.pdf", []byte(`[{"page":3,"line_start":10,"line_end":12}]`), []byte(`{"validation_status":"reviewed"}`), "{pump,flow}", 1.0, "Pump flow"))
	mock.ExpectQuery(`(?s)FROM kb\.search_artifacts sa.*sa\.artifact_type=\$1.*ks\.id::text=\$4`).
		WithArgs("metric", "metric-7", int64(42), "7", "manual").
		WillReturnRows(sqlmock.NewRows([]string{"ks_id", "type", "source_title", "source_version", "source_fingerprint", "validation_status"}).
			AddRow("7", "manual", "Pump guide", "2026-09-14", "abc123", "reviewed"))
	out, err := NewSQLKnowledgeToolBackend(db).Execute(context.Background(), "search_knowledge", ToolInput{Query: "pump", KnowledgeStoreID: "7", DocumentGroup: "manual", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].DocumentID != "42" || out.Items[0].Page != 3 || out.Items[0].LineStart != 10 || !out.Items[0].UntrustedEvidence {
		t.Fatalf("unexpected evidence: %+v", out.Items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLKnowledgeBackendReadsOnlyRequestedLines(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dir := t.TempDir()
	resultPath := filepath.Join(dir, "document.json")
	rawPath := filepath.Join(dir, "document.txt")
	data := "1\t1\tbody\tArial\t10\t[0,0,1,1]\tfirst\n2\t1\tbody\tArial\t10\t[0,0,1,1]\tsecond\n3\t2\tbody\tArial\t10\t[0,0,1,1]\tthird\n"
	if err := os.WriteFile(rawPath, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT i.result_filename, COALESCE(i.title,''), COALESCE(i.type,''), ks.id::text, i.id::text`)).
		WithArgs("7", "42", "manual").
		WillReturnRows(sqlmock.NewRows([]string{"result_filename", "title", "type", "ks_id", "id", "source_version", "source_fingerprint"}).AddRow(resultPath, "Guide", "manual", "7", "42", "2026-09-14", "abc123"))
	out, err := NewSQLKnowledgeToolBackend(db).Execute(context.Background(), "read_source_passages", ToolInput{KnowledgeStoreID: "7", DocumentGroup: "manual", DocumentID: "42", Ranges: []LineRange{{Start: 2, End: 2}}, Limit: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].LineStart != 2 || strings.Contains(string(out.Items[0].Content), "first") || !strings.Contains(string(out.Items[0].Content), "second") {
		t.Fatalf("unexpected passage: %+v", out.Items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFirstLocationSupportsRepositorySpanShapes(t *testing.T) {
	for _, tc := range []struct {
		raw              string
		page, start, end int
	}{
		{`["10:11"]`, 0, 10, 11},
		{`[{"page_number":3,"line_number":9}]`, 3, 9, 9},
		{`[{"page":4,"line_start":12,"line_end":14}]`, 4, 12, 14},
	} {
		page, start, end := firstLocation(json.RawMessage(tc.raw))
		if page != tc.page || start != tc.start || end != tc.end {
			t.Fatalf("firstLocation(%s)=(%d,%d,%d), want (%d,%d,%d)", tc.raw, page, start, end, tc.page, tc.start, tc.end)
		}
	}
}
