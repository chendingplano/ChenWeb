package classfoundation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	ValidationPass          = "pass"
	ValidationFail          = "fail"
	ValidationIndeterminate = "indeterminate"
	ValidationError         = "error"
)

var (
	ErrCapabilityValidatorNotFound = errors.New("capability validator not found")
	ErrIdentityOnlyCapability      = errors.New("identity-only contract cannot advertise this capability")
)

// CapabilityValidationInput is the immutable contract/capability pair a
// named validator evaluates. It intentionally carries the authoritative
// contract payload rather than observed-profile data.
type CapabilityValidationInput struct {
	ContractRevisionID int64
	CapabilityTermID   string
	ContractPayload    string
	EvaluatedBy        string
}

// CapabilityValidationOutcome is the persisted result of one validator
// version. A passing outcome must include supporting JSON evidence.
type CapabilityValidationOutcome struct {
	ValidatorID      string
	ValidatorVersion string
	Result           string
	Evidence         string
	FailureDetails   string
}

// CapabilityValidator validates one declared capability of one contract
// revision. Validators are registered explicitly to keep dispatch versioned
// and deterministic.
type CapabilityValidator interface {
	ID() string
	Version() string
	Validate(context.Context, CapabilityValidationInput) (CapabilityValidationOutcome, error)
}

// CapabilityValidationDispatcher invokes registered validators and records
// the exact validator/version, outcome, evidence, and failure detail. It does
// not activate a metric writer or infer capabilities from observations.
type CapabilityValidationDispatcher struct {
	DB         DBX
	Validators []CapabilityValidator
}

func (d CapabilityValidationDispatcher) ValidateAndPersist(ctx context.Context, validatorID string, input CapabilityValidationInput) (CapabilityValidationOutcome, error) {
	validator, ok := d.validator(validatorID)
	if !ok {
		return CapabilityValidationOutcome{}, fmt.Errorf("%w: %s", ErrCapabilityValidatorNotFound, strings.TrimSpace(validatorID))
	}
	if d.DB == nil {
		return CapabilityValidationOutcome{}, errors.New("db is nil")
	}
	if input.ContractRevisionID <= 0 || strings.TrimSpace(input.CapabilityTermID) == "" {
		return CapabilityValidationOutcome{}, errors.New("contract revision ID and capability term ID are required")
	}
	if err := d.ensureCapabilityAllowed(ctx, input); err != nil {
		return CapabilityValidationOutcome{}, err
	}

	outcome, err := validator.Validate(ctx, input)
	if err != nil {
		return CapabilityValidationOutcome{}, fmt.Errorf("validate capability %s: %w", input.CapabilityTermID, err)
	}
	outcome.ValidatorID = strings.TrimSpace(validator.ID())
	outcome.ValidatorVersion = strings.TrimSpace(validator.Version())
	if err := validateCapabilityOutcome(outcome); err != nil {
		return CapabilityValidationOutcome{}, err
	}
	if err := d.persist(ctx, input, outcome); err != nil {
		return CapabilityValidationOutcome{}, err
	}
	return outcome, nil
}

func (d CapabilityValidationDispatcher) ensureCapabilityAllowed(ctx context.Context, input CapabilityValidationInput) error {
	var definitionState string
	if err := d.DB.QueryRowContext(ctx, `
SELECT definition_state FROM kb.ontology_class_contract_revisions
WHERE id = $1`, input.ContractRevisionID).Scan(&definitionState); err != nil {
		return fmt.Errorf("load contract definition state: %w", err)
	}
	if definitionState == DefinitionIdentityOnly && strings.TrimSpace(input.CapabilityTermID) != "semantic:can_instantiate" {
		return fmt.Errorf("%w: %s", ErrIdentityOnlyCapability, strings.TrimSpace(input.CapabilityTermID))
	}
	return nil
}

func (d CapabilityValidationDispatcher) validator(id string) (CapabilityValidator, bool) {
	for _, validator := range d.Validators {
		if validator != nil && strings.TrimSpace(validator.ID()) == strings.TrimSpace(id) {
			return validator, true
		}
	}
	return nil, false
}

func validateCapabilityOutcome(outcome CapabilityValidationOutcome) error {
	if strings.TrimSpace(outcome.ValidatorID) == "" || strings.TrimSpace(outcome.ValidatorVersion) == "" {
		return errors.New("validator ID and version are required")
	}
	switch outcome.Result {
	case ValidationPass, ValidationFail, ValidationIndeterminate, ValidationError:
	default:
		return fmt.Errorf("unsupported validation result %q", outcome.Result)
	}
	if outcome.Result == ValidationPass {
		evidence := strings.TrimSpace(outcome.Evidence)
		if evidence == "" || evidence == "{}" || !json.Valid([]byte(evidence)) {
			return errors.New("passing capability validation requires JSON evidence")
		}
	}
	return nil
}

func (d CapabilityValidationDispatcher) persist(ctx context.Context, input CapabilityValidationInput, outcome CapabilityValidationOutcome) error {
	capability := strings.TrimSpace(input.CapabilityTermID)
	if _, err := d.DB.ExecContext(ctx, `
INSERT INTO kb.ontology_class_contract_capabilities (
    contract_revision_id, capability_term_id, result_state, declared_time, declared_by
)
VALUES ($1, $2, 'indeterminate', NOW(), $3)
ON CONFLICT (contract_revision_id, capability_term_id) DO NOTHING`,
		input.ContractRevisionID, capability, nullable(input.EvaluatedBy)); err != nil {
		return fmt.Errorf("declare capability: %w", err)
	}
	if _, err := d.DB.ExecContext(ctx, `
INSERT INTO kb.ontology_class_capability_validation_results (
    contract_revision_id, capability_term_id, validator_id, validator_version,
    validation_result, evidence, failure_details, evaluated_by
)
VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8)`,
		input.ContractRevisionID, capability, outcome.ValidatorID, outcome.ValidatorVersion,
		outcome.Result, normalizedJSON(outcome.Evidence), nullableJSON(outcome.FailureDetails), nullable(input.EvaluatedBy)); err != nil {
		return fmt.Errorf("persist capability validation: %w", err)
	}
	state := "indeterminate"
	if outcome.Result == ValidationPass {
		state = "enabled"
	} else if outcome.Result == ValidationFail {
		state = "disabled"
	}
	if _, err := d.DB.ExecContext(ctx, `
UPDATE kb.ontology_class_contract_capabilities
SET result_state = $1
WHERE contract_revision_id = $2 AND capability_term_id = $3`, state, input.ContractRevisionID, capability); err != nil {
		return fmt.Errorf("set capability state: %w", err)
	}
	return nil
}

func nullableJSON(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
