package kbhandler

import (
	"context"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

func TestSearchCallContext(t *testing.T) {
	if got := searchCallFrom(context.Background()); got != (searchCall{}) {
		t.Fatalf("empty context call = %+v", got)
	}
	ctx := withSearchCall(context.Background(), searchCall{runID: 9, userID: "u"})
	if got := searchCallFrom(ctx); got.runID != 9 || got.userID != "u" {
		t.Fatalf("call = %+v", got)
	}
}

func TestSearchQueryKeywordsCarriesSearchRun(t *testing.T) {
	stub := &stubKeywordExtractor{payload: keywordPayload("cuff pressure", "袖带压力")}
	useStubKeywordExtractor(t, stub)
	ctx := withSearchCall(context.Background(), searchCall{runID: 17, userID: "u2"})
	if kws := searchQueryKeywords(ctx, "cuff pressure"); len(kws) == 0 {
		t.Fatal("no keywords")
	}
	if stub.last.RunID != 17 || stub.last.UserID != "u2" {
		t.Fatalf("keyword call run/user = %d/%q, want 17/u2", stub.last.RunID, stub.last.UserID)
	}
}

func TestSearchQueryKeywordsRunOverrideAndFresh(t *testing.T) {
	stub := &stubKeywordExtractor{payload: keywordPayload("cuff pressure", "袖带压力")}
	useStubKeywordExtractor(t, stub)
	t.Setenv(envSearchKeywords, "off")
	on, off := true, false
	if kws := searchQueryKeywords(withSearchCall(context.Background(), searchCall{keywords: &on}), "cuff pressure"); len(kws) == 0 || stub.calls != 1 {
		t.Fatalf("override on: keywords=%v calls=%d", kws, stub.calls)
	}
	if kws := searchQueryKeywords(withSearchCall(context.Background(), searchCall{keywords: &on}), "cuff pressure"); len(kws) == 0 || stub.calls != 1 {
		t.Fatalf("cached: keywords=%v calls=%d, want a cache hit", kws, stub.calls)
	}
	if kws := searchQueryKeywords(withSearchCall(context.Background(), searchCall{keywords: &on, freshKeywords: true}), "cuff pressure"); len(kws) == 0 || stub.calls != 2 {
		t.Fatalf("fresh: keywords=%v calls=%d, want a new call", kws, stub.calls)
	}
	t.Setenv(envSearchKeywords, "on")
	if kws := searchQueryKeywords(withSearchCall(context.Background(), searchCall{keywords: &off}), "cuff pressure"); kws != nil {
		t.Fatalf("override off: keywords=%v", kws)
	}
}

// A search started by a pipeline has no searchCall on its context; its keyword and embedding
// calls must still carry the pipeline's user, not an empty user_id.
func TestSearchCallFromFallsBackToPipelineUser(t *testing.T) {
	ctx := docprocessing.WithLLMUserID(context.Background(), "user-pipeline")
	if got := searchCallFrom(ctx).userID; got != "user-pipeline" {
		t.Fatalf("pipeline search userID = %q, want %q", got, "user-pipeline")
	}
}

// An explicit searchCall user (request handlers) wins over the pipeline user.
func TestSearchCallFromPrefersExplicitUser(t *testing.T) {
	ctx := withSearchCall(docprocessing.WithLLMUserID(context.Background(), "user-pipeline"), searchCall{userID: "user-request"})
	if got := searchCallFrom(ctx).userID; got != "user-request" {
		t.Fatalf("request search userID = %q, want %q", got, "user-request")
	}
}

// A search started by a pipeline (doc review) sets no searchCall; its keyword and embedding calls
// must carry the pipeline's run ID instead of run 0.
func TestSearchCallFromInheritsPipelineRunID(t *testing.T) {
	ctx := docprocessing.WithLLMRunID(context.Background(), 4774)
	if got := searchCallFrom(ctx); got.runID != 4774 {
		t.Fatalf("runID = %d, want 4774", got.runID)
	}
	ctx = withSearchCall(ctx, searchCall{runID: 42})
	if got := searchCallFrom(ctx); got.runID != 42 {
		t.Fatalf("explicit runID = %d, want 42", got.runID)
	}
}
