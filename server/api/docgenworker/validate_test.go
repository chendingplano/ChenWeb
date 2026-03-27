package docgenworker_test

import (
	"testing"

	"github.com/chendingplano/deepdoc/server/api/docgenworker"
)

func TestValidateSQLStatement_AcceptsSelect(t *testing.T) {
	if err := docgenworker.ValidateSQLStatement("SELECT id, name FROM customers"); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateSQLStatement_RejectsInsert(t *testing.T) {
	if err := docgenworker.ValidateSQLStatement("INSERT INTO foo VALUES (1)"); err == nil {
		t.Fatal("expected error for INSERT, got nil")
	}
}

func TestValidateSQLStatement_RejectsEmpty(t *testing.T) {
	if err := docgenworker.ValidateSQLStatement(""); err == nil {
		t.Fatal("expected error for empty SQL, got nil")
	}
}

func TestValidateConverter_AcceptsValidJSON(t *testing.T) {
	conv := `{"cust_id":"customer_id","cust_name":"customer_name","cust_email":"email"}`
	m, err := docgenworker.ValidateConverter(conv)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if m["cust_id"] != "customer_id" {
		t.Fatalf("expected customer_id mapping, got: %v", m)
	}
}

func TestValidateConverter_RejectsMissingRequiredValue(t *testing.T) {
	conv := `{"cust_id":"customer_id","cust_name":"customer_name"}`
	if _, err := docgenworker.ValidateConverter(conv); err == nil {
		t.Fatal("expected error for missing email mapping, got nil")
	}
}

func TestValidateConverter_RejectsInvalidJSON(t *testing.T) {
	if _, err := docgenworker.ValidateConverter("not json"); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
