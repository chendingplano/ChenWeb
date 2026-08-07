package keywords

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
)

// AuthorityRole is the governed role of one source subset and release.
type AuthorityRole string

const (
	ExactIdentityAuthority       AuthorityRole = "exact_identity_authority"
	ConditionalIdentityAuthority AuthorityRole = "conditional_identity_authority"
	ProposalOnly                 AuthorityRole = "proposal_only"
	ContextOnly                  AuthorityRole = "context_only"
)

const (
	LicenseReviewUnreviewed = "unreviewed"
	LicenseReviewApproved   = "approved"
	LicenseReviewRejected   = "rejected"
)

var (
	ErrImmutableSourceRelease  = errors.New("source release is immutable")
	ErrImmutableSourceArtifact = errors.New("source artifact is immutable")
	sha256Pattern              = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

var allowedAuthorityRoles = map[AuthorityRole]bool{
	ExactIdentityAuthority: true, ConditionalIdentityAuthority: true,
	ProposalOnly: true, ContextOnly: true,
}

var allowedIdentityRelations = map[string]bool{
	"exact_equivalent": true,
	"related":          true,
	"broader":          true,
	"narrower":         true,
	"translation":      true,
	"probabilistic":    true,
	"other":            true,
}

// SourcePolicy is immutable governance for one source subset and release.
// IdentityAuthority is retained for the initial triple provider and is
// validated as a derived compatibility value, never as an independent grant.
type SourcePolicy struct {
	ProviderID             string
	Source                 string
	SourceSubset           string
	Release                string
	RetrievedAt            time.Time
	ContentChecksum        string
	License                string
	LicenseReviewStatus    string
	AuthorityRole          AuthorityRole
	AuthoritativeRelations []string
	AllowedScopes          []string
	Languages              []string
	AdapterVersion         string
	ProvenanceLocator      string
	ApprovedBy             string
	ApprovedAt             *time.Time
	IdentityAuthority      bool
	Notes                  string
}

// Validate rejects incomplete governance and authority shortcuts. Stage 1
// recognizes exact identity authority only; conditional policies remain
// explicitly non-authorizing until their conditions have a governed design.
func (p SourcePolicy) Validate() error {
	for name, value := range map[string]string{
		"provider_id": p.ProviderID, "source": p.Source, "source_subset": p.SourceSubset,
		"license": p.License, "adapter_version": p.AdapterVersion,
		"provenance_locator": p.ProvenanceLocator, "approved_by": p.ApprovedBy,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("source policy %s is required", name)
		}
	}
	if p.RetrievedAt.IsZero() {
		return errors.New("source policy retrieved_at is required")
	}
	if !sha256Pattern.MatchString(p.ContentChecksum) {
		return errors.New("source policy content_checksum must be 64 lowercase SHA-256 hex characters")
	}
	if p.LicenseReviewStatus != LicenseReviewApproved {
		return errors.New("source policy license review must be approved")
	}
	if !allowedAuthorityRoles[p.AuthorityRole] {
		return fmt.Errorf("unknown source authority role %q", p.AuthorityRole)
	}
	for _, relation := range p.AuthoritativeRelations {
		if !allowedIdentityRelations[relation] {
			return fmt.Errorf("unknown authoritative relation %q", relation)
		}
	}
	if len(p.AllowedScopes) == 0 {
		return errors.New("source policy requires at least one allowed scope")
	}
	for _, scope := range p.AllowedScopes {
		if strings.TrimSpace(scope) == "" {
			return errors.New("source policy allowed scopes cannot contain blank values")
		}
	}
	if len(p.Languages) == 0 {
		return errors.New("source policy requires at least one language")
	}
	for _, language := range p.Languages {
		if strings.TrimSpace(language) == "" {
			return errors.New("source policy languages cannot contain blank values")
		}
	}
	if p.ApprovedAt == nil || p.ApprovedAt.IsZero() {
		return errors.New("source policy approved_at is required")
	}

	hasExact := containsString(p.AuthoritativeRelations, "exact_equivalent")
	if p.AuthorityRole == ExactIdentityAuthority && !hasExact {
		return errors.New("exact identity authority requires exact_equivalent relation")
	}
	wantCompatibilityAuthority := p.AuthorityRole == ExactIdentityAuthority && hasExact
	if p.IdentityAuthority != wantCompatibilityAuthority {
		return errors.New("identity_authority is inconsistent with structured source policy")
	}
	return nil
}

