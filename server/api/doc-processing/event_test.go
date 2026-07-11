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

func TestShouldSkipLineFileGeneratedEvent(t *testing.T) {
	tests := []struct {
		name string
		evt  LineFileGeneratedEvent
		want bool
	}{
		{
			name: "success pdf is processed",
			evt:  LineFileGeneratedEvent{RecordID: 1, Type: "pdf", Status: "success"},
			want: false,
		},
		{
			name: "non pdf is skipped",
			evt:  LineFileGeneratedEvent{RecordID: 1, Type: "txt", Status: "success"},
			want: true,
		},
		{
			name: "failed status is skipped",
			evt:  LineFileGeneratedEvent{RecordID: 1, Type: "pdf", Status: "failed"},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldSkipLineFileGeneratedEvent(tc.evt); got != tc.want {
				t.Fatalf("ShouldSkipLineFileGeneratedEvent()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestSkipReasonLineFileGeneratedEvent(t *testing.T) {
	if got := skipReasonLineFileGeneratedEvent(LineFileGeneratedEvent{Type: "txt", Status: "success"}); got != `type="txt"` {
		t.Fatalf("type skip reason=%q, want %q", got, `type="txt"`)
	}
	if got := skipReasonLineFileGeneratedEvent(LineFileGeneratedEvent{Type: "pdf", Status: "failed"}); got != `status="failed"` {
		t.Fatalf("status skip reason=%q, want %q", got, `status="failed"`)
	}
	if got := skipReasonLineFileGeneratedEvent(LineFileGeneratedEvent{Type: "pdf", Status: "success"}); got != "" {
		t.Fatalf("success skip reason=%q, want empty", got)
	}
}

func TestParseLineFileGeneratedEvent_ForceClearDefaultsFalse(t *testing.T) {
	evt, err := ParseLineFileGeneratedEvent([]byte(`{"record_id":"42","force":true}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.ForceClear != false {
		t.Fatalf("ForceClear = %v, want false when omitted", evt.ForceClear)
	}
}

func TestParseLineFileGeneratedEvent_ForceClearTrue(t *testing.T) {
	evt, err := ParseLineFileGeneratedEvent([]byte(`{"record_id":"42","force":true,"force_clear":true}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.ForceClear != true {
		t.Fatalf("ForceClear = %v, want true", evt.ForceClear)
	}
}
