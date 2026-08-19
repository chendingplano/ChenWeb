package assertions

import "testing"

func TestNullableObjectLiteralUsesSQLNullForMissingPayload(t *testing.T) {
	if got := nullableObjectLiteral(nil); got != nil {
		t.Fatalf("nullableObjectLiteral(nil) = %#v, want SQL NULL", got)
	}
	if got := nullableObjectLiteral([]byte(`{"value": 42}`)); got != `{"value": 42}` {
		t.Fatalf("nullableObjectLiteral(value) = %#v, want JSON text", got)
	}
}