// SourceArtifact is immutable metadata and adapter-owned payload for one input
// artifact. Changed bytes require a new artifact or release identity.
type SourceArtifact struct {
	Source            string
	Release           string
	ArtifactID        string
	ContentChecksum   string
	MediaType         string
	ProvenanceLocator string
	Payload           []byte
}

func (a SourceArtifact) normalized() (SourceArtifact, error) {
	if strings.TrimSpace(a.Source) == "" || strings.TrimSpace(a.ArtifactID) == "" || strings.TrimSpace(a.ProvenanceLocator) == "" {
		return SourceArtifact{}, errors.New("source artifact requires source, artifact_id, and provenance_locator")
	}
	if !sha256Pattern.MatchString(a.ContentChecksum) {
		return SourceArtifact{}, errors.New("source artifact content_checksum must be 64 lowercase SHA-256 hex characters")
	}
	var payload any
	if len(a.Payload) == 0 {
		a.Payload = []byte(`{}`)
	} else if err := json.Unmarshal(a.Payload, &payload); err != nil {
		return SourceArtifact{}, fmt.Errorf("source artifact payload: %w", err)
	} else {
		canonical, err := json.Marshal(payload)
		if err != nil {
			return SourceArtifact{}, fmt.Errorf("canonicalize source artifact payload: %w", err)
		}
		a.Payload = canonical
	}
	return a, nil
}

// SourcePolicyStore persists immutable source policies and artifacts and owns
// the separate mutable deployment pointer.
type SourcePolicyStore struct {
	DB DBX
}

const sourcePolicyColumns = `provider_id, source, source_subset, release, retrieved_at, content_checksum, license,
license_review_status, authority_role, authoritative_relations, allowed_scopes, languages,
adapter_version, provenance_locator, approved_by, approved_at, identity_authority, notes`

// Register inserts a source release or verifies that an exact replay is
// identical. It deliberately contains no UPDATE path.
func (s SourcePolicyStore) Register(ctx context.Context, policy SourcePolicy) error {
	policy = canonicalSourcePolicy(policy)
	if err := policy.Validate(); err != nil {
		return err
	}
	if s.DB == nil {
		return errors.New("source policy store database is nil")
	}

	return withKeywordIdentityMutation(ctx, s.DB, func(db DBX) error {
		return registerSourcePolicy(ctx, db, policy)
	})
}

func registerSourcePolicy(ctx context.Context, db DBX, policy SourcePolicy) error {
	insert := `INSERT INTO kb.keyword_sources (` + sourcePolicyColumns + `)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
	ON CONFLICT (source, release) DO NOTHING`
	if _, err := db.ExecContext(ctx, insert,
		policy.ProviderID, policy.Source, policy.SourceSubset, policy.Release, policy.RetrievedAt,
		policy.ContentChecksum, policy.License, policy.LicenseReviewStatus, policy.AuthorityRole,
		pq.Array(policy.AuthoritativeRelations), pq.Array(policy.AllowedScopes), pq.Array(policy.Languages),
		policy.AdapterVersion, policy.ProvenanceLocator, policy.ApprovedBy, policy.ApprovedAt,
		policy.IdentityAuthority, policy.Notes,
	); err != nil {
		return fmt.Errorf("insert source policy: %w", err)
	}
	row := db.QueryRowContext(ctx,
		`SELECT `+sourcePolicyColumns+` FROM kb.keyword_sources WHERE source = $1 AND release = $2`,
		policy.Source, policy.Release,
	)
	existing, err := scanSourcePolicy(row.Scan)
	if err != nil {
		return fmt.Errorf("register source policy: %w", err)
	}
	if !reflect.DeepEqual(policy, canonicalSourcePolicy(existing)) {
		return fmt.Errorf("%w: %s/%s differs from registered metadata", ErrImmutableSourceRelease, policy.Source, policy.Release)
	}
	return nil
}

