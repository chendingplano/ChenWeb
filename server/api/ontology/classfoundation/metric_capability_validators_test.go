package classfoundation

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCanInstantiateValidatorAlwaysPasses(t *testing.T) {
	outcome, err := CanInstantiateValidator{}.Validate(context.Background(), CapabilityValidationInput{
		ContractRevisionID: 1, CapabilityTermID: CapabilityCanInstantiate, ContractPayload: "identity_only",
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if outcome.Result != ValidationPass {
		t.Fatalf("result = %q, want pass", outcome.Result)
	}
	if outcome.Evidence == "" || !json.Valid([]byte(outcome.Evidence)) {
		t.Fatalf("evidence = %q, want valid non-empty JSON", outcome.Evidence)
	}
}

func TestCanValidateValueValidatorPassesWithCompletePayload(t *testing.T) {
	outcome, err := CanValidateValueValidator{}.Validate(context.Background(), CapabilityValidationInput{
		ContractRevisionID: 2, CapabilityTermID: CapabilityCanValidateValue,
		ContractPayload: `{"value_type":"number","permitted_unit_term_ids":["quantity:unit_CD-PER-M2"]}`,
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if outcome.Result != ValidationPass {
		t.Fatalf("result = %q, want pass", outcome.Result)
	}
}

func TestCanValidateValueValidatorFailsWithMissingValueType(t *testing.T) {
	outcome, err := CanValidateValueValidator{}.Validate(context.Background(), CapabilityValidationInput{
		ContractRevisionID: 3, CapabilityTermID: CapabilityCanValidateValue,
		ContractPayload: `{"permitted_unit_term_ids":["quantity:unit_CD-PER-M2"]}`,
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if outcome.Result != ValidationFail {
		t.Fatalf("result = %q, want fail", outcome.Result)
	}
}

func TestCanValidateValueValidatorFailsWithNoPermittedUnits(t *testing.T) {
	outcome, err := CanValidateValueValidator{}.Validate(context.Background(), CapabilityValidationInput{
		ContractRevisionID: 4, CapabilityTermID: CapabilityCanValidateValue,
		ContractPayload: `{"value_type":"number","permitted_unit_term_ids":[]}`,
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if outcome.Result != ValidationFail {
		t.Fatalf("result = %q, want fail", outcome.Result)
	}
}

func TestCanValidateValueValidatorFailsOnMalformedPayload(t *testing.T) {
	outcome, err := CanValidateValueValidator{}.Validate(context.Background(), CapabilityValidationInput{
		ContractRevisionID: 5, CapabilityTermID: CapabilityCanValidateValue,
		ContractPayload: `not json`,
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if outcome.Result != ValidationFail {
		t.Fatalf("result = %q, want fail", outcome.Result)
	}
}
