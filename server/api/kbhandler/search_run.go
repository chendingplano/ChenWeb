package kbhandler

import (
	"context"
	"database/sql"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

// searchCall identifies one search in public.llm_usage_event: its query embedding and keyword
// calls carry the search's run ID (from kb.search_run_id_seq) and user. It travels in the context
// so computeQueryEmbedding and searchQueryKeywords keep their signatures.
type searchCall struct {
	runID  int64
	userID string
	// keywords, when set, overrides SEARCH_QUERY_KEYWORDS for this search (a benchmark run chooses).
	keywords *bool
	// freshKeywords skips the keyword cache lookup, so the LLM call is made (and can be timed).
	freshKeywords bool
}

type searchCallKey struct{}

func withSearchCall(ctx context.Context, call searchCall) context.Context {
	return context.WithValue(ctx, searchCallKey{}, call)
}

func searchCallFrom(ctx context.Context) searchCall {
	call, _ := ctx.Value(searchCallKey{}).(searchCall)
	return call
}

// newSearchCall allocates the run ID for one search. A failure only costs the usage-log
// correlation, so it is logged and the search goes on with run ID 0.
func newSearchCall(ctx context.Context, db *sql.DB, userID string) searchCall {
	call := searchCall{userID: userID}
	if err := db.QueryRowContext(ctx, `SELECT nextval('kb.search_run_id_seq')`).Scan(&call.runID); err != nil {
		loggerutil.CreateDefaultLogger("20261024-746").Warn("allocate search run id failed", "error", err)
	}
	return call
}
