package kbhandler

import (
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestReadTopicIDsForCategory_StructuredTopicsFile(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "safety", "inspection", "topics.txt"), `record_id: 99,
topic_type: "procedure"
lines: [405-406]
topic_keywords: [腰背肌力, 测试方法]
topic: "First topic"

record_id: 99,
topic_type: "procedure"
lines: [410-412]
topic_keywords: [背力计]
topic: "Second topic"

record_id: 105,
topic_type: "warning"
lines: [500-501]
topic_keywords: [风险]
topic: "Third topic"
`)

	got, err := readTopicIDsForCategory(root, "safety/inspection")
	if err != nil {
		t.Fatalf("readTopicIDsForCategory: %v", err)
	}

	want := []string{"105_1", "99_1", "99_2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected topic ids: got=%v want=%v", got, want)
	}
}

func TestReadTopicGraphTopicIDs_StructuredTopicsFile(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "topics.txt"), `record_id: 99,
topic_type: "procedure"
lines: [405-406]
topic: "First topic"

record_id: 105,
topic_type: "warning"
lines: [500-501]
topic: "Second topic"
`)

	got, ok := readTopicGraphTopicIDs(root)
	if !ok {
		t.Fatalf("expected topics.txt to be detected")
	}

	want := []string{"105_1", "99_1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected topic ids: got=%v want=%v", got, want)
	}
}

func TestReadTopicCategoryRecords_StructuredTopicsFile(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	artifactDir := t.TempDir()
	topicTreeDir := t.TempDir()
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

	mustWriteFile(t, filepath.Join(topicTreeDir, "safety", "inspection", "topics.txt"), `record_id: 99,
topic_type: "procedure"
lines: [338-342]
topic_keywords: [clinic, health]
topic: "Requirements text"
`)
	mustWriteFile(t, filepath.Join(artifactDir, "0", "99", "std_20039_opendata.topics"), `topic_id: 1
topic_type: "procedure"
lines: ["338-342"]
topic_keywords: ["clinic", "health"]
topic: "Requirements text"
category_paths: []
`)
	mustWriteFile(t, filepath.Join(artifactDir, "0", "99", "std_20039_opendata.txt"), "338\t17\tparagraph\tSong\t12\t[10,20,30,40]\tA\n339\t17\tparagraph\tSong\t12\t[11,21,31,41]\tB\n342\t17\tparagraph\tSong\t12\t[12,22,32,42]\tC\n")

	results, err := readTopicCategoryRecords(topicTreeDir, "safety/inspection")
	if err != nil {
		t.Fatalf("readTopicCategoryRecords: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(results))
	}
	if results[0].ID != "99_1" {
		t.Fatalf("unexpected topic id: %+v", results[0])
	}
	if results[0].TopicText != "Requirements text" {
		t.Fatalf("unexpected topic text: %+v", results[0])
	}
	if len(results[0].SourceLineSpecs) != 1 || results[0].SourceLineSpecs[0] != "338-342" {
		t.Fatalf("unexpected source line specs: %+v", results[0].SourceLineSpecs)
	}
	if results[0].Page != 17 {
		t.Fatalf("expected page 17, got %+v", results[0])
	}
	if len(results[0].Targets) == 0 || results[0].Targets[0].Page != 17 {
		t.Fatalf("expected highlight targets from line artifact, got %+v", results[0].Targets)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestReadTopicCategoryRecords_ResolvesStructuredTopicWithoutTopicIDByTopicText(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	artifactDir := t.TempDir()
	topicTreeDir := t.TempDir()
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

	mustWriteFile(t, filepath.Join(topicTreeDir, "safety", "inspection", "topics.txt"), `record_id: 99,
topic_type: "list"
lines: [493-501]
topic_keywords: [传染病筛查, 免疫]
topic: "应定期对消防员进行传染病筛查和免疫。"
`)
	mustWriteFile(t, filepath.Join(artifactDir, "0", "99", "std_20039_opendata.topics"), `topic_id: 1
topic_type: "general"
lines: ["1-3"]
topic_keywords: ["overview"]
topic: "消防员职业健康标准 (Standard on occupational health for fire fighter) - 发布信息"
category_paths: []

topic_id: 149
topic_type: "list"
lines: ["493-501"]
topic_keywords: ["传染病筛查", "免疫"]
topic: "应定期对消防员进行传染病筛查和免疫。"
category_paths: []
`)
	mustWriteFile(t, filepath.Join(artifactDir, "0", "99", "std_20039_opendata.txt"), "493\t17\tparagraph\tSong\t12\t[10,20,30,40]\tA\n501\t17\tparagraph\tSong\t12\t[11,21,31,41]\tB\n")

	results, err := readTopicCategoryRecords(topicTreeDir, "safety/inspection")
	if err != nil {
		t.Fatalf("readTopicCategoryRecords: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(results))
	}
	if results[0].ID != "99_149" {
		t.Fatalf("expected resolved artifact topic id, got %+v", results[0])
	}
	if results[0].TopicText != "应定期对消防员进行传染病筛查和免疫。" {
		t.Fatalf("unexpected topic text: %+v", results[0])
	}
	if len(results[0].SourceLineSpecs) != 1 || results[0].SourceLineSpecs[0] != "493-501" {
		t.Fatalf("unexpected source line specs: %+v", results[0].SourceLineSpecs)
	}
	if results[0].Page != 17 {
		t.Fatalf("expected page 17, got %+v", results[0])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