// RegisterArtifact inserts an artifact or verifies an exact replay. It never
// updates artifact metadata or payload in place.
func (s SourcePolicyStore) RegisterArtifact(ctx context.Context, artifact SourceArtifact) error {
	artifact, err := artifact.normalized()
	if err != nil {
		return err
	}
	if s.DB == nil {
		return errors.New("source policy store database is nil")
	}
	insert := `INSERT INTO kb.keyword_source_artifacts
		(source, release, artifact_id, content_checksum, media_type, provenance_locator, payload)
	VALUES ($1,$2,$3,$4,$5,$6,$7)
	ON CONFLICT (source, release, artifact_id) DO NOTHING`
	if _, err := s.DB.ExecContext(ctx, insert,
		artifact.Source, artifact.Release, artifact.ArtifactID, artifact.ContentChecksum,
		artifact.MediaType, artifact.ProvenanceLocator, artifact.Payload,
	); err != nil {
		return fmt.Errorf("insert source artifact: %w", err)
	}
	var existing SourceArtifact
	if err := s.DB.QueryRowContext(ctx,
		`SELECT source, release, artifact_id, content_checksum, media_type, provenance_locator, payload FROM kb.keyword_source_artifacts WHERE source = $1 AND release = $2 AND artifact_id = $3`,
		artifact.Source, artifact.Release, artifact.ArtifactID,
	).Scan(
		&existing.Source, &existing.Release, &existing.ArtifactID, &existing.ContentChecksum,
		&existing.MediaType, &existing.ProvenanceLocator, &existing.Payload,
	); err != nil {
		return fmt.Errorf("register source artifact: %w", err)
	}
	existing, err = existing.normalized()
	if err != nil {
		return fmt.Errorf("registered source artifact is invalid: %w", err)
	}
	if existing.Source != artifact.Source || existing.Release != artifact.Release ||
		existing.ArtifactID != artifact.ArtifactID || existing.ContentChecksum != artifact.ContentChecksum ||
		existing.MediaType != artifact.MediaType || existing.ProvenanceLocator != artifact.ProvenanceLocator ||
		!bytes.Equal(existing.Payload, artifact.Payload) {
		return fmt.Errorf("%w: %s/%s/%s differs from registered metadata", ErrImmutableSourceArtifact, artifact.Source, artifact.Release, artifact.ArtifactID)
	}
	return nil
}

func scanSourcePolicy(scan func(...any) error) (SourcePolicy, error) {
	var (
		p          SourcePolicy
		relations  pq.StringArray
		scopes     pq.StringArray
		languages  pq.StringArray
		role       string
		approved   sql.NullString
		approvedAt sql.NullTime
	)
	err := scan(
		&p.ProviderID, &p.Source, &p.SourceSubset, &p.Release, &p.RetrievedAt,
		&p.ContentChecksum, &p.License, &p.LicenseReviewStatus, &role,
		&relations, &scopes, &languages, &p.AdapterVersion, &p.ProvenanceLocator,
		&approved, &approvedAt, &p.IdentityAuthority, &p.Notes,
	)
	if err != nil {
		return SourcePolicy{}, err
	}
	p.AuthorityRole = AuthorityRole(role)
	p.AuthoritativeRelations = []string(relations)
	p.AllowedScopes = []string(scopes)
	p.Languages = []string(languages)
	if approved.Valid {
		p.ApprovedBy = approved.String
	}
	if approvedAt.Valid {
		p.ApprovedAt = &approvedAt.Time
	}
	return p, nil
}

