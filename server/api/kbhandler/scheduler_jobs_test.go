package kbhandler

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDefaultSchedulerRegistryIncludesTerminologyRefresh(t *testing.T) {
	reg := DefaultSchedulerRegistry()
	desc, ok := reg["terminology_refresh"]
	if !ok {
		t.Fatal("registry missing terminology_refresh job")
	}
	if desc.Label != "Refresh External Terminology Resources" {
		t.Fatalf("label = %q, want Refresh External Terminology Resources", desc.Label)
	}
	if desc.Run == nil {
		t.Fatal("terminology_refresh must have a Run func")
	}
}

func TestDefaultSchedulerRegistryIncludesBM25Maintenance(t *testing.T) {
	desc, ok := DefaultSchedulerRegistry()["maintain_bm25_indexes"]
	if !ok || desc.Run == nil || desc.Label != "Maintain Search BM25 Indexes" {
		t.Fatalf("maintain_bm25_indexes descriptor = %+v (present=%v)", desc, ok)
	}
}

func TestRunMaintainBM25IndexesJobMapsParams(t *testing.T) {
	_, mock := installPolicyDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("NOT i.indisvalid")).WillReturnRows(sqlmock.NewRows([]string{"relname"}))
	mock.ExpectQuery(regexp.QuoteMeta("FROM pg_indexes")).WillReturnRows(sqlmock.NewRows([]string{"indexname"}).AddRow("kb_search_artifacts_topic_bm25"))
	mock.ExpectQuery(regexp.QuoteMeta("paradedb.index_info")).
		WillReturnRows(sqlmock.NewRows([]string{"m", "l", "d"}).AddRow(int64(361), int64(3361), int64(0)))

	// Schedule params arrive JSON-decoded, so numbers are float64. 361 mutable documents exceed
	// max_mutable_docs=300; dry_run keeps the job from rebuilding (no REINDEX is expected).
	got, err := runMaintainBM25IndexesJob(context.Background(), map[string]any{
		"max_mutable_docs": float64(300), "max_deleted_ratio": 0.9, "dry_run": true,
	}, nil)
	if err != nil {
		t.Fatalf("runMaintainBM25IndexesJob: %v", err)
	}
	indexes, _ := got["indexes"].([]any)
	if len(indexes) != 1 || indexes[0].(map[string]any)["action"] != "would_rebuild" || got["dry_run"] != true {
		t.Fatalf("result = %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunTerminologyRefreshJobRequiresDirectory(t *testing.T) {
	t.Setenv("TERMINOLOGY_DIR", "")
	t.Setenv("DATA_HOME_DIR", "")
	_, err := runTerminologyRefreshJob(context.Background(), map[string]any{"sources": "wikidata"}, nil)
	if err == nil || !strings.Contains(err.Error(), "TERMINOLOGY_DIR") {
		t.Fatalf("err = %v, want missing directory error", err)
	}
}

func TestRunTerminologyRefreshJobReportsPerSourceErrors(t *testing.T) {
	t.Setenv("TERMINOLOGY_DIR", t.TempDir())
	got, err := runTerminologyRefreshJob(context.Background(), map[string]any{"sources": "no-such,iec-60050-845"}, nil)
	if err != nil {
		t.Fatalf("runTerminologyRefreshJob: %v", err)
	}
	sources, ok := got["sources"].([]map[string]any)
	if !ok || len(sources) != 2 {
		t.Fatalf("sources = %#v, want 2 per-source entries", got["sources"])
	}
	if sources[0]["source"] != "no-such" || !strings.Contains(sources[0]["error"].(string), "unknown terminology resource") {
		t.Fatalf("entry 0 = %#v", sources[0])
	}
	if sources[1]["source"] != "iec-60050-845" || !strings.Contains(sources[1]["error"].(string), "requires permission") {
		t.Fatalf("entry 1 = %#v", sources[1])
	}
}
