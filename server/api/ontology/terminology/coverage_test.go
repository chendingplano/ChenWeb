package terminology

import (
	"context"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBuildCoverageOrdersConceptsByFrequencyThenIdentity(t *testing.T) {
	report, err := BuildCoverage(Acceptance{SchemaVersion: 1, Scope: "display", Corpus: []CorpusRef{{ArtifactType: "spec"}}, TargetCoverage: 0.5, Approver: "board", TargetSeedRelease: SeedRelease{Source: "iec-seed", Release: "v1"}}, CorpusData{
		Concepts: []ConceptRecord{
			{ConceptID: "c-z", PrefLabel: "Zulu", Scope: "display", Status: "active", ExactAuthority: true},
			{ConceptID: "c-b", PrefLabel: "Alpha", Scope: "display", Status: "provisional"},
			{ConceptID: "c-a", PrefLabel: "Alpha", Scope: "display", Status: "active"},
		},
		Surfaces: []SurfaceRecord{
			{ConceptID: "c-z", Surface: "Zulu", NormKey: "zulu", Lang: "en", Scope: "display"},
			{ConceptID: "c-b", Surface: "Alpha B", NormKey: "alpha b", Lang: "en", Scope: "display"},
			{ConceptID: "c-a", Surface: "Alpha A", NormKey: "alpha a", Lang: "en", Scope: "display"},
		},
		Occurrences: []OccurrenceRecord{
			{OccurrenceID: 1, ArtifactType: "spec", Scope: "display", ConceptID: "c-z", NormKey: "zulu"},
			{OccurrenceID: 2, ArtifactType: "spec", Scope: "display", ConceptID: "c-z", NormKey: "zulu"},
			{OccurrenceID: 3, ArtifactType: "spec", Scope: "display", ConceptID: "c-a", NormKey: "alpha a"},
			{OccurrenceID: 4, ArtifactType: "spec", Scope: "display", ConceptID: "c-b", NormKey: "alpha b"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{report.Inventory[0].ConceptID, report.Inventory[1].ConceptID, report.Inventory[2].ConceptID}
	if want := []string{"c-z", "c-a", "c-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order=%v, want %v", got, want)
	}
}

func TestBuildCoverageIsolatesSelectedScopeAndCorpus(t *testing.T) {
	acceptance := Acceptance{
		SchemaVersion: 1, Scope: "display", Corpus: []CorpusRef{{ArtifactType: "spec", ArtifactIDs: []string{"pilot-1"}}}, Approver: "board",
		TargetSeedRelease: SeedRelease{Source: "iec-seed", Release: "v1"},
	}
	report, err := BuildCoverage(acceptance, CorpusData{
		Concepts: []ConceptRecord{
			{ConceptID: "included", PrefLabel: "Luminance", Scope: "display", Status: "active"},
			{ConceptID: "wrong-scope", PrefLabel: "Luminance", Scope: "clinical", Status: "active"},
		},
		Surfaces: []SurfaceRecord{{ConceptID: "included", Surface: "luminance", NormKey: "luminance", Lang: "en", Scope: "display"}},
		Occurrences: []OccurrenceRecord{
			{OccurrenceID: 1, ArtifactType: "spec", ArtifactID: "pilot-1", Scope: "display", ConceptID: "included", NormKey: "luminance"},
			{OccurrenceID: 2, ArtifactType: "spec", ArtifactID: "other", Scope: "display", ConceptID: "included", NormKey: "luminance"},
			{OccurrenceID: 3, ArtifactType: "spec", ArtifactID: "pilot-1", Scope: "clinical", ConceptID: "wrong-scope", NormKey: "luminance"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Inventory) != 1 || report.Inventory[0].ConceptID != "included" || report.Inventory[0].Frequency != 1 {
		t.Fatalf("inventory=%+v", report.Inventory)
	}
}

func TestBuildCoverageGeneratesUnresolvedBilingualBacklog(t *testing.T) {
	report, err := BuildCoverage(Acceptance{SchemaVersion: 1, Scope: "display", Corpus: []CorpusRef{{ArtifactType: "spec"}}, Approver: "board", TargetSeedRelease: SeedRelease{Source: "iec-seed", Release: "v1"}}, CorpusData{
		Concepts: []ConceptRecord{
			{ConceptID: "english-only", PrefLabel: "Luminance", Scope: "display", Status: "active"},
			{ConceptID: "bilingual", PrefLabel: "Contrast", Scope: "display", Status: "active"},
		},
		Surfaces: []SurfaceRecord{
			{ConceptID: "english-only", Surface: "luminance", NormKey: "luminance", Lang: "en", Scope: "display"},
			{ConceptID: "bilingual", Surface: "contrast", NormKey: "contrast", Lang: "en", Scope: "display"},
			{ConceptID: "bilingual", Surface: "对比度", NormKey: "对比度", Lang: "zh", Scope: "display"},
		},
		Occurrences: []OccurrenceRecord{
			{OccurrenceID: 1, ArtifactType: "spec", RawName: "luminance", Scope: "display", ConceptID: "english-only", NormKey: "luminance"},
			{OccurrenceID: 2, ArtifactType: "spec", RawName: "contrast", Scope: "display", ConceptID: "bilingual", NormKey: "contrast"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.UnresolvedBilingualPairs) != 2 {
		t.Fatalf("backlog=%+v", report.UnresolvedBilingualPairs)
	}
	if got := report.UnresolvedBilingualPairs[0]; got.ConceptID != "bilingual" || len(got.ProposedMissingLanguageSurfaces) != 1 || got.ProposedMissingLanguageSurfaces[0].Surface != "对比度" {
		t.Fatalf("bilingual proposal=%+v", got)
	}
	if got := report.UnresolvedBilingualPairs[1]; got.ConceptID != "english-only" || len(got.ProposedMissingLanguageSurfaces) != 0 {
		t.Fatalf("english-only backlog=%+v", got)
	}
}

func TestBuildCoverageDoesNotCountOffCorpusCatalogAliasAsObserved(t *testing.T) {
	report, err := BuildCoverage(Acceptance{SchemaVersion: 1, Scope: "display", Corpus: []CorpusRef{{ArtifactType: "spec", ArtifactIDs: []string{"pilot"}}}, Approver: "board", TargetSeedRelease: SeedRelease{Source: "iec-seed", Release: "v1"}}, CorpusData{
		Concepts: []ConceptRecord{{ConceptID: "luminance", PrefLabel: "Luminance", Scope: "display", Status: "active"}},
		Surfaces: []SurfaceRecord{
			{ConceptID: "luminance", Surface: "luminance", NormKey: "luminance", Lang: "en", Scope: "display"},
			{ConceptID: "luminance", Surface: "亮度", NormKey: "亮度", Lang: "zh", Scope: "display"},
		},
		Occurrences: []OccurrenceRecord{
			{OccurrenceID: 7, ArtifactType: "spec", ArtifactID: "pilot", FieldPath: "metrics[0].name", RawName: "Luminance", Scope: "display", ConceptID: "luminance", NormKey: "luminance"},
			{OccurrenceID: 8, ArtifactType: "spec", ArtifactID: "outside", FieldPath: "metrics[1].name", RawName: "亮度", Scope: "display", ConceptID: "luminance", NormKey: "亮度"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantObserved := []ObservedSurface{{
		Surface: "Luminance", NormKey: "luminance", Lang: "en", Frequency: 1,
		Occurrences: []OccurrenceProvenance{{OccurrenceID: 7, ArtifactType: "spec", ArtifactID: "pilot", FieldPath: "metrics[0].name"}},
	}}
	if !reflect.DeepEqual(report.Inventory[0].ObservedSurfaces, wantObserved) {
		t.Fatalf("observed surfaces=%+v, want %+v", report.Inventory[0].ObservedSurfaces, wantObserved)
	}
	wantBacklog := []BilingualBacklogItem{{
		ConceptID: "luminance", PrefLabel: "Luminance", Frequency: 1,
		MissingLanguages: []string{"zh"}, ObservedSurfaces: wantObserved,
		ProposedMissingLanguageSurfaces: []CatalogSurfaceProposal{{ConceptID: "luminance", Surface: "亮度", NormKey: "亮度", Lang: "zh"}},
	}}
	if !reflect.DeepEqual(report.UnresolvedBilingualPairs, wantBacklog) {
		t.Fatalf("backlog=%+v, want %+v", report.UnresolvedBilingualPairs, wantBacklog)
	}
}

func TestBuildCoverageDoesNotInventBilingualPairAcrossConcepts(t *testing.T) {
	report, err := BuildCoverage(Acceptance{SchemaVersion: 1, Scope: "display", Corpus: []CorpusRef{{ArtifactType: "spec"}}, Approver: "board", TargetSeedRelease: SeedRelease{Source: "iec-seed", Release: "v1"}}, CorpusData{
		Concepts: []ConceptRecord{
			{ConceptID: "english", PrefLabel: "Brightness", Scope: "display", Status: "active"},
			{ConceptID: "chinese", PrefLabel: "亮度", Scope: "display", Status: "provisional"},
		},
		Surfaces: []SurfaceRecord{
			{ConceptID: "english", Surface: "brightness", NormKey: "brightness", Lang: "en", Scope: "display"},
			{ConceptID: "chinese", Surface: "亮度", NormKey: "亮度", Lang: "zh", Scope: "display"},
		},
		Occurrences: []OccurrenceRecord{
			{OccurrenceID: 1, ArtifactType: "spec", RawName: "Brightness", Scope: "display", ConceptID: "english", NormKey: "brightness"},
			{OccurrenceID: 2, ArtifactType: "spec", RawName: "亮度", Scope: "display", ConceptID: "chinese", NormKey: "亮度"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.UnresolvedBilingualPairs) != 2 {
		t.Fatalf("backlog=%+v", report.UnresolvedBilingualPairs)
	}
	for _, item := range report.UnresolvedBilingualPairs {
		if len(item.ProposedMissingLanguageSurfaces) != 0 {
			t.Fatalf("invented cross-concept proposal: %+v", item)
		}
	}
}

func TestBuildCoverageDoesNotInferLanguageFromScript(t *testing.T) {
	report, err := BuildCoverage(Acceptance{SchemaVersion: 1, Scope: "display", Corpus: []CorpusRef{{ArtifactType: "spec"}}, Approver: "board", TargetSeedRelease: SeedRelease{Source: "iec-seed", Release: "v1"}}, CorpusData{
		Concepts: []ConceptRecord{{ConceptID: "unmatched", PrefLabel: "Unmatched", Scope: "display", Status: "active"}},
		Occurrences: []OccurrenceRecord{
			{OccurrenceID: 1, ArtifactType: "spec", RawName: "Unregistered Latin", Scope: "display", ConceptID: "unmatched", NormKey: "unregistered latin"},
			{OccurrenceID: 2, ArtifactType: "spec", RawName: "未登记", Scope: "display", ConceptID: "unmatched", NormKey: "未登记"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"und"}; !reflect.DeepEqual(report.Inventory[0].Languages, want) {
		t.Fatalf("languages=%v, want %v", report.Inventory[0].Languages, want)
	}
	for _, surface := range report.Inventory[0].ObservedSurfaces {
		if surface.Lang != "und" {
			t.Fatalf("surface inferred language: %+v", surface)
		}
	}
}

func TestBuildCoverageTreatsAmbiguousCatalogLanguageMatchAsUndetermined(t *testing.T) {
	report, err := BuildCoverage(Acceptance{SchemaVersion: 1, Scope: "display", Corpus: []CorpusRef{{ArtifactType: "spec"}}, Approver: "board", TargetSeedRelease: SeedRelease{Source: "iec-seed", Release: "v1"}}, CorpusData{
		Concepts: []ConceptRecord{{ConceptID: "ambiguous-lang", PrefLabel: "Shared", Scope: "display", Status: "active"}},
		Surfaces: []SurfaceRecord{
			{ConceptID: "ambiguous-lang", Surface: "shared", NormKey: "shared", Lang: "en", Scope: "display"},
			{ConceptID: "ambiguous-lang", Surface: "shared", NormKey: "shared", Lang: "fr", Scope: "display"},
		},
		Occurrences: []OccurrenceRecord{{OccurrenceID: 1, ArtifactType: "spec", RawName: "shared", Scope: "display", ConceptID: "ambiguous-lang", NormKey: "shared"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Inventory[0].ObservedSurfaces[0].Lang; got != "und" {
		t.Fatalf("language=%q, want und", got)
	}
}

func TestBuildCoverageExcludesContextSensitiveSurfaces(t *testing.T) {
	report, err := BuildCoverage(Acceptance{SchemaVersion: 1, Scope: "display", Corpus: []CorpusRef{{ArtifactType: "spec"}}, TargetCoverage: 1, Approver: "board", TargetSeedRelease: SeedRelease{Source: "iec-seed", Release: "v1"}}, CorpusData{
		Concepts: []ConceptRecord{
			{ConceptID: "luminance", PrefLabel: "Luminance", Scope: "display", Status: "active", ExactAuthority: true},
			{ConceptID: "brightness", PrefLabel: "Brightness", Scope: "display", Status: "active"},
		},
		Surfaces: []SurfaceRecord{
			{ConceptID: "luminance", Surface: "亮度", NormKey: "亮度", Lang: "zh", Scope: "display"},
			{ConceptID: "brightness", Surface: "亮度", NormKey: "亮度", Lang: "zh", Scope: "display"},
			{ConceptID: "luminance", Surface: "luminance", NormKey: "luminance", Lang: "en", Scope: "display"},
		},
		Occurrences: []OccurrenceRecord{
			{OccurrenceID: 1, ArtifactType: "spec", Scope: "display", ConceptID: "brightness", NormKey: "亮度"},
			{OccurrenceID: 2, ArtifactType: "spec", Scope: "display", ConceptID: "luminance", NormKey: "luminance"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.EligibleFrequency != 1 || report.CoveredFrequency != 1 || !report.TargetMet {
		t.Fatalf("coverage=%+v", report)
	}
	if len(report.ContextSensitiveSurfaces) != 1 || report.ContextSensitiveSurfaces[0].NormKey != "亮度" || report.ContextSensitiveSurfaces[0].Frequency != 1 {
		t.Fatalf("context-sensitive=%+v", report.ContextSensitiveSurfaces)
	}
}

func TestBuildCoverageCalculatesTargetAndRiskReadiness(t *testing.T) {
	data := CorpusData{
		Concepts: []ConceptRecord{
			{ConceptID: "covered", PrefLabel: "Luminance", Scope: "display", Status: "active", ExactAuthority: true},
			{ConceptID: "uncovered", PrefLabel: "Contrast", Scope: "display", Status: "active"},
		},
		Surfaces: []SurfaceRecord{
			{ConceptID: "covered", Surface: "luminance", NormKey: "luminance", Lang: "en", Scope: "display"},
			{ConceptID: "uncovered", Surface: "contrast", NormKey: "contrast", Lang: "en", Scope: "display"},
		},
		Occurrences: []OccurrenceRecord{
			{OccurrenceID: 1, ArtifactType: "spec", Scope: "display", ConceptID: "covered", NormKey: "luminance"},
			{OccurrenceID: 2, ArtifactType: "spec", Scope: "display", ConceptID: "covered", NormKey: "luminance"},
			{OccurrenceID: 3, ArtifactType: "spec", Scope: "display", ConceptID: "covered", NormKey: "luminance"},
			{OccurrenceID: 4, ArtifactType: "spec", Scope: "display", ConceptID: "uncovered", NormKey: "contrast"},
		},
	}
	base := Acceptance{SchemaVersion: 1, Scope: "display", Corpus: []CorpusRef{{ArtifactType: "spec"}}, TargetCoverage: 0.75, RiskTerms: []string{"luminance"}, Approver: "ontology-board", TargetSeedRelease: SeedRelease{Source: "iec-seed", Release: "v1"}}
	report, err := BuildCoverage(base, data)
	if err != nil {
		t.Fatal(err)
	}
	if report.Coverage != 0.75 || !report.TargetMet || !report.RiskTermsMet || !report.Ready || report.Approval != "operator_required" {
		t.Fatalf("report=%+v", report)
	}
	base.TargetCoverage = 0.76
	base.RiskTerms = []string{"contrast", "missing term"}
	report, err = BuildCoverage(base, data)
	if err != nil {
		t.Fatal(err)
	}
	if report.TargetMet || report.RiskTermsMet || report.Ready {
		t.Fatalf("report unexpectedly ready: %+v", report)
	}
	if want := []string{"contrast", "missing term"}; !reflect.DeepEqual(report.UncoveredRiskTerms, want) {
		t.Fatalf("uncovered risks=%v, want %v", report.UncoveredRiskTerms, want)
	}
}

type recordingReader struct {
	query CoverageQuery
}

func (r *recordingReader) Load(_ context.Context, query CoverageQuery) (CorpusData, error) {
	r.query = query
	return CorpusData{}, nil
}

func TestMeasurePassesAcceptanceSelectionToReader(t *testing.T) {
	r := &recordingReader{}
	acceptance := Acceptance{SchemaVersion: 1, Scope: "display", Corpus: []CorpusRef{{ArtifactType: "spec", ArtifactIDs: []string{"pilot"}}}, Approver: "board", TargetSeedRelease: SeedRelease{Source: "iec-seed", Release: "v1"}}
	if _, err := Measure(context.Background(), r, acceptance); err != nil {
		t.Fatal(err)
	}
	want := CoverageQuery{Scope: "display", Corpus: acceptance.Corpus, TargetSeedRelease: acceptance.TargetSeedRelease}
	if !reflect.DeepEqual(r.query, want) {
		t.Fatalf("query=%+v, want %+v", r.query, want)
	}
}

func TestSQLReaderLoadsObservedSurfaceAndOccurrenceProvenance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.keyword_concepts c")).
		WithArgs("display", "iec-seed", "v1").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id", "pref_label", "scope", "status", "exact_authority"}).
			AddRow("c1", "Luminance", "display", "active", true))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.keyword_surfaces s")).
		WithArgs("display").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id", "surface", "norm_key", "lang", "scope"}).
			AddRow("c1", "luminance", "luminance", "en", "display"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT occurrence_id, artifact_type, artifact_id, field_path, raw_name,")).
		WithArgs("display").
		WillReturnRows(sqlmock.NewRows([]string{"occurrence_id", "artifact_type", "artifact_id", "field_path", "raw_name", "concept_id", "norm_key", "scope", "resolution_status"}).
			AddRow(9, "spec", "pilot", "metrics[0].name", "Luminance", "c1", "luminance", "display", "lexical_resolved"))

	data, err := (SQLReader{DB: db}).Load(context.Background(), CoverageQuery{Scope: "display", TargetSeedRelease: SeedRelease{Source: "iec-seed", Release: "v1"}})
	if err != nil {
		t.Fatal(err)
	}
	want := OccurrenceRecord{OccurrenceID: 9, ArtifactType: "spec", ArtifactID: "pilot", FieldPath: "metrics[0].name", RawName: "Luminance", ConceptID: "c1", NormKey: "luminance", Scope: "display", ResolutionStatus: "lexical_resolved"}
	if !reflect.DeepEqual(data.Occurrences, []OccurrenceRecord{want}) {
		t.Fatalf("occurrences=%+v", data.Occurrences)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
