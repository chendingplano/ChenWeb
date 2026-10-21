package meshbrowser

import "testing"

func TestValidID(t *testing.T) {
	for _, id := range []string{"D000001", "M0000001", "A01.111", "D02.355.291.933.125"} {
		if !validID(id) {
			t.Errorf("validID(%q) = false, want true", id)
		}
	}
	for _, id := range []string{"", "D000001'; DROP", "a b", "x%", "http://id.nlm.nih.gov/mesh/2025/D000001"} {
		if validID(id) {
			t.Errorf("validID(%q) = true, want false", id)
		}
	}
}

func TestValidKind(t *testing.T) {
	if !validKind("TopicalDescriptor") || !validKind("SCR_Chemical") {
		t.Error("expected class names to be valid")
	}
	if validKind("") || validKind("Topical Descriptor") || validKind("x}>") {
		t.Error("expected unsafe kinds to be rejected")
	}
}

func TestTopCategories(t *testing.T) {
	if len(topCategories) != 16 {
		t.Fatalf("got %d top categories, want 16", len(topCategories))
	}
	for _, id := range []string{"A", "N", "V", "Z"} {
		if _, ok := topCategory(id); !ok {
			t.Errorf("topCategory(%q) missing", id)
		}
	}
	for _, id := range []string{"O", "A01", ""} {
		if _, ok := topCategory(id); ok {
			t.Errorf("topCategory(%q) found, want none", id)
		}
	}
}
