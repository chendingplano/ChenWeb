package assertions

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestMetricAssertionKindTermIDUsesReleasedCatalogVocabulary locks in the
// critical contract: exact_value is not a Go-maintained allowlist member.
// It becomes usable whenever the existing termExists check finds its mea term
// in an included release.
func TestMetricAssertionKindTermIDUsesReleasedCatalogVocabulary(t *testing.T) {
	if got, ok := metricAssertionKindTermID("exact_value"); !ok || got != "mea:exact_value" {
		t.Fatalf("metricAssertionKindTermID(exact_value) = (%q, %t), want (mea:exact_value, true)", got, ok)
	}
	if _, ok := metricAssertionKindTermID("unparsed"); ok {
		t.Fatal("unparsed must not be eligible for governed-term lookup")
	}
}

func TestMetricQualifiersCarryMetricDefinitionTermID(t *testing.T) {
	p := metricCandidatePayload{
		MetricName:             "Organic matter",
		MetricDefinitionTermID: "mea:organic_matter_mass_fraction",
	}
	if got := metricQualifiers(p)["metric_definition_term_id"]; got != p.MetricDefinitionTermID {
		t.Fatalf("metric_definition_term_id qualifier = %#v, want %q", got, p.MetricDefinitionTermID)
	}
}

func TestGovernedTermDependencyFingerprintChangesOnlyWhenAvailabilityChanges(t *testing.T) {
	missing := governedTermDependencyFingerprint("mea:measured_by", false, "mea:exact_value", false)
	stillMissing := governedTermDependencyFingerprint("mea:measured_by", false, "mea:exact_value", false)
	released := governedTermDependencyFingerprint("mea:measured_by", true, "mea:exact_value", true)
	if missing != stillMissing {
		t.Fatalf("identical availability must have the same fingerprint: %q != %q", missing, stillMissing)
	}
	if missing == released {
		t.Fatalf("availability change must change fingerprint: %q", missing)
	}
}

func TestMetricSubjectObjectLinkedRequiresResolvedObjectID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (\n\tSELECT 1 FROM kb.artifact_objects")).
		WithArgs(int64(416), "416_mtc_47").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	linked, err := (AssociateSemantics{DB: db}).metricSubjectObjectLinked(context.Background(), 416, "416_mtc_47")
	if err != nil || linked {
		t.Fatalf("linked=%t err=%v, want false nil", linked, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
