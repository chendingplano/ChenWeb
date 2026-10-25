package kbhandler

import (
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/kbsearch"
)

var hybridResultColumns = []string{
	"artifact_type", "artifact_id", "input_record_id", "primary_label", "secondary_label", "source_title", "source_filename", "source_line_spans", "semantic_payload", "keywords", "score", "snippet",
}

var traceColumns = []string{"stage", "artifact_type", "artifact_id", "stage_rank", "score", "partition_rank", "lexical_rank", "semantic_rank"}

func hybridResultRows(ids ...string) *sqlmock.Rows {
	rows := sqlmock.NewRows(hybridResultColumns)
	for _, id := range ids {
		rows.AddRow("metric", id, int64(9), id, "", "", "", `[]`, `{}`, "{}", 0.03, id)
	}
	return rows
}

// The trace of a target 2nd in BM25 (1st in its partition) and 1st in semantic search.
func traceRows(fusedFirst string) *sqlmock.Rows {
	return sqlmock.NewRows(traceColumns).
		AddRow("lexical_partition", "metric", "9_mtc_2", int64(1), 12.5, nil, nil, nil).
		AddRow("lexical", "metric", "9_mtc_2", int64(2), 12.5, int64(1), nil, nil).
		AddRow("semantic", "metric", "9_mtc_1", int64(1), 0.91, nil, nil, nil).
		AddRow("fused", "metric", fusedFirst, int64(1), 0.032, nil, int64(2), int64(1))
}

func TestHybridSearchParadeDBTraceRunsInSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	vec := kbsearch.FormatVectorLiteral([]float64{0.25, 0.75})
	mock.ExpectBegin()
	mock.ExpectExec("SET LOCAL hnsw\\.iterative_scan = relaxed_order").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("WITH lexical_partitions AS .*LIMIT \\$4 OFFSET \\$5").
		WithArgs("battery", vec, "metric", 20, 0).
		WillReturnRows(hybridResultRows("9_mtc_1"))
	// Same CTEs, trace SELECT, no page size or offset.
	mock.ExpectQuery("WITH lexical_partitions AS .*lexical_partition_ranks AS .*PARTITION BY lp\\.artifact_type.*'semantic'.*'fused'").
		WithArgs("battery", vec, "metric").
		WillReturnRows(traceRows("9_mtc_1"))
	mock.ExpectCommit()

	sink := &searchTraceSink{}
	cfg := registrySearchConfig{lexicalWeight: 1, semanticWeight: 1, trace: sink}
	results, err := queryHybridSearchResultsParadeDB(db, "metric", "battery", []float64{0.25, 0.75}, artifactSearchFilters{}, 1, 20, cfg)
	if err != nil || len(results) != 1 {
		t.Fatalf("results=%#v err=%v", results, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
	if len(sink.entries) != 1 {
		t.Fatalf("entries=%#v", sink.entries)
	}
	e := sink.entries[0]
	if e.Query != "battery" || e.Error != "" || len(e.Keywords) != 0 || len(e.Candidates) != 4 {
		t.Fatalf("entry=%#v", e)
	}
	lexical := e.Candidates[1]
	if lexical.Stage != searchTraceStageLexical || lexical.PartitionName != "kb.search_artifacts_metric" || lexical.Rank != 2 ||
		lexical.PartitionRank == nil || *lexical.PartitionRank != 1 {
		t.Fatalf("lexical=%#v", lexical)
	}
	fused := e.Candidates[3]
	if fused.LexicalRank == nil || *fused.LexicalRank != 2 || fused.SemanticRank == nil || *fused.SemanticRank != 1 || fused.PartitionRank != nil {
		t.Fatalf("fused=%#v", fused)
	}
}

func TestHybridSearchPostgresTraceHasNoPartitionStage(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	vec := kbsearch.FormatVectorLiteral([]float64{0.25, 0.75})
	mock.ExpectBegin()
	mock.ExpectExec("SET LOCAL").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("WITH query_input AS .*LIMIT \\$4 OFFSET \\$5").WithArgs("battery", vec, "metric", 20, 0).
		WillReturnRows(hybridResultRows("9_mtc_1"))
	mock.ExpectQuery("WITH query_input AS .*SELECT 'lexical' AS stage.*'fused'").WithArgs("battery", vec, "metric").
		WillReturnRows(sqlmock.NewRows(traceColumns).AddRow("fused", "metric", "9_mtc_1", int64(1), 0.016, nil, nil, int64(1)))
	mock.ExpectCommit()

	sink := &searchTraceSink{}
	cfg := registrySearchConfig{dictionary: "simple", previewMaxWords: 30, lexicalWeight: 1, semanticWeight: 1, trace: sink}
	if _, err := queryHybridSearchResults(db, "metric", "battery", []float64{0.25, 0.75}, artifactSearchFilters{}, 1, 20, cfg); err != nil {
		t.Fatalf("queryHybridSearchResults returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
	if got := hybridTraceSelect(false); strings.Contains(got, "lexical_partition") {
		t.Fatalf("tsvector trace must not have per-partition rows: %s", got)
	}
	if len(sink.entries) != 1 || sink.entries[0].Error != "" {
		t.Fatalf("entries=%#v", sink.entries)
	}
}

func TestHybridSearchTraceMismatchAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		trace  func(*sqlmock.ExpectedQuery)
		commit bool
		want   string
	}{
		{"mismatch", func(q *sqlmock.ExpectedQuery) { q.WillReturnRows(traceRows("9_mtc_9")) }, true, searchTraceErrMismatch},
		{"failure", func(q *sqlmock.ExpectedQuery) { q.WillReturnError(errors.New("boom")) }, false, "trace failed: boom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			defer db.Close()
			mock.ExpectBegin()
			mock.ExpectExec("SET LOCAL").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("WITH lexical_partitions AS .*LIMIT \\$4 OFFSET \\$5").WillReturnRows(hybridResultRows("9_mtc_1"))
			tc.trace(mock.ExpectQuery("lexical_partition_ranks AS"))
			if tc.commit {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}

			sink := &searchTraceSink{}
			cfg := registrySearchConfig{lexicalWeight: 1, semanticWeight: 1, trace: sink}
			results, err := queryHybridSearchResultsParadeDB(db, "metric", "battery", []float64{0.25, 0.75}, artifactSearchFilters{}, 1, 20, cfg)
			// A trace problem never fails the search.
			if err != nil || len(results) != 1 {
				t.Fatalf("results=%#v err=%v", results, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet db expectations: %v", err)
			}
			if len(sink.entries) != 1 || sink.entries[0].Error != tc.want {
				t.Fatalf("entries=%#v", sink.entries)
			}
		})
	}
}

func TestTraceMatchesResultsUsesOffset(t *testing.T) {
	candidates := []searchTraceCandidate{
		{Stage: searchTraceStageFused, ArtifactType: "metric", ArtifactID: "a", Rank: 1},
		{Stage: searchTraceStageFused, ArtifactType: "metric", ArtifactID: "b", Rank: 2},
		{Stage: searchTraceStageLexical, ArtifactType: "metric", ArtifactID: "b", Rank: 1},
	}
	second := []artifactSearchResult{{ArtifactType: "metric", ArtifactID: "b"}}
	if !traceMatchesResults(candidates, second, 1) || traceMatchesResults(candidates, second, 0) {
		t.Fatal("offset not applied")
	}
}
