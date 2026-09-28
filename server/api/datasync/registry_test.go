package datasync

import "testing"

func TestProductNamesSyncIncludesMechanicalImports(t *testing.T) {
	item, ok := ItemByID("kb_product_names")
	if !ok {
		t.Fatal("kb_product_names is not registered")
	}
	if item.Filter != "status = 'approved'" {
		t.Fatalf("Filter = %q, want all approved product names regardless of source", item.Filter)
	}
	for _, col := range []string{"code_group", "child_code_group", "industry_code", "cpc", "entry_no", "entry_no_new"} {
		found := false
		for _, registered := range item.Columns {
			if registered == col {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("mechanical import column %q is not synced", col)
		}
	}
}
