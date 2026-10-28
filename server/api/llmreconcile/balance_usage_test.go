package llmreconcile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/llmusage"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	sharedllm "github.com/chendingplano/shared/go/api/llm"
	"github.com/spf13/viper"
)

// Exercise the production adapter and default database sink together: a revoked
// key must still produce an account-linked usage event before FetchBalance returns.
func TestBalanceFetchPersistsFailedRequestThroughDefaultSink(t *testing.T) {
	originalConfig := config.AppConfig
	t.Cleanup(func() { config.AppConfig = originalConfig })
	vp := viper.New()
	vp.SetConfigType("toml")
	if err := vp.ReadConfig(strings.NewReader(`[model-balance-queries]
    deepseek = "deepseek-flash-processor"
    `)); err != nil {
		t.Fatal(err)
	}
	if err := vp.Unmarshal(&config.AppConfig); err != nil {
		t.Fatal(err)
	}

	kratos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("credentials_identifier") != "scheduler" {
			t.Errorf("unexpected identity lookup: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{{"id": "scheduler-id", "state": "active", "traits": map[string]any{"username": "scheduler"}}})
	}))
	defer kratos.Close()
	t.Setenv("KRATOS_ADMIN_URL", kratos.URL)
	const response = `{"error":{"message":"Unauthorized"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/balance" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT id, provider FROM llm_account").
		WithArgs(server.URL, "revoked-key").
		WillReturnRows(sqlmock.NewRows([]string{"id", "provider"}).AddRow("account-1", "deepseek"))
	mock.ExpectQuery("SELECT id FROM llm_account_model_profile").
		WithArgs("account-1", "deepseek-flash-processor").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("profile-1"))
	mock.ExpectExec("INSERT INTO llm_usage_event").WithArgs(
		"balance-event-1", "account-1", "profile-1", "scheduler-id", "deepseek", "deepseek-flash-processor", "provider_balance",
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		int64(0), int64(0), int64(0), int64(0), int64(0), sqlmock.AnyArg(), 0,
		"deepseek balance request failed: status 401", response, "", sqlmock.AnyArg(), "",
		`{"capture_source":"shared_llm"}`, nil, "provider_balance", "LLM_BALANCE_001", nil,
	).WillReturnResult(sqlmock.NewResult(1, 1))
	original := sharedllm.DefaultUsageCaptureSink
	sharedllm.DefaultUsageCaptureSink = &llmusage.Sink{
		DB: db, ArchiveRoot: t.TempDir(), WorkspaceTZ: time.UTC,
		NewID: func() string { return "balance-event-1" },
	}
	t.Cleanup(func() { sharedllm.DefaultUsageCaptureSink = original })
	client := &DeepSeekBalanceClient{HTTPClient: server.Client()}
	if _, err := client.FetchBalance(context.Background(), server.URL, "revoked-key"); err == nil {
		t.Fatal("expected revoked-key error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBalanceFetchRequiresActiveScheduler(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"missing", `[]`, 200},
		{"inactive", `[{"id":"scheduler-id","state":"inactive","traits":{"username":"scheduler"}}]`, 200},
		{"unavailable", `{}`, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kratos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) }))
			defer kratos.Close()
			t.Setenv("KRATOS_ADMIN_URL", kratos.URL)
			providerCalled := false
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { providerCalled = true }))
			defer provider.Close()
			client := &DeepSeekBalanceClient{HTTPClient: provider.Client()}
			if _, err := client.FetchBalance(context.Background(), provider.URL, "key"); err == nil {
				t.Fatal("expected scheduler identity error")
			}
			if providerCalled {
				t.Fatal("provider called without an active scheduler identity")
			}
		})
	}
}
