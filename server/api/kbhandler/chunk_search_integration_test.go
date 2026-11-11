package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// Run only against an empty disposable database supplied through PG* variables.
func TestChunkSearchPostgresIntegration(t *testing.T) {
	if os.Getenv("CHUNK_SEARCH_INTEGRATION") != "1" {
		t.Skip("requires disposable PostgreSQL database with vector and pg_search")
	}
	db, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := docprocessing.WithLLMUserID(context.Background(), "chunk-integration-test")
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE EXTENSION IF NOT EXISTS vector; CREATE EXTENSION IF NOT EXISTS pg_search; CREATE SCHEMA kb;
 CREATE TABLE kb.inputs (id bigint PRIMARY KEY, result_filename text, staging_filename text, parser_name text, file_name text, status jsonb DEFAULT '[]', doc_metadata jsonb DEFAULT '{}');
 CREATE TABLE kb.search_artifacts (
 artifact_type text NOT NULL, artifact_id text NOT NULL, input_record_id bigint REFERENCES kb.inputs(id) ON DELETE CASCADE,
 source_row_id bigint, primary_label text DEFAULT '', secondary_label text DEFAULT '', search_document text DEFAULT '', search_vector tsvector,
 snippet_basis text DEFAULT '', source_title text DEFAULT '', source_filename text DEFAULT '', category_paths jsonb DEFAULT '[]', source_line_spans jsonb DEFAULT '[]',
 semantic_payload jsonb DEFAULT '{}', keywords text[], embedding_text text, embedding vector(1024), updated_at timestamptz DEFAULT now(),
 PRIMARY KEY (artifact_type,artifact_id)) PARTITION BY LIST (artifact_type);`)
	migration, err := os.ReadFile("../../../project_migrations/20261111000001_add_chunk_hybrid_search.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(migration), "-- +goose Down", 2)
	exec(parts[0])
	exec(parts[1])
	exec(parts[0])
	t.Log("migration up/down/up passed with BM25 and HNSW indexes")

	root := t.TempDir()
	t.Setenv("ARTIFACT_DIR", root)
	t.Setenv("SEARCH_SEMANTIC_ENABLED", "true")
	t.Setenv("SEARCH_QUERY_KEYWORDS", "off")
	t.Setenv("EMBEDDING_DIMENSIONS", "1024")
	t.Setenv("EMBEDDING_MAX_GOROUTINES", "1")
	t.Setenv("EMBEDDING_BATCH_SIZE", "1")
	first := make([]float64, 1024)
	first[0] = 1
	tail := make([]float64, 1024)
	tail[1] = 1
	var fail atomic.Bool
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input any `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		var inputs []string
		switch x := req.Input.(type) {
		case string:
			inputs = []string{x}
		case []any:
			for _, v := range x {
				inputs = append(inputs, v.(string))
			}
		}
		var data []map[string]any
		for _, s := range inputs {
			if fail.Load() && strings.Contains(s, "tailneedle") {
				w.WriteHeader(400)
				return
			}
			vec := first
			if strings.Contains(s, "tailneedle") {
				vec = tail
			}
			data = append(data, map[string]any{"embedding": vec, "index": len(data)})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer model.Close()
	modelFile := filepath.Join(root, "models.toml")
	if err := os.WriteFile(modelFile, []byte("[chunk-test]\nhost = \"cloud\"\nmodel_name = \"chunk-test-vector\"\napi_key = \"test\"\nbase_url = \""+model.URL+"\"\ntimeout_sec = 5\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EMBEDDING_MODEL_NAME", "chunk-test")
	t.Setenv("MODELS_FILE", modelFile)
	source := filepath.Join(root, "source_test.txt")
	body := "1\t1\tparagraph\tF\t12\t[1,1,2,2]\tshort introduction\n2\t2\tparagraph\tF\t12\t[1,1,2,2]\t" + strings.Repeat("中", 6200) + " tailneedle\n"
	if err := os.WriteFile(source, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "0", "42")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "source_test.chunks"), []byte("overlap: []\nlines: [1]\noverlap: [1]\nlines: [2]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO kb.inputs (id,result_filename,staging_filename,parser_name,file_name) VALUES (42,$1,'source.pdf','test','source.pdf')`, source)
	logger := loggerutil.CreateDefaultLogger("20261111-733")
	res, err := docprocessing.BackfillChunkSearch(ctx, db, 0, 42, 1, logger)
	if err != nil || res.Indexed != 1 {
		t.Fatalf("backfill: %+v %v", res, err)
	}
	var windows int
	if err := db.QueryRow(`SELECT count(*) FROM kb.chunk_search_windows`).Scan(&windows); err != nil || windows != 1 {
		t.Fatalf("extra windows=%d err=%v", windows, err)
	}

	cfg := registrySearchConfig{dictionary: "simple", previewMaxWords: 20, lexicalWeight: 1, semanticWeight: 1, candidateLimit: 20}
	for _, backend := range []string{"postgres", "paradedb"} {
		t.Run(backend, func(t *testing.T) {
			var got []artifactSearchResult
			var err error
			filters := artifactSearchFilters{ArtifactTypes: []string{"chunk"}, InputRecordID: func() *int64 { v := int64(42); return &v }()}
			if backend == "postgres" {
				got, err = queryHybridSearchResults(db, "all", "semantic-only-query", tail, filters, 1, 20, cfg)
			} else {
				got, err = queryHybridSearchResultsParadeDB(db, "all", "semantic-only-query", tail, filters, 1, 20, cfg)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 || got[0].ArtifactID != "42_chk_2" {
				t.Fatalf("tail match not first or duplicate chunks: %+v", got)
			}
			filters.InputRecordID = func() *int64 { v := int64(99); return &v }()
			got, err = queryHybridSearchResults(db, "all", "semantic-only-query", tail, filters, 1, 20, cfg)
			if err != nil || len(got) != 0 {
				t.Fatalf("window filter leaked: %+v %v", got, err)
			}
		})
	}
	// Exact lexical tail retrieval and global chunk selection use real BM25.
	t.Setenv("SEARCH_LEXICAL_BACKEND", "paradedb")
	got, err := queryRegistrySearchResultsParadeDB(db, "chunk", "tailneedle", artifactSearchFilters{}, 1, 10, cfg)
	if err != nil || len(got) != 1 || got[0].ArtifactID != "42_chk_2" {
		t.Fatalf("lexical tail: %+v %v", got, err)
	}

	// A failed later window is retained as text and recovered by ordinary embedding backfill.
	fail.Store(true)
	res, err = docprocessing.BackfillChunkSearch(ctx, db, 0, 42, 1, logger)
	if err != nil || res.Indexed != 1 {
		t.Fatalf("lexical fallback backfill: %+v %v", res, err)
	}
	var missing int
	if err := db.QueryRow(`SELECT count(*) FROM kb.chunk_search_windows WHERE embedding IS NULL AND embedding_text LIKE '%tailneedle%'`).Scan(&missing); err != nil || missing != 1 {
		t.Fatalf("failed tail not retained: %d %v", missing, err)
	}
	recovery, err := kbsearch.BackfillEmbeddings(ctx, db, func(_ context.Context, s string) ([]float64, bool) {
		if strings.Contains(s, "tailneedle") {
			return tail, true
		}
		return first, true
	}, "chunk", false, 10)
	if err != nil || recovery.Embedded != 1 {
		t.Fatalf("window recovery: %+v %v", recovery, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM kb.chunk_search_windows WHERE embedding IS NOT NULL`).Scan(&windows); err != nil || windows != 1 {
		t.Fatalf("tail did not recover: %d %v", windows, err)
	}

	// A new generation wins over a backfill snapshot captured before invalidation.
	generation, err := kbsearch.ChunkSearchGeneration(ctx, db, 42)
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE kb.inputs SET parser_name = 'new-parser' WHERE id=42`)
	err = kbsearch.ReplaceChunkSearchRows(ctx, db, 42, nil, generation)
	if err == nil || !strings.Contains(err.Error(), "source changed") {
		t.Fatalf("stale writer restored chunks: %v", err)
	}
	var remaining int
	if err := db.QueryRow(`SELECT count(*) FROM kb.search_artifacts`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("stale chunks remain: %d %v", remaining, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM kb.chunk_search_windows`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("orphan windows remain: %d %v", remaining, err)
	}
	t.Log("long-tail retrieval, window deduplication, source filtering, recovery and stale-write rejection passed")

	// A competing backfill must wait until a source rewrite and invalidation finish.
	unlock, err := kbsearch.LockChunkSearchSource(ctx, db, 42)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan docprocessing.ChunkSearchBackfillResult, 1)
	go func() {
		res, err := docprocessing.BackfillChunkSearch(ctx, db, 0, 42, 1, logger)
		if err != nil {
			res.Failed++
		}
		done <- res
	}()
	select {
	case <-done:
		unlock()
		t.Fatal("backfill read source while rewrite lock was held")
	case <-time.After(100 * time.Millisecond):
	}
	exec(`UPDATE kb.inputs SET parser_name='test' WHERE id=42`)
	if err := os.Remove(filepath.Join(dir, "source_test.chunks")); err != nil {
		unlock()
		t.Fatal(err)
	}
	if err := kbsearch.InvalidateChunkSearch(ctx, db, 42); err != nil {
		unlock()
		t.Fatal(err)
	}
	unlock()
	select {
	case res := <-done:
		if res.Failed != 1 || res.Indexed != 0 {
			t.Fatalf("restored pre-rewrite source: %+v", res)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("source lock was not released")
	}
	t.Log("backfill/source-mutation serialization passed")
	// Admission reserves work capacity even with a two-connection centralized pool.
	db.SetMaxOpenConns(2)
	exec(`INSERT INTO kb.inputs(id) VALUES (99)`)
	for _, otherID := range []int64{42, 99} {
		hold, err := kbsearch.LockChunkSearchSource(ctx, db, 42)
		if err != nil {
			t.Fatal(err)
		}
		finished := make(chan error, 1)
		limited, cancel := context.WithTimeout(ctx, 2*time.Second)
		go func(id int64) {
			release, err := kbsearch.LockChunkSearchSource(limited, db, id)
			if err == nil {
				_, err = kbsearch.ChunkSearchGeneration(limited, db, id)
				release()
			}
			finished <- err
		}(otherID)
		var n int
		if err := db.QueryRowContext(limited, `SELECT 1`).Scan(&n); err != nil {
			hold()
			cancel()
			t.Fatalf("lock callers starved work pool: %v", err)
		}
		hold()
		if err := <-finished; err != nil {
			cancel()
			t.Fatal(err)
		}
		cancel()
	}
	t.Log("bounded-pool same/different-record concurrency passed")
}
