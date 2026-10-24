package kbsearch

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const (
	summaryIdx = "kb_search_artifacts_summary_bm25"
	metricIdx  = "kb_search_artifacts_metric_bm25"
	topicIdx   = "kb_search_artifacts_topic_bm25"
)

func expectBM25Listing(mock sqlmock.Sqlmock, leftovers []string, indexes ...string) {
	lr := sqlmock.NewRows([]string{"relname"})
	for _, l := range leftovers {
		lr.AddRow(l)
	}
	mock.ExpectQuery(regexp.QuoteMeta("NOT i.indisvalid")).WillReturnRows(lr)
	ir := sqlmock.NewRows([]string{"indexname"})
	for _, i := range indexes {
		ir.AddRow(i)
	}
	mock.ExpectQuery(regexp.QuoteMeta("FROM pg_indexes")).WillReturnRows(ir)
}

func expectBM25Counts(mock sqlmock.Sqlmock, index string, mutable, live, deleted int64) {
	mock.ExpectQuery(regexp.QuoteMeta("paradedb.index_info")).
		WithArgs(`kb."` + index + `"`).
		WillReturnRows(sqlmock.NewRows([]string{"m", "l", "d"}).AddRow(mutable, live, deleted))
}

func expectReindex(mock sqlmock.Sqlmock, index string) *sqlmock.ExpectedExec {
	return mock.ExpectExec(regexp.QuoteMeta(`REINDEX INDEX CONCURRENTLY kb."` + index + `"`))
}

func newBM25Mock(t *testing.T) (sqlmock.Sqlmock, func() (BM25MaintenanceResult, error), func(BM25MaintenanceOptions) (BM25MaintenanceResult, error)) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	run := func(o BM25MaintenanceOptions) (BM25MaintenanceResult, error) {
		return MaintainBM25Indexes(context.Background(), db, o, nil)
	}
	return mock, func() (BM25MaintenanceResult, error) { return run(BM25MaintenanceOptions{}) }, run
}

func actions(res BM25MaintenanceResult) map[string]string {
	out := map[string]string{}
	for _, r := range res.Indexes {
		out[r.Index] = r.Action
	}
	return out
}

