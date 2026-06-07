package docprocessing

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestParseMetricCategoriesText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"empty array", "[]", nil},
		{"json array", `["energy","safety"]`, []string{"energy", "safety"}},
		{"json array dedup+trim", `[" energy ","energy","safety"]`, []string{"energy", "safety"}},
		{"comma fallback", "energy, safety ,energy", []string{"energy", "safety"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseMetricCategoriesText(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("parseMetricCategoriesText(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestLineSetFromSpansAndOverlap(t *testing.T) {
	set := lineSetFromSpans([]string{"3", "5:7", "0", "-2"})
	for _, n := range []int{3, 5, 6, 7} {
		if _, ok := set[n]; !ok {
			t.Errorf("expected line %d in set", n)
		}
	}
	if _, ok := set[4]; ok {
		t.Error("line 4 should not be in set")
	}
	if _, ok := set[0]; ok {
		t.Error("zero line must be discarded")
	}

	if !spansOverlapLineSet([]string{"6:9"}, set) {
		t.Error("expected overlap on 6:9")
	}
	if spansOverlapLineSet([]string{"10:12"}, set) {
		t.Error("did not expect overlap on 10:12")
	}
	if spansOverlapLineSet([]string{"4"}, map[int]struct{}{}) {
		t.Error("empty set must never overlap")
	}
}

func TestLineSetFromSpansAndOverlapAcceptsHyphenRanges(t *testing.T) {
	set := lineSetFromSpans([]string{"34"})
	if !spansOverlapLineSet([]string{"22-45"}, set) {
		t.Error("expected semantic projection range 22-45 to overlap metric line 34")
	}

	got := normalizeSourceLineSpans([]any{"22-45"})
	want := []string{"22:45"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeSourceLineSpans hyphen range = %v, want %v", got, want)
	}
}

func TestChunkOverlapsLineSet(t *testing.T) {
	ch := Block{Index: 2, Lines: []BlockLine{{LineNumber: 5}, {LineNumber: 6}}}
	if !chunkOverlapsLineSet(ch, lineSetFromSpans([]string{"6"})) {
		t.Error("expected chunk to overlap line 6")
	}
	if chunkOverlapsLineSet(ch, lineSetFromSpans([]string{"9"})) {
		t.Error("did not expect chunk to overlap line 9")
	}
}

func TestOverlappingRefIDs(t *testing.T) {
	refs := []ArtifactRef{
		{Type: "topic", ID: "10_tpc_1", Spans: []string{"1:3"}},
		{Type: "topic", ID: "10_tpc_2", Spans: []string{"50:60"}},
		{Type: "topic", ID: "10_tpc_3", Spans: []string{"5"}},
	}
	got := overlappingRefIDs(refs, lineSetFromSpans([]string{"2", "5"}))
	sort.Strings(got)
	want := []string{"10_tpc_1", "10_tpc_3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("overlappingRefIDs = %v, want %v", got, want)
	}
	// Always non-nil (marshals to [] not null).
	if overlappingRefIDs(nil, lineSetFromSpans([]string{"1"})) == nil {
		t.Error("overlappingRefIDs must return non-nil empty slice")
	}
}

func TestConnectedArtifactsMarshalsEmptyArrays(t *testing.T) {
	ca := connectedArtifacts{
		Chunks:           []string{},
		SemanticProjects: []string{},
		Summaries:        []string{},
		Topics:           []string{},
		Scenes:           []string{},
		Provisions:       []string{},
		Entities:         []string{},
		Relations:        []string{},
		InvItems:         []string{},
	}
	bs, err := json.Marshal(ca)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"chunks":[],"semantic_projects":[],"summaries":[],"topics":[],"scenes":[],"provisions":[],"entities":[],"relations":[],"inv_items":[]}`
	if string(bs) != want {
		t.Errorf("connected_artifacts JSON = %s, want %s", bs, want)
	}
}

type jsonSubstringArg struct {
	want []string
}

func (m jsonSubstringArg) Match(v driver.Value) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	for _, want := range m.want {
		if !strings.Contains(s, want) {
			return false
		}
	}
	return true
}

func TestIndexSemanticProjectionsForRecordBuildsConnectedArtifacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("SELECT semantic_proj_id,").
		WithArgs(int64(100)).
		WillReturnRows(sqlmock.NewRows([]string{"semantic_proj_id", "line_spans"}).
			AddRow("100_0_1", []byte(`["5:6"]`)))

	for _, tc := range []struct {
		artifactType string
		artifactID   string
	}{
		{searchArtifactTopic, "100_tpc_1"},
		{searchArtifactSceneBlock, "100_scn_1"},
		{searchArtifactProvision, "100_prv_1"},
		{searchArtifactEntity, "100_ent_1"},
		{searchArtifactMetric, "100_met_1"},
		{searchArtifactInventoryItem, "100_inv_1"},
	} {
		mock.ExpectQuery("SELECT artifact_id, source_line_spans\\s+FROM kb.search_artifacts").
			WithArgs(tc.artifactType, int64(100)).
			WillReturnRows(sqlmock.NewRows([]string{"artifact_id", "source_line_spans"}).
				AddRow(tc.artifactID, []byte(`["5:6"]`)))
	}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.semantic_projections SET connected_artifacts = $1::jsonb WHERE input_record_id = $2 AND semantic_proj_id = $3")).
		WithArgs(sqlmock.AnyArg(), int64(100), "100_0_1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	IndexSemanticProjectionsForRecord(context.Background(), 100, []Block{{Index: 1, Lines: []BlockLine{{LineNumber: 5}}}}, nil)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestBuildArtifactConnectedArtifactsDoesNotRequireSemanticProjectsForSemanticProjectionSource(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	for _, tc := range []struct {
		artifactType string
		artifactID   string
	}{
		{searchArtifactTopic, "100_tpc_1"},
		{searchArtifactSceneBlock, "100_scn_1"},
		{searchArtifactProvision, "100_prv_1"},
		{searchArtifactEntity, "100_ent_1"},
		{searchArtifactMetric, "100_met_1"},
		{searchArtifactInventoryItem, "100_inv_1"},
	} {
		mock.ExpectQuery("SELECT artifact_id, source_line_spans\\s+FROM kb.search_artifacts").
			WithArgs(tc.artifactType, int64(100)).
			WillReturnRows(sqlmock.NewRows([]string{"artifact_id", "source_line_spans"}).
				AddRow(tc.artifactID, []byte(`["5:6"]`)))
	}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.semantic_projections SET connected_artifacts = $1::jsonb WHERE input_record_id = $2 AND semantic_proj_id = $3")).
		WithArgs(sqlmock.AnyArg(), int64(100), "100_0_1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	got := buildArtifactConnectedArtifacts(context.Background(), db, 100, []Block{{Index: 1, Lines: []BlockLine{{LineNumber: 5}}}}, []indexedArtifact{{
		ID:          "100_0_1",
		SourceSpans: []string{"5:6"},
	}}, semanticProjectionIndexConfig, nil)
	if got != 1 {
		t.Fatalf("buildArtifactConnectedArtifacts updated=%d, want 1", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestBuildArtifactConnectedArtifactsUsesUpdateKeyAndIncludesSummaryRelationRefs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	for _, tc := range []struct {
		artifactType string
		artifactID   string
	}{
		{searchArtifactSemanticProjection, "100_0_1"},
		{searchArtifactSummary, "100_sum_1"},
		{searchArtifactSceneBlock, "100_scn_1"},
		{searchArtifactProvision, "100_prv_1"},
		{searchArtifactEntity, "100_ent_1"},
		{searchArtifactRelation, "100_rel_1"},
		{searchArtifactMetric, "100_met_1"},
		{searchArtifactInventoryItem, "100_inv_1"},
	} {
		if tc.artifactType == searchArtifactSemanticProjection {
			mock.ExpectQuery("SELECT semantic_proj_id,").
				WithArgs(int64(100)).
				WillReturnRows(sqlmock.NewRows([]string{"semantic_proj_id", "line_spans"}).
					AddRow(tc.artifactID, []byte(`["5:6"]`)))
			continue
		}
		mock.ExpectQuery("SELECT artifact_id, source_line_spans\\s+FROM kb.search_artifacts").
			WithArgs(tc.artifactType, int64(100)).
			WillReturnRows(sqlmock.NewRows([]string{"artifact_id", "source_line_spans"}).
				AddRow(tc.artifactID, []byte(`["5:6"]`)))
	}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.topics SET connected_artifacts = $1::jsonb WHERE input_record_id = $2 AND topic_id = $3")).
		WithArgs(jsonSubstringArg{want: []string{`"summaries":["100_sum_1"]`, `"relations":["100_rel_1"]`}}, int64(100), "1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	got := buildArtifactConnectedArtifacts(context.Background(), db, 100, []Block{{Index: 1, Lines: []BlockLine{{LineNumber: 5}}}}, []indexedArtifact{{
		ID:          "100_tpc_1",
		UpdateKey:   "1",
		SourceSpans: []string{"5:6"},
	}}, topicIndexConfig, nil)
	if got != 1 {
		t.Fatalf("buildArtifactConnectedArtifacts updated=%d, want 1", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestBuildArtifactConnectedArtifactsUsesIntegerUpdateKeyForProvisions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	for _, tc := range []struct {
		artifactType string
		artifactID   string
	}{
		{searchArtifactSemanticProjection, "100_0_1"},
		{searchArtifactSummary, "100_sum_1"},
		{searchArtifactTopic, "100_tpc_1"},
		{searchArtifactSceneBlock, "100_scn_1"},
		{searchArtifactEntity, "100_ent_1"},
		{searchArtifactRelation, "100_rel_1"},
		{searchArtifactMetric, "100_met_1"},
		{searchArtifactInventoryItem, "100_inv_1"},
	} {
		if tc.artifactType == searchArtifactSemanticProjection {
			mock.ExpectQuery("SELECT semantic_proj_id,").
				WithArgs(int64(100)).
				WillReturnRows(sqlmock.NewRows([]string{"semantic_proj_id", "line_spans"}).
					AddRow(tc.artifactID, []byte(`["5:6"]`)))
			continue
		}
		mock.ExpectQuery("SELECT artifact_id, source_line_spans\\s+FROM kb.search_artifacts").
			WithArgs(tc.artifactType, int64(100)).
			WillReturnRows(sqlmock.NewRows([]string{"artifact_id", "source_line_spans"}).
				AddRow(tc.artifactID, []byte(`["5:6"]`)))
	}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.provisions SET connected_artifacts = $1::jsonb WHERE input_record_id = $2 AND prov_id = $3")).
		WithArgs(sqlmock.AnyArg(), int64(100), 7).
		WillReturnResult(sqlmock.NewResult(0, 1))

	got := buildArtifactConnectedArtifacts(context.Background(), db, 100, []Block{{Index: 1, Lines: []BlockLine{{LineNumber: 5}}}}, []indexedArtifact{{
		ID:          "100_prv_7",
		UpdateKey:   7,
		SourceSpans: []string{"5:6"},
	}}, provisionIndexConfig, nil)
	if got != 1 {
		t.Fatalf("buildArtifactConnectedArtifacts updated=%d, want 1", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

type fakeMetricCategoryResolver struct {
	id    int64
	err   error
	calls []string
}

func (f *fakeMetricCategoryResolver) ResolveBatch(_ context.Context, _ string, reqs []categoryRequest, _ int) (map[string]int64, map[string]error) {
	ids := make(map[string]int64)
	errs := make(map[string]error)
	for _, r := range reqs {
		f.calls = append(f.calls, r.RawKey)
		nk := normalizeCategoryKey(r.RawKey)
		if f.err != nil {
			errs[nk] = f.err
			continue
		}
		ids[nk] = f.id
	}
	return ids, errs
}

func TestUpsertMetricCategoryInstancesResolvesAndUpserts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	metrics := []indexedMetric{{
		MetricID:       "100_1",
		Categories:     []string{"energy_efficiency"},
		SearchDocument: "energy efficiency rating 5 stars",
	}}

	// Category resolution is delegated to the resolver (faked here); this function
	// only upserts the category_instance row with the returned category_id.
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.category_instance (category_id, artifact_id, input_record_id, extra_info)`)).
		WithArgs(int64(42), "100_1", int64(100), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	resolver := &fakeMetricCategoryResolver{id: 42}
	if got := upsertMetricCategoryInstances(context.Background(), db, 100, metrics, resolver, nil); got != 1 {
		t.Fatalf("upsertMetricCategoryInstances = %d, want 1", got)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != "energy_efficiency" {
		t.Fatalf("resolver calls = %#v, want [energy_efficiency]", resolver.calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSanitizeTSDictionary(t *testing.T) {
	cases := map[string]string{
		"":            "simple",
		"  ":          "simple",
		"english":     "english",
		"simple":      "simple",
		"en'; DROP--": "simple",
		"weird space": "simple",
	}
	for in, want := range cases {
		if got := sanitizeTSDictionary(in); got != want {
			t.Errorf("sanitizeTSDictionary(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMetricConnectEnvDefaults(t *testing.T) {
	t.Setenv("ARTIFACT_CONNECT_MIN_COSINE", "")
	t.Setenv("ARTIFACT_CONNECT_MAX_LINKS", "")
	if got := metricConnectMinCosine(); got != defaultMetricConnectMinCosine {
		t.Errorf("default min cosine = %v, want %v", got, defaultMetricConnectMinCosine)
	}
	if got := metricConnectMaxLinks(); got != defaultMetricConnectMaxLinks {
		t.Errorf("default max links = %v, want %v", got, defaultMetricConnectMaxLinks)
	}

	t.Setenv("ARTIFACT_CONNECT_MIN_COSINE", "0.9")
	t.Setenv("ARTIFACT_CONNECT_MAX_LINKS", "3")
	if got := metricConnectMinCosine(); got != 0.9 {
		t.Errorf("min cosine = %v, want 0.9", got)
	}
	if got := metricConnectMaxLinks(); got != 3 {
		t.Errorf("max links = %v, want 3", got)
	}

	// Invalid values fall back to defaults.
	t.Setenv("ARTIFACT_CONNECT_MAX_LINKS", "0")
	if got := metricConnectMaxLinks(); got != defaultMetricConnectMaxLinks {
		t.Errorf("invalid max links = %v, want default %v", got, defaultMetricConnectMaxLinks)
	}
}

func TestParseVectorLiteral(t *testing.T) {
	got, err := parseVectorLiteral("[0.25, -0.5, 1]")
	if err != nil {
		t.Fatalf("parseVectorLiteral returned error: %v", err)
	}
	want := []float64{0.25, -0.5, 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseVectorLiteral = %v, want %v", got, want)
	}
}

func TestHydrateArtifactEmbeddingsLoadsStoredVectors(t *testing.T) {
	t.Setenv("SEARCH_SEMANTIC_ENABLED", "true")

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	vecText := "[" + strings.TrimSpace(strings.Repeat("0.1,", 1535)) + "0.1]"
	mock.ExpectQuery("SELECT artifact_id, embedding::text\\s+FROM kb.search_artifacts").
		WithArgs(searchArtifactEntity, int64(177)).
		WillReturnRows(sqlmock.NewRows([]string{"artifact_id", "embedding"}).
			AddRow("177_art_1", vecText))

	artifacts := []indexedArtifact{
		{ID: "177_art_1"},
		{ID: "177_art_2"},
	}
	hydrateArtifactEmbeddings(context.Background(), db, 177, searchArtifactEntity, artifacts, nil, "entity indexing")

	if got := len(artifacts[0].Embedding); got != 1536 {
		t.Fatalf("hydrated embedding len = %d, want 1536", got)
	}
	if len(artifacts[1].Embedding) != 0 {
		t.Fatalf("unexpected embedding for second artifact: len=%d", len(artifacts[1].Embedding))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMetricsTxtLeafDirUpsertAndRemove(t *testing.T) {
	dir := t.TempDir()
	leaf := filepath.Join(dir, "energy", "efficiency")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}

	// Upsert is idempotent and sorted.
	for _, id := range []string{"100_2", "100_1", "100_1"} {
		if err := upsertMetricToLeafDir(leaf, id); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(leaf, "metrics.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "100_1\n100_2" {
		t.Errorf("metrics.txt = %q, want sorted unique ids", body)
	}

	// A different record's id co-exists.
	if err := upsertMetricToLeafDir(leaf, "200_1"); err != nil {
		t.Fatal(err)
	}
	// removeMetricTreeRecord drops only record 100's entries.
	if err := removeMetricTreeRecord(dir, 100); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(filepath.Join(leaf, "metrics.txt"))
	if string(body) != "200_1" {
		t.Errorf("after remove(100), metrics.txt = %q, want %q", body, "200_1")
	}
}

func TestArtifactConnectionsHybridSearchPartitionMigrationExists(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../.."))
	migrationDir := filepath.Join(repoRoot, "project_migrations")

	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", migrationDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(migrationDir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", entry.Name(), err)
		}
		text := string(body)
		if strings.Contains(text, "artifact_connections_hybrid_search") &&
			strings.Contains(text, "FOR VALUES IN ('hybrid_search')") {
			return
		}
	}

	t.Fatal("expected a kb.artifact_connections hybrid_search partition migration")
}
