package terminology

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

type adapterFixture struct {
	name    string
	dir     string
	adapter Adapter
}

var adapterFixtures = []adapterFixture{
	{"sirp", "testdata/fixtures/sirp", SIRPAdapter{}},
	{"iec", "testdata/fixtures/iec", IECSeedAdapter{}},
	{"wikidata", "testdata/fixtures/wikidata", WikidataAdapter{}},
	{"ucum", "testdata/fixtures/ucum", UCUMAdapter{}},
	{"qudt", "testdata/fixtures/qudt", QUDTAdapter{}},
}

func convertFixture(t *testing.T, fixture adapterFixture) (keywords.SourcePolicy, CatalogSnapshot) {
	t.Helper()
	manifest, artifacts, err := ParseAndVerifyManifest(filepath.Join(fixture.dir, "manifest.json"))
	if err != nil {
		t.Fatalf("%s manifest: %v", fixture.name, err)
	}
	policy := manifest.Policy.SourcePolicy()
	snapshot, err := fixture.adapter.Convert(context.Background(), policy, artifacts)
	if err != nil {
		t.Fatalf("%s convert: %v", fixture.name, err)
	}
	return policy, snapshot
}

func convertMalformed(t *testing.T, fixture adapterFixture, malformedPath, wantError string) {
	t.Helper()
	manifest, _, err := ParseAndVerifyManifest(filepath.Join(fixture.dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(malformedPath)
	if err != nil {
		t.Fatal(err)
	}
	artifact := VerifiedArtifact{
		ManifestArtifact: ManifestArtifact{
			ID: filepath.Base(malformedPath), Path: malformedPath, SHA256: "00", MediaType: "text/plain",
			ProvenanceLocator: "https://example.test/malformed",
		},
		ResolvedPath: malformedPath, Content: content,
	}
	_, err = fixture.adapter.Convert(context.Background(), manifest.Policy.SourcePolicy(), []VerifiedArtifact{artifact})
	if err == nil || !strings.Contains(err.Error(), wantError) {
		t.Fatalf("%s %s error = %v, want containing %q", fixture.name, filepath.Base(malformedPath), err, wantError)
	}
}

func TestAdapterContractDeterministicOrderingAndReplay(t *testing.T) {
	for _, fixture := range adapterFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			_, first := convertFixture(t, fixture)
			_, second := convertFixture(t, fixture)
			firstJSON, err := json.Marshal(first)
			if err != nil {
				t.Fatal(err)
			}
			secondJSON, err := json.Marshal(second)
			if err != nil {
				t.Fatal(err)
			}
			if string(firstJSON) != string(secondJSON) {
				t.Fatalf("%s conversion is not deterministic:\n%s\n%s", fixture.name, firstJSON, secondJSON)
			}
			if len(first.Entries) > 0 && first.Entries[0].ExternalID > first.Entries[len(first.Entries)-1].ExternalID {
				t.Fatalf("%s entries are not sorted", fixture.name)
			}
		})
	}
}

func TestAdapterContractProvenanceRetainedOnEveryRow(t *testing.T) {
	for _, fixture := range adapterFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			_, snapshot := convertFixture(t, fixture)
			for i, entry := range snapshot.Entries {
				if entry.ProvenanceLocator == "" {
					t.Fatalf("%s entry[%d] has no provenance", fixture.name, i)
				}
			}
			for i, label := range snapshot.Labels {
				if label.ProvenanceLocator == "" {
					t.Fatalf("%s label[%d] has no provenance", fixture.name, i)
				}
			}
			for i, relation := range snapshot.Relations {
				if relation.ProvenanceLocator == "" {
					t.Fatalf("%s relation[%d] has no provenance", fixture.name, i)
				}
			}
			for i, negative := range snapshot.NegativeDecisions {
				if negative.ProvenanceLocator == "" {
					t.Fatalf("%s negative[%d] has no provenance", fixture.name, i)
				}
			}
			for i, code := range snapshot.UCUMCodes {
				if code.ProvenanceLocator == "" {
					t.Fatalf("%s ucum[%d] has no provenance", fixture.name, i)
				}
			}
		})
	}
}

