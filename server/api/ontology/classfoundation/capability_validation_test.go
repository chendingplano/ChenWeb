package classfoundation

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type passingCapabilityValidator struct{}

func (passingCapabilityValidator) ID() string      { return "metric-contract" }
func (passingCapabilityValidator) Version() string { return "v1" }
func (passingCapabilityValidator) Validate(context.Context, CapabilityValidationInput) (CapabilityValidationOutcome, error) {
	return CapabilityValidationOutcome{Result: ValidationPass, Evidence: `{"rule":"numeric-range"}`}, nil
}

func TestCapabilityValidationDispatcherPersistsNamedPassingEvidence(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT definition_state FROM kb.ontology_class_contract_revisions")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"definition_state"}).AddRow(DefinitionPartiallyDefined))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.ontology_class_contract_capabilities")).
		WithArgs(int64(7), "semantic:can_validate_values", "validator").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.ontology_class_capability_validation_results")).
		WithArgs(int64(7), "semantic:can_validate_values", "metric-contract", "v1", "pass", `{"rule":"numeric-range"}`, nil, "validator").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.ontology_class_contract_capabilities")).
		WithArgs("enabled", int64(7), "semantic:can_validate_values").
		WillReturnResult(sqlmock.NewResult(0, 1))

	dispatcher := CapabilityValidationDispatcher{
		DB:         db,
		Validators: []CapabilityValidator{passingCapabilityValidator{}},
	}
	outcome, err := dispatcher.ValidateAndPersist(context.Background(), "metric-contract", CapabilityValidationInput{
		ContractRevisionID: 7,
		CapabilityTermID:   "semantic:can_validate_values",
		ContractPayload:    `{"value_type":"decimal"}`,
		EvaluatedBy:        "validator",
	})
	if err != nil {
		t.Fatalf("ValidateAndPersist: %v", err)
	}
	if outcome.Result != ValidationPass || outcome.ValidatorID != "metric-contract" || outcome.ValidatorVersion != "v1" {
		t.Fatalf("unexpected outcome: %#v", outcome)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCapabilityValidationDispatcherRefusesComparisonAndValidationForIdentityOnlyClass(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dispatcher := CapabilityValidationDispatcher{
		DB:         db,
		Validators: []CapabilityValidator{passingCapabilityValidator{}},
	}
	for _, capability := range []string{"semantic:can_compare_instances", "semantic:can_validate_values"} {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT definition_state FROM kb.ontology_class_contract_revisions")).
			WithArgs(int64(7)).
			WillReturnRows(sqlmock.NewRows([]string{"definition_state"}).AddRow(DefinitionIdentityOnly))
		_, err = dispatcher.ValidateAndPersist(context.Background(), "metric-contract", CapabilityValidationInput{
			ContractRevisionID: 7,
			CapabilityTermID:   capability,
		})
		if !errors.Is(err, ErrIdentityOnlyCapability) {
			t.Fatalf("ValidateAndPersist(%s) error = %v, want ErrIdentityOnlyCapability", capability, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCapabilityValidationDispatcherRejectsUnknownValidator(t *testing.T) {
	dispatcher := CapabilityValidationDispatcher{}
	_, err := dispatcher.ValidateAndPersist(context.Background(), "unknown", CapabilityValidationInput{ContractRevisionID: 1, CapabilityTermID: "semantic:can_compare_instances"})
	if !errors.Is(err, ErrCapabilityValidatorNotFound) {
		t.Fatalf("ValidateAndPersist error = %v, want ErrCapabilityValidatorNotFound", err)
	}
}
