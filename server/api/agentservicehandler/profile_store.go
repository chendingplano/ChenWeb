package agentservicehandler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrProfileVersionConflict = errors.New("agentic service profile version conflict")

// ProfileVersionStore is the persistence boundary used by profile handlers.
type ProfileVersionStore interface {
	EnsureSeeded(context.Context, *ProfileRegistry) error
	ListActive(context.Context) ([]AgentServiceProfile, error)
	LoadVersion(context.Context, string, string) (AgentServiceProfile, error)
	ActivateVersion(context.Context, AgentServiceProfile, string, string) (AgentServiceProfile, string, error)
	RestoreActiveVersion(context.Context, string, string, string) error
}

type SQLProfileVersionStore struct{ db *sql.DB }

func NewSQLProfileVersionStore(db *sql.DB) *SQLProfileVersionStore {
	return &SQLProfileVersionStore{db: db}
}

var profileSlugSet = map[string]struct{}{"knowledge-guide": {}, "problem-diagnostics": {}}

func (s *SQLProfileVersionStore) EnsureSeeded(ctx context.Context, registry *ProfileRegistry) error {
	if registry == nil {
		return errors.New("profile registry is required")
	}
	if s == nil || s.db == nil {
		return errors.New("profile version database is unavailable")
	}
	profiles := make([]AgentServiceProfile, 0, 2)
	for _, slug := range []string{"knowledge-guide", "problem-diagnostics"} {
		version := registry.active[slug]
		profile, ok := registry.versions[slug][version]
		if !ok {
			return fmt.Errorf("seed profile %q: active version %q is missing", slug, version)
		}
		profiles = append(profiles, profile)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A transaction-scoped advisory lock serializes initialization across
	// processes, including when no rows exist yet to lock.
	var locked any
	if err = tx.QueryRowContext(ctx, `SELECT pg_advisory_xact_lock(721904251)`).Scan(&locked); err != nil {
		return err
	}
	var hasVersions bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM kb.agentic_service_profile_versions)`).Scan(&hasVersions); err != nil {
		return err
	}
	if hasVersions {
		return tx.Commit()
	}
	for _, profile := range profiles {
		profileJSON, marshalErr := marshalStoredProfile(profile)
		if marshalErr != nil {
			return marshalErr
		}
		hash := sha256.Sum256([]byte(profile.SystemPrompt))
		_, err = tx.ExecContext(ctx, `INSERT INTO kb.agentic_service_profile_versions
(service_slug, version, profile_json, prompt_ref, prompt_hash, prompt_snapshot, created_by)
			VALUES ($1, $2, $3::jsonb, $4, $5, $6, 'system')`,
			profile.Slug, profile.Version, profileJSON, profile.PromptFile, hex.EncodeToString(hash[:]), profile.SystemPrompt)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO kb.agentic_service_profiles (service_slug, active_version)
VALUES ($1, $2)`, profile.Slug, profile.Version)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

const profileSelect = `SELECT v.profile_json, v.prompt_snapshot
FROM kb.agentic_service_profiles a
JOIN kb.agentic_service_profile_versions v
  ON v.service_slug = a.service_slug AND v.version = a.active_version`

