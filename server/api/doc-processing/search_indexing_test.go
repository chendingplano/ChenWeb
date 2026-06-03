package docprocessing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestBuildSummaryRegistryRowsReadsSummaryArtifacts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_DIR", tmp)
	recordDir := filepath.Join(tmp, "1", "1042")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `summary_id: "1042_1_0001"
record_id: 1042
level: 1
lines: ["2:10","2:11"]
keywords: ["energy","efficiency"]
category_paths: ["performance","energy"]
summary_begin
Energy efficiency requirements apply to the tested device.
summary_end
`
	if err := os.WriteFile(filepath.Join(recordDir, "summary_1_0001.txt"), []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rows, err := buildSummaryRegistryRowsFromFiles(1042, "doc.pdf")
	if err != nil {
		t.Fatalf("buildSummaryRegistryRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len=%d", len(rows))
	}
	if rows[0].ArtifactID != "1042_sum_1" {
		t.Fatalf("artifact_id=%q", rows[0].ArtifactID)
	}
	if rows[0].PrimaryLabel == "" || rows[0].SnippetBasis == "" {
		t.Fatalf("expected non-empty label/snippet basis: %#v", rows[0])
	}
}

func TestBuildTopicRegistryRowsReadsTopicArtifacts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_DIR", tmp)
	recordDir := filepath.Join(tmp, "1", "1042")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `topic_id: 1
topic_type: "requirement"
lines: ["2:10"]
topic_keywords: ["battery","charging"]
topic_desc: "Battery charging safety"
category_paths: ["safety","battery"]
`
	if err := os.WriteFile(filepath.Join(recordDir, "std.topics"), []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rows, err := buildTopicRegistryRowsFromFiles(1042, "doc.pdf")
	if err != nil {
		t.Fatalf("buildTopicRegistryRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows len=%d", len(rows))
	}
	if rows[0].ArtifactID != "1042_tpc_1" {
		t.Fatalf("artifact_id=%q", rows[0].ArtifactID)
	}
	if rows[0].SecondaryLabel != "requirement" {
		t.Fatalf("secondary_label=%q", rows[0].SecondaryLabel)
	}
}

func TestReplaceRegistryRowsDeletesThenInserts(t *testing.T) {
	t.Setenv("SEARCH_SEMANTIC_ENABLED", "")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.search_artifacts WHERE artifact_type = $1 AND input_record_id = $2`)).
		WithArgs("summary", int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	payload, _ := json.Marshal(map[string]any{"kind": "summary"})
	mock.ExpectExec("INSERT INTO kb.search_artifacts").
		WithArgs(
			"summary", "42_sum_1", int64(42), nil,
			"Energy summary", "Level 1",
			"Energy summary text",
			"Energy summary text",
			"doc.pdf",
			"doc.pdf",
			`["performance"]`,
			`["2:10"]`,
			string(payload),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	rows := []kbsearch.RegistryRow{{
		ArtifactType:    "summary",
		ArtifactID:      "42_sum_1",
		InputRecordID:   42,
		PrimaryLabel:    "Energy summary",
		SecondaryLabel:  "Level 1",
		SearchDocument:  "Energy summary text",
		SnippetBasis:    "Energy summary text",
		SourceTitle:     "doc.pdf",
		SourceFilename:  "doc.pdf",
		CategoryPaths:   json.RawMessage(`["performance"]`),
		SourceLineSpans: json.RawMessage(`["2:10"]`),
		SemanticPayload: json.RawMessage(payload),
	}}

	if err := replaceRegistryRows(context.Background(), db, "summary", 42, rows, nil); err != nil {
		t.Fatalf("replaceRegistryRows: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestReplaceSummaryArtifactsForRecordDeletesThenInserts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.summaries WHERE input_record_id = $1`)).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO kb.summaries").
		WithArgs(
			"42_1_0001",
			int64(42),
			1,
			1,
			`["2:10"]`,
			`[]`,
			`["energy"]`,
			`[]`,
			`["performance"]`,
			`[]`,
			`[]`,
			`[]`,
			`[]`,
			"Energy summary",
			"",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = ReplaceSummaryArtifactsForRecord(context.Background(), 42, []SummaryItem{{
		SummaryID:     "42_1_0001",
		RecordID:      42,
		Level:         1,
		SeqNo:         1,
		Lines:         []string{"2:10"},
		Keywords:      []string{"energy"},
		CategoryPaths: []string{"performance"},
		Summary:       "Energy summary",
	}}, nil)
	if err != nil {
		t.Fatalf("ReplaceSummaryArtifactsForRecord: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestReplaceTopicArtifactsForRecordDeletesThenInserts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.topics WHERE input_record_id = $1`)).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO kb.topics").
		WithArgs(
			"1",
			int64(42),
			1,
			"requirement",
			"",
			`["2:10"]`,
			`["battery"]`,
			`[]`,
			`["safety"]`,
			`[]`,
			`[{"path_keywords":null,"path_confidence":0,"category_path":[{"name":"safety","keywords":null,"confidence":0}]}]`,
			`[]`,
			"Battery charging safety",
			"",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = ReplaceTopicArtifactsForRecord(context.Background(), 42, []TopicItem{{
		SeqNo:     1,
		TopicType: "requirement",
		Lines:     []string{"2:10"},
		Keywords:  []string{"battery"},
		Topic:     "Battery charging safety",
		CategoryPathDetail: []CategoryPathEntry{{
			Nodes: []CategoryPathNode{{Name: "safety"}},
		}},
	}}, nil)
	if err != nil {
		t.Fatalf("ReplaceTopicArtifactsForRecord: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
