package jetstreamhandler

import "testing"

func TestValidateSubjectPayload_LineFileGeneratedAcceptsValidJSON(t *testing.T) {
	err := validateSubjectPayload("kb.line-file-generated", `{"record_id":"1","type":"pdf","status":"success"}`)
	if err != nil {
		t.Fatalf("validateSubjectPayload() error = %v, want nil", err)
	}
}

func TestValidateSubjectPayload_LineFileGeneratedRejectsInvalidJSON(t *testing.T) {
	err := validateSubjectPayload("kb.line-file-generated", `{"record_id":"1""type":"pdf","status":"success"}`)
	if err == nil {
		t.Fatal("validateSubjectPayload() error = nil, want error")
	}
}
