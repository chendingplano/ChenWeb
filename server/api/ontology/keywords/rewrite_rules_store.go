package keywords

import (
	"context"
	"fmt"
	"strings"
	"time"
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
	if r.RuleID == "" {
		return fmt.Errorf("rule_id is required")
	}
	if r.Pattern == "" {
		return fmt.Errorf("pattern is required")
	}
	if r.Replacement == "" {
		return fmt.Errorf("replacement is required")
	}
	// Safety check: no capture groups, no backreferences — simple literal only.
	if strings.Contains(r.Pattern, "(") || strings.Contains(r.Pattern, ")") ||
		strings.Contains(r.Pattern, "\\") {
		return fmt.Errorf("pattern must be a simple literal (no capture groups/backreferences)")
	}
	return nil
}

// RewriteRuleStore persists rewrite rule rows.
type RewriteRuleStore struct {
	DB DBX
}

// CreateRule inserts a new rewrite rule (disabled by default).
func (s RewriteRuleStore) CreateRule(ctx context.Context, r RewriteRule) (RewriteRule, error) {
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
		return fmt.Errorf("rewrite rule %s not found", ruleID)
	}
	return nil
}
