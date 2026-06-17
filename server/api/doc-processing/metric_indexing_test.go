package docprocessing

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
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

func TestBuildArtifactCategoryConnectionsUsesCategoryNameShape(t *testing.T) {
	metrics := []indexedMetric{{
		MetricID:       "100_1",
		Categories:     []string{"energy_efficiency"},
		SearchDocument: "energy efficiency rating 5 stars",
	}}
	artifacts := metricsToIndexedArtifacts(metrics)
	categories := map[string]resolvedCategory{
		normalizeCategoryKey("energy_efficiency"): {ID: 42, Type: "metric", Key: "energy_efficiency"},
	}

	conns := buildArtifactCategoryConnections(100, artifacts, metricIndexConfig, categories)
	if len(conns) != 1 {
		t.Fatalf("want 1 category connection, got %d: %+v", len(conns), conns)
	}
	got := conns[0]
	if got.SourceRecordID != 100 || got.TargetRecordID != 100 ||
		got.SourceType != searchArtifactMetric || got.SourceID != "100_1" ||
		got.TargetType != "metric" || got.TargetID != "energy_efficiency" ||
		got.RelationName != RelationBelongTo || got.RelationMethod != RelationMethodCategoryName {
		t.Fatalf("category connection shape wrong: %+v", got)
	}
	if got.ExtraInfo["category_id"] != int64(42) {
		t.Fatalf("category connection extra_info category_id = %#v, want 42", got.ExtraInfo["category_id"])
	}
}

