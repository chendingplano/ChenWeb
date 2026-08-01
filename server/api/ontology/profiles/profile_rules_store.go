package profiles

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// ProfileRule is one version of a governed rule belonging to a profile.
// RuleConfig is interpreted only by a registered rule kind in the evaluator.
type ProfileRule struct {
	ID             int64           `json:"id"`
	RuleID         string          `json:"rule_id"`
	Version        int             `json:"version"`
	ProfileID      string          `json:"profile_id"`
	ProfileVersion int             `json:"profile_version"`
	RuleKind       string          `json:"rule_kind"`
	Status         string          `json:"status"`
	Severity       string          `json:"severity"`
	RuleConfig     json.RawMessage `json:"rule_config"`
	Applicability  json.RawMessage `json:"applicability"`
	ReleaseID      int64           `json:"release_id"`
	CreateTime     time.Time       `json:"create_time"`
	CreateBy       string          `json:"create_by"`
	ModifyTime     time.Time       `json:"modify_time"`
	ModifyBy       string          `json:"modify_by"`
}

// ProfileRuleStore reads profile rules from the database or a compiler
// transaction. It intentionally has no direct normative-write path.
type ProfileRuleStore struct {
	DB terms.DBX
}

var profileRuleTransitions = map[string]map[string]bool{
	"draft":               {"in_review": true, "rejected": true},
	"in_review":           {"approved": true, "rejected": true},
	"approved":            {"included_in_release": true, "superseded": true},
	"included_in_release": {"superseded": true},
}

func (s ProfileRuleStore) GetProfileRule(ctx context.Context, ruleID string, version int) (ProfileRule, error) {
	if s.DB == nil {
		return ProfileRule{}, errors.New("db is nil")
	}
	const stmt = `SELECT id, rule_id, version, profile_id, profile_version, rule_kind, status, severity, rule_config, applicability, create_time, COALESCE(create_by, ''), modify_time, COALESCE(modify_by, '') FROM kb.ontology_profile_rules WHERE rule_id = $1 AND version = $2`
	var r ProfileRule
	err := s.DB.QueryRowContext(ctx, stmt, strings.TrimSpace(ruleID), version).Scan(&r.ID, &r.RuleID, &r.Version, &r.ProfileID, &r.ProfileVersion, &r.RuleKind, &r.Status, &r.Severity, &r.RuleConfig, &r.Applicability, &r.CreateTime, &r.CreateBy, &r.ModifyTime, &r.ModifyBy)
	return r, err
}

