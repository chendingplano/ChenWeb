package llmadminhandler

import (
	"context"
	"database/sql"
	"time"

	"github.com/chendingplano/deepdoc/server/api/llmimport"
)

type Account struct {
	ID                      string    `json:"id"`
	AccountName             string    `json:"account_name"`
	Provider                string    `json:"provider"`
	BaseURL                 string    `json:"base_url"`
	Status                  string    `json:"status"`
	IsReconciliationEnabled bool      `json:"is_reconciliation_enabled"`
	DefaultModelName        string    `json:"default_model_name"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	ProfileCount            int       `json:"profile_count"`
}

type CreateAccountInput struct {
	AccountName             string
	Provider                string
	BaseURL                 string
	APIKeyRef               string
	Status                  string
	ReconciliationKind      string
	IsReconciliationEnabled bool
	DefaultModelName        string
}

type Store struct {
	db *sql.DB
}

type ImportResult struct {
	AccountsImported int `json:"accounts_imported"`
	ProfilesImported int `json:"profiles_imported"`
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	const query = `SELECT a.id, a.account_name, a.provider, a.base_url, a.status,
COALESCE(a.is_reconciliation_enabled, FALSE) AS is_reconciliation_enabled,
COALESCE(a.default_model_name, '') AS default_model_name,
a.created_at, a.updated_at,
COUNT(p.id) AS profile_count
FROM llm_account a
LEFT JOIN llm_account_model_profile p ON p.account_id = a.id
GROUP BY a.id
ORDER BY a.account_name ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Account{}
	for rows.Next() {
		var account Account
		if err := rows.Scan(
			&account.ID,
			&account.AccountName,
			&account.Provider,
			&account.BaseURL,
			&account.Status,
			&account.IsReconciliationEnabled,
			&account.DefaultModelName,
			&account.CreatedAt,
			&account.UpdatedAt,
			&account.ProfileCount,
		); err != nil {
			return nil, err
		}
		out = append(out, account)
	}
	return out, rows.Err()
}

func (s *Store) CreateAccount(ctx context.Context, in CreateAccountInput) (Account, error) {
	const query = `INSERT INTO llm_account (
    account_name, provider, base_url, api_key_ref, status,
    reconciliation_kind, is_reconciliation_enabled, default_model_name, metadata_json
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb
)
RETURNING id, account_name, provider, base_url, status,
          is_reconciliation_enabled, default_model_name, created_at, updated_at`

	var account Account
	if err := s.db.QueryRowContext(
		ctx,
		query,
		in.AccountName,
		in.Provider,
		in.BaseURL,
		in.APIKeyRef,
		in.Status,
		in.ReconciliationKind,
		in.IsReconciliationEnabled,
		in.DefaultModelName,
		`{}`,
	).Scan(
		&account.ID,
		&account.AccountName,
		&account.Provider,
		&account.BaseURL,
		&account.Status,
		&account.IsReconciliationEnabled,
		&account.DefaultModelName,
		&account.CreatedAt,
		&account.UpdatedAt,
	); err != nil {
		return Account{}, err
	}
	return account, nil
}

func (s *Store) UpdateAccount(ctx context.Context, id string, in CreateAccountInput) (Account, error) {
	const query = `UPDATE llm_account
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
          is_reconciliation_enabled, default_model_name, created_at, updated_at`

	var account Account
	if err := s.db.QueryRowContext(
		ctx,
		query,
		id,
		in.AccountName,
		in.Provider,
		in.BaseURL,
		in.APIKeyRef,
		in.Status,
		in.ReconciliationKind,
		in.IsReconciliationEnabled,
		in.DefaultModelName,
	).Scan(
		&account.ID,
		&account.AccountName,
		&account.Provider,
		&account.BaseURL,
		&account.Status,
		&account.IsReconciliationEnabled,
		&account.DefaultModelName,
		&account.CreatedAt,
		&account.UpdatedAt,
	); err != nil {
		return Account{}, err
	}
	return account, nil
}

func (s *Store) ImportParsedModels(ctx context.Context, parsed llmimport.ParsedModels) (ImportResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	profilesByAccount := map[string][]llmimport.ImportedProfile{}
	for _, profile := range parsed.Profiles {
		profilesByAccount[profile.AccountKey] = append(profilesByAccount[profile.AccountKey], profile)
	}

	accountIDs := map[string]string{}
	const upsertAccount = `INSERT INTO llm_account (
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
RETURNING id`

	for _, account := range parsed.Accounts {
		defaultModelName := ""
		if profiles := profilesByAccount[account.AccountKey]; len(profiles) > 0 {
			defaultModelName = profiles[0].ModelName
		}
		var accountID string
		if scanErr := tx.QueryRowContext(
			ctx,
			upsertAccount,
			account.Name,
			account.Provider,
			account.BaseURL,
			account.APIKey,
			"active",
			"",
			false,
			defaultModelName,
			`{}`,
		).Scan(&accountID); scanErr != nil {
			err = scanErr
			return ImportResult{}, err
		}
		accountIDs[account.AccountKey] = accountID
	}

	const upsertProfile = `INSERT INTO llm_account_model_profile (
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
    updated_at = NOW()`

	for _, profile := range parsed.Profiles {
		accountID := accountIDs[profile.AccountKey]
		if _, execErr := tx.ExecContext(
			ctx,
			upsertProfile,
			accountID,
			profile.ProfileName,
			profile.ModelName,
			profile.ThinkingType,
			profile.TimeoutSec,
			profile.MaxInflight,
			profile.MaxRequestsPerMinute,
			profile.MaxTokensPerMinute,
			profile.TokenReservePerCall,
			true,
			`{}`,
		); execErr != nil {
			err = execErr
			return ImportResult{}, err
		}
	}

	if err = tx.Commit(); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{
		AccountsImported: len(parsed.Accounts),
		ProfilesImported: len(parsed.Profiles),
	}, nil
}
