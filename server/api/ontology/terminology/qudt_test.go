package terminology

import (
	"os"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

func TestQUDTAdapterPreservesLanguagesAndRelationDistinctions(t *testing.T) {
	fixture := adapterFixture{"qudt", "testdata/fixtures/qudt", QUDTAdapter{}}
	_, snapshot := convertFixture(t, fixture)

	if len(snapshot.Entries) != 5 {
		t.Fatalf("entries=%+v", snapshot.Entries)
	}
	var luminance *CatalogEntry
	for i := range snapshot.Entries {
		if snapshot.Entries[i].ExternalID == "http://qudt.org/vocab/quantitykind/Luminance" {
			luminance = &snapshot.Entries[i]
		}
	}
	if luminance == nil || luminance.EntryStatus != "current" {
		t.Fatalf("luminance=%+v", luminance)
	}

	labels := map[string]string{}
	for _, label := range snapshot.Labels {
		if label.ExternalID == luminance.ExternalID {
			labels[label.Language+"\x00"+label.LabelRole] = label.Label
		}
	}
	for _, want := range []struct{ key, value string }{
		{"en\x00preferred", "Luminance"},
		{"zh\x00preferred", "亮度"},
		{"fr\x00preferred", "Luminance énergétique"},
		{"und\x00preferred", "luminance"},
	} {
		if labels[want.key] != want.value {
			t.Fatalf("label %q = %q, want %q (labels=%+v)", want.key, labels[want.key], want.value, labels)
		}
	}

	relations := map[string]CatalogRelation{}
	for _, relation := range snapshot.Relations {
		if relation.SubjectExternalID == luminance.ExternalID {
			relations[relation.Relation+"\x00"+relation.ObjectSource+"\x00"+relation.ObjectExternalID] = relation
		}
	}
	exact, ok := relations["exact_equivalent\x00bipm-sirp-quantity\x00https://si-digital-framework.org/quantities/LUMA"]
	if !ok || exact.Relation != "exact_equivalent" {
		t.Fatalf("SIRP exact crosswalk missing: %+v", relations)
	}
	wikidata, ok := relations["wikidata_match\x00wikidata\x00http://www.wikidata.org/entity/Q355386"]
	if !ok || wikidata.Relation != "wikidata_match" {
		t.Fatalf("wikidataMatch must stay proposal-only: %+v", relations)
	}
	if _, ok := relations["exact_equivalent\x00wikidata\x00http://www.wikidata.org/entity/Q355386"]; ok {
		t.Fatal("wikidataMatch must never become exact_equivalent")
	}
	if _, ok := relations["broader\x00qudt-quantity-kind\x00http://qudt.org/vocab/quantitykind/RadiometricQuantity"]; !ok {
		t.Fatalf("broader relation missing: %+v", relations)
	}
	brightnessRelations := map[string]CatalogRelation{}
	for _, relation := range snapshot.Relations {
		if relation.SubjectExternalID == "http://qudt.org/vocab/quantitykind/Brightness" {
			brightnessRelations[relation.Relation+"\x00"+relation.ObjectExternalID] = relation
		}
	}
	closeMatch, ok := brightnessRelations["close_match\x00http://qudt.org/vocab/quantitykind/Luminance"]
	if !ok || closeMatch.Relation != "close_match" {
		t.Fatalf("skos:closeMatch must be retained as a non-authoritative close_match: %+v", brightnessRelations)
	}
}

func TestQUDTAdapterRetainsDeprecationAndReplacement(t *testing.T) {
	_, snapshot := convertFixture(t, adapterFixture{"qudt", "testdata/fixtures/qudt", QUDTAdapter{}})
	byID := map[string]CatalogEntry{}
	for _, entry := range snapshot.Entries {
		byID[entry.ExternalID] = entry
	}
	old := byID["http://qudt.org/vocab/quantitykind/LuminousIntensityOld"]
	new := byID["http://qudt.org/vocab/quantitykind/LuminousIntensityNew"]
	if old.EntryStatus != "deprecated" {
		t.Fatalf("old=%+v, want deprecated", old)
	}
	if new.EntryStatus != "current" {
		t.Fatalf("new=%+v, want current", new)
	}
	foundReplacement := false
	for _, relation := range snapshot.Relations {
		if relation.SubjectExternalID == new.ExternalID && relation.Relation == "replaces" &&
			relation.ObjectExternalID == old.ExternalID {
			foundReplacement = true
		}
	}
	if !foundReplacement {
		t.Fatalf("replacement relation missing: %+v", snapshot.Relations)
	}
}

func TestQUDTSIRPCrosswalkNormalizesUnderExactPolicy(t *testing.T) {
	fixture := adapterFixture{"qudt", "testdata/fixtures/qudt", QUDTAdapter{}}
	policy, snapshot := convertFixture(t, fixture)
	if policy.AuthorityRole != keywords.ExactIdentityAuthority {
		t.Fatalf("qudt policy role=%q, want exact identity authority", policy.AuthorityRole)
	}
	if policy.AuthoritativeRelations[0] != "exact_equivalent" {
		t.Fatalf("qudt relations=%v", policy.AuthoritativeRelations)
	}
	// The SIRP fixture must resolve the same persistent identifier.
	sirpPolicy, sirpSnapshot := convertFixture(t, adapterFixture{"sirp", "testdata/fixtures/sirp", SIRPAdapter{}})
	_ = sirpPolicy
	var luma *CatalogEntry
	for i := range sirpSnapshot.Entries {
		if sirpSnapshot.Entries[i].ExternalID == "https://si-digital-framework.org/quantities/LUMA" {
			luma = &sirpSnapshot.Entries[i]
		}
	}
	if luma == nil {
		t.Fatal("SIRP fixture must contain LUMA")
	}
	var crosswalk *CatalogRelation
	for i := range snapshot.Relations {
		relation := snapshot.Relations[i]
		if relation.Relation == "exact_equivalent" && relation.ObjectSource == SIRPSourceID &&
			relation.ObjectExternalID == luma.ExternalID {
			crosswalk = &relation
		}
	}
	if crosswalk == nil {
		t.Fatal("QUDT Luminance must normalize to SIRP LUMA through the exact crosswalk")
	}
	if crosswalk.ObjectRelease != SIRPRelease {
		t.Fatalf("QUDT->SIRP crosswalk release = %q, want %q", crosswalk.ObjectRelease, SIRPRelease)
	}
	if crosswalk.ObjectSource != SIRPSourceID {
		t.Fatalf("QUDT->SIRP crosswalk source = %q, want %q", crosswalk.ObjectSource, SIRPSourceID)
	}
}

func TestParseQUDTGraphSharedWithImportCommand(t *testing.T) {
	content, err := readFixture("testdata/fixtures/qudt/quantity-kinds.ttl")
	if err != nil {
		t.Fatal(err)
	}
	resources, err := ParseQUDTGraph(content)
	if err != nil {
		t.Fatalf("ParseQUDTGraph: %v", err)
	}
	if len(resources) != 5 {
		t.Fatalf("resources=%d", len(resources))
	}
	// The shared parse must be usable from qudt-import: sorted, deterministic.
	first, err := ParseQUDTGraph(content)
	if err != nil {
		t.Fatal(err)
	}
	for i := range resources {
		if resources[i].IRI != first[i].IRI || len(resources[i].Labels) != len(first[i].Labels) {
			t.Fatalf("ParseQUDTGraph is not deterministic at %d", i)
		}
	}
}

func readFixture(path string) ([]byte, error) {
	return os.ReadFile(path)
}
