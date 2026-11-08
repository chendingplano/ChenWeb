package docprocessing

import (
	"reflect"
	"testing"
	"time"
)

func settleTestBlocks() []Block {
	return []Block{
		{Index: 1, Lines: []BlockLine{{Flag: "n", LineNumber: 49}, {Flag: "n", LineNumber: 50}}},
		{Index: 2, Lines: []BlockLine{{Flag: "o", LineNumber: 50}, {Flag: "n", LineNumber: 51}, {Flag: "n", LineNumber: 52}}},
	}
}

func settleTestRow(chunk int, overlapOnly bool, name, relation string, lines ...string) map[string]any {
	return map[string]any{
		"product_name": name, "canonical_name": name, "relation_type": relation,
		"evidence_lines": lines, productRowChunkKey: chunk, productRowOverlapOnlyKey: overlapOnly,
	}
}

func TestSettleMergedChunkRows_DropsOverlapOnlyRowWhenOwningChunkHasTheProduct(t *testing.T) {
	rows := []map[string]any{
		// Chunk 2 finished first, as chunks may.
		settleTestRow(2, true, "collection vehicle", "design_requirement", "50"),
		settleTestRow(2, true, "filter unit", "usage_condition", "50"),
		settleTestRow(2, false, "sorting bin", "storage_requirement", "51"),
		settleTestRow(1, false, "collection vehicle", "requirement_target", "50"),
	}
	got := settleMergedChunkRows(rows, settleTestBlocks())
	var names []string
	for _, row := range got {
		names = append(names, asString(row["product_name"])+"/"+asString(row["relation_type"]))
		if _, ok := row[productRowChunkKey]; ok {
			t.Fatalf("bookkeeping key %q leaked into %v", productRowChunkKey, row)
		}
		if _, ok := row[productRowOverlapOnlyKey]; ok {
			t.Fatalf("bookkeeping key %q leaked into %v", productRowOverlapOnlyKey, row)
		}
	}
	// Chunk 1 owns line 50 and extracted the vehicle there, so chunk 2's
	// overlap-only vehicle row goes. Chunk 1 missed the filter unit, so that
	// overlap-only row stays. Rows come out in chunk order.
	want := []string{"collection vehicle/requirement_target", "filter unit/usage_condition", "sorting bin/storage_requirement"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("rows=%v, want %v", names, want)
	}
}

func TestMergeProductRelationRows_FoldsOneProductRelationAcrossTheDocument(t *testing.T) {
	rows := []map[string]any{
		{
			"product_name": "used batteries", "canonical_name": "batteries", "relation_type": "storage_requirement",
			"evidence_lines": []string{"12"}, "evidence_quote": "Used batteries shall be deposited separately.",
			"obligation_level": "recommended", "requirement_text": "Used batteries should be deposited separately",
			"conditions": []string{}, "parameters": []any{}, "related_products": []any{},
			"responsible_actor": "", "confidence": 0.8,
		},
		{
			"product_name": "used batteries", "canonical_name": "batteries", "relation_type": "scope",
			"evidence_lines": []string{"5"}, "obligation_level": "descriptive", "confidence": 0.9,
		},
		{
			"product_name": "used batteries", "canonical_name": "batteries", "relation_type": "storage_requirement",
			"evidence_lines": []string{"40", "12"}, "evidence_quote": "The operator shall collect used batteries weekly.",
			"obligation_level": "mandatory", "requirement_text": "The operator shall collect used batteries weekly",
			"conditions":        []string{"where more than 10 kg are stored"},
			"parameters":        []any{map[string]any{"name": "interval", "value": "1", "unit": "week"}},
			"responsible_actor": "the operator", "confidence": 0.95, "confidence_reason": "explicit",
		},
	}
	got := mergeProductRelationRows(rows)
	if len(got) != 2 {
		t.Fatalf("rows=%d, want 2 (one per relation type)", len(got))
	}
	row := got[0]
	checks := map[string][2]any{
		"evidence_lines":    {toStringSlice(row["evidence_lines"]), []string{"12", "40"}},
		"requirement_text":  {row["requirement_text"], "Used batteries should be deposited separately … The operator shall collect used batteries weekly"},
		"obligation_level":  {row["obligation_level"], "mandatory"},
		"conditions":        {toStringSlice(row["conditions"]), []string{"where more than 10 kg are stored"}},
		"responsible_actor": {row["responsible_actor"], "the operator"},
		"confidence":        {row["confidence"], 0.95},
		"evidence_quote":    {row["evidence_quote"], "Used batteries shall be deposited separately.\nThe operator shall collect used batteries weekly."},
	}
	for field, c := range checks {
		if !reflect.DeepEqual(c[0], c[1]) {
			t.Errorf("%s=%#v, want %#v", field, c[0], c[1])
		}
	}
	if params, _ := normalizeProductItems(row["parameters"]); len(params) != 1 {
		t.Errorf("parameters=%v, want the one parameter of the second row", row["parameters"])
	}
	if asString(got[1]["relation_type"]) != "scope" {
		t.Errorf("second row relation=%v, want scope kept separate", got[1]["relation_type"])
	}
}

func TestBuildProductOutputRows_RecordsThePromptThatWroteTheRows(t *testing.T) {
	p := &ProductsProcessor{PromptRef: "prompt-enrich-product-mention-v4.md", MergedPromptRef: "prompt-extract-products-merged-v3.md"}
	rows := []map[string]any{{"product_name": "pump"}}
	if got := p.buildProductOutputRows(rows, time.Now(), 1, "m")[0]["prompt_name"]; got != "prompt-enrich-product-mention-v4.md" {
		t.Fatalf("two-pass prompt_name=%v, want the Pass 2 prompt", got)
	}
	p.MergedPass = true
	if got := p.buildProductOutputRows(rows, time.Now(), 1, "m")[0]["prompt_name"]; got != "prompt-extract-products-merged-v3.md" {
		t.Fatalf("merged prompt_name=%v, want the merged prompt", got)
	}
}

func TestProductMergedPromptUsesV2_AcceptsV3(t *testing.T) {
	for ref, want := range map[string]bool{
		"prompt-extract-products-merged-v1.md":         false,
		"prompt-extract-products-merged-v2.md":         true,
		"prompts/prompt-extract-products-merged-v3.md": true,
		"prompt-extract-products-merged-v3-custom.md":  false,
	} {
		if got := productMergedPromptUsesV2(ref); got != want {
			t.Errorf("productMergedPromptUsesV2(%q)=%v, want %v", ref, got, want)
		}
	}
}
