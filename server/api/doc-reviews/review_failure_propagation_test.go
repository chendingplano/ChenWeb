package docreviews

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/searchengine"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestComplianceModelFailureReachesScheduler(t *testing.T) {
	failure := errors.New("model unavailable")
	logger := loggerutil.CreateDefaultLogger("20261111-472")
	for _, input := range []string{"per-chunk", "per-block"} {
		t.Run(input, func(t *testing.T) {
			reviewer := &legalComplianceReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger}
			tasks, unsupported := buildPromptCacheReviewTasks(19, DocMetadataInputRecord{ID: 19}, []Line{{LineNo: 1, PageNo: 1, Content: "Log activities"}}, []reviewRunner{{reviewer: reviewer, cfg: ReviewerConfig{Input: input}}}, nil)
			if len(unsupported) != 0 || len(tasks) != 1 {
				t.Fatalf("unexpected dispatch: tasks=%d unsupported=%d", len(tasks), len(unsupported))
			}
			findings, errs := runReviewTasksForPromptCache(context.Background(), 1, tasks)
			if len(findings) != 0 || len(errs) != 1 || !errors.Is(errs[0], failure) {
				t.Fatalf("findings=%v errors=%v; want model failure", findings, errs)
			}
		})
	}
}

func TestArtifactFailureCancelsSiblingAndSkipsQueuedUnits(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "60")
	failure := errors.New("source/model unavailable")
	siblingStarted := make(chan struct{})
	siblingCanceled := make(chan struct{})
	units := []artifactReviewUnit{
		{chunkIdx: 0, run: func(ctx context.Context) ([]ReviewFinding, error) {
			close(siblingStarted)
			<-ctx.Done()
			close(siblingCanceled)
			return nil, ctx.Err()
		}},
		{chunkIdx: 1, run: func(ctx context.Context) ([]ReviewFinding, error) { <-siblingStarted; return nil, failure }},
		{chunkIdx: 0, run: func(context.Context) ([]ReviewFinding, error) {
			t.Error("queued unit started after reviewer failure")
			return nil, nil
		}},
	}
	findings, err := runArtifactUnitsChunkGrouped(context.Background(), 2, units, ReviewerConfig{MaxFindings: 99, MaxAnalyses: 99}, "provisions", loggerutil.CreateDefaultLogger("20261111-794"), 19, nil)
	if !errors.Is(err, failure) || len(findings) != 0 {
		t.Fatalf("findings=%v err=%v; want original error with no published partial results", findings, err)
	}
	select {
	case <-siblingCanceled:
	default:
		t.Fatal("in-flight sibling was not canceled")
	}
}

func TestMalformedOneShotPayloadFailsReviewer(t *testing.T) {
	for _, payload := range []map[string]any{nil, {}, {"findings": nil}, {"findings": "none"}, {"findings": []any{nil}}, {"findings": []any{map[string]any{"description": "missing title"}}}} {
		reviewer := &legalComplianceReviewer{client: &fakeJSONExtractor{out: payload}, logger: loggerutil.CreateDefaultLogger("20261111-369")}
		tasks, _ := buildPromptCacheReviewTasks(19, DocMetadataInputRecord{ID: 19}, []Line{{LineNo: 1, PageNo: 1, Content: "Test"}}, []reviewRunner{{reviewer: reviewer, cfg: ReviewerConfig{Input: "per-chunk"}}}, nil)
		findings, errs := runReviewTasksForPromptCache(context.Background(), 1, tasks)
		if len(errs) != 1 || len(findings) != 0 {
			t.Errorf("payload=%v findings=%v errors=%v; want failed reviewer", payload, findings, errs)
		}
	}
}

