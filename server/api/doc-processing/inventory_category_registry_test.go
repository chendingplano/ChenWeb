package docprocessing

import (
	"math"
	"reflect"
	"testing"
)

func TestBuildInventoryValidationFlagsMarksUnknownCategory(t *testing.T) {
	// Known category, clean item: no flags.
	if got := buildInventoryValidationFlags(nil, nil, 0.9, []string{"42"}, true); len(got) != 0 {
		t.Fatalf("known/clean flags=%#v, want none", got)
	}
	// Unknown (out-of-vocabulary) category: must be flagged, even when everything
	// else is clean — this is the drift signal that was previously missing.
	got := buildInventoryValidationFlags(nil, nil, 0.9, []string{"42"}, false)
	if !reflect.DeepEqual(got, []string{"unknown_category"}) {
		t.Fatalf("unknown-category flags=%#v, want [unknown_category]", got)
	}
}

func TestNormalizeInventoryItemRowsFlagsUnknownCategory(t *testing.T) {
	dict := inventoryDictionary{
		Version:    "dict-v1",
		Categories: map[string]inventoryCategorySchema{"bearing": {}},
	}
	rows := normalizeInventoryItemRows([]any{
		map[string]any{
			"item_name":       "理疗仪",
			"item_categories": []any{"medical_device"}, // not in dict
			"lines":           []any{"296"},
			"confidence":      0.95,
		},
	}, 1, dict)
	if len(rows) != 1 {
		t.Fatalf("rows len=%d, want 1", len(rows))
	}
	flags, _ := rows[0]["validation_flags"].([]string)
	if !reflect.DeepEqual(flags, []string{"unknown_category"}) {
		t.Fatalf("validation_flags=%#v, want [unknown_category]", flags)
	}
}

func TestDeriveInventoryCategoryStatus(t *testing.T) {
	statuses := map[string]string{
		"pump":           InventoryCategoryStatusApproved,
		"medical_device": InventoryCategoryStatusPending,
		"bogus":          InventoryCategoryStatusRejected,
	}
	cases := []struct {
		category string
		want     string
	}{
		{"pump", InventoryCategoryStatusApproved},
		{"Pump", InventoryCategoryStatusApproved}, // normalization
		{"medical_device", InventoryCategoryStatusPending},
		{"bogus", InventoryCategoryStatusRejected},
		{"never_seen", InventoryCategoryStatusPending}, // absent → needs review
	}
	for _, tc := range cases {
		if got := deriveInventoryCategoryStatus(statuses, tc.category); got != tc.want {
			t.Fatalf("status(%q)=%q, want %q", tc.category, got, tc.want)
		}
	}
	// Registry unavailable (nil map) → conservatively needs review.
	if got := deriveInventoryCategoryStatus(nil, "pump"); got != InventoryCategoryStatusPending {
		t.Fatalf("nil-statuses=%q, want pending", got)
	}
}

func TestInventoryCategoryCosine(t *testing.T) {
	if got := inventoryCategoryCosine([]float64{1, 0}, []float64{1, 0}); math.Abs(got-1) > 1e-9 {
		t.Fatalf("identical cosine=%v, want 1", got)
	}
	if got := inventoryCategoryCosine([]float64{1, 0}, []float64{0, 1}); math.Abs(got) > 1e-9 {
		t.Fatalf("orthogonal cosine=%v, want 0", got)
	}
	// Mismatched / empty dimensions must not panic and score 0.
	if got := inventoryCategoryCosine([]float64{1, 2, 3}, []float64{1, 2}); got != 0 {
		t.Fatalf("mismatched cosine=%v, want 0", got)
	}
}

func TestCollectAndDedupeInventoryItemCategories(t *testing.T) {
	items := []map[string]any{
		{"item_categories": []any{"medical_device"}},
		{"item_categories": []any{"Medical Device"}}, // same after normalization
		{"item_categories": []any{""}},               // dropped
		{"item_categories": []any{"pump"}},
	}
	got := dedupeNonEmptyNormalized(collectInventoryItemCategories(items))
	if len(got) != 2 {
		t.Fatalf("deduped=%#v, want 2 distinct", got)
	}
}
