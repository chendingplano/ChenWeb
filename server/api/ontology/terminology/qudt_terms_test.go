package terminology

import "testing"

func TestClassifyQUDTTermsCoversAllThreeClasses(t *testing.T) {
	resources := []QUDTQuantityKind{
		{
			IRI: "http://qudt.org/vocab/unit/M", Class: qudtUnitClass, Symbol: "m",
			Labels: []QUDTLabel{{Value: "Metre", Language: "en", Role: "preferred"}},
		},
		{
			IRI: "http://qudt.org/vocab/quantitykind/Luminance", Class: qudtQuantityKindClass,
			Labels: []QUDTLabel{{Value: "Luminance", Language: "en", Role: "preferred"}},
		},
		{
			IRI: "http://qudt.org/vocab/dimensionvector/L", Class: qudtDimensionClass,
			Labels: []QUDTLabel{{Value: "Length", Language: "en", Role: "preferred"}},
		},
		{
			// Deprecated entries must be skipped regardless of class.
			IRI: "http://qudt.org/vocab/quantitykind/Old", Class: qudtQuantityKindClass, Deprecated: true,
			Labels: []QUDTLabel{{Value: "Old", Language: "en", Role: "preferred"}},
		},
		{
			// A class this module doesn't govern must be ignored, not error.
			IRI: "http://qudt.org/vocab/other/Thing", Class: "http://qudt.org/schema/qudt/SomethingElse",
		},
	}

	got := ClassifyQUDTTerms(resources)
	if len(got) != 3 {
		t.Fatalf("got %d terms, want 3: %+v", len(got), got)
	}
	byID := map[string]QUDTImportedTerm{}
	for _, term := range got {
		byID[term.TermID] = term
	}

	unit, ok := byID["quantity:unit_M"]
	if !ok || unit.Kind != "unit" || unit.SourceIRI != "http://qudt.org/vocab/unit/M" ||
		unit.PrefLabel != "Metre" || unit.Symbol != "m" {
		t.Fatalf("unit=%+v ok=%v", unit, ok)
	}
	qk, ok := byID["quantity:qk_Luminance"]
	if !ok || qk.Kind != "quantity_kind" || qk.SourceIRI != "http://qudt.org/vocab/quantitykind/Luminance" ||
		qk.PrefLabel != "Luminance" {
		t.Fatalf("quantity kind=%+v ok=%v", qk, ok)
	}
	dim, ok := byID["quantity:dim_L"]
	if !ok || dim.Kind != "dimension" || dim.SourceIRI != "http://qudt.org/vocab/dimensionvector/L" ||
		dim.PrefLabel != "Length" {
		t.Fatalf("dimension=%+v ok=%v", dim, ok)
	}
	if _, ok := byID["quantity:qk_Old"]; ok {
		t.Fatal("deprecated resource must not be classified")
	}
}

func TestPickQUDTLabelPrefersEnglishPreferred(t *testing.T) {
	cases := []struct {
		name   string
		labels []QUDTLabel
		want   string
	}{
		{"english preferred wins", []QUDTLabel{
			{Value: "Metre", Language: "en", Role: "preferred"},
			{Value: "Mètre", Language: "fr", Role: "preferred"},
		}, "Metre"},
		{"falls back to any preferred", []QUDTLabel{
			{Value: "Mètre", Language: "fr", Role: "preferred"},
		}, "Mètre"},
		{"falls back to first label", []QUDTLabel{
			{Value: "m (symbol)", Language: "und", Role: "alternative"},
		}, "m (symbol)"},
		{"empty when no labels", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickQUDTLabel(tc.labels); got != tc.want {
				t.Fatalf("pickQUDTLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}
