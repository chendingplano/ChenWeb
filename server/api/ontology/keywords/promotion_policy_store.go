package keywords

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// PromotionPolicy is one resource's admin-configurable override of whether
// its staged catalog entries auto-promote into keyword concepts. Absence of
// a row means enabled -- an admin only writes a row to opt a resource out
// (or to explicitly re-enable one previously opted out).
type PromotionPolicy struct {
	Source      string
	AutoPromote bool
	SetBy       string
}

// PromotionPolicyStore persists the per-source auto-promotion override in
// kb.keyword_source_promotion_policy, a deliberately mutable table (unlike
// kb.keyword_sources, which is immutable by trigger).
type PromotionPolicyStore struct {
	DB DBX
}

// IsEnabled reports whether source should auto-promote its staged catalog
// entries. No row for source means enabled.
func (s PromotionPolicyStore) IsEnabled(ctx context.Context, source string) (bool, error) {
	if s.DB == nil {
		return false, errors.New("db is nil")
	}
	source = strings.TrimSpace(source)
	var enabled bool
	err := s.DB.QueryRowContext(ctx,
		`SELECT auto_promote_enabled FROM kb.keyword_source_promotion_policy WHERE source = $1`,
		source,
	).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return enabled, nil
}

// Set upserts source's auto-promotion override.
func (s PromotionPolicyStore) Set(ctx context.Context, source string, enabled bool, setBy string) (PromotionPolicy, error) {
	if s.DB == nil {
		return PromotionPolicy{}, errors.New("db is nil")
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return PromotionPolicy{}, errors.New("source is required")
	}
	_, err := s.DB.ExecContext(ctx, `
INSERT INTO kb.keyword_source_promotion_policy (source, auto_promote_enabled, set_by, set_at, modify_time)
VALUES ($1, $2, $3, NOW(), NOW())
ON CONFLICT (source) DO UPDATE SET
	auto_promote_enabled = EXCLUDED.auto_promote_enabled,
	set_by = EXCLUDED.set_by,
	set_at = NOW(),
	modify_time = NOW()`,
		source, enabled, nullableString(setBy),
	)
	if err != nil {
		return PromotionPolicy{}, err
	}
	return PromotionPolicy{Source: source, AutoPromote: enabled, SetBy: setBy}, nil
}
