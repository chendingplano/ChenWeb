package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

// Golden search results (openspec hybrid-search-engine-api, task 1.2). Live and read-only: it needs
// a dev database with kb.search_artifacts and an embedding model, so it runs only when
// SEARCH_GOLDEN_MODE is set.
//
//	SEARCH_GOLDEN_MODE=record  SEARCH_GOLDEN_FILE=<path>   writes today's result lists
//	SEARCH_GOLDEN_MODE=compare SEARCH_GOLDEN_FILE=<path>   fails when a list differs
//
// Run it once per configuration (SEARCH_SEMANTIC_ENABLED, SEARCH_LEXICAL_BACKEND), with a separate
// file each. SEARCH_GOLDEN_IDS_ONLY=1 compares the artifact order only (the lexical-only fallback
// reports raw lexical scores and highlighted snippets). Keyword extraction is switched off so runs
// do not depend on an LLM. Connection: PG_HOST, PG_PORT, PG_DB_NAME, PG_USER_NAME and PGPASSWORD.
type goldenHit struct {
	Type    string  `json:"type"`
	ID      string  `json:"id"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

type goldenCase struct {
	Total   int64       `json:"total"`
	Results []goldenHit `json:"results"`
	Error   string      `json:"error,omitempty"`
}

var goldenPurposeType = map[string]string{
	"search_metrics": "metric", "search_provisions": "provision", "search_products": "product",
	"search_entities": "entity", "search_relations": "relation", "search_inventory_items": "inventory_item",
	"search_scene_blocks": "scene_block", "search_semantic_projections": "semantic_projection",
	"search_summaries": "summary", "search_topics": "topic", "search_mixed": "",
}

func goldenDB(t *testing.T) *sql.DB {
	t.Helper()
	user := os.Getenv("PG_USER_NAME")
	if user == "" {
		user = "admin"
	}
	dsn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s sslmode=disable",
		os.Getenv("PG_HOST"), os.Getenv("PG_PORT"), os.Getenv("PG_DB_NAME"), user)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func goldenQueries(t *testing.T, db *sql.DB) []struct{ key, typ, text string } {
	t.Helper()
	rows, err := db.Query(`SELECT purpose, question FROM (
		SELECT purpose, question, id, ROW_NUMBER() OVER (PARTITION BY purpose ORDER BY id) AS n
		FROM testbed.generated_questions) q WHERE n <= 4 ORDER BY purpose, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []struct{ key, typ, text string }
	for rows.Next() {
		var purpose, q string
		if err := rows.Scan(&purpose, &q); err != nil {
			t.Fatal(err)
		}
		out = append(out, struct{ key, typ, text string }{purpose + "|" + q, goldenPurposeType[purpose], q})
	}
	for _, q := range []string{"最大工作压力 MPa", "maximum working pressure", "safety valve relief", "试验方法 温度"} {
		out = append(out, struct{ key, typ, text string }{"free|" + q, "", q})
		out = append(out, struct{ key, typ, text string }{"free-metric|" + q, "metric", q})
	}
	return out
}

func goldenRun(db *sql.DB, typ, text string, filters artifactSearchFilters, page, size int) goldenCase {
	cfg := loadRegistrySearchConfig()
	var gc goldenCase
	total, err := countRegistrySearchResults(db, typ, text, filters, cfg)
	if err != nil {
		gc.Error = "count: " + err.Error()
	}
	gc.Total = total
	res, err := queryRegistrySearchResults(db, typ, text, filters, page, size, cfg)
	if err != nil {
		gc.Error += " query: " + err.Error()
	}
	for _, r := range res {
		gc.Results = append(gc.Results, goldenHit{Type: r.ArtifactType, ID: r.ArtifactID, Score: math.Round(r.Score*1e9) / 1e9, Snippet: r.Snippet})
	}
	return gc
}

func TestSearchGoldenLive(t *testing.T) {
	mode, path := os.Getenv("SEARCH_GOLDEN_MODE"), os.Getenv("SEARCH_GOLDEN_FILE")
	if mode == "" || path == "" {
		t.Skip("SEARCH_GOLDEN_MODE / SEARCH_GOLDEN_FILE not set")
	}
	t.Setenv("SEARCH_QUERY_KEYWORDS", "off")
	db := goldenDB(t)
	got := map[string]goldenCase{}
	for _, q := range goldenQueries(t, db) {
		got[fmt.Sprintf("%s|type=%s|p1", q.key, q.typ)] = goldenRun(db, q.typ, q.text, artifactSearchFilters{}, 1, 20)
		if q.typ == "" {
			got[fmt.Sprintf("%s|type=|p2", q.key)] = goldenRun(db, q.typ, q.text, artifactSearchFilters{}, 2, 10)
			got[fmt.Sprintf("%s|types=metric,provision", q.key)] = goldenRun(db, "", q.text,
				artifactSearchFilters{ArtifactTypes: []string{"metric", "provision"}}, 1, 20)
		}
	}
	// Scope filters: one record, and a structural facet.
	var rec int64
	if err := db.QueryRow(`SELECT input_record_id FROM kb.search_artifacts WHERE artifact_type='metric' LIMIT 1`).Scan(&rec); err == nil {
		got["record-filter"] = goldenRun(db, "", "maximum working pressure", artifactSearchFilters{InputRecordID: &rec}, 1, 20)
	}
	got["provision-type"] = goldenRun(db, "provision", "安全要求", artifactSearchFilters{ProvisionType: "requirement"}, 1, 20)

	switch mode {
	case "record":
		raw, _ := json.MarshalIndent(got, "", " ")
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %d cases to %s", len(got), path)
	case "compare":
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var want map[string]goldenCase
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		idsOnly := os.Getenv("SEARCH_GOLDEN_IDS_ONLY") != ""
		diffs := 0
		for key, w := range want {
			g, ok := got[key]
			if !ok {
				t.Errorf("%s: missing", key)
				continue
			}
			if d := goldenDiff(w, g, idsOnly); d != "" {
				diffs++
				if diffs <= 15 {
					t.Errorf("%s: %s", key, d)
				}
			}
		}
		t.Logf("compared %d cases, %d differ", len(want), diffs)
	default:
		t.Fatalf("SEARCH_GOLDEN_MODE must be record or compare, got %q", mode)
	}
}

func goldenDiff(w, g goldenCase, idsOnly bool) string {
	if w.Error != g.Error {
		return fmt.Sprintf("error %q vs %q", w.Error, g.Error)
	}
	if !idsOnly && w.Total != g.Total {
		return fmt.Sprintf("total %d vs %d", w.Total, g.Total)
	}
	if len(w.Results) != len(g.Results) {
		return fmt.Sprintf("%d results vs %d", len(w.Results), len(g.Results))
	}
	for i := range w.Results {
		a, b := w.Results[i], g.Results[i]
		if a.Type != b.Type || a.ID != b.ID {
			return fmt.Sprintf("rank %d: %s/%s vs %s/%s", i+1, a.Type, a.ID, b.Type, b.ID)
		}
		if idsOnly {
			continue
		}
		if math.Abs(a.Score-b.Score) > 1e-9 {
			return fmt.Sprintf("rank %d %s score %.9f vs %.9f", i+1, a.ID, a.Score, b.Score)
		}
		if strings.TrimSpace(a.Snippet) != strings.TrimSpace(b.Snippet) {
			return fmt.Sprintf("rank %d %s snippet differs", i+1, a.ID)
		}
	}
	return ""
}
