package kbhandler

import (
	"context"
	"testing"
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
