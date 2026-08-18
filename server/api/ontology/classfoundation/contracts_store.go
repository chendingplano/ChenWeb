package classfoundation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DBX is the database subset used by foundation stores. It permits callers to
// provide either a database handle or the transaction that governs a class
// resolution and its contract append.
type DBX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const (
	DefinitionIdentityOnly     = "identity_only"
	DefinitionPartiallyDefined = "partially_defined"
	DefinitionValidated        = "validated"
)

// ClassIdentity contains the stable identity metadata required to create a
// provisional class. A contract revision is deliberately separate.
type ClassIdentity struct {
	TermID   string
	ModuleID string
	By       string
}

// ContractRevision is the authoritative, append-only definition attached to
// one stable class identity. JSON fields are retained verbatim as governed
// contract/provenance documents rather than inferred from observed profiles.
type ContractRevision struct {
	ID                    int64
	TermID                string
	Revision              int
	ContractSchemaVersion string
	IdentitySchemaVersion string
	DefinitionState       string
	ContractPayload       string
	SynthesisMethod       string
	Confidence            *float64
	PolicyVersion         string
	Provenance            string
	CreateTime            time.Time
	CreateBy              string
}

// ContractStore persists stable class identities and their immutable contract
// history. It does not infer contracts from observations or enable writers.
type ContractStore struct {
	DB DBX
}

// CreateIdentityOnlyClass creates a stable class header and its first,
// intentionally capability-free contract revision. Repeating the operation
// for an existing term appends a further identity-only revision; callers that
// need idempotent resolution must resolve first (task 5.2).
func (s ContractStore) CreateIdentityOnlyClass(ctx context.Context, identity ClassIdentity) (ContractRevision, error) {
	if s.DB == nil {
		return ContractRevision{}, errors.New("db is nil")
	}
	termID := strings.TrimSpace(identity.TermID)
	moduleID := strings.TrimSpace(identity.ModuleID)
	if termID == "" || moduleID == "" {
		return ContractRevision{}, errors.New("term_id and module_id are required")
	}
	if _, err := s.DB.ExecContext(ctx, `
INSERT INTO kb.ontology_term_headers (term_id, term_kind, module_id, create_time, create_by)
VALUES ($1, 'class', $2, NOW(), $3)
ON CONFLICT (term_id) DO NOTHING`, termID, moduleID, nullable(identity.By)); err != nil {
		return ContractRevision{}, fmt.Errorf("create class header: %w", err)
	}
	return s.AppendContractRevision(ctx, ContractRevision{
		TermID:                termID,
		ContractSchemaVersion: "contract/v1",
		IdentitySchemaVersion: "identity/v1",
		DefinitionState:       DefinitionIdentityOnly,
		ContractPayload:       "{}",
		SynthesisMethod:       "deterministic_resolution",
		Provenance:            "{}",
		CreateBy:              identity.By,
	})
}

// AppendContractRevision adds a new immutable revision and advances only the
// header pointer. It never changes the stable term_id.
func (s ContractStore) AppendContractRevision(ctx context.Context, revision ContractRevision) (ContractRevision, error) {
	if err := validateContractRevision(revision); err != nil {
		return ContractRevision{}, err
	}
	if s.DB == nil {
		return ContractRevision{}, errors.New("db is nil")
	}
	termID := strings.TrimSpace(revision.TermID)
	row := s.DB.QueryRowContext(ctx, `
INSERT INTO kb.ontology_class_contract_revisions (
    term_id, revision, contract_schema_version, identity_schema_version,
    definition_state, contract_payload, synthesis_method, confidence,
    policy_version, provenance, create_by
)
VALUES (
    $1, (SELECT COALESCE(MAX(revision), 0) + 1
          FROM kb.ontology_class_contract_revisions WHERE term_id = $1),
    $2, $3, $4, $5::jsonb, $6, $7, $8, $9::jsonb, $10
)
RETURNING id, term_id, revision, definition_state, create_time`,
		termID, strings.TrimSpace(revision.ContractSchemaVersion), strings.TrimSpace(revision.IdentitySchemaVersion),
		revision.DefinitionState, normalizedJSON(revision.ContractPayload), strings.TrimSpace(revision.SynthesisMethod),
		revision.Confidence, nullable(revision.PolicyVersion), normalizedJSON(revision.Provenance), nullable(revision.CreateBy))
	if err := row.Scan(&revision.ID, &revision.TermID, &revision.Revision, &revision.DefinitionState, &revision.CreateTime); err != nil {
		return ContractRevision{}, fmt.Errorf("append class contract revision: %w", err)
	}
	if _, err := s.DB.ExecContext(ctx, `
UPDATE kb.ontology_term_headers
SET current_contract_revision_id = $1
WHERE term_id = $2`, revision.ID, termID); err != nil {
		return ContractRevision{}, fmt.Errorf("advance current contract revision: %w", err)
	}
	return revision, nil
}

func validateContractRevision(revision ContractRevision) error {
	if strings.TrimSpace(revision.TermID) == "" {
		return errors.New("term_id is required")
	}
	if !map[string]bool{
		DefinitionIdentityOnly: true, DefinitionPartiallyDefined: true, DefinitionValidated: true,
	}[revision.DefinitionState] {
		return fmt.Errorf("unsupported definition state %q", revision.DefinitionState)
	}
	if strings.TrimSpace(revision.ContractSchemaVersion) == "" || strings.TrimSpace(revision.IdentitySchemaVersion) == "" {
		return errors.New("contract and identity schema versions are required")
	}
	if strings.TrimSpace(revision.SynthesisMethod) == "" {
		return errors.New("synthesis method is required")
	}
	return nil
}

func normalizedJSON(value string) string {
	if value = strings.TrimSpace(value); value == "" {
		return "{}"
	}
	return value
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