func (s *SQLProfileVersionStore) ListActive(ctx context.Context) ([]AgentServiceProfile, error) {
	rows, err := s.db.QueryContext(ctx, profileSelect+` ORDER BY a.service_slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := make([]AgentServiceProfile, 0, 2)
	for rows.Next() {
		var raw []byte
		var prompt string
		if err := rows.Scan(&raw, &prompt); err != nil {
			return nil, err
		}
		profile, err := decodeStoredProfile(raw, prompt)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (s *SQLProfileVersionStore) LoadVersion(ctx context.Context, slug, version string) (AgentServiceProfile, error) {
	if _, ok := profileSlugSet[slug]; !ok {
		return AgentServiceProfile{}, ErrProfileNotFound
	}
	var raw []byte
	var prompt string
	err := s.db.QueryRowContext(ctx, `SELECT profile_json, prompt_snapshot
FROM kb.agentic_service_profile_versions WHERE service_slug=$1 AND version=$2`, slug, version).Scan(&raw, &prompt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentServiceProfile{}, ErrProfileNotFound
	}
	if err != nil {
		return AgentServiceProfile{}, err
	}
	return decodeStoredProfile(raw, prompt)
}

func decodeStoredProfile(raw []byte, prompt string) (AgentServiceProfile, error) {
	var stored storedAgentServiceProfile
	if err := json.Unmarshal(raw, &stored); err != nil {
		return AgentServiceProfile{}, err
	}
	profile := stored.AgentServiceProfile
	profile.PilotUsers = append([]string(nil), stored.PilotUsers...)
	profile.SystemPrompt = prompt
	return profile, nil
}

type storedAgentServiceProfile struct {
	AgentServiceProfile
	PilotUsers []string `json:"pilot_users"`
}

func marshalStoredProfile(profile AgentServiceProfile) ([]byte, error) {
	return json.Marshal(storedAgentServiceProfile{
		AgentServiceProfile: profile,
		PilotUsers:          append([]string(nil), profile.PilotUsers...),
	})
}

func (s *SQLProfileVersionStore) ActivateVersion(ctx context.Context, profile AgentServiceProfile, expectedVersion, createdBy string) (AgentServiceProfile, string, error) {
	if _, ok := profileSlugSet[profile.Slug]; !ok {
		return AgentServiceProfile{}, "", ErrProfileNotFound
	}
	if strings.TrimSpace(createdBy) == "" {
		return AgentServiceProfile{}, "", errors.New("created_by is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentServiceProfile{}, "", err
	}
	defer tx.Rollback()
	var active string
	err = tx.QueryRowContext(ctx, `SELECT active_version FROM kb.agentic_service_profiles WHERE service_slug=$1 FOR UPDATE`, profile.Slug).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && active != expectedVersion) {
		return AgentServiceProfile{}, active, ErrProfileVersionConflict
	}
	if err != nil {
		return AgentServiceProfile{}, "", err
	}
	var max int
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(SUBSTRING(version FROM 2)::integer), 0)
FROM kb.agentic_service_profile_versions WHERE service_slug=$1 AND version ~ '^v[0-9]+$'`, profile.Slug).Scan(&max)
	if err != nil {
		return AgentServiceProfile{}, "", err
	}
	profile.Version = "v" + strconv.Itoa(max+1)
	profileJSON, err := marshalStoredProfile(profile)
	if err != nil {
		return AgentServiceProfile{}, "", err
	}
	hash := sha256.Sum256([]byte(profile.SystemPrompt))
	_, err = tx.ExecContext(ctx, `INSERT INTO kb.agentic_service_profile_versions
(service_slug, version, profile_json, prompt_ref, prompt_hash, prompt_snapshot, created_by)
VALUES ($1,$2,$3::jsonb,$4,$5,$6,$7)`, profile.Slug, profile.Version, profileJSON, profile.PromptFile, hex.EncodeToString(hash[:]), profile.SystemPrompt, createdBy)
	if err != nil {
		return AgentServiceProfile{}, "", err
	}
	_, err = tx.ExecContext(ctx, `UPDATE kb.agentic_service_profiles SET active_version=$2, updated_at=NOW()
WHERE service_slug=$1`, profile.Slug, profile.Version)
	if err != nil {
		return AgentServiceProfile{}, "", err
	}
	if err = tx.Commit(); err != nil {
		return AgentServiceProfile{}, "", err
	}
	return profile, active, nil
}

func (s *SQLProfileVersionStore) RestoreActiveVersion(ctx context.Context, slug, activatedVersion, previousVersion string) error {
	if _, ok := profileSlugSet[slug]; !ok {
		return ErrProfileNotFound
	}
	result, err := s.db.ExecContext(ctx, `UPDATE kb.agentic_service_profiles SET active_version=$3, updated_at=NOW()
WHERE service_slug=$1 AND active_version=$2`, slug, activatedVersion, previousVersion)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrProfileVersionConflict
	}
	return nil
}
