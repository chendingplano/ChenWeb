package keywords

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Unresolved is one backlog entry for a surface form that could not be
// resolved to a keyword concept. Keyed on (norm_key, scope).
type Unresolved struct {
	NormKey     string    `json:"norm_key"`
	Scope       string    `json:"scope"`
	Surfaces    []string  `json:"surfaces"`
	Contexts    []string  `json:"contexts"`
	Hits        int       `json:"hits"`
	Status      string    `json:"status"`
	Attempts    int       `json:"attempts"`
	LastAttempt *string   `json:"last_attempt"`
	Priority    float64   `json:"priority"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

// AllowedUnresolvedStatuses mirrors the schema CHECK constraint.
var AllowedUnresolvedStatuses = map[string]bool{
	"pending":              true,
	"batched":              true,
	"needs_human":          true,
	"resolved":             true,
	"junk":                 true,
	"insufficient_context": true,
}

// UnresolvedStore persists unresolved keyword backlog rows.
type UnresolvedStore struct {
	DB DBX
}

// UpsertUnresolved inserts or updates a backlog row. Surfaces are deduplicated
// and capped at 10; contexts keep a reservoir sample of ≤5 snippets at ≤200
// chars each.
func (s UnresolvedStore) UpsertUnresolved(ctx context.Context, normKey, scope, surface, contextText string) error {
	// Fetch current row if it exists.
	var (
		existingSurfaces []byte
		existingContexts []byte
		hits             int
	)
	row := s.DB.QueryRowContext(ctx, `
		SELECT surfaces, contexts, hits FROM kb.keyword_unresolved
		WHERE norm_key = $1 AND scope = $2`, normKey, scope)
	err := row.Scan(&existingSurfaces, &existingContexts, &hits)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("upsert unresolved: %w", err)
	}

	var surfaces []string
	var contexts []string
	if err == nil {
		// Existing row: merge.
		_ = json.Unmarshal(existingSurfaces, &surfaces)
		_ = json.Unmarshal(existingContexts, &contexts)
	}

	// Deduplicate and cap surfaces.
	if surface != "" {
		found := false
		for _, s := range surfaces {
			if s == surface {
				found = true
				break
			}
		}
		if !found {
			surfaces = append(surfaces, surface)
			if len(surfaces) > 10 {
				surfaces = surfaces[len(surfaces)-10:]
			}
		}
	}

	// Reservoir sample contexts (cap at 5, ≤200 chars each).
	if contextText != "" {
		if len(contextText) > 200 {
			contextText = contextText[:200]
		}
		contexts = append(contexts, contextText)
		if len(contexts) > 5 {
			contexts = contexts[len(contexts)-5:]
		}
	}

	surfacesJSON, _ := json.Marshal(surfaces)
	contextsJSON, _ := json.Marshal(contexts)

	if err == sql.ErrNoRows {
		_, err = s.DB.ExecContext(ctx, `
			INSERT INTO kb.keyword_unresolved (norm_key, scope, surfaces, contexts, hits)
			VALUES ($1, $2, $3, $4, 1)`,
			normKey, scope, surfacesJSON, contextsJSON)
	} else {
		_, err = s.DB.ExecContext(ctx, `
			UPDATE kb.keyword_unresolved
			SET surfaces = $3, contexts = $4, hits = hits + 1, last_seen = NOW()
			WHERE norm_key = $1 AND scope = $2`,
			normKey, scope, surfacesJSON, contextsJSON)
	}
	return err
}

// ListUnresolved returns unresolved entries, filtered by scope and status.
func (s UnresolvedStore) ListUnresolved(ctx context.Context, scope, status string, limit int) ([]Unresolved, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT norm_key, scope, surfaces, contexts, hits, status, attempts, last_attempt, priority, first_seen, last_seen
		FROM kb.keyword_unresolved
		WHERE scope = $1 AND status = $2
		ORDER BY priority DESC, hits DESC
		LIMIT $3`, scope, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Unresolved
	for rows.Next() {
		u, err := scanUnresolved(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUnresolvedStatus transitions a backlog entry's status and increments attempts.
func (s UnresolvedStore) UpdateUnresolvedStatus(ctx context.Context, normKey, scope, status, lastAttempt string) error {
	if !AllowedUnresolvedStatuses[status] {
		return fmt.Errorf("invalid unresolved status: %s", status)
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE kb.keyword_unresolved
		SET status = $3, attempts = attempts + 1, last_attempt = $4, last_seen = NOW()
		WHERE norm_key = $1 AND scope = $2`,
		normKey, scope, status, nullableString(lastAttempt))
	return err
}

func scanUnresolved(scan func(dest ...any) error) (Unresolved, error) {
	var (
		u            Unresolved
		surfacesJSON []byte
		contextsJSON []byte
		lastAttempt  sql.NullString
	)
	if err := scan(&u.NormKey, &u.Scope, &surfacesJSON, &contextsJSON, &u.Hits,
		&u.Status, &u.Attempts, &lastAttempt, &u.Priority, &u.FirstSeen, &u.LastSeen); err != nil {
		return Unresolved{}, err
	}
	_ = json.Unmarshal(surfacesJSON, &u.Surfaces)
	_ = json.Unmarshal(contextsJSON, &u.Contexts)
	if u.Surfaces == nil {
		u.Surfaces = []string{}
	}
	if u.Contexts == nil {
		u.Contexts = []string{}
	}
	if lastAttempt.Valid {
		u.LastAttempt = &lastAttempt.String
	}
	return u, nil
}
