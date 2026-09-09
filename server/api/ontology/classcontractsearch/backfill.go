package classcontractsearch

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
)

// classTermPredicate selects the governed class terms this index covers: a term
// that has a current class-contract revision, OR is referenced as a metric
// assertion's resolved class (instance_of_term_id). Deliberately NOT keyed on
// term_kind — synthesized metric classes carry term_kind 'metric_definition',
// not 'class' (the 'class' kind is the ~20 abstract framework classes, which
// have no metric instances).
const classTermPredicate = `(
    h.current_contract_revision_id IS NOT NULL
    OR h.term_id IN (
        SELECT DISTINCT instance_of_term_id FROM kb.semantic_assertions
        WHERE COALESCE(instance_of_term_id, '') <> ''
    )
)`

// BackfillClassContractSearch (re)indexes governed class-term rows in
// kb.ontology_class_contract_search (see classTermPredicate for what counts as a
// class term).
//
//   - Default (reembedAll=false): only class terms that have NO row yet. Each
//     batch creates rows, which then stop matching, so calling repeatedly with a
//     bounded limit drains the corpus (Remaining reaches 0).
//   - reembedAll=true: every class term is reindexed (document + embedding
//     recomputed). This does not page — call it once with limit >= the number of
//     class terms. Remaining after that pass is just the count that errored.
//
// limit bounds rows processed per call (<=0 = no bound). The
// kbsearch.BackfillResult fields carry: Scanned = class terms attempted,
// Embedded = rows reindexed OK (whether or not a vector was produced),
// Failed = reindex errors, Skipped = unused here.
func BackfillClassContractSearch(ctx context.Context, db *sql.DB, embed EmbedFunc, limit int, reembedAll bool) (kbsearch.BackfillResult, error) {
	var res kbsearch.BackfillResult
	if db == nil {
		return res, fmt.Errorf("db is nil")
	}
	store := Store{DB: db}

	where := classTermPredicate
	if !reembedAll {
		where += " AND occs.class_term_id IS NULL"
	}

	var total int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM kb.ontology_term_headers h
LEFT JOIN kb.ontology_class_contract_search occs ON occs.class_term_id = h.term_id
WHERE `+where).Scan(&total); err != nil {
		return res, fmt.Errorf("count class terms: %w", err)
	}

	selectSQL := `
SELECT h.term_id
FROM kb.ontology_term_headers h
LEFT JOIN kb.ontology_class_contract_search occs ON occs.class_term_id = h.term_id
WHERE ` + where + `
ORDER BY h.term_id`
	if limit > 0 {
		selectSQL += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.QueryContext(ctx, selectSQL)
	if err != nil {
		return res, fmt.Errorf("select class terms: %w", err)
	}
	ids := make([]string, 0, 256)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return res, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return res, err
	}
	rows.Close()

	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		res.Scanned++
		if err := store.Reindex(ctx, id, embed); err != nil {
			res.Failed++
			continue
		}
		res.Embedded++
	}

	res.Remaining = total - res.Embedded
	if res.Remaining < 0 {
		res.Remaining = 0
	}
	return res, nil
}
