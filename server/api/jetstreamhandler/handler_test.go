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

func TestValidateSubjectPayload_PDFParsedRejectsStartDocProcessingPayload(t *testing.T) {
	err := validateSubjectPayload("kb.pdf.parsed", `{"record_ids":[376],"doc-processors":["extract_metrics"],"failed-proc-only":true}`)
	if err == nil {
		t.Fatal("validateSubjectPayload() error = nil, want error")
	}
}

func TestValidateSubjectPayload_StartDocProcessingAcceptsRecordIDs(t *testing.T) {
	err := validateSubjectPayload("kb.pdf.start-doc-processing", `{"record_ids":[376],"doc-processors":["extract_metrics"],"failed-proc-only":true}`)
	if err != nil {
		t.Fatalf("validateSubjectPayload() error = %v, want nil", err)
	}
}

func TestNormalizeSubjectPayload_PDFParsedConvertsStringRecordIDToNumber(t *testing.T) {
	got, err := normalizeSubjectPayload("kb.pdf.parsed", `{"record_id":"376","type":"pdf","status":"success","force":true}`)
	if err != nil {
		t.Fatalf("normalizeSubjectPayload() error = %v, want nil", err)
	}
	want := `{"force":true,"record_id":376,"status":"success","type":"pdf"}`
	if got != want {
		t.Fatalf("normalizeSubjectPayload() = %s, want %s", got, want)
	}
}
