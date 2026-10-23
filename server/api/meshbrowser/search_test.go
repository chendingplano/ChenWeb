package meshbrowser

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
)

func TestSearchOptions(t *testing.T) {
	for _, query := range []string{"", "q=heart&field=invalid", "q=heart&match=invalid", "q=heart&match=all+any", "q=heart&words=invalid", "q=heart&sort=invalid", "q=heart&limit=0", "q=heart&offset=-1"} {
		c := echo.New().NewContext(httptest.NewRequest("GET", "/?"+query, nil), httptest.NewRecorder())
		if _, err := searchOptions(c); err == nil {
			t.Errorf("accepted invalid search %q", query)
		}
	}
	c := echo.New().NewContext(httptest.NewRequest("GET", "/?q=heart", nil), httptest.NewRecorder())
	o, err := searchOptions(c)
	if err != nil || o.Field != "all_terms" || o.Match != "all" || o.Words != "full" || o.Limit != 50 {
		t.Fatalf("unexpected defaults: %+v, %v", o, err)
	}
}

func TestSearchParameterized(t *testing.T) {
	for _, field := range strings.Fields("all_terms descriptor qualifier scr id scope_note annotation registry pharmacological_action heading_mapped_to free_text") {
		for _, mode := range []string{"all", "any", "exact"} {
			query, args := searchSQL(SearchOptions{Query: "heart' OR 1=1 --", Field: field, Match: mode, Words: "substring", Limit: 20})
			if strings.Contains(query, "heart'") || len(args) < 3 || args[len(args)-2] != 21 {
				t.Fatalf("unsafe query or missing lookahead: %s, %v", query, args)
			}
		}
	}
}

// Optional read-only checks against an imported release. No tables or data are changed.
func TestSearchImportedRelease(t *testing.T) {
	dsn := os.Getenv("MESH_TEST_DSN")
	if dsn == "" {
		t.Skip("set MESH_TEST_DSN to check an imported MeSH release")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Store{DB: db}
	for _, tc := range []struct{ q, field, mode, words, id string }{
		{"D006321", "id", "exact", "full", "D006321"},
		{"Heart", "descriptor", "exact", "full", "D006321"},
		{"Hearts", "all_terms", "exact", "full", "D006321"},
		{"heart", "descriptor", "all", "substring", ""},
		{"heart lung", "descriptor", "all", "full", ""},
		{"heart lung", "descriptor", "any", "full", ""},
		{"heart", "scope_note", "all", "full", ""},
		{"123456789noSuchMeshTerm", "all_terms", "exact", "full", "EMPTY"},
	} {
		t.Run(tc.field+"/"+tc.q, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			rows, err := s.Search(ctx, SearchOptions{Query: tc.q, Field: tc.field, Match: tc.mode, Words: tc.words, Sort: "label", Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			if tc.id == "EMPTY" {
				if len(rows) != 0 {
					t.Fatalf("expected no matches, got %d", len(rows))
				}
				return
			}
			if len(rows) == 0 {
				t.Fatal("expected search results")
			}
			if tc.id != "" {
				found := false
				for _, row := range rows {
					if row.ID == tc.id {
						found = true
					}
				}
				if !found {
					t.Fatalf("expected %s in results: %v", tc.id, rows)
				}
			}
		})
	}
	for _, field := range strings.Fields("qualifier scr annotation registry pharmacological_action heading_mapped_to free_text") {
		t.Run("field/"+field, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_, err := s.Search(ctx, SearchOptions{Query: "heart", Field: field, Match: "all", Words: "full", Sort: "id", Limit: 2})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSearchPatterns(t *testing.T) {
	for _, tc := range []struct {
		q, match, words string
		want            string
	}{
		{"a%b_c", "all", "substring", `%a\%b\_c%`},
		{"heart (left)", "exact", "full", "heart (left)"},
		{"C++", "all", "full", `(^|[^[:alnum:]_])C\+\+($|[^[:alnum:]_])`},
	} {
		_, args := searchSQL(SearchOptions{Query: tc.q, Field: "all_terms", Match: tc.match, Words: tc.words, Sort: "label", Limit: 20})
		if args[0] != tc.want {
			t.Errorf("pattern for %q: got %q, want %q", tc.q, args[0], tc.want)
		}
	}
}
