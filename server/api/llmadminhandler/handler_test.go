package llmadminhandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/llmimport"
	"github.com/labstack/echo/v4"
)

type stubAdminStore struct {
	listAccountsResult   []Account
	listAccountsErr      error
	createAccountResult  Account
	createAccountErr     error
	lastCreateAccountReq CreateAccountInput
	updateAccountResult  Account
	updateAccountErr     error
	lastUpdateAccountID  string
	lastUpdateAccountReq CreateAccountInput
	importResult         ImportResult
	importErr            error
	lastImportParsed     int
}

func (s *stubAdminStore) ListAccounts(_ context.Context) ([]Account, error) {
	return s.listAccountsResult, s.listAccountsErr
}

func (s *stubAdminStore) CreateAccount(_ context.Context, in CreateAccountInput) (Account, error) {
	s.lastCreateAccountReq = in
	return s.createAccountResult, s.createAccountErr
}

func (s *stubAdminStore) UpdateAccount(_ context.Context, id string, in CreateAccountInput) (Account, error) {
	s.lastUpdateAccountID = id
	s.lastUpdateAccountReq = in
	return s.updateAccountResult, s.updateAccountErr
}

func (s *stubAdminStore) ImportParsedModels(_ context.Context, parsed llmimport.ParsedModels) (ImportResult, error) {
	s.lastImportParsed = len(parsed.Accounts) + len(parsed.Profiles)
	return s.importResult, s.importErr
}

func (s *stubAdminStore) ListProfiles(_ context.Context) ([]ModelProfile, error) {
	return nil, nil
}

func (s *stubAdminStore) CreateProfile(_ context.Context, _ CreateProfileInput) (ModelProfile, error) {
	return ModelProfile{}, nil
}

func (s *stubAdminStore) UpdateProfile(_ context.Context, _ string, _ CreateProfileInput) (ModelProfile, error) {
	return ModelProfile{}, nil
}

func (s *stubAdminStore) UpsertAccountAndProfile(_ context.Context, _ CreateAccountInput, _ CreateProfileInput) (ModelProfile, error) {
	return ModelProfile{}, nil
}

func TestImportModelsTOMLPreviewReturnsParsedAccountsAndProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	modelsPath := filepath.Join(tmpDir, ".models.toml")
	raw := `
[deepseek-v4-flash]
host = "cloud"
model_name = "deepseek-v4-flash"
api_key = "sk-deepseek"
base_url = "https://api.deepseek.com"
timeout_sec = 200

[deepseek-v4-pro]
host = "cloud"
model_name = "deepseek-v4-pro"
api_key = "sk-deepseek"
base_url = "https://api.deepseek.com"
timeout_sec = 300
`
	if err := os.WriteFile(modelsPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("CHENWEB_MODELS_TOML", modelsPath)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/llm/accounts/import-models-toml", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ImportModelsTOMLPreview(c); err != nil {
		t.Fatalf("ImportModelsTOMLPreview() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"accounts"`) || !strings.Contains(body, `"profiles"`) {
		t.Fatalf("unexpected body = %s", body)
	}
	if !strings.Contains(body, `deepseek-v4-flash`) || !strings.Contains(body, `deepseek-v4-pro`) {
		t.Fatalf("expected profile names in body = %s", body)
	}
}

func TestListAccountsReturnsStoreRows(t *testing.T) {
	prev := adminStoreFactory
	t.Cleanup(func() { adminStoreFactory = prev })
	adminStoreFactory = func() accountAdminStore {
		return &stubAdminStore{
			listAccountsResult: []Account{{
				ID:           "acct_1",
				AccountName:  "DeepSeek Prod",
				Provider:     "deepseek",
				ProfileCount: 2,
			}},
		}
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/llm/accounts", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListAccounts(c); err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"DeepSeek Prod"`) {
		t.Fatalf("unexpected body = %s", rec.Body.String())
	}
}

func TestCreateAccountPersistsRequest(t *testing.T) {
	prev := adminStoreFactory
	t.Cleanup(func() { adminStoreFactory = prev })
	store := &stubAdminStore{
		createAccountResult: Account{
			ID:          "acct_1",
			AccountName: "DeepSeek Prod",
			Provider:    "deepseek",
		},
	}
	adminStoreFactory = func() accountAdminStore { return store }

	payload := map[string]any{
		"account_name":              "DeepSeek Prod",
		"provider":                  "deepseek",
		"base_url":                  "https://api.deepseek.com",
		"api_key":                   "sk-deepseek",
		"status":                    "active",
		"reconciliation_kind":       "deepseek_balance",
		"is_reconciliation_enabled": true,
		"default_model_name":        "deepseek-v4-flash",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/llm/accounts", strings.NewReader(string(body)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := CreateAccount(c); err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if store.lastCreateAccountReq.AccountName != "DeepSeek Prod" || store.lastCreateAccountReq.APIKeyRef != "sk-deepseek" {
		t.Fatalf("unexpected create request = %+v", store.lastCreateAccountReq)
	}
}

func TestUpdateAccountPersistsRequest(t *testing.T) {
	prev := adminStoreFactory
	t.Cleanup(func() { adminStoreFactory = prev })
	store := &stubAdminStore{
		updateAccountResult: Account{
			ID:                      "acct_1",
			AccountName:             "DeepSeek Prod",
			Provider:                "deepseek",
			IsReconciliationEnabled: true,
		},
	}
	adminStoreFactory = func() accountAdminStore { return store }

	payload := map[string]any{
		"account_name":              "DeepSeek Prod",
		"provider":                  "deepseek",
		"base_url":                  "https://api.deepseek.com",
		"api_key":                   "sk-updated",
		"status":                    "active",
		"reconciliation_kind":       "provider_balance",
		"is_reconciliation_enabled": true,
		"default_model_name":        "deepseek-v4-flash",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/llm/accounts/acct_1", strings.NewReader(string(body)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("acct_1")

	if err := UpdateAccount(c); err != nil {
		t.Fatalf("UpdateAccount() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if store.lastUpdateAccountID != "acct_1" || store.lastUpdateAccountReq.APIKeyRef != "sk-updated" {
		t.Fatalf("unexpected update request = id=%q req=%+v", store.lastUpdateAccountID, store.lastUpdateAccountReq)
	}
}

func TestImportModelsTOMLApplyImportsParsedModels(t *testing.T) {
	tmpDir := t.TempDir()
	modelsPath := filepath.Join(tmpDir, ".models.toml")
	raw := `
[deepseek-chat]
host = "cloud"
model_name = "deepseek-chat"
api_key = "sk-deepseek"
base_url = "https://api.deepseek.com"
timeout_sec = 200
`
	if err := os.WriteFile(modelsPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("CHENWEB_MODELS_TOML", modelsPath)

	prev := adminStoreFactory
	t.Cleanup(func() { adminStoreFactory = prev })
	store := &stubAdminStore{
		importResult: ImportResult{
			AccountsImported: 1,
			ProfilesImported: 1,
		},
	}
	adminStoreFactory = func() accountAdminStore { return store }

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/llm/accounts/import-models-toml/apply", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ImportModelsTOMLApply(c); err != nil {
		t.Fatalf("ImportModelsTOMLApply() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if store.lastImportParsed != 2 {
		t.Fatalf("expected parsed accounts+profiles to reach store, got %d", store.lastImportParsed)
	}
	if !strings.Contains(rec.Body.String(), `"accounts_imported":1`) {
		t.Fatalf("unexpected body = %s", rec.Body.String())
	}
}