func TestAdapterContractFailClosedUnknownRelations(t *testing.T) {
	cases := []struct {
		fixture       adapterFixture
		malformedPath string
		wantError     string
	}{
		{adapterFixture{"sirp", "testdata/fixtures/sirp", SIRPAdapter{}}, "testdata/malformed/sirp/unknown-relation.ttl", "unknown SIRP relation predicate"},
		{adapterFixture{"sirp", "testdata/fixtures/sirp", SIRPAdapter{}}, "testdata/malformed/sirp/not-turtle.ttl", "parse"},
		{adapterFixture{"qudt", "testdata/fixtures/qudt", QUDTAdapter{}}, "testdata/malformed/qudt/unknown-relation.ttl", "unknown QUDT relation predicate"},
		{adapterFixture{"qudt", "testdata/fixtures/qudt", QUDTAdapter{}}, "testdata/malformed/qudt/not-turtle.ttl", "parse"},
		{adapterFixture{"iec", "testdata/fixtures/iec", IECSeedAdapter{}}, "testdata/malformed/iec/unreviewed-exact.json", "reviewer"},
		{adapterFixture{"iec", "testdata/fixtures/iec", IECSeedAdapter{}}, "testdata/malformed/iec/copied-definition.json", "unknown field"},
		{adapterFixture{"iec", "testdata/fixtures/iec", IECSeedAdapter{}}, "testdata/malformed/iec/duplicate-entry.json", "duplicated"},
		{adapterFixture{"wikidata", "testdata/fixtures/wikidata", WikidataAdapter{}}, "testdata/malformed/wikidata/unknown-statement.jsonl", "unknown wikidata statement type"},
		{adapterFixture{"wikidata", "testdata/fixtures/wikidata", WikidataAdapter{}}, "testdata/malformed/wikidata/duplicate-qid.jsonl", "duplicated"},
		{adapterFixture{"ucum", "testdata/fixtures/ucum", UCUMAdapter{}}, "testdata/malformed/ucum/duplicate-code.xml", "duplicated"},
		{adapterFixture{"ucum", "testdata/fixtures/ucum", UCUMAdapter{}}, "testdata/malformed/ucum/missing-code.xml", "missing its Code"},
	}
	for _, tc := range cases {
		t.Run(filepath.Base(tc.malformedPath), func(t *testing.T) {
			convertMalformed(t, tc.fixture, tc.malformedPath, tc.wantError)
		})
	}
}

func TestSIRPAdapterPreservesIdentitiesLanguagesAndDeprecation(t *testing.T) {
	_, snapshot := convertFixture(t, adapterFixture{"sirp", "testdata/fixtures/sirp", SIRPAdapter{}})
	if len(snapshot.Entries) != 2 {
		t.Fatalf("entries=%+v", snapshot.Entries)
	}
	luma := snapshot.Entries[0]
	if luma.ExternalID != "https://si-digital-framework.org/quantities/LUMA" || luma.EntryStatus != "current" {
		t.Fatalf("luma=%+v", luma)
	}
	obsolete := snapshot.Entries[1]
	if obsolete.ExternalID != "https://si-digital-framework.org/quantities/LUMINANCE-OBSOLETE" || obsolete.EntryStatus != "deprecated" {
		t.Fatalf("obsolete=%+v", obsolete)
	}
	for _, entry := range snapshot.Entries {
		if entry.ExternalID == "https://si-digital-framework.org/quantities/" {
			t.Fatalf("the ontology header must not be imported as an entry: %+v", snapshot.Entries)
		}
	}

	lumaLabels := map[string]string{}
	obsoleteLabels := map[string]string{}
	for _, label := range snapshot.Labels {
		if label.ExternalID == luma.ExternalID {
			lumaLabels[label.Language+"\x00"+label.LabelRole] = label.Label
		} else {
			obsoleteLabels[label.Language+"\x00"+label.LabelRole] = label.Label
		}
	}
	if lumaLabels["en\x00preferred"] != "luminance" || lumaLabels["fr\x00preferred"] != "luminance énergétique" ||
		lumaLabels["en\x00alternative"] != "photometric luminance" {
		t.Fatalf("luma labels=%+v", lumaLabels)
	}
	if obsoleteLabels["en\x00preferred"] != "luminance (obsolete)" {
		t.Fatalf("obsolete labels=%+v", obsoleteLabels)
	}
	// The duplicate luminance@en triple must be deduplicated.
	enPreferred := 0
	for _, label := range snapshot.Labels {
		if label.ExternalID == luma.ExternalID && label.Language == "en" && label.LabelRole == "preferred" && label.Label == "luminance" {
			enPreferred++
		}
	}
	if enPreferred != 1 {
		t.Fatalf("duplicate label was not deduplicated (count=%d)", enPreferred)
	}

	if len(snapshot.Relations) != 2 {
		t.Fatalf("relations=%+v", snapshot.Relations)
	}
	for _, relation := range snapshot.Relations {
		if relation.Relation != "exact_equivalent" || relation.ObjectExternalID == "" {
			t.Fatalf("relation=%+v", relation)
		}
	}
}

