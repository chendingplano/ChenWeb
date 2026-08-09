package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
)

const quantityKindsFixture = "../../api/ontology/terminology/testdata/fixtures/qudt/quantity-kinds.ttl"

func TestParseTermsFromFileQuantityKindsFixture(t *testing.T) {
	terms := parseTermsFromFile(quantityKindsFixture)
	if len(terms) != 4 {
		t.Fatalf("parsed %d terms, want 4 (deprecated LuminousIntensityOld skipped)", len(terms))
	}
	byID := map[string]terminology.QUDTImportedTerm{}
	for _, term := range terms {
		byID[term.TermID] = term
	}
	luminance, ok := byID["quantity:qk_Luminance"]
	if !ok {
		t.Fatalf("Luminance missing: %+v", byID)
	}
	if luminance.Kind != "quantity_kind" || luminance.SourceIRI != "http://qudt.org/vocab/quantitykind/Luminance" {
		t.Fatalf("luminance=%+v", luminance)
	}
	if luminance.PrefLabel != "Luminance" {
		t.Fatalf("luminance prefLabel=%q, want English preferred label", luminance.PrefLabel)
	}
	if luminance.Symbol != "Lv" {
		t.Fatalf("luminance symbol=%q, want Lv", luminance.Symbol)
	}
	if _, ok := byID["quantity:qk_LuminousIntensityOld"]; ok {
		t.Fatal("deprecated LuminousIntensityOld must not be imported")
	}
}

func TestParseTermsFromFileClassifiesEachClassFromItsOwnRDFType(t *testing.T) {
	dir := t.TempDir()
	writeTTL := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	units := writeTTL("units.ttl", `@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix owl: <http://www.w3.org/2002/07/owl#> .
@prefix qudt: <http://qudt.org/schema/qudt/> .

<http://qudt.org/vocab/unit/M> a qudt:Unit ;
    rdfs:label "Metre"@en ;
    qudt:symbol "m" .

<http://qudt.org/vocab/unit/OLD> a qudt:Unit ;
    owl:deprecated true ;
    rdfs:label "old unit"@en .
`)
	quantityKinds := writeTTL("quantity-kinds.ttl", `@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix qudt: <http://qudt.org/schema/qudt/> .

<http://qudt.org/vocab/quantitykind/LuminousIntensity> a qudt:QuantityKind ;
    rdfs:label "luminous intensity"@en .
`)
	dimensions := writeTTL("dimensions.ttl", `@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix qudt: <http://qudt.org/schema/qudt/> .

<http://qudt.org/vocab/dimensionvector/L> a qudt:DimensionVector ;
    rdfs:label "Length"@en .
`)
	combined := writeTTL("qudt-all.ttl", `@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix qudt: <http://qudt.org/schema/qudt/> .

<http://qudt.org/vocab/unit/M> a qudt:Unit ;
    rdfs:label "Metre"@en ;
    qudt:symbol "m" .

<http://qudt.org/vocab/quantitykind/LuminousIntensity> a qudt:QuantityKind ;
    rdfs:label "luminous intensity"@en .

<http://qudt.org/vocab/dimensionvector/L> a qudt:DimensionVector ;
    rdfs:label "Length"@en .
`)

	unitTerms := parseTermsFromFile(units)
	if len(unitTerms) != 1 || unitTerms[0].TermID != "quantity:unit_M" || unitTerms[0].Kind != "unit" ||
		unitTerms[0].PrefLabel != "Metre" || unitTerms[0].Symbol != "m" {
		t.Fatalf("unitTerms=%+v", unitTerms)
	}
	qkTerms := parseTermsFromFile(quantityKinds)
	if len(qkTerms) != 1 || qkTerms[0].TermID != "quantity:qk_LuminousIntensity" || qkTerms[0].Kind != "quantity_kind" {
		t.Fatalf("qkTerms=%+v", qkTerms)
	}
	dimTerms := parseTermsFromFile(dimensions)
	if len(dimTerms) != 1 || dimTerms[0].TermID != "quantity:dim_L" || dimTerms[0].Kind != "dimension" {
		t.Fatalf("dimTerms=%+v", dimTerms)
	}

	// A single combined artifact (e.g. the real qudt-all.ttl) mixing all three
	// classes must classify each resource correctly in one pass -- this is
	// exactly the shape the Approve handler feeds it (design.md Decision 2).
	combinedTerms := parseTermsFromFile(combined)
	if len(combinedTerms) != 3 {
		t.Fatalf("combinedTerms=%+v, want 3 (one per class)", combinedTerms)
	}
	byID := map[string]terminology.QUDTImportedTerm{}
	for _, term := range combinedTerms {
		byID[term.TermID] = term
	}
	if byID["quantity:unit_M"].Kind != "unit" {
		t.Fatalf("combined unit misclassified: %+v", byID["quantity:unit_M"])
	}
	if byID["quantity:qk_LuminousIntensity"].Kind != "quantity_kind" {
		t.Fatalf("combined quantity kind misclassified: %+v", byID["quantity:qk_LuminousIntensity"])
	}
	if byID["quantity:dim_L"].Kind != "dimension" {
		t.Fatalf("combined dimension misclassified: %+v", byID["quantity:dim_L"])
	}
}
