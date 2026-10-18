package metricvalue

import "testing"

func TestNormalizeStructure(t *testing.T) {
	for _, tc := range []struct {
		raw   any
		valid bool
	}{
		{nil, true}, {`null`, true},
		{map[string]any{"kind": "alternatives", "values": []any{"10", "20", "30"}}, true},
		{`{"kind":"required_points","values":["10","30"]}`, true},
		{map[string]any{"kind": "composite", "values": []string{"250", "200"}}, true},
		{map[string]any{"kind": "expression"}, true},
		{map[string]any{"kind": "unresolved"}, true},
		{`{"kind":"alternatives","values":["10"]}`, false},
		{`{"kind":"alternatives","values":["10","10"]}`, false},
		{`{"kind":"alternatives","values":[10,20]}`, false},
		{`{"kind":"required_points","values":["10",""]}`, false},
		{`{"kind":"unknown"}`, false}, {`{bad`, false}, {`{}`, false},
	} {
		_, err := NormalizeStructure(tc.raw)
		if (err == nil) != tc.valid {
			t.Errorf("NormalizeStructure(%v): %v", tc.raw, err)
		}
	}
}

func TestStructureEqualPreservesRelationship(t *testing.T) {
	a := `{"kind":"alternatives","values":["10","20","30"]}`
	if !StructureEqual(a, `{"kind":"alternatives","values":["30","10","20"]}`) {
		t.Fatal("set order should not change meaning")
	}
	for _, b := range []any{nil, `{"kind":"required_points","values":["10","20","30"]}`, `{"kind":"alternatives","values":["10","20"]}`} {
		if StructureEqual(a, b) {
			t.Fatalf("different meaning considered equal: %v", b)
		}
	}
	if StructureEqual(`{"kind":"composite","values":["250","200"]}`, `{"kind":"composite","values":["200","250"]}`) {
		t.Fatal("composite order lost")
	}
}

func TestStructureMembersMustMatchDisplay(t *testing.T) {
	s := `{"kind":"alternatives","values":["10","20","30"]}`
	for _, text := range []string{"10", "10、20", "10、20、30、40"} {
		if err := ValidateForValue(s, text); err == nil {
			t.Errorf("structure hid inconsistent display %q", text)
		}
	}
	if err := ValidateForValue(s, "10.0、20、30"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateForValue(s, "１０、２０、３０"); err != nil {
		t.Fatal(err)
	}
}