func TestIECSeedAdapterDistinctIEVIdentitiesAndExactBilingualMapping(t *testing.T) {
	policy, snapshot := convertFixture(t, adapterFixture{"iec", "testdata/fixtures/iec", IECSeedAdapter{}})
	if len(snapshot.Entries) != 2 {
		t.Fatalf("entries=%+v", snapshot.Entries)
	}
	if snapshot.Entries[0].ExternalID != "845-21-050" || snapshot.Entries[1].ExternalID != "845-22-059" {
		t.Fatalf("IEV identities must remain distinct: %+v", snapshot.Entries)
	}

	labels := map[string]string{}
	for _, label := range snapshot.Labels {
		labels[label.ExternalID+"\x00"+label.Language] = label.Label
	}
	if labels["845-21-050\x00en"] != "luminance" || labels["845-21-050\x00zh"] != "亮度" {
		t.Fatalf("luminance labels=%+v", labels)
	}
	if labels["845-22-059\x00en"] != "brightness" || labels["845-22-059\x00zh"] != "视亮度" {
		t.Fatalf("brightness labels=%+v", labels)
	}

	if len(snapshot.NegativeDecisions) != 1 {
		t.Fatalf("negative decisions=%+v", snapshot.NegativeDecisions)
	}
	negative := snapshot.NegativeDecisions[0]
	if negative.SubjectExternalID != "845-21-050" || negative.ObjectExternalID != "845-22-059" ||
		negative.Relation != "different_from" || negative.ObjectSource != policy.Source {
		t.Fatalf("negative=%+v", negative)
	}
	// Reviewed decision metadata must be retained in the native payload.
	entry := snapshot.Entries[0]
	var payload map[string]any
	if err := json.Unmarshal(entry.NativePayload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["decision"] != "exact" || payload["scope"] != "display" || payload["reviewer"] != "domain-board" {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestWikidataAdapterStreamsProposalRows(t *testing.T) {
	policy, snapshot := convertFixture(t, adapterFixture{"wikidata", "testdata/fixtures/wikidata", WikidataAdapter{}})
	if len(snapshot.Entries) != 2 {
		t.Fatalf("entries=%+v", snapshot.Entries)
	}
	if snapshot.Entries[0].ExternalID != "Q355386" || snapshot.Entries[0].EntryStatus != "proposal" {
		t.Fatalf("entry=%+v", snapshot.Entries[0])
	}
	if len(snapshot.Labels) != 5 {
		t.Fatalf("labels=%+v", snapshot.Labels)
	}
	if len(snapshot.Relations) != 2 {
		t.Fatalf("relations=%+v", snapshot.Relations)
	}
	if len(snapshot.NegativeDecisions) != 2 {
		t.Fatalf("negative decisions=%+v", snapshot.NegativeDecisions)
	}
	var payload map[string]any
	if err := json.Unmarshal(snapshot.Entries[0].NativePayload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["revision"] != float64(123456789) {
		t.Fatalf("revision payload=%+v", payload)
	}
	externalIDs := payload["external_ids"].([]any)
	if len(externalIDs) != 1 || externalIDs[0].(map[string]any)["property"] != "P1628" {
		t.Fatalf("external ids=%+v", externalIDs)
	}

	var unitRelation *CatalogRelation
	for i := range snapshot.Relations {
		if snapshot.Relations[i].Relation == "unit_statement" {
			unitRelation = &snapshot.Relations[i]
		}
	}
	if unitRelation == nil || unitRelation.ObjectExternalID != "cd/m2" || unitRelation.ObjectSource != "ucum" {
		t.Fatalf("unit statement=%+v", unitRelation)
	}
	if policy.AuthorityRole != keywords.ProposalOnly {
		t.Fatalf("wikidata policy role=%q, want proposal_only", policy.AuthorityRole)
	}
}

func TestUCUMAdapterEmitsUnitCodesOnly(t *testing.T) {
	_, snapshot := convertFixture(t, adapterFixture{"ucum", "testdata/fixtures/ucum", UCUMAdapter{}})
	if len(snapshot.Entries) != 0 || len(snapshot.Labels) != 0 || len(snapshot.Relations) != 0 || len(snapshot.NegativeDecisions) != 0 {
		t.Fatalf("ucum adapter must emit no identity rows: %+v", snapshot)
	}
	if len(snapshot.UCUMCodes) != 3 {
		t.Fatalf("ucum codes=%+v", snapshot.UCUMCodes)
	}
	byCode := map[string]UCUMCode{}
	for _, code := range snapshot.UCUMCodes {
		byCode[code.Code] = code
	}
	if byCode["cd"].PrintSymbol != "cd" || byCode["cd"].Dimension != "J" {
		t.Fatalf("cd=%+v", byCode["cd"])
	}
	if byCode["cd/m2"].PrintSymbol != "cd/m²" || byCode["cd/m2"].Dimension != "J/L2" {
		t.Fatalf("cd/m2=%+v", byCode["cd/m2"])
	}
}

func TestUCUMAdapterAcceptsASCIIDeclaredEssence(t *testing.T) {
	// The official UCUM essence declares encoding="ascii"; Go's xml decoder
	// rejects that unless a CharsetReader is installed.
	content := []byte(`<?xml version="1.0" encoding="ascii"?>
<root xmlns="http://unitsofmeasure.org/ucum-essence">
  <unit Code="cd" CODE="cd" dim="J" isMetric="yes" class="si"><printSymbol>cd</printSymbol><name>candela</name></unit>
  <unit Code="m" CODE="m" dim="L" isMetric="yes" class="si"><printSymbol>m</printSymbol><name>metre</name></unit>
</root>`)
	snapshot, err := (UCUMAdapter{}).Convert(context.Background(), keywords.SourcePolicy{Source: "ucum", Release: "2.2"}, []VerifiedArtifact{{
		ManifestArtifact: ManifestArtifact{
			ID: "ucum-essence.xml", Path: "ucum-essence.xml", SHA256: "00",
			MediaType: "application/xml", ProvenanceLocator: "https://example.test/essence.xml",
		},
		ResolvedPath: "ucum-essence.xml", Content: content,
	}})
	if err != nil {
		t.Fatalf("convert ascii-declared essence: %v", err)
	}
	if len(snapshot.UCUMCodes) != 2 {
		t.Fatalf("ucum codes=%+v", snapshot.UCUMCodes)
	}
	if snapshot.UCUMCodes[0].Code != "cd" || snapshot.UCUMCodes[1].Code != "m" {
		t.Fatalf("unexpected codes: %+v", snapshot.UCUMCodes)
	}
}
