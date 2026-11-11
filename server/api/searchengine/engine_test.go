package searchengine

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRequestValidation(t *testing.T) {
	cases := []struct {
		name string
		q    Query
		want error
	}{
		{"neither", Query{}, ErrNoQuery},
		{"blank text", Query{Text: "  "}, ErrNoQuery},
		{"both", Query{Text: "x", Ref: &ArtifactRef{"provision", "1"}}, ErrBothQueryForms},
		{"text", Query{Text: "x"}, nil},
		{"ref", Query{Ref: &ArtifactRef{"provision", "1"}}, nil},
	}
	for _, c := range cases {
		if err := (Request{Query: c.q}).validate(); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	if err := (Request{Query: Query{Ref: &ArtifactRef{Type: "provision"}}}).validate(); err == nil {
		t.Error("a reference without an id must be rejected")
	}
}

func TestFuseScoresAndRanks(t *testing.T) {
	cfg := Config{LexicalWeight: 1, SemanticWeight: 1}
	lex := []lexHit{{"provision", "a", 9}, {"provision", "b", 5}}
	vec := []vecHit{{candKey{"provision", "c"}, 0.1}, {candKey{"provision", "d"}, 0.2}, {candKey{"provision", "a"}, 0.3}}
	got := fuse(cfg, lex, vec)

	byID := map[string]*fusedCand{}
	for _, c := range got {
		byID[c.id] = c
	}
	// a is 1st lexical and 3rd vector: 1/61 + 1/63.
	want := 1.0/61 + 1.0/63
	if d := byID["a"].score - want; d > 1e-12 || d < -1e-12 {
		t.Errorf("score(a) = %v, want %v", byID["a"].score, want)
	}
	if *byID["a"].diag.LexicalRank != 1 || *byID["a"].diag.VectorRank != 3 {
		t.Errorf("diag(a) = %+v", byID["a"].diag)
	}
	// c is vector-only: no lexical rank or score, score from the vector rank alone.
	if byID["c"].diag.LexicalRank != nil || byID["c"].diag.LexicalScore != nil {
		t.Errorf("vector-only result has lexical diagnostics: %+v", byID["c"].diag)
	}
	if byID["c"].score != 1.0/61 {
		t.Errorf("score(c) = %v, want 1/61", byID["c"].score)
	}
	if got[0].id != "a" {
		t.Errorf("first = %s, want a", got[0].id)
	}
	// c (vector 1) and b (lexical 2) : 1/61 vs 1/62.
	order := []string{}
	for _, c := range got {
		order = append(order, c.id)
	}
	if strings.Join(order, "") != "acbd" {
		t.Errorf("order = %v, want a c b d", order)
	}
}

func TestFuseTieBreaksByID(t *testing.T) {
	cfg := Config{LexicalWeight: 1, SemanticWeight: 1}
	got := fuse(cfg, []lexHit{{"t", "z", 1}}, []vecHit{{candKey{"t", "a"}, 0.1}})
	if got[0].id != "a" {
		t.Errorf("equal scores must order by artifact id, got %s first", got[0].id)
	}
}

func TestResolveKeywordListOrder(t *testing.T) {
	defer SetKeywordExtractor(nil)
	calls := 0
	SetKeywordExtractor(func(_ context.Context, q string, force bool) []string {
		calls++
		return []string{"alpha", " alpha ", "", "--", "beta"}
	})
	ctx := context.Background()
	text := Request{Query: Query{Text: "how much alpha"}}
	pg := Config{Backend: BackendPostgres}
	pdb := Config{Backend: BackendParadeDB}

	// Extracted keywords are cleaned.
	if s := resolveKeywordList(ctx, pdb, text, nil); len(s.keywords) != 2 || s.keywords[0].Term != "alpha" || s.keywords[0].Weight != 1 {
		t.Errorf("extracted = %+v", s)
	}
	// Extraction off: the raw query, no call.
	calls = 0
	off := text
	off.Options.KeywordExtraction = KeywordExtractionOff
	if s := resolveKeywordList(ctx, pdb, off, nil); s.raw != "how much alpha" || calls != 0 {
		t.Errorf("off: spec %+v calls %d", s, calls)
	}
	// Caller keywords skip extraction and keep weights; excluded ones never enter the list.
	withKw := text
	withKw.Criteria.Keywords = []Keyword{{Term: "GB 16889", Weight: 3}, {Term: "填埋"}, {Term: "医疗", Excluded: true}}
	s := resolveKeywordList(ctx, pdb, withKw, nil)
	if calls != 0 || len(s.keywords) != 2 || s.keywords[0].Weight != 3 || s.keywords[1].Weight != 1 {
		t.Errorf("caller keywords: spec %+v calls %d", s, calls)
	}
	// No extractor registered: raw query.
	SetKeywordExtractor(nil)
	if s := resolveKeywordList(ctx, pdb, text, nil); s.raw == "" {
		t.Errorf("no extractor: %+v", s)
	}

	// Reference queries: stored keywords, else the stored text.
	ref := Request{Query: Query{Ref: &ArtifactRef{"provision", "1"}}}
	if s := resolveKeywordList(ctx, pdb, ref, &refRow{keywords: []string{"k1", "k2"}, doc: "ignored"}); len(s.keywords) != 2 {
		t.Errorf("stored keywords: %+v", s)
	}
	if s := resolveKeywordList(ctx, pdb, ref, &refRow{doc: "some stored text"}); s.mlt != "some stored text" {
		t.Errorf("paradedb fallback must use more-like-this: %+v", s)
	}
	s = resolveKeywordList(ctx, pg, ref, &refRow{doc: "pump pump valve valve valve seat"})
	if len(s.keywords) == 0 || s.keywords[0].Term != "valve" || s.mlt != "" || s.raw != "" {
		t.Errorf("postgres fallback must select frequent terms: %+v", s)
	}
	if s := resolveKeywordList(ctx, pdb, ref, &refRow{}); !s.empty() {
		t.Errorf("nothing stored must give an empty spec: %+v", s)
	}
}

func TestFilterClausesApplyExclusionsAndSelf(t *testing.T) {
	var a argList
	c := filterClauses(&a, Filters{
		ArtifactTypes:      []string{"provision"},
		ExcludeRecordIDs:   []int64{416},
		ExcludeArtifactIDs: []string{"x"},
		KnowledgeStoreID:   "ks-1",
	}, &ArtifactRef{"provision", "416_prv_24"}, true)
	sqlText := strings.Join(c, " AND ")
	for _, want := range []string{"sa.artifact_type = ANY($1)", "NOT (sa.input_record_id = ANY($2))", "NOT (sa.artifact_id = ANY($3))",
		"NOT (sa.artifact_type = $4 AND sa.artifact_id = $5)", "agent_store.id::text=$6"} {
		if !strings.Contains(sqlText, want) {
			t.Errorf("missing %q in %s", want, sqlText)
		}
	}
	if len(a.vals) != 6 {
		t.Errorf("args = %d, want 6", len(a.vals))
	}
	var b argList
	if got := filterClauses(&b, Filters{ArtifactTypes: []string{"provision"}}, nil, false); len(got) != 0 {
		t.Errorf("a partition branch needs no type condition: %v", got)
	}
}

func TestFieldsClauseBoost(t *testing.T) {
	if c := fieldsClause("&&&", "$1", 1); strings.Contains(c, "boost") {
		t.Errorf("weight 1 must not boost: %s", c)
	}
	c := fieldsClause("&&&", "$2", 3)
	if !strings.Contains(c, "sa.search_document &&& $2::pdb.boost(3)") || strings.Count(c, "pdb.boost(3)") != len(paradeDBFields) {
		t.Errorf("weight 3 must boost every field: %s", c)
	}
	if !strings.Contains(fieldsClause("&&&", "$2", 0.5), "pdb.boost(0.5)") {
		t.Error("fractional weights must render as numbers")
	}
}

func TestPartitionsForRejectsUnknownType(t *testing.T) {
	if _, err := partitionsFor([]string{"provision; DROP TABLE x"}); err == nil {
		t.Error("an unknown type must be rejected, never rendered into SQL")
	}
	all, _ := partitionsFor(nil)
	if len(all) != len(partitionTypes) {
		t.Errorf("no types = every partition, got %d", len(all))
	}
}

func TestCleanKeywordsAndFrequentTerms(t *testing.T) {
	long := strings.Repeat("a", 65)
	got := cleanKeywords([]string{" a1 ", "a1", "", "!!", long, "填埋"})
	if strings.Join(got, "|") != "a1|填埋" {
		t.Errorf("cleanKeywords = %v", got)
	}
	var many []string
	for i := 0; i < 20; i++ {
		many = append(many, "k"+string(rune('a'+i)))
	}
	if n := len(cleanKeywords(many)); n != maxKeywordStrings {
		t.Errorf("kept %d, want %d", n, maxKeywordStrings)
	}
	if ft := frequentTerms("Valve valve a seat seat valve", 2); strings.Join(ft, "|") != "valve|seat" {
		t.Errorf("frequentTerms = %v", ft)
	}
}
