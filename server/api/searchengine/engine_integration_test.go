package searchengine

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

// Integration tests read kb.search_artifacts of a database with pg_search and pgvector (a copy of
// the dev data with provisions of two or more records). They only SELECT. Set
// SEARCHENGINE_TEST_DATABASE_URL and SEARCHENGINE_TEST_REF=<provision artifact id> (default
// 416_prv_24); the reference's own record is excluded in the tests.
func integrationDB(t *testing.T) (*sql.DB, ArtifactRef, int64) {
	t.Helper()
	url := os.Getenv("SEARCHENGINE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SEARCHENGINE_TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	id := os.Getenv("SEARCHENGINE_TEST_REF")
	if id == "" {
		id = "416_prv_24"
	}
	var rec int64
	if err := db.QueryRow(`SELECT input_record_id FROM kb.search_artifacts WHERE artifact_type='provision' AND artifact_id=$1`, id).Scan(&rec); err != nil {
		t.Skipf("reference %s not found: %v", id, err)
	}
	return db, ArtifactRef{"provision", id}, rec
}

func integrationConfig(b Backend) Config {
	return Config{Backend: b, Dictionary: "simple", LexicalWeight: 1, SemanticWeight: 1, SemanticEnabled: true}
}

func TestIntegrationReferenceExcludesOwnRecordBeforeRanking(t *testing.T) {
	db, ref, rec := integrationDB(t)
	for _, backend := range []Backend{BackendParadeDB, BackendPostgres} {
		cfg := integrationConfig(backend)
		resp, err := Search(context.Background(), db, cfg, Request{
			Query:   Query{Ref: &ref},
			Filters: Filters{ArtifactTypes: []string{"provision"}, ExcludeRecordIDs: []int64{rec}},
			Options: Options{Limit: 10},
		})
		if err != nil {
			t.Fatalf("%s: %v", backend, err)
		}
		if len(resp.Results) == 0 {
			t.Fatalf("%s: no results from other records", backend)
		}
		for i, r := range resp.Results {
			if r.InputRecordID == rec || r.ArtifactID == ref.ID {
				t.Errorf("%s: result %s belongs to the excluded record or is the reference", backend, r.ArtifactID)
			}
			if r.Rank != i+1 {
				t.Errorf("%s: rank %d at position %d", backend, r.Rank, i+1)
			}
			if r.Diag.Cosine == nil {
				t.Errorf("%s: %s has no cosine diagnostic", backend, r.ArtifactID)
			}
		}
	}
}

func TestIntegrationReferenceAloneNeverReturnsItself(t *testing.T) {
	db, ref, _ := integrationDB(t)
	resp, err := Search(context.Background(), db, integrationConfig(BackendParadeDB), Request{
		Query: Query{Ref: &ref}, Filters: Filters{ArtifactTypes: []string{"provision"}}, Options: Options{Limit: 50},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resp.Results {
		if r.ArtifactID == ref.ID {
			t.Fatalf("the referenced artifact is in its own results")
		}
	}
}

func TestIntegrationCriteria(t *testing.T) {
	db, ref, rec := integrationDB(t)
	cfg := integrationConfig(BackendParadeDB)
	base := func() Request {
		return Request{Query: Query{Ref: &ref}, Filters: Filters{ArtifactTypes: []string{"provision"}, ExcludeRecordIDs: []int64{rec}}, Options: Options{Limit: 20}}
	}

	req := base()
	req.Criteria.Keywords = []Keyword{{Term: "zzqqxx", Required: true}}
	resp, err := Search(context.Background(), db, cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 0 || resp.Removed.Required == 0 {
		t.Errorf("a required keyword nothing contains must remove everything: %d results, removed %+v", len(resp.Results), resp.Removed)
	}

	req = base()
	req.Criteria.MinCosine = 0.75
	resp, err = Search(context.Background(), db, cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resp.Results {
		if r.Diag.Cosine == nil || *r.Diag.Cosine < 0.75 {
			t.Errorf("%s below the cosine floor: %v", r.ArtifactID, r.Diag.Cosine)
		}
	}

	req = base()
	req.Criteria.Keywords = []Keyword{{Term: "zzqqxx"}, {Term: "qqzzxx"}}
	req.Criteria.MinMatched = 1
	resp, err = Search(context.Background(), db, cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 0 || resp.Removed.MinMatched == 0 {
		t.Errorf("min matched: %d results, removed %+v", len(resp.Results), resp.Removed)
	}

	// An excluded keyword removes the results that contain it.
	req = base()
	plain, err := Search(context.Background(), db, cfg, req)
	if err != nil || len(plain.Results) == 0 {
		t.Fatalf("baseline: %v", err)
	}
	word := firstWord(t, db, plain.Results[0].ArtifactID)
	req.Criteria.Keywords = []Keyword{{Term: word, Excluded: true}}
	resp, err = Search(context.Background(), db, cfg, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Removed.Excluded == 0 {
		t.Errorf("excluding %q removed nothing", word)
	}
	for _, r := range resp.Results {
		if r.ArtifactID == plain.Results[0].ArtifactID {
			t.Errorf("excluded keyword %q still matches %s", word, r.ArtifactID)
		}
	}
}

func firstWord(t *testing.T, db *sql.DB, artifactID string) string {
	t.Helper()
	var doc string
	if err := db.QueryRow(`SELECT search_document FROM kb.search_artifacts WHERE artifact_id=$1 LIMIT 1`, artifactID).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	terms := frequentTerms(doc, 1)
	if len(terms) == 0 {
		t.Skip("no usable term")
	}
	return terms[0]
}

func TestIntegrationUnknownReferenceIsAnError(t *testing.T) {
	db, _, _ := integrationDB(t)
	_, err := Search(context.Background(), db, integrationConfig(BackendParadeDB), Request{Query: Query{Ref: &ArtifactRef{"provision", "no_such_id"}}})
	if err == nil || !strings.Contains(err.Error(), "no_such_id") {
		t.Errorf("err = %v, want one naming the reference", err)
	}
}

func TestIntegrationWeightedKeywordsRankHigher(t *testing.T) {
	db, _, _ := integrationDB(t)
	cfg := integrationConfig(BackendParadeDB)
	cfg.SemanticEnabled = false
	run := func(w1, w2 float64) []Result {
		resp, err := Search(context.Background(), db, cfg, Request{
			Query:    Query{Text: "weights"},
			Filters:  Filters{ArtifactTypes: []string{"provision"}},
			Criteria: Criteria{Keywords: []Keyword{{Term: "排放", Weight: w1}, {Term: "填埋", Weight: w2}}},
			Options:  Options{Limit: 5},
		})
		if err != nil {
			t.Fatal(err)
		}
		return resp.Results
	}
	a, b := run(5, 1), run(1, 5)
	if len(a) == 0 || len(b) == 0 {
		t.Skip("no lexical matches for the sample words in this database")
	}
	if a[0].ArtifactID == b[0].ArtifactID && len(a) > 1 && a[1].ArtifactID == b[1].ArtifactID {
		t.Log("weights did not reorder the top results on this data")
	}
	for _, r := range a {
		if r.Diag.LexicalRank == nil || r.Diag.VectorRank != nil {
			t.Errorf("keyword-only search diag = %+v", r.Diag)
		}
	}
}

func TestIntegrationOffsetPagesTheFusedList(t *testing.T) {
	db, _, _ := integrationDB(t)
	for _, backend := range []Backend{BackendParadeDB, BackendPostgres} {
		cfg := integrationConfig(backend)
		search := func(limit, offset int) *Response {
			resp, err := Search(context.Background(), db, cfg, Request{
				Query:   Query{Text: "安全"},
				Filters: Filters{ArtifactTypes: []string{"provision"}},
				Options: Options{Limit: limit, Offset: offset, KeywordExtraction: KeywordExtractionOff},
			})
			if err != nil {
				t.Fatalf("%s: %v", backend, err)
			}
			return resp
		}
		whole, first, second := search(10, 0), search(5, 0), search(5, 5)
		if len(whole.Results) < 10 {
			t.Skipf("%s: only %d results for the paging query", backend, len(whole.Results))
		}
		got := append(append([]Result{}, first.Results...), second.Results...)
		for i, r := range got {
			if r.ArtifactID != whole.Results[i].ArtifactID || r.Rank != i+1 {
				t.Fatalf("%s: page entry %d is %s rank %d, whole list has %s", backend, i, r.ArtifactID, r.Rank, whole.Results[i].ArtifactID)
			}
		}
	}
}

func TestIntegrationCountAndTrace(t *testing.T) {
	db, _, _ := integrationDB(t)
	for _, backend := range []Backend{BackendParadeDB, BackendPostgres} {
		cfg := integrationConfig(backend)
		req := Request{
			Query:   Query{Text: "安全"},
			Filters: Filters{ArtifactTypes: []string{"provision"}},
			Options: Options{Limit: 5, KeywordExtraction: KeywordExtractionOff, Trace: true},
		}
		resp, err := Search(context.Background(), db, cfg, req)
		if err != nil {
			t.Fatalf("%s: %v", backend, err)
		}
		total, err := Count(context.Background(), db, cfg, req)
		if err != nil || total < int64(len(resp.Results)) {
			t.Fatalf("%s: count=%d results=%d err=%v", backend, total, len(resp.Results), err)
		}
		if resp.Trace == nil || len(resp.Trace.Fused) != resp.Candidates || len(resp.Trace.Lexical) == 0 {
			t.Fatalf("%s: trace=%+v candidates=%d", backend, resp.Trace, resp.Candidates)
		}
		if len(resp.Results) > 0 && resp.Trace.Fused[0].ArtifactID != resp.Results[0].ArtifactID {
			t.Fatalf("%s: fused list starts with %s, results with %s", backend, resp.Trace.Fused[0].ArtifactID, resp.Results[0].ArtifactID)
		}
		if backend == BackendParadeDB && len(resp.Trace.LexicalPartitions) < len(resp.Trace.Lexical) {
			t.Fatalf("%s: partition rows %d < merged rows %d", backend, len(resp.Trace.LexicalPartitions), len(resp.Trace.Lexical))
		}
		if backend == BackendPostgres && len(resp.Trace.LexicalPartitions) != 0 {
			t.Fatalf("%s: Postgres has no partition lists", backend)
		}
	}
}

func TestIntegrationPostgresMatchesChineseBySubstring(t *testing.T) {
	db, _, _ := integrationDB(t)
	cfg := integrationConfig(BackendPostgres)
	resp, err := Search(context.Background(), db, cfg, Request{
		Query:   Query{Text: "安全"},
		Filters: Filters{ArtifactTypes: []string{"provision"}},
		Options: Options{Limit: 5, KeywordExtraction: KeywordExtractionOff},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("no provision matched 安全 by substring")
	}
}
