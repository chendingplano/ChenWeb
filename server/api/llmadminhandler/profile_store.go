package llmadminhandler

import (
	"context"
	"time"
)

// ModelProfile represents a row in llm_account_model_profile joined with llm_account.
type ModelProfile struct {
	ID                   string    `json:"id"`
	AccountID            string    `json:"account_id"`
	AccountName          string    `json:"account_name"`
	ProfileName          string    `json:"profile_name"`
	ModelName            string    `json:"model_name"`
	ThinkingType         string    `json:"thinking_type"`
	TimeoutSec           int       `json:"timeout_sec"`
	MaxInflight          int       `json:"max_inflight"`
	MaxRequestsPerMinute int       `json:"max_requests_per_minute"`
	MaxTokensPerMinute   int       `json:"max_tokens_per_minute"`
	TokenReservePerCall  int       `json:"token_reserve_per_call"`
	IsActive             bool      `json:"is_active"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// CreateProfileInput is the input for creating or updating a model profile.
type CreateProfileInput struct {
	AccountID            string
	ProfileName          string
	ModelName            string
	ThinkingType         string
	TimeoutSec           int
	MaxInflight          int
	MaxRequestsPerMinute int
	MaxTokensPerMinute   int
	TokenReservePerCall  int
	IsActive             bool
}

func (s *Store) ListProfiles(ctx context.Context) ([]ModelProfile, error) {
	const query = `
SELECT p.id, p.account_id, a.account_name, p.profile_name, p.model_name,
	COALESCE(p.thinking_type, '') AS thinking_type,
	p.timeout_sec, p.max_inflight, p.max_requests_per_minute,
	p.max_tokens_per_minute, p.token_reserve_per_call, p.is_active,
	p.created_at, p.updated_at
FROM llm_account_model_profile p
JOIN llm_account a ON a.id = p.account_id
ORDER BY a.account_name ASC, p.profile_name ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ModelProfile{}
	for rows.Next() {
		var p ModelProfile
		if err := rows.Scan(
			&p.ID, &p.AccountID, &p.AccountName, &p.ProfileName, &p.ModelName,
			&p.ThinkingType, &p.TimeoutSec, &p.MaxInflight, &p.MaxRequestsPerMinute,
			&p.MaxTokensPerMinute, &p.TokenReservePerCall, &p.IsActive,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CreateProfile(ctx context.Context, in CreateProfileInput) (ModelProfile, error) {
	const query = `
INSERT INTO llm_account_model_profile (
	account_id, profile_name, model_name, thinking_type, timeout_sec,
	max_inflight, max_requests_per_minute, max_tokens_per_minute, token_reserve_per_call,
	is_active, metadata_json
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb)
RETURNING id, account_id, profile_name, model_name, thinking_type,
	timeout_sec, max_inflight, max_requests_per_minute, max_tokens_per_minute,
	token_reserve_per_call, is_active, created_at, updated_at`

	var p ModelProfile
	if err := s.db.QueryRowContext(
		ctx, query,
		in.AccountID, in.ProfileName, in.ModelName, in.ThinkingType, in.TimeoutSec,
		in.MaxInflight, in.MaxRequestsPerMinute, in.MaxTokensPerMinute, in.TokenReservePerCall,
		in.IsActive, `{}`,
	).Scan(
		&p.ID, &p.AccountID, &p.ProfileName, &p.ModelName, &p.ThinkingType,
		&p.TimeoutSec, &p.MaxInflight, &p.MaxRequestsPerMinute, &p.MaxTokensPerMinute,
		&p.TokenReservePerCall, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return ModelProfile{}, err
	}
	return p, nil
}

func (s *Store) UpdateProfile(ctx context.Context, id string, in CreateProfileInput) (ModelProfile, error) {
	const query = `
UPDATE llm_account_model_profile
SET account_id = $2,
    profile_name = $3,
    model_name = $4,
    thinking_type = $5,
    timeout_sec = $6,
    max_inflight = $7,
    max_requests_per_minute = $8,
    max_tokens_per_minute = $9,
    token_reserve_per_call = $10,
    is_active = $11,
    updated_at = NOW()
WHERE id = $1
RETURNING id, account_id, profile_name, model_name, thinking_type,
	timeout_sec, max_inflight, max_requests_per_minute, max_tokens_per_minute,
	token_reserve_per_call, is_active, created_at, updated_at`

	var p ModelProfile
	if err := s.db.QueryRowContext(
		ctx, query,
		id, in.AccountID, in.ProfileName, in.ModelName, in.ThinkingType, in.TimeoutSec,
		in.MaxInflight, in.MaxRequestsPerMinute, in.MaxTokensPerMinute, in.TokenReservePerCall,
		in.IsActive,
	).Scan(
		&p.ID, &p.AccountID, &p.ProfileName, &p.ModelName, &p.ThinkingType,
		&p.TimeoutSec, &p.MaxInflight, &p.MaxRequestsPerMinute, &p.MaxTokensPerMinute,
		&p.TokenReservePerCall, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return ModelProfile{}, err
	}
	return p, nil
}

// UpsertAccountAndProfile atomically upserts an llm_account and an llm_account_model_profile.
// Used by the AddModel handler (ADR 2026070501 steps 2–3).
func (s *Store) UpsertAccountAndProfile(ctx context.Context, accountIn CreateAccountInput, profileIn CreateProfileInput) (ModelProfile, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ModelProfile{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const upsertAccount = `
INSERT INTO llm_account (
	account_name, provider, base_url, api_key_ref, status,
	reconciliation_kind, is_reconciliation_enabled, default_model_name, metadata_json
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)
ON CONFLICT (LOWER(account_name)) DO UPDATE SET
	provider = EXCLUDED.provider,
	base_url = EXCLUDED.base_url,
	api_key_ref = CASE WHEN EXCLUDED.api_key_ref = '' THEN llm_account.api_key_ref ELSE EXCLUDED.api_key_ref END,
	updated_at = NOW()
RETURNING id`

	var accountID string
	if err = tx.QueryRowContext(
		ctx, upsertAccount,
		accountIn.AccountName, accountIn.Provider, accountIn.BaseURL, accountIn.APIKeyRef,
		accountIn.Status, accountIn.ReconciliationKind, accountIn.IsReconciliationEnabled,
		accountIn.DefaultModelName, `{}`,
	).Scan(&accountID); err != nil {
		return ModelProfile{}, err
	}

	const upsertProfile = `
INSERT INTO llm_account_model_profile (
	account_id, profile_name, model_name, thinking_type, timeout_sec,
	max_inflight, max_requests_per_minute, max_tokens_per_minute, token_reserve_per_call,
	is_active, metadata_json
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb)
ON CONFLICT (account_id, LOWER(profile_name)) DO UPDATE SET
	model_name = EXCLUDED.model_name,
	thinking_type = EXCLUDED.thinking_type,
	timeout_sec = EXCLUDED.timeout_sec,
	max_inflight = EXCLUDED.max_inflight,
	max_requests_per_minute = EXCLUDED.max_requests_per_minute,
	max_tokens_per_minute = EXCLUDED.max_tokens_per_minute,
	token_reserve_per_call = EXCLUDED.token_reserve_per_call,
	is_active = EXCLUDED.is_active,
	updated_at = NOW()
RETURNING id, account_id, profile_name, model_name, thinking_type,
	timeout_sec, max_inflight, max_requests_per_minute, max_tokens_per_minute,
	token_reserve_per_call, is_active, created_at, updated_at`

	var p ModelProfile
	if err = tx.QueryRowContext(
		ctx, upsertProfile,
		accountID, profileIn.ProfileName, profileIn.ModelName, profileIn.ThinkingType, profileIn.TimeoutSec,
		profileIn.MaxInflight, profileIn.MaxRequestsPerMinute, profileIn.MaxTokensPerMinute,
		profileIn.TokenReservePerCall, profileIn.IsActive, `{}`,
	).Scan(
		&p.ID, &p.AccountID, &p.ProfileName, &p.ModelName, &p.ThinkingType,
		&p.TimeoutSec, &p.MaxInflight, &p.MaxRequestsPerMinute, &p.MaxTokensPerMinute,
		&p.TokenReservePerCall, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return ModelProfile{}, err
	}

	if err = tx.Commit(); err != nil {
		return ModelProfile{}, err
	}
	return p, nil
}
