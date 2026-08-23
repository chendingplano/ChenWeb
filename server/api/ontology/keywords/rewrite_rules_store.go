package keywords

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrRewriteRuleInvalid  = errors.New("invalid rewrite rule")
	ErrRewriteRuleNotFound = errors.New("rewrite rule not found")
)

// RewriteRule is a pattern-based surface rewrite rule. Rules are disabled
// by default; a human enables them per scope. Applied in tier 3 as a
// pre-normalization pass.
type RewriteRule struct {
	RuleID      string    `json:"rule_id"`
	Pattern     string    `json:"pattern"`
	Replacement string    `json:"replacement"`
	Scope       string    `json:"scope"`
	Enabled     bool      `json:"enabled"`
	Provenance  string    `json:"provenance"`
	CreateTime  time.Time `json:"create_time"`
	ModifyTime  time.Time `json:"modify_time"`
}

const rewriteRuleColumns = `rule_id, pattern, replacement, scope, enabled, provenance, create_time, modify_time`

const rewriteRuleFrom = `FROM kb.keyword_rewrite_rules`

func scanRewriteRule(scan func(dest ...any) error) (RewriteRule, error) {
	var r RewriteRule
	if err := scan(&r.RuleID, &r.Pattern, &r.Replacement, &r.Scope, &r.Enabled, &r.Provenance, &r.CreateTime, &r.ModifyTime); err != nil {
		return RewriteRule{}, err
	}
	return r, nil
}

func validateRewriteRule(r RewriteRule) error {
	r.RuleID = strings.TrimSpace(r.RuleID)
	r.Scope = strings.TrimSpace(r.Scope)
	r.Provenance = strings.TrimSpace(r.Provenance)
	if r.RuleID == "" {
		return fmt.Errorf("%w: rule_id is required", ErrRewriteRuleInvalid)
	}
	if strings.TrimSpace(r.Pattern) == "" {
		return fmt.Errorf("%w: pattern is required", ErrRewriteRuleInvalid)
	}
	if strings.TrimSpace(r.Replacement) == "" {
		return fmt.Errorf("%w: replacement is required", ErrRewriteRuleInvalid)
	}
	if r.Scope == "" {
		return fmt.Errorf("%w: scope is required", ErrRewriteRuleInvalid)
	}
	if r.Provenance == "" {
		return fmt.Errorf("%w: provenance is required", ErrRewriteRuleInvalid)
	}
	// Safety check: no capture groups, no backreferences — simple literal only.
	if strings.Contains(r.Pattern, "(") || strings.Contains(r.Pattern, ")") ||
		strings.Contains(r.Pattern, "\\") {
		return fmt.Errorf("%w: pattern must be a simple literal (no capture groups/backreferences)", ErrRewriteRuleInvalid)
	}
	return nil
}

// RewriteRuleStore persists rewrite rule rows.
type RewriteRuleStore struct {
	DB DBX
}

// CreateRule inserts a new rewrite rule (disabled by default).
func (s RewriteRuleStore) CreateRule(ctx context.Context, r RewriteRule) (RewriteRule, error) {
	r.RuleID, r.Scope, r.Provenance = strings.TrimSpace(r.RuleID), strings.TrimSpace(r.Scope), strings.TrimSpace(r.Provenance)
	if r.Scope == "" {
		r.Scope = "_"
	}
	if r.Provenance == "" {
		r.Provenance = "human:"
	}
	if err := validateRewriteRule(r); err != nil {
		return RewriteRule{}, err
	}
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO kb.keyword_rewrite_rules (rule_id, pattern, replacement, scope, enabled, provenance)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+rewriteRuleColumns,
		r.RuleID, r.Pattern, r.Replacement, r.Scope, r.Enabled, r.Provenance)
	return scanRewriteRule(row.Scan)
}

// GetRule retrieves a rewrite rule by its id.
func (s RewriteRuleStore) GetRule(ctx context.Context, ruleID string) (RewriteRule, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT `+rewriteRuleColumns+`
		`+rewriteRuleFrom+`
		WHERE rule_id = $1`, ruleID)
	return scanRewriteRule(row.Scan)
}

// ListEnabledRules returns enabled rewrite rules for a scope.
func (s RewriteRuleStore) ListEnabledRules(ctx context.Context, scope string) ([]RewriteRule, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+rewriteRuleColumns+`
		`+rewriteRuleFrom+`
		WHERE enabled = true AND scope = $1
		ORDER BY rule_id`, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RewriteRule
	for rows.Next() {
		r, err := scanRewriteRule(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRules lists enabled and disabled rules, optionally for one exact scope.
func (s RewriteRuleStore) ListRules(ctx context.Context, scope string) ([]RewriteRule, error) {
	query := `SELECT ` + rewriteRuleColumns + ` ` + rewriteRuleFrom
	args := []any{}
	if scope != "" {
		query += ` WHERE scope = $1`
		args = append(args, scope)
	}
	query += ` ORDER BY rule_id`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RewriteRule, 0)
	for rows.Next() {
		r, err := scanRewriteRule(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListScopes returns all nonblank scopes used by the keyword lexicon and rules.
func (s RewriteRuleStore) ListScopes(ctx context.Context) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT DISTINCT scope FROM (
			SELECT scope FROM kb.keyword_concepts
			UNION ALL SELECT scope FROM kb.keyword_surfaces
			UNION ALL SELECT scope FROM kb.keyword_rewrite_rules
		) scopes WHERE NULLIF(BTRIM(scope), '') IS NOT NULL ORDER BY scope`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{"_": true}
	out := []string{"_"}
	for rows.Next() {
		var scope string
		if err := rows.Scan(&scope); err != nil {
			return nil, err
		}
		scope = strings.TrimSpace(scope)
		if scope != "" && !seen[scope] {
			seen[scope] = true
			out = append(out, scope)
		}
	}
	return out, rows.Err()
}

func (s RewriteRuleStore) UpdateRule(ctx context.Context, ruleID string, r RewriteRule) (RewriteRule, error) {
	r.RuleID = strings.TrimSpace(ruleID)
	r.Scope, r.Provenance = strings.TrimSpace(r.Scope), strings.TrimSpace(r.Provenance)
	if err := validateRewriteRule(r); err != nil {
		return RewriteRule{}, err
	}
	row := s.DB.QueryRowContext(ctx, `UPDATE kb.keyword_rewrite_rules SET pattern=$2, replacement=$3, scope=$4, enabled=$5, provenance=$6, modify_time=NOW() WHERE rule_id=$1 RETURNING `+rewriteRuleColumns, r.RuleID, r.Pattern, r.Replacement, r.Scope, r.Enabled, r.Provenance)
	updated, err := scanRewriteRule(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return RewriteRule{}, ErrRewriteRuleNotFound
	}
	return updated, err
}

// UpdateRuleEnabled enables or disables a rewrite rule.
func (s RewriteRuleStore) UpdateRuleEnabled(ctx context.Context, ruleID string, enabled bool) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE kb.keyword_rewrite_rules
		SET enabled = $2, modify_time = NOW()
		WHERE rule_id = $1`, ruleID, enabled)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrRewriteRuleNotFound
	}
	return nil
}
