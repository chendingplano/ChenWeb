package docprocessing

import (
	"context"
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
		Topics:           []string{},
		Scenes:           []string{},
		Provisions:       []string{},
		Entities:         []string{},
		InvItems:         []string{},
	}
	bs, err := json.Marshal(ca)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"chunks":[],"semantic_projects":[],"topics":[],"scenes":[],"provisions":[],"entities":[],"inv_items":[]}`
	if string(bs) != want {
		t.Errorf("connected_artifacts JSON = %s, want %s", bs, want)
	}
}

func TestUpsertMetricCategoryInstancesCreatesMissingArtifactCategory(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	metrics := []indexedMetric{{
		MetricID:   "100_1",
		Categories: []string{"energy_efficiency"},
	}}

	mock.ExpectQuery("INSERT INTO kb\\.artifact_categories").
		WithArgs("energy_efficiency").
		WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(42)))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.category_instance (category_id, artifact_id, input_record_id, extra_info)`)).
		WithArgs(int64(42), "100_1", int64(100), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if got := upsertMetricCategoryInstances(context.Background(), db, 100, metrics, nil); got != 1 {
		t.Fatalf("upsertMetricCategoryInstances = %d, want 1", got)
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
	t.Setenv("METRIC_CONNECT_MIN_COSINE", "")
	t.Setenv("METRIC_CONNECT_MAX_LINKS", "")
	if got := metricConnectMinCosine(); got != defaultMetricConnectMinCosine {
		t.Errorf("default min cosine = %v, want %v", got, defaultMetricConnectMinCosine)
	}
	if got := metricConnectMaxLinks(); got != defaultMetricConnectMaxLinks {
		t.Errorf("default max links = %v, want %v", got, defaultMetricConnectMaxLinks)
	}

	t.Setenv("METRIC_CONNECT_MIN_COSINE", "0.9")
	t.Setenv("METRIC_CONNECT_MAX_LINKS", "3")
	if got := metricConnectMinCosine(); got != 0.9 {
		t.Errorf("min cosine = %v, want 0.9", got)
	}
	if got := metricConnectMaxLinks(); got != 3 {
		t.Errorf("max links = %v, want 3", got)
	}

	// Invalid values fall back to defaults.
	t.Setenv("METRIC_CONNECT_MAX_LINKS", "0")
	if got := metricConnectMaxLinks(); got != defaultMetricConnectMaxLinks {
		t.Errorf("invalid max links = %v, want default %v", got, defaultMetricConnectMaxLinks)
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
