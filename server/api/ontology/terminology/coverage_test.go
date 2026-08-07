package terminology

import (
	"context"
	"reflect"
	"testing"
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
			{OccurrenceID: 1, ArtifactType: "spec", Scope: "display", ConceptID: "english-only", NormKey: "luminance"},
			{OccurrenceID: 2, ArtifactType: "spec", Scope: "display", ConceptID: "bilingual", NormKey: "contrast"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []BilingualBacklogItem{{ConceptID: "english-only", PrefLabel: "Luminance", Frequency: 1, MissingLanguages: []string{"zh"}}}
	if !reflect.DeepEqual(report.UnresolvedBilingualPairs, want) {
		t.Fatalf("backlog=%+v, want %+v", report.UnresolvedBilingualPairs, want)
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
