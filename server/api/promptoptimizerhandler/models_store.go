package promptoptimizerhandler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// errModelNotFound is returned by lookups that miss either because the row
// does not exist or because it belongs to a different user. Callers must
// not distinguish — user-scoped tenancy makes a foreign row indistinguish-
// able from "not found".
var errModelNotFound = errors.New("promptoptimizerhandler: model not found")

// insertModel persists a new prompt_optimizer_models row and returns the
// generated id.
func insertModel(
	db *sql.DB,
	userID, name, provider, baseURL, model, apiKeyCiphertext string,
	defaultParams json.RawMessage,
	enabled bool,
) (string, error) {
	if len(defaultParams) == 0 {
		defaultParams = json.RawMessage(`{}`)
	}
	const stmt = `
		INSERT INTO prompt_optimizer_models
		    (user_id, name, provider, base_url, model, api_key_encrypted, default_params, enabled)
		VALUES
		    ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8)
		RETURNING id
	`
	var id string
	err := db.QueryRow(stmt,
		userID, name, provider, baseURL, model, apiKeyCiphertext,
		[]byte(defaultParams), enabled,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert model: %w", err)
	}
	return id, nil
}

// listModelsForUser returns every model owned by userID, newest first.
func listModelsForUser(db *sql.DB, userID string) ([]Model, error) {
	const stmt = `
		SELECT id, user_id, name, provider, COALESCE(base_url, ''), model,
		       default_params, enabled,
		       (api_key_encrypted IS NOT NULL AND api_key_encrypted <> ''),
		       created_at, updated_at
		FROM prompt_optimizer_models
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := db.Query(stmt, userID)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer rows.Close()

	out := make([]Model, 0)
	for rows.Next() {
		var m Model
		var params []byte
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.Name, &m.Provider, &m.BaseURL, &m.Model,
			&params, &m.Enabled, &m.HasAPIKey, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan model row: %w", err)
		}
		m.DefaultParams = json.RawMessage(params)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate model rows: %w", err)
	}
	return out, nil
}

// getModelForUser fetches one model, enforcing user ownership.
func getModelForUser(db *sql.DB, userID, id string) (*Model, error) {
	const stmt = `
		SELECT id, user_id, name, provider, COALESCE(base_url, ''), model,
		       default_params, enabled,
		       (api_key_encrypted IS NOT NULL AND api_key_encrypted <> ''),
		       created_at, updated_at
		FROM prompt_optimizer_models
		WHERE id = $1 AND user_id = $2
	`
	var m Model
	var params []byte
	err := db.QueryRow(stmt, id, userID).Scan(
		&m.ID, &m.UserID, &m.Name, &m.Provider, &m.BaseURL, &m.Model,
		&params, &m.Enabled, &m.HasAPIKey, &m.CreatedAt, &m.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get model: %w", err)
	}
	m.DefaultParams = json.RawMessage(params)
	return &m, nil
}

// getAPIKeyCiphertextForUser returns the raw ciphertext for a user-owned
// model, suitable for feeding security.DecryptString. Separated from
// getModelForUser to keep plaintext-adjacent values off the JSON path.
func getAPIKeyCiphertextForUser(db *sql.DB, userID, id string) (string, error) {
	const stmt = `
		SELECT api_key_encrypted
		FROM prompt_optimizer_models
		WHERE id = $1 AND user_id = $2
	`
	var ct string
	err := db.QueryRow(stmt, id, userID).Scan(&ct)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errModelNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get api key: %w", err)
	}
	return ct, nil
}

// updateModelForUser patches mutable fields. apiKeyCiphertext == "" keeps
// the existing ciphertext; a non-empty value replaces it.
func updateModelForUser(
	db *sql.DB,
	userID, id, name, provider, baseURL, model, apiKeyCiphertext string,
	defaultParams json.RawMessage,
	enabled bool,
) error {
	if len(defaultParams) == 0 {
		defaultParams = json.RawMessage(`{}`)
	}
	const stmt = `
		UPDATE prompt_optimizer_models SET
		    name              = $3,
		    provider          = $4,
		    base_url          = NULLIF($5, ''),
		    model             = $6,
		    api_key_encrypted = CASE WHEN $7 = '' THEN api_key_encrypted ELSE $7 END,
		    default_params    = $8,
		    enabled           = $9,
		    updated_at        = NOW()
		WHERE id = $1 AND user_id = $2
	`
	res, err := db.Exec(stmt,
		id, userID, name, provider, baseURL, model, apiKeyCiphertext,
		[]byte(defaultParams), enabled,
	)
	if err != nil {
		return fmt.Errorf("update model: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errModelNotFound
	}
	return nil
}

// deleteModelForUser deletes the row if the user owns it.
func deleteModelForUser(db *sql.DB, userID, id string) error {
	const stmt = `DELETE FROM prompt_optimizer_models WHERE id = $1 AND user_id = $2`
	res, err := db.Exec(stmt, id, userID)
	if err != nil {
		return fmt.Errorf("delete model: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errModelNotFound
	}
	return nil
}
