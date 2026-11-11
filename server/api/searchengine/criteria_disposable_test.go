package searchengine

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// Run only against an empty disposable database, supplied by PG* variables.
func TestDisposableCriteria(t *testing.T) {
	if os.Getenv("SEARCHENGINE_DISPOSABLE_INTEGRATION") != "1" {
		t.Skip("requires an empty disposable database with pg_search and vector")
	}
	db, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE EXTENSION vector; CREATE EXTENSION pg_search; CREATE SCHEMA kb;
 CREATE TABLE kb.search_artifacts (artifact_type text NOT NULL, artifact_id text NOT NULL, input_record_id bigint NOT NULL, primary_label text DEFAULT '', secondary_label text DEFAULT '', search_document text DEFAULT '', search_vector tsvector, snippet_basis text DEFAULT '', source_title text DEFAULT '', source_filename text DEFAULT '', source_line_spans jsonb DEFAULT '[]', semantic_payload jsonb DEFAULT '{}', keywords text[], embedding vector(2), PRIMARY KEY (artifact_type, artifact_id)) PARTITION BY LIST (artifact_type);
 CREATE TABLE kb.search_artifacts_provision PARTITION OF kb.search_artifacts FOR VALUES IN ('provision');
 CREATE TABLE kb.chunk_search_windows (artifact_id text, embedding vector(2));
 CREATE INDEX provision_bm25 ON kb.search_artifacts_provision USING bm25 (artifact_id, search_document, primary_label, secondary_label, snippet_basis, source_title, source_filename) WITH (key_field='artifact_id');
 INSERT INTO kb.search_artifacts (artifact_type,artifact_id,input_record_id,search_document,search_vector,keywords,embedding) VALUES
 ('provision','ref',1,'needle',to_tsvector('simple','needle'),ARRAY['needle'],'[1,0]'),
 ('provision','own',1,'needle',to_tsvector('simple','needle'),ARRAY['needle'],'[1,0]'),
 ('provision','vector',2,'other',to_tsvector('simple','other'),ARRAY['other'],'[0.99,0.1]'),
 ('provision','lexical',3,'needle',to_tsvector('simple','needle'),ARRAY['needle'],'[0,1]');`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, backend := range []Backend{BackendPostgres, BackendParadeDB} {
		t.Run(string(backend), func(t *testing.T) {
			cfg := Config{Backend: backend, Dictionary: "simple", LexicalWeight: 1, SemanticWeight: 1, SemanticEnabled: true, CandidateLimit: 1}
			req := Request{Query: Query{Ref: &ArtifactRef{Type: "provision", ID: "ref"}}, Filters: Filters{ArtifactTypes: []string{"provision"}, ExcludeRecordIDs: []int64{1}}, Options: Options{Limit: 10}}
			base, err := Search(ctx, db, cfg, req)
			if err != nil {
				t.Fatal(err)
			}
			byID := map[string]Result{}
			for _, r := range base.Results {
				byID[r.ArtifactID] = r
			}
			if len(byID) != 2 || byID["vector"].Diag.VectorRank == nil || byID["vector"].Diag.LexicalRank != nil || byID["lexical"].Diag.LexicalRank == nil || byID["lexical"].Diag.VectorRank != nil {
				t.Fatalf("expected vector-only and lexical-only peers after excluding own record: %+v", base.Results)
			}
			req.Criteria.Keywords = []Keyword{{Term: "needle", Required: true}}
			required, err := Search(ctx, db, cfg, req)
			if err != nil {
				t.Fatal(err)
			}
			if len(required.Results) != 1 || required.Results[0].ArtifactID != "lexical" || required.Removed.Required != 1 {
				t.Fatalf("required: %+v", required)
			}
			req.Criteria = Criteria{MinCosine: 0.75}
			floor, err := Search(ctx, db, cfg, req)
			if err != nil {
				t.Fatal(err)
			}
			if len(floor.Results) != 1 || floor.Results[0].ArtifactID != "vector" || floor.Removed.MinCosine != 1 {
				t.Fatalf("floor: %+v", floor)
			}
		})
	}
}
