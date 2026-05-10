package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestReadRecordTopicCards_AllowsMissingCorrectedArtifact(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)

	expectResolveInputTablePlural(mock)
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("kb", "inputs", "staging_filename").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("kb", "inputs", "parser_name").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	recordID := int64(99)
	query := regexp.QuoteMeta(`SELECT COALESCE(i.staging_filename, '') AS staging_filename, COALESCE(i.parser_name, '') AS parser_name, i.file_name FROM kb.inputs i WHERE i.id = $1`)
	mock.ExpectQuery(query).
		WithArgs(recordID).
		WillReturnRows(sqlmock.NewRows([]string{"staging_filename", "parser_name", "file_name"}).
			AddRow("std_20039.pdf", "opendata", "/tmp/standards/std_20039.pdf"))

	mustWriteFile(t, filepath.Join(artifactDir, "0", "99", "std_20039_opendata.topics"), `topic_id: 1
topic_type: "general"
lines: ["1-3"]
topic_keywords: ["scope", "definitions"]
topic: "Scope and definitions"
category_paths: []
`)

	results, err := readRecordTopicCards(nil, recordID)
	if err != nil {
		t.Fatalf("readRecordTopicCards: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Fatalf("unexpected topic id: %+v", results[0])
	}
	if len(results[0].SourceLineSpecs) != 1 || results[0].SourceLineSpecs[0] != "1-3" {
		t.Fatalf("expected line specs to round-trip, got %+v", results[0].SourceLineSpecs)
	}
	if results[0].Page != 1 {
		t.Fatalf("expected default page 1 without corrected artifact, got %+v", results[0])
	}
	if len(results[0].Coords) != 0 {
		t.Fatalf("expected empty coords without corrected artifact, got %+v", results[0])
	}
	if len(results[0].Targets) != 0 {
		t.Fatalf("expected empty targets without corrected artifact, got %+v", results[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestGetRecordTopics_AllowsMissingCorrectedArtifact(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)

	expectResolveInputTablePlural(mock)
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("kb", "inputs", "staging_filename").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("kb", "inputs", "parser_name").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	recordID := int64(99)
	query := regexp.QuoteMeta(`SELECT COALESCE(i.staging_filename, '') AS staging_filename, COALESCE(i.parser_name, '') AS parser_name, i.file_name FROM kb.inputs i WHERE i.id = $1`)
	mock.ExpectQuery(query).
		WithArgs(recordID).
		WillReturnRows(sqlmock.NewRows([]string{"staging_filename", "parser_name", "file_name"}).
			AddRow("std_20039.pdf", "opendata", "/tmp/standards/std_20039.pdf"))

	mustWriteFile(t, filepath.Join(artifactDir, "0", "99", "std_20039_opendata.topics"), `topic_id: 1
topic_type: "general"
lines: ["1-3"]
topic_keywords: ["scope", "definitions"]
topic: "Scope and definitions"
category_paths: []
`)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/record-topics?record_id=99", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetRecordTopics(c); err != nil {
		t.Fatalf("GetRecordTopics returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Status   bool                  `json:"status"`
		RecordID int64                 `json:"recordId"`
		Topics   []topicCategoryRecord `json:"topics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Status || payload.RecordID != recordID || len(payload.Topics) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if len(payload.Topics[0].SourceLineSpecs) != 1 || payload.Topics[0].SourceLineSpecs[0] != "1-3" {
		t.Fatalf("expected line specs in payload, got %+v", payload.Topics[0].SourceLineSpecs)
	}
	if payload.Topics[0].Page != 1 || len(payload.Topics[0].Targets) != 0 {
		t.Fatalf("expected empty highlight data, got %+v", payload.Topics[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestReadRecordTopicCards_UsesTxtLineArtifactForPageTargets(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)

	expectResolveInputTablePlural(mock)
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("kb", "inputs", "staging_filename").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("kb", "inputs", "parser_name").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	recordID := int64(99)
	query := regexp.QuoteMeta(`SELECT COALESCE(i.staging_filename, '') AS staging_filename, COALESCE(i.parser_name, '') AS parser_name, i.file_name FROM kb.inputs i WHERE i.id = $1`)
	mock.ExpectQuery(query).
		WithArgs(recordID).
		WillReturnRows(sqlmock.NewRows([]string{"staging_filename", "parser_name", "file_name"}).
			AddRow("std_20039.pdf", "opendata", "/tmp/standards/std_20039.pdf"))

	mustWriteFile(t, filepath.Join(artifactDir, "0", "99", "std_20039_opendata.topics"), `topic_id: 101
topic_type: "requirement"
lines: ["338-342"]
topic_keywords: ["clinic", "health"]
topic: "Requirements text"
category_paths: []
`)
	mustWriteFile(t, filepath.Join(artifactDir, "0", "99", "std_20039_opendata.txt"), "338\t17\tparagraph\tSong\t12\t[10,20,30,40]\tA\n339\t17\tparagraph\tSong\t12\t[11,21,31,41]\tB\n342\t17\tparagraph\tSong\t12\t[12,22,32,42]\tC\n")

	results, err := readRecordTopicCards(nil, recordID)
	if err != nil {
		t.Fatalf("readRecordTopicCards: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(results))
	}
	if results[0].Page != 17 {
		t.Fatalf("expected topic page from txt line artifact, got %+v", results[0])
	}
	if len(results[0].Targets) == 0 || results[0].Targets[0].Page != 17 {
		t.Fatalf("expected topic targets from txt line artifact, got %+v", results[0].Targets)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
