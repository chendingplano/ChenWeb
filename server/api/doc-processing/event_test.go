package docprocessing

import (
	"strings"
	"testing"
)

func TestParseLineFileGeneratedEvent_AcceptsJSONEncodedObjectString(t *testing.T) {
	payload := []byte(`"{\"record_id\":\"1\",\"type\":\"pdf\",\"status\":\"success\"}"`)

	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		t.Fatalf("ParseLineFileGeneratedEvent() error = %v", err)
	}
	if evt.RecordID != 1 {
		t.Fatalf("RecordID=%d, want 1", evt.RecordID)
	}
	if evt.Type != "pdf" {
		t.Fatalf("Type=%q, want pdf", evt.Type)
	}
	if evt.Status != "success" {
		t.Fatalf("Status=%q, want success", evt.Status)
	}
}

func TestParseLineFileGeneratedEvent_InvalidPayloadIncludesPreview(t *testing.T) {
	_, err := ParseLineFileGeneratedEvent([]byte(`{"record_id":"1""type":"pdf"}`))
	if err == nil {
		t.Fatal("ParseLineFileGeneratedEvent() error = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, `invalid JSON payload preview="{\"record_id\":\"1\"\"type\":\"pdf\"}"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
