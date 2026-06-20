package llmadminhandler

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStoreListAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"id", "account_name", "provider", "base_url", "status",
		"is_reconciliation_enabled", "default_model_name", "created_at", "updated_at", "profile_count",
	}).AddRow(
		"acct_1", "DeepSeek Prod", "deepseek", "https://api.deepseek.com", "active",
		true, "deepseek-v4-flash", time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC), time.Date(2026, 6, 20, 2, 0, 0, 0, time.UTC), 2,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT a.id, a.account_name, a.provider, a.base_url, a.status,
COALESCE(a.is_reconciliation_enabled, FALSE) AS is_reconciliation_enabled,
COALESCE(a.default_model_name, '') AS default_model_name,
a.created_at, a.updated_at,
COUNT(p.id) AS profile_count
FROM llm_account a
LEFT JOIN llm_account_model_profile p ON p.account_id = a.id
GROUP BY a.id
ORDER BY a.account_name ASC`)).WillReturnRows(rows)

	store := NewStore(db)
	got, err := store.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(ListAccounts()) = %d, want 1", len(got))
	}
	if got[0].AccountName != "DeepSeek Prod" || got[0].ProfileCount != 2 {
		t.Fatalf("unexpected row = %+v", got[0])
	}
}

func TestStoreCreateAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO llm_account (
    account_name, provider, base_url, api_key_ref, status,
    reconciliation_kind, is_reconciliation_enabled, default_model_name, metadata_json
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb
)
RETURNING id, account_name, provider, base_url, status,
          is_reconciliation_enabled, default_model_name, created_at, updated_at`)).
		WithArgs(
			"DeepSeek Prod", "deepseek", "https://api.deepseek.com", "sk-deepseek",
			"active", "deepseek_balance", true, "deepseek-v4-flash", `{}`,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_name", "provider", "base_url", "status",
			"is_reconciliation_enabled", "default_model_name", "created_at", "updated_at",
		}).AddRow(
			"acct_1", "DeepSeek Prod", "deepseek", "https://api.deepseek.com", "active",
			true, "deepseek-v4-flash", time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC), time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC),
		))

	store := NewStore(db)
	got, err := store.CreateAccount(context.Background(), CreateAccountInput{
		AccountName:             "DeepSeek Prod",
		Provider:                "deepseek",
		BaseURL:                 "https://api.deepseek.com",
		APIKeyRef:               "sk-deepseek",
		Status:                  "active",
		ReconciliationKind:      "deepseek_balance",
		IsReconciliationEnabled: true,
		DefaultModelName:        "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	if got.ID != "acct_1" || got.AccountName != "DeepSeek Prod" {
		t.Fatalf("unexpected created account = %+v", got)
	}
}