func canonicalSourcePolicy(p SourcePolicy) SourcePolicy {
	p.RetrievedAt = canonicalPostgresTime(p.RetrievedAt)
	if p.ApprovedAt != nil {
		approvedAt := canonicalPostgresTime(*p.ApprovedAt)
		p.ApprovedAt = &approvedAt
	}
	p.AuthoritativeRelations = sortedUniqueTrimmed(p.AuthoritativeRelations)
	p.AllowedScopes = sortedUniqueTrimmed(p.AllowedScopes)
	p.Languages = sortedUniqueTrimmed(p.Languages)
	return p
}

func canonicalPostgresTime(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return value.UTC().Round(time.Microsecond)
}

func sortedUniqueTrimmed(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strings.TrimSpace(value)
	}
	sort.Strings(out)
	n := 0
	for _, value := range out {
		if n == 0 || out[n-1] != value {
			out[n] = value
			n++
		}
	}
	return out[:n]
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// DeploymentAction is the audited reason for moving an enablement pointer.
type DeploymentAction string

const (
	DeploymentActivate DeploymentAction = "activate"
	DeploymentDisable  DeploymentAction = "disable"
)

type DeploymentChange struct {
	DeploymentKey string
	Source        string
	Release       string
	Enabled       bool
	Action        DeploymentAction
	ChangedBy     string
}

func (c DeploymentChange) validate() error {
	if strings.TrimSpace(c.DeploymentKey) == "" || strings.TrimSpace(c.Source) == "" || strings.TrimSpace(c.ChangedBy) == "" {
		return errors.New("deployment change requires deployment_key, source, and changed_by")
	}
	if c.Action != DeploymentActivate && c.Action != DeploymentDisable {
		return fmt.Errorf("unknown deployment action %q", c.Action)
	}
	if c.Action == DeploymentActivate && !c.Enabled {
		return errors.New("activate action requires enabled=true")
	}
	if c.Action == DeploymentDisable && c.Enabled {
		return errors.New("disable action requires enabled=false")
	}
	return nil
}

// SetDeployment mutates only the deployment pointer and appends an audit row.
// Source releases and artifacts are not changed. Exact state replay is a no-op.
func (s SourcePolicyStore) SetDeployment(ctx context.Context, change DeploymentChange) error {
	if err := change.validate(); err != nil {
		return err
	}
	return withKeywordIdentityMutation(ctx, s.DB, func(db DBX) error {
		return setDeployment(ctx, db, change)
	})
}

func setDeployment(ctx context.Context, db DBX, change DeploymentChange) error {
	var source, release string
	var enabled bool
	err := db.QueryRowContext(ctx,
		`SELECT source, release, enabled FROM kb.keyword_identity_deployments WHERE deployment_key = $1 FOR UPDATE`,
		change.DeploymentKey,
	).Scan(&source, &release, &enabled)
	if err == nil && source == change.Source && release == change.Release && enabled == change.Enabled {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read source deployment: %w", err)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO kb.keyword_identity_deployments
		(deployment_key, source, release, enabled, changed_by)
	VALUES ($1,$2,$3,$4,$5)
	ON CONFLICT (deployment_key) DO UPDATE SET
		source = EXCLUDED.source, release = EXCLUDED.release, enabled = EXCLUDED.enabled,
		changed_by = EXCLUDED.changed_by, changed_at = now()`,
		change.DeploymentKey, change.Source, change.Release, change.Enabled, change.ChangedBy,
	); err != nil {
		return fmt.Errorf("write source deployment pointer: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO kb.keyword_identity_deployment_history
		(deployment_key, source, release, enabled, action, changed_by)
	VALUES ($1,$2,$3,$4,$5,$6)`,
		change.DeploymentKey, change.Source, change.Release, change.Enabled, change.Action, change.ChangedBy,
	); err != nil {
		return fmt.Errorf("append source deployment history: %w", err)
	}
	return nil
}