func TestConnectArtifactsBySearchWritesHybridConnections(t *testing.T) {
	t.Setenv("SEARCH_SEMANTIC_ENABLED", "false")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	artifacts := []indexedArtifact{{
		ID:             "100_met_1",
		SearchDocument: "energy efficiency rating",
	}}

	mock.ExpectQuery(`(?s)WITH lexical .*FROM lexical`).
		WithArgs("energy efficiency rating", "100_met_1").
		WillReturnRows(sqlmock.NewRows([]string{
			"artifact_type", "artifact_id", "input_record_id", "primary_label", "rrf_score", "lex_score", "cosine_sim",
		}).AddRow(searchArtifactInventoryItem, "200_inv_1", int64(200), "Efficient pump", 0.4, 0.25, nil))
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM kb\.artifact_connections`).
		WithArgs(searchArtifactMetric, int64(100), RelationMethodHybridSearch, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectPrepare(`INSERT INTO kb\.artifact_connections`).
		ExpectExec().
		WithArgs(
			int64(100), int64(200),
			searchArtifactMetric, "100_met_1",
			searchArtifactInventoryItem, "200_inv_1",
			RelationSemanticallyRelated, RelationMethodHybridSearch,
			0.4, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			"metric:100_met_1", "inventory_item:Efficient pump", sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if got := connectArtifactsBySearch(context.Background(), db, 100, artifacts, metricIndexConfig, nil); got != 1 {
		t.Fatalf("connectArtifactsBySearch = %d, want 1", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSearchArtifactIndexersWriteCategoryTreeFiles(t *testing.T) {
	type testCase struct {
		name          string
		cfg           artifactIndexConfig
		loadArtifacts func(context.Context, *sql.DB, int64) ([]indexedArtifact, error)
		run           func(context.Context, int64, []Block, ApiTypes.JimoLogger)
		artifactID    string
		updateKey     any
		sourceSpans   string
		rowQuery      string
		rowValues     []any
		leafFile      string
	}

	testCases := []testCase{
		{
			name:          "scenes",
			cfg:           sceneBlockIndexConfig,
			loadArtifacts: loadIndexedSceneBlocksForRecord,
			run:           IndexSceneBlocksForRecord,
			artifactID:    "100_scn_1",
			sourceSpans:   `["15"]`,
			rowQuery:      "SELECT object_id,\\s+COALESCE\\(line_spans, '\\[\\]'::jsonb\\),\\s+COALESCE\\(search_document, ''\\)",
			rowValues:     []any{"100_scn_1", []byte(`["15"]`), "Scene search document"},
			leafFile:      filepath.Join("public_health", "scenes.txt"),
		},
		{
			name:          "summaries",
			cfg:           summaryIndexConfig,
			loadArtifacts: loadIndexedSummariesForRecord,
			run:           IndexSummariesForRecord,
			artifactID:    "100_sum_1",
			updateKey:     "100_1_0001",
			sourceSpans:   `["15"]`,
			rowQuery:      "SELECT summary_id, summary_seq_no, COALESCE\\(source_line_spans, '\\[\\]'::jsonb\\), COALESCE\\(search_document, ''\\)",
			rowValues:     []any{"100_1_0001", 1, []byte(`["15"]`), "Summary search document"},
			leafFile:      filepath.Join("public_health", "summaries.txt"),
		},
		{
			name:          "topics",
			cfg:           topicIndexConfig,
			loadArtifacts: loadIndexedTopicsForRecord,
			run:           IndexTopicsForRecord,
			artifactID:    "100_tpc_1",
			updateKey:     "100_1",
			sourceSpans:   `["15"]`,
			rowQuery:      "SELECT topic_id, COALESCE\\(source_line_spans, '\\[\\]'::jsonb\\), COALESCE\\(search_document, ''\\)",
			rowValues:     []any{"100_1", []byte(`["15"]`), "Topic search document"},
			leafFile:      filepath.Join("public_health", "topics.txt"),
		},
		{
			name:          "provisions",
			cfg:           provisionIndexConfig,
			loadArtifacts: loadIndexedProvisionsForRecord,
			run:           IndexProvisionsForRecord,
			artifactID:    "100_prv_7",
			updateKey:     7,
			sourceSpans:   `["15"]`,
			rowQuery:      "SELECT prov_id, id, COALESCE\\(source_line_spans, '\\[\\]'::jsonb\\), COALESCE\\(search_document, ''\\)",
			rowValues:     []any{"7", int64(7), []byte(`["15"]`), "Provision search document"},
			leafFile:      filepath.Join("public_health", "provisions.txt"),
		},
		{
			name:          "entities",
			cfg:           entityIndexConfig,
			loadArtifacts: loadIndexedEntitiesForRecord,
			run:           IndexEntitiesForRecord,
			artifactID:    "100_ent_1",
			updateKey:     "100_ent_1",
			sourceSpans:   `["15"]`,
			rowQuery:      "SELECT entity_id, id, COALESCE\\(line_spans, '\\[\\]'::jsonb\\), COALESCE\\(search_document, ''\\)",
			rowValues:     []any{"100_ent_1", int64(1), []byte(`["15"]`), "Entity search document"},
			leafFile:      filepath.Join("public_health", "entities.txt"),
		},
		{
			name:          "relations",
			cfg:           relationIndexConfig,
			loadArtifacts: loadIndexedRelationsForRecord,
			run:           IndexRelationsForRecord,
			artifactID:    "100_rel_1",
			updateKey:     "100_rel_1",
			sourceSpans:   `["15"]`,
			rowQuery:      "SELECT relation_id, id, COALESCE\\(line_spans, '\\[\\]'::jsonb\\), COALESCE\\(search_document, ''\\)",
			rowValues:     []any{"100_rel_1", int64(1), []byte(`["15"]`), "Relation search document"},
			leafFile:      filepath.Join("public_health", "relations.txt"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			defer db.Close()

			oldDB := ApiTypes.ProjectDBHandle
			ApiTypes.ProjectDBHandle = db
			defer func() { ApiTypes.ProjectDBHandle = oldDB }()

			tmp := t.TempDir()
			t.Setenv("ARTIFACT_WEB_DIR", tmp)

			mock.ExpectQuery(tc.rowQuery).
				WithArgs(int64(100)).
				WillReturnRows(newMockRows(tc.rowValues...))

			// connected_artifacts is computed on demand now; indexing only writes the
			// category-path tree files, so the next query is the category-paths lookup.
			mock.ExpectQuery("SELECT COALESCE\\(line_spans, '\\[\\]'::jsonb\\),\\s+COALESCE\\(category_paths, '\\[\\]'::jsonb\\),\\s+COALESCE\\(category_paths_en, '\\[\\]'::jsonb\\)\\s+FROM kb.semantic_projections\\s+WHERE input_record_id = \\$1").
				WithArgs(int64(100)).
				WillReturnRows(sqlmock.NewRows([]string{"line_spans", "category_paths", "category_paths_en"}).
					AddRow(
						[]byte(`["12-18"]`),
						[]byte(`[{"category_path":[{"name":"公共卫生","keywords":["公共卫生"],"confidence":0.9}]}]`),
						[]byte(`[{"category_path":[{"name":"Public Health","keywords":["health"],"confidence":0.95}]}]`),
					))

			tc.run(context.Background(), 100, []Block{{Index: 1, Lines: []BlockLine{{LineNumber: 15}}}}, nil)

			leafPath := filepath.Join(tmp, tc.leafFile)
			body, err := os.ReadFile(leafPath)
			if err != nil {
				t.Fatalf("read category tree leaf %s: %v", leafPath, err)
			}
			if strings.TrimSpace(string(body)) != tc.artifactID {
				t.Fatalf("%s content=%q, want %q", leafPath, string(body), tc.artifactID)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sql expectations: %v", err)
			}
		})
	}
}

func rowColumns(n int) []string {
	switch n {
	case 3:
		return []string{"c1", "c2", "c3"}
	case 4:
		return []string{"c1", "c2", "c3", "c4"}
	default:
		out := make([]string, n)
		for i := range out {
			out[i] = "c" + strconv.Itoa(i+1)
		}
		return out
	}
}

func newMockRows(values ...any) *sqlmock.Rows {
	driverValues := make([]driver.Value, len(values))
	for i, v := range values {
		driverValues[i] = v
	}
	return sqlmock.NewRows(rowColumns(len(values))).AddRow(driverValues...)
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
	assertArtifactConnectionsPartitionMigrationExists(t, "artifact_connections_hybrid_search", "hybrid_search")
}

func TestArtifactConnectionsCategoryNamePartitionMigrationExists(t *testing.T) {
	assertArtifactConnectionsPartitionMigrationExists(t, "artifact_connections_category_name", "category_name")
}

func assertArtifactConnectionsPartitionMigrationExists(t *testing.T, partitionName, relationMethod string) {
	t.Helper()
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
		if strings.Contains(text, partitionName) &&
			strings.Contains(text, "FOR VALUES IN ('"+relationMethod+"')") {
			return
		}
	}

	t.Fatalf("expected a kb.%s partition migration for relation_method=%q", partitionName, relationMethod)
}
