package classfoundation

import (
	"context"
	"encoding/json"
	"fmt"
)

// CapabilityCanInstantiate and CapabilityCanValidateValue are the two
// governed capability term IDs this change activates (seeded in
// server/api/ontology/seed/content.go). Declared here, not imported from the
// semantic package, to avoid a new import edge -- classfoundation already
// treats term IDs as opaque strings everywhere else in this file.
const (
	CapabilityCanInstantiate   = "semantic:can_instantiate"
	CapabilityCanValidateValue = "semantic:can_validate_value"
)

// CanInstantiateValidator passes for any contract, including identity_only --
// a class can always receive source-backed instances (the metric ontology
// user manual's own framing of what identity_only means).
type CanInstantiateValidator struct{}

func (CanInstantiateValidator) ID() string      { return CapabilityCanInstantiate }
func (CanInstantiateValidator) Version() string { return "1.0.0" }

func (CanInstantiateValidator) Validate(_ context.Context, input CapabilityValidationInput) (CapabilityValidationOutcome, error) {
	evidence, _ := json.Marshal(map[string]any{"definition_state": input.ContractPayload})
	return CapabilityValidationOutcome{Result: ValidationPass, Evidence: string(evidence)}, nil
}

// contractPayloadFields is the subset of a contract's payload this change's
// synthesis writes and this validator reads. Kept local rather than shared
// with the assertions package, which builds the same shape independently --
// both sides treat the JSON contract, not a Go type, as the interface.
type contractPayloadFields struct {
	ValueType            string   `json:"value_type"`
	PermittedUnitTermIDs []string `json:"permitted_unit_term_ids"`
}

// CanValidateValueValidator passes only when the contract declares enough to
// check an instance's value against it: a value type and at least one
// permitted unit. capability_validation.go's ensureCapabilityAllowed already
// refuses to invoke this validator against an identity_only contract, so a
// well-formed partially_defined/validated contract from this change's own
// synthesizer always passes; failure here means a contract was authored some
// other way with incomplete fields.
type CanValidateValueValidator struct{}

func (CanValidateValueValidator) ID() string      { return CapabilityCanValidateValue }
func (CanValidateValueValidator) Version() string { return "1.0.0" }

func (CanValidateValueValidator) Validate(_ context.Context, input CapabilityValidationInput) (CapabilityValidationOutcome, error) {
	var fields contractPayloadFields
	if err := json.Unmarshal([]byte(input.ContractPayload), &fields); err != nil {
		evidence, _ := json.Marshal(map[string]any{"error": "contract payload is not valid JSON"})
		return CapabilityValidationOutcome{Result: ValidationFail, Evidence: "{}", FailureDetails: string(evidence)}, nil
	}
	if fields.ValueType == "" || len(fields.PermittedUnitTermIDs) == 0 {
		details, _ := json.Marshal(map[string]any{
			"reason":               "contract declares no value_type or no permitted_unit_term_ids",
			"value_type_present":   fields.ValueType != "",
			"permitted_unit_count": len(fields.PermittedUnitTermIDs),
		})
		return CapabilityValidationOutcome{Result: ValidationFail, Evidence: "{}", FailureDetails: string(details)}, nil
	}
	evidence, _ := json.Marshal(map[string]any{
		"value_type":              fields.ValueType,
		"permitted_unit_term_ids": fields.PermittedUnitTermIDs,
	})
	return CapabilityValidationOutcome{Result: ValidationPass, Evidence: string(evidence)}, nil
}

// declareMetricCapabilities is the shared dispatcher instance this change
// uses -- the two validators above, wired into ContractStore's caller so
// EnsureHeader (can_instantiate) and SynthesizeContractFromObservations
// (can_validate_value) don't each construct their own dispatcher.
func declareMetricCapabilities(db DBX) CapabilityValidationDispatcher {
	return CapabilityValidationDispatcher{DB: db, Validators: []CapabilityValidator{
		CanInstantiateValidator{}, CanValidateValueValidator{},
	}}
}

// capabilityAlreadyDeclared reports whether a contract revision already has
// a row for the given capability, so repeated calls (every write to a
// long-lived class) don't re-run a validator and append a redundant result
// row every time.
func capabilityAlreadyDeclared(ctx context.Context, db DBX, contractRevisionID int64, capabilityTermID string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
SELECT EXISTS (SELECT 1 FROM kb.ontology_class_contract_capabilities
               WHERE contract_revision_id = $1 AND capability_term_id = $2)`,
		contractRevisionID, capabilityTermID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check declared capability: %w", err)
	}
	return exists, nil
}

// DeclareCanInstantiate declares semantic:can_instantiate for a contract
// revision, once. Safe to call on every write to a long-lived class --
// capabilityAlreadyDeclared makes repeat calls a no-op.
func DeclareCanInstantiate(ctx context.Context, db DBX, revision ContractRevision) error {
	return declareOnce(ctx, db, revision, CapabilityCanInstantiate)
}

// DeclareCanValidateValueIfEligible declares semantic:can_validate_value for
// a contract revision that has left identity_only, once. A still-identity_only
// contract is a normal, expected case in this write path (most classes start
// there), not an error, so it's a silent no-op rather than surfacing
// ErrIdentityOnlyCapability up through the caller.
func DeclareCanValidateValueIfEligible(ctx context.Context, db DBX, revision ContractRevision) error {
	if revision.DefinitionState == DefinitionIdentityOnly {
		return nil
	}
	return declareOnce(ctx, db, revision, CapabilityCanValidateValue)
}

func declareOnce(ctx context.Context, db DBX, revision ContractRevision, capabilityTermID string) error {
	if err := EnsureCapabilityTermHeaders(ctx, db); err != nil {
		return err
	}
	declared, err := capabilityAlreadyDeclared(ctx, db, revision.ID, capabilityTermID)
	if err != nil {
		return err
	}
	if declared {
		return nil
	}
	_, err = declareMetricCapabilities(db).ValidateAndPersist(ctx, capabilityTermID, CapabilityValidationInput{
		ContractRevisionID: revision.ID,
		CapabilityTermID:   capabilityTermID,
		ContractPayload:    revision.ContractPayload,
		EvaluatedBy:        "metric_lossless_writer",
	})
	return err
}

// EnsureCapabilityTermHeaders makes the two governed capability terms usable
// as kb.ontology_class_contract_capabilities.capability_term_id foreign-key
// targets. That column references kb.ontology_term_headers, which the
// ontology-seed path (a plain kb.ontology_terms insert) already populates
// via the pre-existing kb_sync_ontology_term_revision_after_insert trigger
// for a term that has been seeded and released -- this call is the fallback
// for an environment where seeding hasn't run yet, so a class-contract write
// never fails on this dependency.
func EnsureCapabilityTermHeaders(ctx context.Context, db DBX) error {
	store := ContractStore{DB: db}
	for _, termID := range []string{CapabilityCanInstantiate, CapabilityCanValidateValue} {
		if _, err := store.EnsureHeader(ctx, ClassIdentity{TermID: termID, ModuleID: "semantic-processing", By: "classfoundation"}); err != nil {
			return fmt.Errorf("ensure capability term header %s: %w", termID, err)
		}
	}
	return nil
}
