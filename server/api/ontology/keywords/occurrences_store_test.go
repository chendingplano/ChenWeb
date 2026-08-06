package keywords

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestInsertOccurrence(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := OccurrenceStore{DB: db}
	ctx := context.Background()

	conceptID := "kwc_test"
	decisionID := int64(42)
	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO kb.keyword_occurrences`)).
		WithArgs("document", "art:1", "", "Luminance", "luminance", "_",
			"the luminance of the display", nil, conceptID, nil, "lexical_resolved", decisionID).
		WillReturnRows(sqlmock.NewRows([]string{"occurrence_id"}).AddRow(7))

	id, err := store.InsertOccurrence(ctx, Occurrence{
		ArtifactType:     "document",
		ArtifactID:       "art:1",
		RawName:          "Luminance",
		NormKey:          "luminance",
		ContextText:      "the luminance of the display",
		ConceptID:        &conceptID,
		ResolutionStatus: "lexical_resolved",
		DecisionLogID:    &decisionID,
	})
	if err != nil {
		t.Fatalf("InsertOccurrence: %v", err)
	}
	if id != 7 {
		t.Errorf("expected occurrence_id 7, got %d", id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestInsertOccurrenceValidation(t *testing.T) {
	store := OccurrenceStore{DB: nil}
	ctx := context.Background()

	tests := []struct {
		name string
		occ  Occurrence
	}{
		{"missing raw_name", Occurrence{NormKey: "x", ResolutionStatus: "unresolved"}},
		{"missing norm_key", Occurrence{RawName: "x", ResolutionStatus: "unresolved"}},
		{"invalid resolution_status", Occurrence{RawName: "x", NormKey: "x", ResolutionStatus: "bogus"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := store.InsertOccurrence(ctx, tt.occ); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestListOccurrences(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := OccurrenceStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT `+occurrenceColumns+` `+occurrenceFrom+` WHERE norm_key = $1 AND scope = $2 ORDER BY occurrence_id DESC LIMIT $3`)).
		WithArgs("luminance", "_", 50).
		WillReturnRows(sqlmock.NewRows([]string{
			"occurrence_id", "artifact_type", "artifact_id", "field_path",
			"raw_name", "norm_key", "scope", "context_text", "chunk_ref",
			"concept_id", "term_id", "resolution_status", "decision_log_id", "create_time",
		}).AddRow(1, "document", "art:1", "", "Luminance", "luminance", "_",
			"ctx", nil, "kwc_test", nil, "lexical_resolved", 42, testNow))

	out, err := store.ListOccurrences(ctx, "luminance", "_", 0)
	if err != nil {
		t.Fatalf("ListOccurrences: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 occurrence, got %d", len(out))
	}
	if out[0].RawName != "Luminance" || out[0].ResolutionStatus != "lexical_resolved" {
		t.Errorf("unexpected occurrence: %#v", out[0])
	}
	if out[0].ConceptID == nil || *out[0].ConceptID != "kwc_test" {
		t.Errorf("expected concept link, got %v", out[0].ConceptID)
	}
	if out[0].DecisionLogID == nil || *out[0].DecisionLogID != 42 {
		t.Errorf("expected decision log link 42, got %v", out[0].DecisionLogID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestStatusForVerdictMapping(t *testing.T) {
	// The keyword family is lexical: an accepted match is lexical_resolved,
	// never term_resolved — promoting an ungoverned identity to a governed
	// one silently would erase the layer distinction (§9.5).
	if s := statusForVerdict("auto_accepted"); s != "lexical_resolved" {
		t.Errorf("auto_accepted: got %s", s)
	}
	if s := statusForVerdict("ambiguous"); s != "ambiguous" {
		t.Errorf("ambiguous: got %s", s)
	}
	if s := statusForVerdict("deferred"); s != "unresolved" {
		t.Errorf("deferred: got %s", s)
	}
	if s := statusForVerdict("human_review"); s != "unresolved" {
		t.Errorf("human_review: got %s", s)
	}
}
