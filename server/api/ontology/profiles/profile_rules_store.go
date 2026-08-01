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
