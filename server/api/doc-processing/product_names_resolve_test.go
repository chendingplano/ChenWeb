package docprocessing

import (
	"context"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type fakeProductNameStore struct {
	lookupResult *productNameMatch
	lookupErr    error
	fuzzyResult  []ProductNameCandidate
	fuzzyErr     error
	createErr    error
	createdID    int64
	aliasCalls   []struct {
		id    int64
		alias string
	}
	createCalls []struct {
		name, nameEN string
		candidates   []ProductNameCandidate
	}
}

func (f *fakeProductNameStore) LookupTiers(_ context.Context, _ string, _ semid.KeySet) (*productNameMatch, error) {
	return f.lookupResult, f.lookupErr
}

func (f *fakeProductNameStore) FuzzyCandidates(_ context.Context, _ string, _ int, _ float64) ([]ProductNameCandidate, error) {
	return f.fuzzyResult, f.fuzzyErr
}

func (f *fakeProductNameStore) AppendAlias(_ context.Context, id int64, alias string) error {
	f.aliasCalls = append(f.aliasCalls, struct {
		id    int64
		alias string
	}{id, alias})
	return nil
}

func (f *fakeProductNameStore) CreateProposed(_ context.Context, name, nameEN string, _ []byte, candidates []ProductNameCandidate) (int64, error) {
	f.createCalls = append(f.createCalls, struct {
		name, nameEN string
		candidates   []ProductNameCandidate
	}{name, nameEN, candidates})
	if f.createErr != nil {
		return 0, f.createErr
	}
	return f.createdID, nil
}

func testProductsProcessor(store ProductNameStore) *ProductsProcessor {
	return &ProductsProcessor{Logger: loggerutil.CreateDefaultLogger("MID_TEST_PNI"), ProductNames: store}
}

func TestResolveProductName_EmptyNameSkipsLookup(t *testing.T) {
	store := &fakeProductNameStore{}
	p := testProductsProcessor(store)
	if res := p.resolveProductName(context.Background(), "  ", ""); res.ID != 0 {
		t.Fatalf("id=%d, want 0", res.ID)
	}
	if len(store.createCalls) != 0 {
		t.Fatalf("expected no CreateProposed call for empty name")
	}
}

func TestResolveProductName_NilStoreSkips(t *testing.T) {
	p := testProductsProcessor(nil)
	if res := p.resolveProductName(context.Background(), "infusion pump", ""); res.ID != 0 {
		t.Fatalf("id=%d, want 0", res.ID)
	}
}

func TestResolveProductName_ExactHitDoesNotAppendAlias(t *testing.T) {
	store := &fakeProductNameStore{lookupResult: &productNameMatch{ID: 42, ProductName: "infusion pump", Exact: true}}
	p := testProductsProcessor(store)
	res := p.resolveProductName(context.Background(), "infusion pump", "")
	if res.ID != 42 {
		t.Fatalf("id=%d, want 42", res.ID)
	}
	if len(store.aliasCalls) != 0 {
		t.Fatalf("expected no alias append on an exact hit, got %v", store.aliasCalls)
	}
}

func TestResolveProductName_NonExactHitAppendsAliasAndCarriesCategory(t *testing.T) {
	store := &fakeProductNameStore{lookupResult: &productNameMatch{
		ID: 42, ProductName: "输液泵", Exact: false,
		SubCatalog: "01 有源手术器械", CategoryL1: "01 超声手术设备及附件", CategoryL2: "01.1 超声手术设备",
	}}
	p := testProductsProcessor(store)
	res := p.resolveProductName(context.Background(), "输液  泵", "")
	if res.ID != 42 {
		t.Fatalf("id=%d, want 42", res.ID)
	}
	if res.SubCatalog != "01 有源手术器械" || res.CategoryL1 != "01 超声手术设备及附件" || res.CategoryL2 != "01.1 超声手术设备" {
		t.Fatalf("category fields not carried through: %+v", res)
	}
	if len(store.aliasCalls) != 1 || store.aliasCalls[0].id != 42 || store.aliasCalls[0].alias != "输液  泵" {
		t.Fatalf("unexpected alias calls: %v", store.aliasCalls)
	}
}

func TestResolveProductName_MissWithFuzzyCandidatesProposesWithCandidates(t *testing.T) {
	store := &fakeProductNameStore{
		lookupResult: nil,
		fuzzyResult:  []ProductNameCandidate{{ID: 7, ProductName: "输液泵系统", Similarity: 0.62}},
		createdID:    99,
	}
	p := testProductsProcessor(store)
	res := p.resolveProductName(context.Background(), "新型输液泵", "novel infusion pump")
	if res.ID != 99 {
		t.Fatalf("id=%d, want 99", res.ID)
	}
	if res.SubCatalog != "" || res.CategoryL1 != "" || res.CategoryL2 != "" {
		t.Fatalf("a newly proposed row must carry no category, got %+v", res)
	}
	if len(store.createCalls) != 1 {
		t.Fatalf("expected exactly one CreateProposed call, got %d", len(store.createCalls))
	}
	call := store.createCalls[0]
	if call.name != "新型输液泵" || call.nameEN != "novel infusion pump" {
		t.Fatalf("unexpected create call: %+v", call)
	}
	if len(call.candidates) != 1 || call.candidates[0].ID != 7 {
		t.Fatalf("expected the fuzzy candidate to be passed through, got %+v", call.candidates)
	}
}

func TestResolveProductName_TrueMissProposesWithoutCandidates(t *testing.T) {
	store := &fakeProductNameStore{lookupResult: nil, fuzzyResult: nil, createdID: 100}
	p := testProductsProcessor(store)
	res := p.resolveProductName(context.Background(), "全新设备名称", "")
	if res.ID != 100 {
		t.Fatalf("id=%d, want 100", res.ID)
	}
	if len(store.createCalls) != 1 || len(store.createCalls[0].candidates) != 0 {
		t.Fatalf("expected a create call with no candidates, got %+v", store.createCalls)
	}
}

func TestResolveProductName_LookupErrorSkipsGracefully(t *testing.T) {
	store := &fakeProductNameStore{lookupErr: context.DeadlineExceeded}
	p := testProductsProcessor(store)
	if res := p.resolveProductName(context.Background(), "infusion pump", ""); res.ID != 0 {
		t.Fatalf("id=%d, want 0 on lookup error", res.ID)
	}
	if len(store.createCalls) != 0 {
		t.Fatalf("expected no create attempt after a lookup error")
	}
}

func TestNullableProductNameID(t *testing.T) {
	if v := nullableProductNameID(int64(0)); v != nil {
		t.Fatalf("nullableProductNameID(0) = %v, want nil", v)
	}
	if v := nullableProductNameID(nil); v != nil {
		t.Fatalf("nullableProductNameID(nil) = %v, want nil", v)
	}
	if v := nullableProductNameID(int64(5)); v != int64(5) {
		t.Fatalf("nullableProductNameID(5) = %v, want 5", v)
	}
}

// byNameProductNameStore returns a distinct, deterministic match per query
// name, for exercising resolveProductNamesAndCategories' concurrent fan-out
// across multiple distinct names (unlike fakeProductNameStore's single fixed
// response).
type byNameProductNameStore struct {
	byName map[string]productNameMatch
}

func (f *byNameProductNameStore) LookupTiers(_ context.Context, name string, _ semid.KeySet) (*productNameMatch, error) {
	if m, ok := f.byName[name]; ok {
		return &m, nil
	}
	return nil, nil
}
func (f *byNameProductNameStore) FuzzyCandidates(context.Context, string, int, float64) ([]ProductNameCandidate, error) {
	return nil, nil
}
func (f *byNameProductNameStore) AppendAlias(context.Context, int64, string) error { return nil }
func (f *byNameProductNameStore) CreateProposed(context.Context, string, string, []byte, []ProductNameCandidate) (int64, error) {
	return 0, nil
}

func TestResolveProductNamesAndCategories_ConcurrentDistinctNamesDontCrossContaminate(t *testing.T) {
	store := &byNameProductNameStore{byName: map[string]productNameMatch{
		"甲设备": {ID: 1, ProductName: "甲设备", Exact: true, SubCatalog: "S1", CategoryL1: "L1a", CategoryL2: "L2a"},
		"乙设备": {ID: 2, ProductName: "乙设备", Exact: true, SubCatalog: "S2", CategoryL1: "L1b", CategoryL2: "L2b"},
		"丙设备": {ID: 3, ProductName: "丙设备", Exact: true, SubCatalog: "S3", CategoryL1: "L1c", CategoryL2: "L2c"},
	}}
	p := testProductsProcessor(store)
	p.MaxTasks = 3
	products := []map[string]any{
		{"product_name": "甲设备"},
		{"product_name": "乙设备"},
		{"product_name": "丙设备"},
		{"product_name": "甲设备"}, // repeat: must resolve to the same id/category as row 0
	}
	out, err := p.resolveProductNamesAndCategories(context.Background(), products)
	if err != nil {
		t.Fatalf("resolveProductNamesAndCategories: %v", err)
	}
	want := map[string]int64{"甲设备": 1, "乙设备": 2, "丙设备": 3}
	for i, row := range out {
		name := row["product_name"].(string)
		if got := row["product_name_id"]; got != want[name] {
			t.Fatalf("row %d (%s): product_name_id=%v, want %d", i, name, got, want[name])
		}
		paths, ok := row["category_paths"].([]CategoryPathEntry)
		if !ok || len(paths) != 1 || len(paths[0].Nodes) != 3 || paths[0].Nodes[0].Name != store.byName[name].SubCatalog {
			t.Fatalf("row %d (%s): unexpected category_paths %#v", i, name, row["category_paths"])
		}
	}
	if out[0]["product_name_id"] != out[3]["product_name_id"] {
		t.Fatalf("repeated name resolved to different ids: %v vs %v", out[0]["product_name_id"], out[3]["product_name_id"])
	}
}

// TestResolveProductNamesAndCategories_ClearsStrayLLMCategoryPaths is a
// regression test: Pass 2's own prompt (prompt-enrich-product-mention-v2.md,
// before its "Extract Category Paths" section was removed) independently
// asked the LLM for free-form category_paths/category_paths_en, which
// normalizeProductList passed straight onto the row before this pass ever
// ran. The bug: this pass only overwrote category_paths on a catalog hit and
// never touched category_paths_en at all, so a miss (or any hit) silently
// left the LLM's own guess in kb.products and, from there, in the category
// tree filesystem index. This pass must be authoritative: clear on a miss,
// always clear category_paths_en (kb.product_names has no English catalog
// translation to put there).
func TestResolveProductNamesAndCategories_ClearsStrayLLMCategoryPaths(t *testing.T) {
	store := &byNameProductNameStore{byName: map[string]productNameMatch{
		"血压计": {ID: 1, ProductName: "血压计", Exact: true, SubCatalog: "S1", CategoryL1: "L1", CategoryL2: "L2"},
	}}
	p := testProductsProcessor(store)
	stray := []CategoryPathEntry{{Nodes: []CategoryPathNode{{Name: "medical/sphygmomanometer"}}}}
	products := []map[string]any{
		{"product_name": "血压计", "category_paths": stray, "category_paths_en": stray}, // catalog hit: must be replaced
		{"product_name": "全新血压计具", "category_paths": stray, "category_paths_en": stray}, // catalog miss: must be cleared, not left as the LLM's guess
	}
	out, err := p.resolveProductNamesAndCategories(context.Background(), products)
	if err != nil {
		t.Fatalf("resolveProductNamesAndCategories: %v", err)
	}
	if paths, ok := out[0]["category_paths"].([]CategoryPathEntry); !ok || paths[0].Nodes[0].Name != "S1" {
		t.Fatalf("catalog hit: category_paths not replaced with the catalog match, got %#v", out[0]["category_paths"])
	}
	if _, present := out[0]["category_paths_en"]; present {
		t.Fatalf("category_paths_en must be cleared even on a hit (no English catalog translation exists), got %#v", out[0]["category_paths_en"])
	}
	if _, present := out[1]["category_paths"]; present {
		t.Fatalf("catalog miss: the LLM's stray category_paths must be cleared, not left in place, got %#v", out[1]["category_paths"])
	}
	if _, present := out[1]["category_paths_en"]; present {
		t.Fatalf("catalog miss: category_paths_en must be cleared, got %#v", out[1]["category_paths_en"])
	}
}

func TestCategoryPathsFromCatalog(t *testing.T) {
	t.Run("full category builds one path", func(t *testing.T) {
		paths := categoryPathsFromCatalog(productNameResolution{SubCatalog: "A", CategoryL1: "B", CategoryL2: "C"})
		if len(paths) != 1 || len(paths[0].Nodes) != 3 {
			t.Fatalf("unexpected paths: %+v", paths)
		}
		if paths[0].Nodes[0].Name != "A" || paths[0].Nodes[1].Name != "B" || paths[0].Nodes[2].Name != "C" {
			t.Fatalf("unexpected node names: %+v", paths[0].Nodes)
		}
		if paths[0].PathConfidence != 1.0 || paths[0].Nodes[0].Confidence != 1.0 {
			t.Fatalf("expected deterministic confidence 1.0, got %+v", paths[0])
		}
	})
	t.Run("no category returns nil", func(t *testing.T) {
		if paths := categoryPathsFromCatalog(productNameResolution{}); paths != nil {
			t.Fatalf("expected nil paths for an empty category, got %+v", paths)
		}
	})
}
