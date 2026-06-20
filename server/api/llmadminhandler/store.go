package llmadminhandler

import (
	"context"
	"database/sql"
	"time"
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
