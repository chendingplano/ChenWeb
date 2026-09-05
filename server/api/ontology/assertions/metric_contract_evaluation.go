package assertions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/chendingplano/deepdoc/server/api/ontology/classfoundation"
	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

// contractPayloadFields mirrors classfoundation's own local type of the same
// name -- both sides treat the JSON contract, not a shared Go type, as the
// interface (classfoundation/metric_capability_validators.go's comment
// explains why this isn't a shared exported type).
type contractPayloadFields struct {
	ValueType            string   `json:"value_type"`
	PermittedUnitTermIDs []string `json:"permitted_unit_term_ids"`
}

// metricObservationState maps a value state term ID to the short observed-
// profile observation_state string (metric-class-contract-synthesis's
// agreement check only counts "present" rows; the rest stay visible as
// evidence but never grant contract authority).
func metricObservationState(valueStateTermID string) string {
	switch valueStateTermID {
	case semantic.ValuePresent:
		return "present"
	case semantic.ValueMissing:
		return "missing"
	case semantic.ValueUnparsed:
		return "unparsed"
	case semantic.ValueDatatypeMismatch:
		return "datatype_mismatch"
	case semantic.ValueNotApplicable:
		return "not_applicable"
	default:
		return "unknown"
	}
}

// metricConformanceState is task 7.1's per-instance check: it compares this
// specific occurrence's resolved value type and unit against its class's
// *current* contract (as of just before this write), independently of
// whatever capability declarations exist on that contract. An identity_only
// contract -- the state of every newly resolved class, and of any class
// whose evidence hasn't yet met the synthesis bar -- always yields
// not_evaluated: the honest answer, not a fault (metric-ontology-v1.0-en.md
// §11.1). A value that itself isn't present (missing/unparsed) has nothing
// to compare, so it also stays not_evaluated rather than being flagged as a
// contract violation it may not actually be.
func metricConformanceState(contract classfoundation.ContractRevision, valueStateTermID, unitTermID, valueDataType string) string {
	if contract.DefinitionState == classfoundation.DefinitionIdentityOnly {
		return semantic.ConformanceNotEvaluated
	}
	if valueStateTermID != semantic.ValuePresent {
		return semantic.ConformanceNotEvaluated
	}
	var fields contractPayloadFields
	if err := json.Unmarshal([]byte(contract.ContractPayload), &fields); err != nil {
		return semantic.ConformanceNotEvaluated
	}
	if fields.ValueType == "" || len(fields.PermittedUnitTermIDs) == 0 {
		return semantic.ConformanceNotEvaluated
	}
	if fields.ValueType != valueDataType {
		return semantic.ConformanceContractViolation
	}
	for _, permitted := range fields.PermittedUnitTermIDs {
		if permitted == unitTermID {
			return semantic.Conforms
		}
	}
	return semantic.ConformanceContractViolation
}

// recordMetricContractEvidence is task 3.1/6.2/5.3's write-path orchestration,
// called after the assertion is persisted (it needs the assertion's own ID
// as evidence provenance) and still inside the same transaction. It records
// this occurrence as observed-profile evidence, attempts deterministic
// contract synthesis from all accumulated evidence for the class, and
// declares semantic:can_validate_value once a contract becomes eligible.
// None of this can change the assertion just persisted -- a promotion here
// applies to the *next* write to this class, not retroactively; the backfill
// command (task 8) catches up assertions written before their class's
// contract was promoted.
func recordMetricContractEvidence(ctx context.Context, tx *sql.Tx, classTermID string, p metricCandidatePayload, inputRecordID, assertionID int64, unitTermID, valueStateTermID string) error {
	assertionIDCopy := assertionID
	if err := (classfoundation.ObservedProfileStore{DB: tx}).Record(ctx, classfoundation.ObservedProfileObservation{
		ClassTermID:       classTermID,
		AttributeKey:      "value",
		LogicalDatatype:   p.ValueDataType,
		ValueForm:         p.ValueForm,
		UnitTermID:        unitTermID,
		ObservationState:  metricObservationState(valueStateTermID),
		AggregationMethod: "metric_lossless_writer",
		MethodVersion:     MetricLosslessWriterVersion,
		DocumentKey:       strconv.FormatInt(inputRecordID, 10),
		AssertionID:       &assertionIDCopy,
	}); err != nil {
		return fmt.Errorf("record observed-profile evidence: %w", err)
	}

	revision, _, err := classfoundation.SynthesizeContractFromObservations(ctx, tx, classTermID)
	if err != nil {
		return fmt.Errorf("synthesize class contract: %w", err)
	}
	// Declared whether this write caused the promotion or the contract was
	// already partially_defined from an earlier write -- capabilityAlreadyDeclared
	// makes the common case (already declared) a cheap no-op.
	if err := classfoundation.DeclareCanValidateValueIfEligible(ctx, tx, revision); err != nil {
		return fmt.Errorf("declare can_validate_value capability: %w", err)
	}
	return nil
}