func TestMaintainBM25IndexesRebuildsOnlyIndexesOverThreshold(t *testing.T) {
	mock, runDefault, _ := newBM25Mock(t)
	expectBM25Listing(mock, nil, metricIdx, summaryIdx, topicIdx)
	expectBM25Counts(mock, metricIdx, 951, 9208, 2411) // mutable over 500
	expectReindex(mock, metricIdx).WillReturnResult(sqlmock.NewResult(0, 0))
	expectBM25Counts(mock, summaryIdx, 789, 789, 789) // all documents mutable
	expectReindex(mock, summaryIdx).WillReturnResult(sqlmock.NewResult(0, 0))
	expectBM25Counts(mock, topicIdx, 361, 3361, 100) // compact enough

	res, err := runDefault()
	if err != nil {
		t.Fatal(err)
	}
	a := actions(res)
	if a[metricIdx] != BM25ActionRebuilt || a[summaryIdx] != BM25ActionRebuilt || a[topicIdx] != BM25ActionSkipped {
		t.Fatalf("actions=%v", a)
	}
	if res.Rebuilt != 2 || res.Failed != 0 {
		t.Fatalf("rebuilt=%d failed=%d", res.Rebuilt, res.Failed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMaintainBM25IndexesDryRunChangesNothing(t *testing.T) {
	mock, _, run := newBM25Mock(t)
	expectBM25Listing(mock, []string{metricIdx + "_ccnew"}, summaryIdx, topicIdx)
	expectBM25Counts(mock, summaryIdx, 789, 789, 789)
	expectBM25Counts(mock, topicIdx, 0, 3361, 0)

	res, err := run(BM25MaintenanceOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if a := actions(res); a[summaryIdx] != BM25ActionWouldRebuild || a[topicIdx] != BM25ActionSkipped {
		t.Fatalf("actions=%v", a)
	}
	if len(res.DroppedLeftovers) != 0 || res.Rebuilt != 0 || !res.DryRun {
		t.Fatalf("dry run changed something: %+v", res)
	}
	if err := mock.ExpectationsWereMet(); err != nil { // no DROP or REINDEX was expected
		t.Fatal(err)
	}
}

func TestMaintainBM25IndexesContinuesAfterFailureAndHonoursLimit(t *testing.T) {
	mock, _, run := newBM25Mock(t)
	expectBM25Listing(mock, nil, metricIdx, summaryIdx, topicIdx)
	expectBM25Counts(mock, metricIdx, 951, 9208, 0)
	expectReindex(mock, metricIdx).WillReturnError(errors.New("could not obtain lock"))
	expectBM25Counts(mock, summaryIdx, 789, 789, 0)
	expectReindex(mock, summaryIdx).WillReturnResult(sqlmock.NewResult(0, 0))
	expectBM25Counts(mock, topicIdx, 600, 3361, 0) // needs a rebuild, but the limit of 2 is used up

	res, err := run(BM25MaintenanceOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	a := actions(res)
	if a[metricIdx] != BM25ActionFailed || a[summaryIdx] != BM25ActionRebuilt || a[topicIdx] != BM25ActionOverLimit {
		t.Fatalf("actions=%v", a)
	}
	if res.Failed != 1 || res.Rebuilt != 1 || res.Indexes[0].Error == "" {
		t.Fatalf("result=%+v", res)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMaintainBM25IndexesDropsLeftoversFirst(t *testing.T) {
	mock, runDefault, _ := newBM25Mock(t)
	leftover := metricIdx + "_ccnew"
	mock.ExpectQuery(regexp.QuoteMeta("NOT i.indisvalid")).WillReturnRows(sqlmock.NewRows([]string{"relname"}).AddRow(leftover))
	mock.ExpectExec(regexp.QuoteMeta(`DROP INDEX CONCURRENTLY IF EXISTS kb."` + leftover + `"`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("FROM pg_indexes")).WillReturnRows(sqlmock.NewRows([]string{"indexname"}))

	res, err := runDefault()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.DroppedLeftovers) != 1 || res.DroppedLeftovers[0] != leftover {
		t.Fatalf("dropped=%v", res.DroppedLeftovers)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMaintainBM25IndexesFailsWhenIndexesCannotBeListed(t *testing.T) {
	mock, runDefault, _ := newBM25Mock(t)
	mock.ExpectQuery(regexp.QuoteMeta("NOT i.indisvalid")).WillReturnRows(sqlmock.NewRows([]string{"relname"}))
	mock.ExpectQuery(regexp.QuoteMeta("FROM pg_indexes")).WillReturnError(errors.New("connection reset"))
	if _, err := runDefault(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestBM25RebuildReason(t *testing.T) {
	opts := BM25MaintenanceOptions{MaxMutableDocs: DefaultBM25MaxMutableDocs, MaxDeletedRatio: DefaultBM25MaxDeletedRatio}
	cases := []struct {
		rep  BM25IndexReport
		want bool
	}{
		{BM25IndexReport{MutableDocs: 500, LiveDocs: 1000}, false}, // at the threshold, not over it
		{BM25IndexReport{MutableDocs: 501, LiveDocs: 1000}, true},
		{BM25IndexReport{LiveDocs: 100, DeletedDocs: 100}, false}, // ratio exactly 0.5
		{BM25IndexReport{LiveDocs: 100, DeletedDocs: 101}, true},
		{BM25IndexReport{}, false}, // empty index
	}
	for _, c := range cases {
		if got := bm25RebuildReason(c.rep, opts) != ""; got != c.want {
			t.Fatalf("%+v: rebuild=%v want %v", c.rep, got, c.want)
		}
	}
}