// TransitionStatus enforces the governed lifecycle and deliberately cannot
// change a released rule's content or return it to a mutable status.
func (s ProfileRuleStore) TransitionStatus(ctx context.Context, ruleID string, version int, next, by string) (ProfileRule, error) {
	current, err := s.GetProfileRule(ctx, ruleID, version)
	if err != nil {
		return ProfileRule{}, err
	}
	if !profileRuleTransitions[current.Status][next] {
		return ProfileRule{}, errors.New("illegal profile rule status transition")
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE kb.ontology_profile_rules SET status = $3, modify_time = NOW(), modify_by = $4 WHERE rule_id = $1 AND version = $2`, strings.TrimSpace(ruleID), version, next, nullableText(by)); err != nil {
		return ProfileRule{}, err
	}
	return s.GetProfileRule(ctx, ruleID, version)
}

// CreateProfileRule creates the initial draft version of a rule. A rule's
// profile/version is FK-validated by Postgres; release visibility remains the
// module compiler's separate responsibility.
func (s ProfileRuleStore) CreateProfileRule(ctx context.Context, r ProfileRule) (ProfileRule, error) {
	if s.DB == nil {
		return ProfileRule{}, errors.New("db is nil")
	}
	if strings.TrimSpace(r.RuleID) == "" || strings.TrimSpace(r.ProfileID) == "" || r.ProfileVersion < 1 || strings.TrimSpace(r.RuleKind) == "" {
		return ProfileRule{}, errors.New("rule_id, profile_id/version, and rule_kind are required")
	}
	if _, ok := LookupRuleKind(r.RuleKind); !ok {
		return ProfileRule{}, errors.New("rule_kind is not registered")
	}
	if r.Status == "" {
		r.Status = "draft"
	}
	if r.Status != "draft" {
		return ProfileRule{}, errors.New("new profile rules must start in draft")
	}
	if r.Severity == "" {
		r.Severity = "error"
	}
	if len(r.RuleConfig) == 0 {
		return ProfileRule{}, errors.New("rule_config is required")
	}
	if len(r.Applicability) == 0 {
		r.Applicability = json.RawMessage(`{}`)
	}
	const stmt = `INSERT INTO kb.ontology_profile_rules (rule_id, version, profile_id, profile_version, rule_kind, status, severity, rule_config, applicability, create_by, modify_by) VALUES ($1, 1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9, $10) RETURNING id, rule_id, version, profile_id, profile_version, rule_kind, status, severity, rule_config, applicability, create_time, COALESCE(create_by, ''), modify_time, COALESCE(modify_by, '')`
	row := s.DB.QueryRowContext(ctx, stmt, strings.TrimSpace(r.RuleID), strings.TrimSpace(r.ProfileID), r.ProfileVersion, strings.TrimSpace(r.RuleKind), r.Status, r.Severity, string(r.RuleConfig), string(r.Applicability), nullableText(r.CreateBy), nullableText(r.ModifyBy))
	var out ProfileRule
	err := row.Scan(&out.ID, &out.RuleID, &out.Version, &out.ProfileID, &out.ProfileVersion, &out.RuleKind, &out.Status, &out.Severity, &out.RuleConfig, &out.Applicability, &out.CreateTime, &out.CreateBy, &out.ModifyTime, &out.ModifyBy)
	return out, err
}

// ListActiveProfileRules returns only rules whose profile and rule are both
// included in the same currently activated module release.
func (s ProfileRuleStore) ListActiveProfileRules(ctx context.Context, profileID string, profileVersion int) ([]ProfileRule, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	if strings.TrimSpace(profileID) == "" || profileVersion < 1 {
		return nil, errors.New("profile_id and profile_version are required")
	}
	const stmt = `SELECT
	pr.id, pr.rule_id, pr.version, pr.profile_id, pr.profile_version,
	pr.rule_kind, pr.status, pr.severity, pr.rule_config, pr.applicability,
	ar.release_id, pr.create_time, COALESCE(pr.create_by, ''), pr.modify_time, COALESCE(pr.modify_by, '')
FROM kb.ontology_profile_rules pr
JOIN kb.ontology_profiles p
  ON p.profile_id = pr.profile_id AND p.version = pr.profile_version
JOIN kb.ontology_module_releases r ON r.id = pr.released_in_release_id
JOIN kb.ontology_active_releases ar ON ar.release_id = r.id
WHERE ar.deactivated_at IS NULL
  AND pr.profile_id = $1 AND pr.profile_version = $2
  AND p.status = 'included_in_release'
  AND pr.status = 'included_in_release'
  AND p.released_in_release_id = pr.released_in_release_id
ORDER BY pr.rule_id, pr.version DESC`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(profileID), profileVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]ProfileRule, 0)
	for rows.Next() {
		var r ProfileRule
		if err := rows.Scan(
			&r.ID, &r.RuleID, &r.Version, &r.ProfileID, &r.ProfileVersion,
			&r.RuleKind, &r.Status, &r.Severity, &r.RuleConfig, &r.Applicability,
			&r.ReleaseID, &r.CreateTime, &r.CreateBy, &r.ModifyTime, &r.ModifyBy,
		); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// LoadReleasedRules loads a scope-pinned profile/rule set. Unlike active
// reads, it intentionally does not join the current activation pointer.
func (s ProfileRuleStore) LoadReleasedRules(ctx context.Context, profileID string, profileVersion int, releaseID int64) ([]ProfileRule, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	if strings.TrimSpace(profileID) == "" || profileVersion < 1 || releaseID == 0 {
		return nil, errors.New("profile identity and release_id are required")
	}
	const stmt = `SELECT
	pr.id, pr.rule_id, pr.version, pr.profile_id, pr.profile_version,
	pr.rule_kind, pr.status, pr.severity, pr.rule_config, pr.applicability,
	pr.released_in_release_id, pr.create_time, COALESCE(pr.create_by, ''), pr.modify_time, COALESCE(pr.modify_by, '')
FROM kb.ontology_profile_rules pr
JOIN kb.ontology_profiles p ON p.profile_id = pr.profile_id AND p.version = pr.profile_version
WHERE pr.profile_id = $1 AND pr.profile_version = $2 AND pr.released_in_release_id = $3
  AND p.released_in_release_id = $3 AND p.status = 'included_in_release' AND pr.status = 'included_in_release'
ORDER BY pr.rule_id, pr.version DESC`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(profileID), profileVersion, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []ProfileRule
	for rows.Next() {
		var r ProfileRule
		if err := rows.Scan(&r.ID, &r.RuleID, &r.Version, &r.ProfileID, &r.ProfileVersion, &r.RuleKind, &r.Status, &r.Severity, &r.RuleConfig, &r.Applicability, &r.ReleaseID, &r.CreateTime, &r.CreateBy, &r.ModifyTime, &r.ModifyBy); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// ListApprovedProfileRules returns staged rules for one module compiler
// snapshot. The profile join prevents orphan rules from being released.
func (s ProfileRuleStore) ListApprovedProfileRules(ctx context.Context, moduleID string) ([]ProfileRule, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	if strings.TrimSpace(moduleID) == "" {
		return nil, errors.New("module_id is required")
	}
	const stmt = `SELECT
	pr.id, pr.rule_id, pr.version, pr.profile_id, pr.profile_version,
	pr.rule_kind, pr.status, pr.severity, pr.rule_config, pr.applicability,
	pr.create_time, COALESCE(pr.create_by, ''), pr.modify_time, COALESCE(pr.modify_by, '')
FROM kb.ontology_profile_rules pr
JOIN kb.ontology_profiles p
  ON p.profile_id = pr.profile_id AND p.version = pr.profile_version
WHERE p.module_id = $1
  AND p.status = 'approved'
  AND pr.status = 'approved'
ORDER BY pr.rule_id, pr.version DESC`
	rows, err := s.DB.QueryContext(ctx, stmt, strings.TrimSpace(moduleID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]ProfileRule, 0)
	for rows.Next() {
		var r ProfileRule
		if err := rows.Scan(
			&r.ID, &r.RuleID, &r.Version, &r.ProfileID, &r.ProfileVersion,
			&r.RuleKind, &r.Status, &r.Severity, &r.RuleConfig, &r.Applicability,
			&r.CreateTime, &r.CreateBy, &r.ModifyTime, &r.ModifyBy,
		); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}
