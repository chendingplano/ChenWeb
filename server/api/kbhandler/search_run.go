package kbhandler

import (
	"context"
	"database/sql"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/llmrun"
)

// searchCall identifies one search in public.llm_usage_event: its query embedding and keyword
// calls carry the search's run ID (from shared.id_mgr llm_run_id) and user. It travels in the context
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

// searchCallFrom returns the search's call identity. A search started by a pipeline (doc
// processing or doc review) sets no searchCall, so its user comes from the LLM user already on
// the context; without this fallback its keyword and embedding calls were recorded with no user_id.
func searchCallFrom(ctx context.Context) searchCall {
	call, _ := ctx.Value(searchCallKey{}).(searchCall)
	if call.userID == "" {
		call.userID = docprocessing.LLMUserIDFromContext(ctx)
	}
	if call.runID == 0 {
		call.runID = docprocessing.LLMRunIDFromContext(ctx)
	}
	return call
}

// newSearchCall allocates the run ID for one search. A failure only costs the usage-log
// correlation, so it is logged and the search goes on with run ID 0.
func newSearchCall(ctx context.Context, db *sql.DB, userID string) searchCall {
	call := searchCall{userID: userID}
	call.runID = llmrun.Next(ctx)
	return call
}
