package kbsearch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestChunkEmbeddingWindowsCoverUnicodeTail(t *testing.T) {
	text := strings.Repeat("中", 6100) + "needle-at-the-end"
	windows := ChunkEmbeddingTexts(text)
	if len(windows) != 2 || !strings.HasSuffix(windows[1], "needle-at-the-end") {
		t.Fatalf("windows do not cover tail: %d", len(windows))
	}
	if len([]rune(windows[0])) != 6000 || windows[0][len(windows[0])-900:] != windows[1][:900] {
		t.Fatal("expected 300-rune overlap")
	}
	for _, w := range windows {
		if len([]rune(w)) > 6000 {
			t.Fatal("oversized embedding input")
		}
	}
	if got := ChunkEmbeddingTexts(" "); len(got) != 0 {
		t.Fatalf("blank windows: %v", got)
	}
}

func TestChunkSourceLockRejectsSingleConnectionPool(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err := LockChunkSearchSource(context.Background(), db, 42)
	if err == nil || !strings.Contains(err.Error(), "at least two") {
		t.Fatalf("expected pool configuration error before acquiring a connection: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceChunkSearchRowsRollsBack(t *testing.T) {
	t.Setenv("SEARCH_SEMANTIC_ENABLED", "false")
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT generation").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(0))
	mock.ExpectExec("DELETE FROM kb.search_artifacts").WithArgs("chunk", int64(42)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO kb.search_artifacts").WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()
	err := ReplaceChunkSearchRows(context.Background(), db, 42, []RegistryRow{{ArtifactType: "chunk", ArtifactID: "42_chk_1", InputRecordID: 42}}, 0)
	if err == nil {
		t.Fatal("expected insert error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceChunkSearchRowsRejectsStaleGeneration(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT generation").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(8))
	mock.ExpectRollback()
	err := ReplaceChunkSearchRows(context.Background(), db, 42, nil, 7)
	if err == nil || !strings.Contains(err.Error(), "source changed") {
		t.Fatalf("stale writer accepted: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBackfillChunkEmbeddingsCoversAllWindows(t *testing.T) {
	t.Setenv("EMBEDDING_DIMENSIONS", "2")
	db, mock, _ := sqlmock.New()
	defer db.Close()
	text := strings.Repeat("中", 6100) + "tail"
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT artifact_type, artifact_id").WillReturnRows(sqlmock.NewRows([]string{"artifact_type", "artifact_id", "txt", "input_record_id", "generation"}).AddRow("chunk", "42_chk_1", text, 42, 3))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT generation").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(3))
	mock.ExpectExec("UPDATE kb.search_artifacts").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM kb.chunk_search_windows").WithArgs("42_chk_1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO kb.chunk_search_windows").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	var inputs []string
	res, err := BackfillEmbeddings(context.Background(), db, func(_ context.Context, s string) ([]float64, bool) {
		inputs = append(inputs, s)
		return []float64{1, 0}, true
	}, "chunk", false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if res.Embedded != 1 || len(inputs) != 2 || !strings.HasSuffix(inputs[1], "tail") {
		t.Fatalf("lost tail: %+v, %d", res, len(inputs))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceChunkSearchRowsStoresExtraWindows(t *testing.T) {
	t.Setenv("SEARCH_SEMANTIC_ENABLED", "true")
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT generation").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(0))
	mock.ExpectExec("DELETE FROM kb.search_artifacts").WithArgs("chunk", int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO kb.search_artifacts").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO kb.chunk_search_windows").WithArgs("42_chk_1", 1, "tail", "[0.1,0.2]").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE kb.chunk_search_state").WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err := ReplaceChunkSearchRows(context.Background(), db, 42, []RegistryRow{{ArtifactType: "chunk", ArtifactID: "42_chk_1", InputRecordID: 42, EmbeddingWindows: []EmbeddingWindow{{Text: "tail", Embedding: []float64{0.1, 0.2}}}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
