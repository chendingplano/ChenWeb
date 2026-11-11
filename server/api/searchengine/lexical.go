package searchengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// paradeDBFields are the text fields of the kb.search_artifacts BM25 index (Jieba tokenizer).
var paradeDBFields = []string{"search_document", "primary_label", "secondary_label", "snippet_basis", "source_title", "source_filename"}

// mlt is the maximum text handed to more-like-this.
const mltMaxRunes = 2000

// lexSpec is what the keyword list searches for: weighted keywords (an artifact matches when every
// token of at least one keyword occurs in one field), else the raw query text (any token), else the
// text of a referenced artifact (more-like-this, ParadeDB only).
type lexSpec struct {
	keywords []Keyword
	raw      string
	mlt      string
}

func (s lexSpec) empty() bool { return len(s.keywords) == 0 && s.raw == "" && s.mlt == "" }

type lexHit struct {
	typ, id string
	score   float64
}

// fieldsClause applies one ParadeDB operator with one placeholder to every BM25 field. A weight other
// than 1 boosts that keyword's BM25 contribution; the factor is a number rendered by the engine.
func fieldsClause(op, placeholder string, weight float64) string {
	suffix := ""
	if weight > 0 && weight != 1 {
		suffix = "::pdb.boost(" + strconv.FormatFloat(weight, 'f', -1, 64) + ")"
	}
	parts := make([]string, 0, len(paradeDBFields))
	for _, f := range paradeDBFields {
		parts = append(parts, fmt.Sprintf("sa.%s %s %s%s", f, op, placeholder, suffix))
	}
	return "(\n\t\t" + strings.Join(parts, "\n\t\tOR ") + "\n\t)"
}

// paradeDBClause renders the BM25 condition of a lexSpec, adding its arguments.
func paradeDBClause(a *argList, s lexSpec) string {
	switch {
	case len(s.keywords) > 0:
		per := make([]string, 0, len(s.keywords))
		for _, k := range s.keywords {
			per = append(per, fieldsClause("&&&", a.add(k.Term), k.Weight))
		}
		return "(" + strings.Join(per, "\n\t\tOR ") + ")"
	case s.mlt != "":
		doc, _ := json.Marshal(map[string]string{"search_document": truncateRunes(s.mlt, mltMaxRunes)})
		return "sa.search_document @@@ pdb.more_like_this(" + a.add(string(doc)) + "::text)"
	default:
		return fieldsClause("|||", a.add(s.raw), 1)
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// lexicalParadeDB runs one BM25 query per partition (UNION ALL, each limited) and merges by score.
func lexicalParadeDB(ctx context.Context, db *sql.DB, s lexSpec, f Filters, self *ArtifactRef) ([]lexHit, error) {
	tables, err := partitionsFor(f.ArtifactTypes)
	if err != nil {
		return nil, err
	}
	var a argList
	where := andJoin(append([]string{paradeDBClause(&a, s)}, filterClauses(&a, f, self, false)...))
	branches := make([]string, 0, len(tables))
	for _, t := range tables {
		branches = append(branches, fmt.Sprintf(
			"(SELECT sa.artifact_type, sa.artifact_id, pdb.score(sa.artifact_id)::float8 AS score FROM %s sa WHERE %s ORDER BY pdb.score(sa.artifact_id) DESC LIMIT %d)",
			t, where, candidateLimit))
	}
	return scanLexical(ctx, db, strings.Join(branches, "\nUNION ALL\n"), a.vals)
}

func pgVector(dict string) string {
	return fmt.Sprintf("COALESCE(sa.search_vector, to_tsvector('%s', COALESCE(sa.search_document, '')))", dict)
}

// lexicalPostgres ranks with full text: keywords are OR-ed, each keyword's ts_rank_cd times its weight.
func lexicalPostgres(ctx context.Context, db *sql.DB, dict string, s lexSpec, f Filters, self *ArtifactRef) ([]lexHit, error) {
	var a argList
	vec := pgVector(dict)
	var match, score string
	switch {
	case len(s.keywords) > 0:
		var queries, scores []string
		for _, k := range s.keywords {
			q := fmt.Sprintf("plainto_tsquery('%s', %s)", dict, a.add(k.Term))
			queries = append(queries, q)
			scores = append(scores, fmt.Sprintf("%s * ts_rank_cd(%s, %s)", strconv.FormatFloat(k.Weight, 'f', -1, 64), vec, q))
		}
		match = vec + " @@ (" + strings.Join(queries, " || ") + ")"
		score = strings.Join(scores, " + ")
	case s.raw != "":
		q := fmt.Sprintf("plainto_tsquery('%s', %s)", dict, a.add(s.raw))
		match, score = vec+" @@ "+q, fmt.Sprintf("ts_rank_cd(%s, %s)", vec, q)
	default:
		return nil, fmt.Errorf("searchengine: the Postgres keyword list needs keywords or a query text")
	}
	where := andJoin(append([]string{match}, filterClauses(&a, f, self, true)...))
	q := fmt.Sprintf("SELECT sa.artifact_type, sa.artifact_id, (%s)::float8 AS score FROM kb.search_artifacts sa WHERE %s ORDER BY score DESC, sa.artifact_id ASC LIMIT %d",
		score, where, candidateLimit)
	return scanLexical(ctx, db, q, a.vals)
}

func scanLexical(ctx context.Context, db *sql.DB, query string, args []any) ([]lexHit, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []lexHit
	for rows.Next() {
		var h lexHit
		if err := rows.Scan(&h.typ, &h.id, &h.score); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].id < hits[j].id
	})
	if len(hits) > candidateLimit {
		hits = hits[:candidateLimit]
	}
	return hits, nil
}

// keywordMatches reports, per keyword, which of the candidates match it: every token of the keyword
// occurs in one indexed text field (the same rule as the keyword list).
func keywordMatches(ctx context.Context, db *sql.DB, backend Backend, dict string, terms []string, cands []candKey) ([]map[candKey]bool, error) {
	out := make([]map[candKey]bool, len(terms))
	byType := map[string][]string{}
	for _, c := range cands {
		byType[c.typ] = append(byType[c.typ], c.id)
	}
	for i, term := range terms {
		out[i] = map[candKey]bool{}
		for typ, ids := range byType {
			var a argList
			idsArg := a.add(pqStrings(ids))
			termArg := a.add(term)
			var q string
			if backend == BackendParadeDB {
				if _, err := partitionsFor([]string{typ}); err != nil {
					return nil, err
				}
				q = fmt.Sprintf("SELECT sa.artifact_id FROM kb.search_artifacts_%s sa WHERE sa.artifact_id = ANY(%s) AND %s", typ, idsArg, fieldsClause("&&&", termArg, 1))
			} else {
				q = fmt.Sprintf("SELECT sa.artifact_id FROM kb.search_artifacts sa WHERE sa.artifact_type = '%s' AND sa.artifact_id = ANY(%s) AND %s @@ plainto_tsquery('%s', %s)",
					strings.ReplaceAll(typ, "'", ""), idsArg, pgVector(dict), dict, termArg)
			}
			rows, err := db.QueryContext(ctx, q, a.vals...)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return nil, err
				}
				out[i][candKey{typ, id}] = true
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
