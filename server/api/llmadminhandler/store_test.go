package llmadminhandler

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/llmimport"
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

func TestStoreUpdateAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE llm_account
SET account_name = $2,
    provider = $3,
    base_url = $4,
    api_key_ref = CASE
        WHEN $5 = '' THEN api_key_ref
        ELSE $5
    END,
    status = $6,
    reconciliation_kind = $7,
    is_reconciliation_enabled = $8,
    default_model_name = $9,
    updated_at = NOW()
WHERE id = $1
RETURNING id, account_name, provider, base_url, status,
          is_reconciliation_enabled, default_model_name, created_at, updated_at`)).
		WithArgs(
			"acct_1",
			"DeepSeek Prod",
			"deepseek",
			"https://api.deepseek.com",
			"sk-updated",
			"active",
			"provider_balance",
			true,
			"deepseek-v4-flash",
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_name", "provider", "base_url", "status",
			"is_reconciliation_enabled", "default_model_name", "created_at", "updated_at",
		}).AddRow(
			"acct_1", "DeepSeek Prod", "deepseek", "https://api.deepseek.com", "active",
			true, "deepseek-v4-flash", time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC), time.Date(2026, 6, 20, 3, 0, 0, 0, time.UTC),
		))

	store := NewStore(db)
	got, err := store.UpdateAccount(context.Background(), "acct_1", CreateAccountInput{
		AccountName:             "DeepSeek Prod",
		Provider:                "deepseek",
		BaseURL:                 "https://api.deepseek.com",
		APIKeyRef:               "sk-updated",
		Status:                  "active",
		ReconciliationKind:      "provider_balance",
		IsReconciliationEnabled: true,
		DefaultModelName:        "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatalf("UpdateAccount() error = %v", err)
	}
	if got.ID != "acct_1" || !got.IsReconciliationEnabled {
		t.Fatalf("unexpected updated account = %+v", got)
	}
}

func TestStoreImportParsedModelsUpsertsAccountsAndProfiles(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()

	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO llm_account (
    account_name, provider, base_url, api_key_ref, status,
    reconciliation_kind, is_reconciliation_enabled, default_model_name, metadata_json
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb
)
ON CONFLICT ((LOWER(account_name))) DO UPDATE SET
    provider = EXCLUDED.provider,
    base_url = EXCLUDED.base_url,
    api_key_ref = EXCLUDED.api_key_ref,
    status = EXCLUDED.status,
    updated_at = NOW()
RETURNING id`)).
		WithArgs(
			"deepseek:api.deepseek.com", "deepseek", "https://api.deepseek.com", "sk-deepseek",
			"active", "", false, "deepseek-chat", `{}`,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("acct_1"))

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO llm_account_model_profile (
    account_id, profile_name, model_name, thinking_type, timeout_sec,
    max_inflight, max_requests_per_minute, max_tokens_per_minute, token_reserve_per_call,
    is_active, metadata_json
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11::jsonb
)
ON CONFLICT (account_id, LOWER(profile_name)) DO UPDATE SET
    model_name = EXCLUDED.model_name,
    thinking_type = EXCLUDED.thinking_type,
    timeout_sec = EXCLUDED.timeout_sec,
    max_inflight = EXCLUDED.max_inflight,
    max_requests_per_minute = EXCLUDED.max_requests_per_minute,
    max_tokens_per_minute = EXCLUDED.max_tokens_per_minute,
    token_reserve_per_call = EXCLUDED.token_reserve_per_call,
    is_active = EXCLUDED.is_active,
    updated_at = NOW()`)).
		WithArgs(
			"acct_1", "deepseek-chat", "deepseek-chat", "enabled", 120,
			8, 120, 240000, 256,
			true, `{}`,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	store := NewStore(db)
	result, err := store.ImportParsedModels(context.Background(), llmimport.ParsedModels{
		Accounts: []llmimport.ImportedAccount{{
			AccountKey: "k1",
			Name:       "deepseek:api.deepseek.com",
			Provider:   "deepseek",
			BaseURL:    "https://api.deepseek.com",
			APIKey:     "sk-deepseek",
		}},
		Profiles: []llmimport.ImportedProfile{{
			AccountKey:           "k1",
			ProfileName:          "deepseek-chat",
			ModelName:            "deepseek-chat",
			ThinkingType:         "enabled",
			TimeoutSec:           120,
			MaxInflight:          8,
			MaxRequestsPerMinute: 120,
			MaxTokensPerMinute:   240000,
			TokenReservePerCall:  256,
		}},
	})
	if err != nil {
		t.Fatalf("ImportParsedModels() error = %v", err)
	}
	if result.AccountsImported != 1 || result.ProfilesImported != 1 {
		t.Fatalf("unexpected import result = %+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
