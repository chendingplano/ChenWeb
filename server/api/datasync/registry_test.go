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

// Every compiled item must pass the same shape validation a UI-created item
// does, use a unique id, and reference only parents registered before it: the
// Sync Data page lists items in Registry order and a child synced before its
// parent cannot resolve the parent's natural key.
func TestRegistryItemsAreWellFormedAndOrdered(t *testing.T) {
	position := map[string]int{}
	for i, item := range Registry {
		if _, dup := position[item.ID]; dup {
			t.Errorf("duplicate sync item id %q", item.ID)
		}
		position[item.ID] = i
		if err := validateItem(item); err != nil {
			t.Errorf("item %q is not well formed: %v", item.ID, err)
		}
	}
	for i, item := range Registry {
		for _, ref := range item.References {
			parent, ok := position[ref.ItemID]
			if !ok {
				t.Errorf("item %q references unknown item %q", item.ID, ref.ItemID)
				continue
			}
			if parent >= i {
				t.Errorf("item %q references %q, which is registered after it; sync order would fail", item.ID, ref.ItemID)
			}
			if !containsString(item.Columns, ref.Column) {
				t.Errorf("item %q reference column %q is not in Columns", item.ID, ref.Column)
			}
		}
	}
}

// The keyword catalog tables carry every source release except a rejected one,
// and every child table must be scoped by the same filter as its parent so a
// rejected release can never leak through a child.
func TestKeywordCatalogItemsSyncEverythingExceptRejectedReleases(t *testing.T) {
	sources, ok := ItemByID("keyword_sources")
	if !ok {
		t.Fatal("keyword_sources is not registered")
	}
	if sources.Filter != "license_review_status <> 'rejected'" {
		t.Fatalf("keyword_sources Filter = %q, want every release except rejected ones", sources.Filter)
	}
	for _, id := range []string{
		"keyword_source_artifacts", "keyword_catalog_entries", "keyword_catalog_labels",
		"keyword_catalog_relations", "keyword_catalog_negative_decisions", "keyword_ucum_codes",
	} {
		item, ok := ItemByID(id)
		if !ok {
			t.Errorf("%s is not registered", id)
			continue
		}
		if item.Filter != usableKeywordSourceFilter {
			t.Errorf("%s Filter = %q, want the usable-release filter", id, item.Filter)
		}
	}
}

// The dictionary is mirrored in full: concepts and surfaces carry no filter
// (every row, whatever created it, must reach a deployment), and the two
// identity tables that point at kb.keyword_sources carry the usable-release
// filter so no row can reference a source release the target does not hold.
// Items also have to be registered after the tables their foreign keys point
// at, since those foreign keys are plain text ids, not SyncReferences.
func TestKeywordDictionaryItemsMirrorEverythingInDependencyOrder(t *testing.T) {
	position := map[string]int{}
	for i, item := range Registry {
		position[item.ID] = i
	}
	for _, id := range []string{"keyword_concepts", "keyword_surfaces", "keyword_surface_keys", "keyword_rewrite_rules"} {
		item, ok := ItemByID(id)
		if !ok {
			t.Errorf("%s is not registered", id)
			continue
		}
		if item.Filter != "" {
			t.Errorf("%s Filter = %q, want none (mirror every row)", id, item.Filter)
		}
	}
	for _, id := range []string{"keyword_external_ids", "keyword_surface_evidence"} {
		item, ok := ItemByID(id)
		if !ok {
			t.Errorf("%s is not registered", id)
			continue
		}
		if item.Filter != usableKeywordSourceFilter {
			t.Errorf("%s Filter = %q, want the usable-release filter", id, item.Filter)
		}
	}
	for child, parents := range map[string][]string{
		"keyword_surfaces":         {"keyword_concepts"},
		"keyword_surface_keys":     {"keyword_surfaces"},
		"keyword_external_ids":     {"keyword_sources", "keyword_concepts"},
		"keyword_surface_evidence": {"keyword_sources", "keyword_surfaces"},
	} {
		for _, parent := range parents {
			if position[parent] >= position[child] {
				t.Errorf("%s must be registered after %s (foreign key)", child, parent)
			}
		}
	}
}