func TestReviewSearchMissingIndexIsAnError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT COALESCE\\(search_document").WithArgs("provision", "19_p_1").WillReturnRows(sqlmock.NewRows([]string{"search_document", "keywords", "embedding"}))
	hits, err := findSimilarMatches(context.Background(), db, "provision", "19_p_1", 19, 10)
	if !errors.Is(err, searchengine.ErrReferenceNotIndexed) || len(hits) != 0 {
		t.Fatalf("hits=%v error=%v; missing index is not a completed empty search", hits, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReviewerConcurrentFailureCancelsPendingWork(t *testing.T) {
	failure := errors.New("unit failed")
	calls := 0
	_, err := runReviewerConcurrent(context.Background(), 1, 3, ReviewerConfig{MaxFindings: 99, MaxAnalyses: 99}, "legal_compliance", loggerutil.CreateDefaultLogger("20261111-263"), 19, nil, func(context.Context, int) ([]ReviewFinding, error) { calls++; return nil, failure })
	if !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("calls=%d error=%v; want one attempted unit and original error", calls, err)
	}
}

func TestProvisionAnalysisPersistenceFailureAbortsUnit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	failure := errors.New("analysis storage unavailable")
	mock.ExpectExec("INSERT INTO kb.doc_review_provision_analyses").WillReturnError(failure)
	reviewer := &provisionsReviewer{db: db, logger: loggerutil.CreateDefaultLogger("20261111-417")}
	findings, err := reviewer.reviewProvision(withLLMRunID(context.Background(), 21), 19, 0, ReviewerConfig{}, docProvision{view: provisionView{ProvID: "19_p_1"}}, nil, "", false)
	if !errors.Is(err, failure) || len(findings) != 0 {
		t.Fatalf("findings=%v error=%v; want failed unit", findings, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type failingReviewerLogStore struct{ err error }

func (s failingReviewerLogStore) SaveLogs(context.Context, []ReviewLogEntry) (int64, error) {
	return 0, s.err
}
func (s failingReviewerLogStore) DeleteLogs(context.Context, int64) (int64, error) { return 0, nil }

func TestMetricAuditPersistenceFailureAbortsUnit(t *testing.T) {
	failure := errors.New("audit storage unavailable")
	reviewer := &metricsReviewer{client: &fakeJSONExtractor{out: map[string]any{"findings": []any{}}}, logger: loggerutil.CreateDefaultLogger("20261111-435"), logStore: failingReviewerLogStore{err: failure}, runID: 21}
	findings, err := reviewer.reviewMetric(context.Background(), 19, 0, ReviewerConfig{}, docMetric{}, nil, "", false)
	if !errors.Is(err, failure) || len(findings) != 0 {
		t.Fatalf("findings=%v error=%v; want failed unit", findings, err)
	}
}

func TestEveryChunkAndBlockReviewerPropagatesModelErrors(t *testing.T) {
	failure := errors.New("model unavailable")
	logger := loggerutil.CreateDefaultLogger("20261111-738")
	reviewers := []Reviewer{
		&assumptionsReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&clarityReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&completenessReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&concisenessReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&correctnessReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&crossReferenceCorrectnessReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&currencyReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&diagramsReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&errorHandlingReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&evidenceRationaleReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&examplesReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&formattingConsistencyReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&grammarSpellingReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&headingHierarchyReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&internalContradictionsReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&internalPolicyReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&legalComplianceReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&limitationsReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&localizationReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&logicalFlowReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&modularityReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&navigabilityReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&performanceReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&prerequisitesReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&readabilityReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&regulatoryComplianceReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&relevanceReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&requirementTraceabilityReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&sectionBalanceReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&securityReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&standardsComplianceReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&technicalAccuracyReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&terminologyConsistencyReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&testableClaimsReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
		&toneVoiceReviewer{client: &fakeJSONExtractor{err: failure}, logger: logger},
	}
	for _, reviewer := range reviewers {
		t.Run(reviewer.Name(), func(t *testing.T) {
			if cr, ok := reviewer.(chunkReviewer); ok {
				findings, err := cr.processChunk(context.Background(), 19, 0, ReviewerConfig{}, chunkInput{})
				if !errors.Is(err, failure) || len(findings) != 0 {
					t.Errorf("chunk: findings=%v error=%v", findings, err)
				}
			}
			if br, ok := reviewer.(blockReviewer); ok {
				findings, err := br.processBlock(context.Background(), 19, 0, 1, ReviewerConfig{}, pageBlock{})
				if !errors.Is(err, failure) || len(findings) != 0 {
					t.Errorf("block: findings=%v error=%v", findings, err)
				}
			}
		})
	}
}

func TestReviewerCoordinatorsRejectCanceledContextWithoutFalseProgress(t *testing.T) {
	for _, artifact := range []bool{false, true} {
		for _, deadline := range []bool{false, true} {
			ctx, cancel := context.WithCancel(context.Background())
			cause := error(context.Canceled)
			if deadline {
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				cause = context.DeadlineExceeded
			} else {
				cancel()
			}
			progress := 0
			onProgress := func(ReviewerProgress) error { progress++; return nil }
			var err error
			if artifact {
				_, err = runArtifactUnitsChunkGrouped(ctx, 1, []artifactReviewUnit{{run: func(context.Context) ([]ReviewFinding, error) { t.Error("canceled work started"); return nil, nil }}}, ReviewerConfig{MaxFindings: 99, MaxAnalyses: 99}, "provisions", &captureLogger{}, 19, onProgress)
			} else {
				_, err = runReviewerConcurrent(ctx, 1, 1, ReviewerConfig{MaxFindings: 99, MaxAnalyses: 99}, "legal_compliance", &captureLogger{}, 19, onProgress, func(context.Context, int) ([]ReviewFinding, error) { t.Error("canceled work started"); return nil, nil })
			}
			cancel()
			if !errors.Is(err, cause) || progress != 0 {
				t.Errorf("artifact=%v deadline=%v error=%v progress=%d", artifact, deadline, err, progress)
			}
		}
	}
}
