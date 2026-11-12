package docreviews

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func activityPayload(activity, text string) map[string]any {
	return map[string]any{"activities": []any{map[string]any{
		"activity": activity, "source_text": text, "line_start": 3.0, "line_end": 4.0,
		"search_queries": []any{"audit logging", "log retention"},
	}}}
}

func newTestComplianceReviewer(fake *fakeJSONExtractor, s complianceSearchFunc) *complianceReviewer {
	return &complianceReviewer{client: fake, logger: loggerutil.CreateDefaultLogger("TEST_COMPLIANCE"), search: s}
}

var complianceTestCfg = ReviewerConfig{ModelName: "m", PromptText: "t", PromptRef: "prompt-review-compliance-v1.md"}

func TestComplianceChunkNoGroundingIsNormalResult(t *testing.T) {
	fake := &fakeJSONExtractor{out: activityPayload("record_user_activity", "Log user activities in a database table")}
	var gotReq searchengine.Request
	r := newTestComplianceReviewer(fake, func(_ context.Context, req searchengine.Request) (*searchengine.Response, error) {
		gotReq = req
		return &searchengine.Response{}, nil
	})
	fs, err := r.processChunk(context.Background(), 7, 2, complianceTestCfg, chunkInput{inputJSON: "{}", startLine: 1, endLine: 9})
	if err != nil || len(fs) != 1 {
		t.Fatalf("err=%v findings=%d", err, len(fs))
	}
	if fs[0].FindingType != "compliance_no_grounding" || fs[0].Severity != "info" || fs[0].Aspect != "compliance" {
		t.Errorf("unexpected finding %+v", fs[0])
	}
	if len(gotReq.Filters.ExcludeRecordIDs) != 1 || gotReq.Filters.ExcludeRecordIDs[0] != 7 {
		t.Errorf("reviewed record not excluded: %+v", gotReq.Filters)
	}
	if gotReq.Criteria.MinCosine != 0 {
		t.Errorf("MinCosine must stay unset")
	}
	if got := strings.Join(gotReq.Filters.ArtifactTypes, ","); got != "provision,chunk" {
		t.Errorf("artifact types=%q", got)
	}
}

func TestComplianceChunkReportsCandidatesDeduplicated(t *testing.T) {
	fake := &fakeJSONExtractor{out: activityPayload("record_user_activity", "Log user activities")}
	calls := 0
	r := newTestComplianceReviewer(fake, func(context.Context, searchengine.Request) (*searchengine.Response, error) {
		calls++
		return &searchengine.Response{Results: []searchengine.Result{
			{ArtifactType: "provision", ArtifactID: "p1", InputRecordID: 9, SourceTitle: "GB/T 1", Snippet: "logs shall be kept"},
		}}, nil
	})
	fs, err := r.processChunk(context.Background(), 7, 2, complianceTestCfg, chunkInput{inputJSON: "{}"})
	if err != nil || len(fs) != 1 {
		t.Fatalf("err=%v findings=%d", err, len(fs))
	}
	if calls != 2 {
		t.Errorf("searches=%d want 2 (one per query)", calls)
	}
	if fs[0].FindingType != "compliance_grounding_candidates" || strings.Count(fs[0].Description, "[provision p1]") != 1 {
		t.Errorf("unexpected finding %+v", fs[0])
	}
}

func TestComplianceSearchErrorAbortsChunk(t *testing.T) {
	boom := errors.New("search down")
	fake := &fakeJSONExtractor{out: activityPayload("a", "text")}
	r := newTestComplianceReviewer(fake, func(context.Context, searchengine.Request) (*searchengine.Response, error) {
		return nil, boom
	})
	fs, err := r.processChunk(context.Background(), 1, 1, complianceTestCfg, chunkInput{})
	if !errors.Is(err, boom) || fs != nil {
		t.Fatalf("err=%v findings=%v; want original error, no findings", err, fs)
	}
}

func TestComplianceModelErrorAborts(t *testing.T) {
	boom := errors.New("llm down")
	r := newTestComplianceReviewer(&fakeJSONExtractor{err: boom}, nil)
	if _, err := r.processChunk(context.Background(), 1, 1, complianceTestCfg, chunkInput{}); !errors.Is(err, boom) {
		t.Fatalf("err=%v want %v", err, boom)
	}
}

func TestComplianceMalformedActivitiesRejected(t *testing.T) {
	for _, p := range []map[string]any{
		{},
		{"activities": "x"},
		{"activities": []any{map[string]any{"activity": "a"}}},
	} {
		if _, err := parseComplianceActivities(p, false, chunkInput{}); err == nil {
			t.Errorf("payload %v accepted", p)
		}
	}
}

func TestComplianceEmptyActivitiesSkipsSearch(t *testing.T) {
	r := newTestComplianceReviewer(&fakeJSONExtractor{out: map[string]any{"activities": []any{}}},
		func(context.Context, searchengine.Request) (*searchengine.Response, error) {
			t.Fatal("search must not run without activities")
			return nil, nil
		})
	fs, err := r.processChunk(context.Background(), 1, 3, complianceTestCfg, chunkInput{})
	if err != nil || len(fs) != 0 {
		t.Fatalf("err=%v findings=%d", err, len(fs))
	}
}

func TestComplianceHasNoDocumentFallback(t *testing.T) {
	r := newTestComplianceReviewer(&fakeJSONExtractor{}, nil)
	if _, err := r.ReviewDocument(context.Background(), 1, complianceTestCfg); err == nil {
		t.Fatal("ReviewDocument must fail: per chunk only")
	}
	var _ chunkReviewer = r
}

func TestComplianceFirstChunkAddsDocumentLevelActivity(t *testing.T) {
	fake := &fakeJSONExtractor{seq: []map[string]any{
		{"activities": []any{}},
		activityPayload("document_level", "Automatic blood-pressure monitor"),
	}}
	r := newTestComplianceReviewer(fake, func(context.Context, searchengine.Request) (*searchengine.Response, error) {
		return &searchengine.Response{}, nil
	})
	r.loadPrompt = func(ref string) (string, string, error) { return "doc prompt", ref, nil }
	fs, err := r.processChunk(context.Background(), 1, 0, complianceTestCfg, chunkInput{inputJSON: "{}"})
	if err != nil || len(fs) != 1 {
		t.Fatalf("err=%v findings=%d", err, len(fs))
	}
	if fake.calledCount != 2 || fake.promptNames[1] != complianceDocActivityPromptRef {
		t.Errorf("calls=%d prompts=%v", fake.calledCount, fake.promptNames)
	}
}
