package llmreconcile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/llmusage"
	sharedllm "github.com/chendingplano/shared/go/api/llm"
)

// Exercise the production adapter and default database sink together: a revoked
// key must still produce an account-linked usage event before FetchBalance returns.
func TestBalanceFetchPersistsFailedRequestThroughDefaultSink(t *testing.T) {
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
	mock.ExpectExec("INSERT INTO llm_usage_event").WithArgs(
		"balance-event-1", "account-1", nil, nil, "deepseek", "", "provider_balance",
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		int64(0), int64(0), int64(0), int64(0), int64(0), sqlmock.AnyArg(), 0,
		"deepseek balance request failed: status 401", response, "", sqlmock.AnyArg(), "",
		`{"capture_source":"shared_llm"}`, nil, "provider_balance", "LLM_BALANCE_001", nil,
	).WillReturnResult(sqlmock.NewResult(1, 1))
	// Background reconciliation has no initiating user; retain the sink's
	// existing missing-user audit policy rather than inventing a user identity.
	mock.ExpectExec("INSERT INTO alarms_errors").WillReturnResult(sqlmock.NewResult(1, 1))
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
