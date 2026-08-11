package docprocessing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestIndexProvisionsForRecordLogsObjectEdgeCount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("SELECT prov_id, id, COALESCE\\(source_line_spans").
		WithArgs(int64(100)).
		WillReturnRows(sqlmock.NewRows([]string{"prov_id", "id", "source_line_spans", "search_document"}))

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM kb\\.artifact_connections").
		WithArgs(int64(100), RelationMethodObjectID, RelationBelongTo, searchArtifactProvision).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("FROM kb\\.provisions").
		WithArgs(int64(100), RelationBelongTo, RelationMethodObjectID, searchArtifactProvision, searchArtifactProvision).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	logger := &fakeLogger{}
	IndexProvisionsForRecord(context.Background(), 100, nil, logger)

	entry, ok := findInfoLog(logger.infos, "provision indexing object edges")
	if !ok {
		t.Fatalf("infos = %+v, want a 'provision indexing object edges' log", logger.infos)
	}
	if v, ok := logValue(entry.args, "object_edges"); !ok || v != int64(3) {
		t.Fatalf("object_edges = %v, want 3", v)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestBuildSummaryRegistryRowsReadsSummaryArtifacts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARTIFACT_DIR", tmp)
	recordDir := filepath.Join(tmp, "1", "1042")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `summary_id: "1042_sum_1_0001"
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

func TestBuildSemanticProjectionRegistryRowsIncludesLineSpans(t *testing.T) {
	oldConfig := appconfig.AppConfig
	t.Cleanup(func() {
		appconfig.AppConfig = oldConfig
	})
	appconfig.AppConfig.SemanticProjectionSearchWeights = appconfig.SemanticProjectionSearchWeightsConfig{
		DescriptiveName:    1.0,
		Keywords:           1.0,
		SemanticProjection: 1.0,
		CategoryPaths:      1.0,
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"id",
		"semantic_proj_id",
		"language",
		"descriptive_name",
		"descriptive_name_en",
		"keywords",
		"keywords_en",
		"semantic_projection",
		"semantic_projection_en",
		"category_paths",
		"category_paths_en",
		"line_spans",
	}).AddRow(
		int64(7),
		"177_0_2",
		"en",
		"Artifact connections",
		nil,
		[]byte(`["metric"]`),
		[]byte(`[]`),
		"Projection body",
		nil,
		[]byte(`[{"category_path":[{"name":"metrics","keywords":["metric"],"confidence":0.9}],"path_keywords":["metric"],"path_confidence":0.9}]`),
		[]byte(`[]`),
		[]byte(`["22-45"]`),
	)
	mock.ExpectQuery("FROM kb.semantic_projections").
		WithArgs(int64(177)).
		WillReturnRows(rows)

	got, err := buildSemanticProjectionRegistryRows(context.Background(), db, 177)
	if err != nil {
		t.Fatalf("buildSemanticProjectionRegistryRows: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows len=%d", len(got))
	}
	if string(got[0].SourceLineSpans) != `["22-45"]` {
		t.Fatalf("SourceLineSpans=%s", got[0].SourceLineSpans)
	}
	if got[0].ArtifactID != "177_0_2" {
		t.Fatalf("ArtifactID=%q", got[0].ArtifactID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestBuildSemanticProjectionRegistryRowsAppliesConfiguredWeightsToSearchDocument(t *testing.T) {
	oldConfig := appconfig.AppConfig
	t.Cleanup(func() {
		appconfig.AppConfig = oldConfig
	})
	appconfig.AppConfig.SemanticProjectionSearchWeights = appconfig.SemanticProjectionSearchWeightsConfig{
		DescriptiveName:    2.0,
		Keywords:           1.0,
		SemanticProjection: 0.5,
		CategoryPaths:      0.5,
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"id",
		"semantic_proj_id",
		"language",
		"descriptive_name",
		"descriptive_name_en",
		"keywords",
		"keywords_en",
		"semantic_projection",
		"semantic_projection_en",
		"category_paths",
		"category_paths_en",
		"line_spans",
	}).AddRow(
		int64(8),
		"177_0_3",
		"en",
		"Artifact connections",
		nil,
		[]byte(`["metric"]`),
		[]byte(`[]`),
		"Projection body",
		nil,
		[]byte(`[{"category_path":[{"name":"metrics","keywords":["metric"],"confidence":0.9}],"path_keywords":["metric"],"path_confidence":0.9}]`),
		[]byte(`[]`),
		[]byte(`["30-31"]`),
	)
	mock.ExpectQuery("FROM kb.semantic_projections").
		WithArgs(int64(177)).
		WillReturnRows(rows)

	got, err := buildSemanticProjectionRegistryRows(context.Background(), db, 177)
	if err != nil {
		t.Fatalf("buildSemanticProjectionRegistryRows: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows len=%d", len(got))
	}
	if strings.Count(got[0].SearchDocument, "Artifact connections") <= strings.Count(got[0].SearchDocument, "Projection body") {
		t.Fatalf("search_document=%q", got[0].SearchDocument)
	}
	if !strings.Contains(got[0].SearchDocument, "metric") {
		t.Fatalf("search_document=%q", got[0].SearchDocument)
	}
}

func TestBuildSceneBlockRegistryRowsAppliesConfiguredWeightsToSearchDocument(t *testing.T) {
	oldConfig := appconfig.AppConfig
	t.Cleanup(func() {
		appconfig.AppConfig = oldConfig
	})
	appconfig.AppConfig.SceneBlockSearchWeights = appconfig.SceneBlockSearchWeightsConfig{
		Title:     2.0,
		SceneType: 1.0,
		Summary:   0.5,
		Keywords:  1.0,
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"id", "object_id", "title", "scene_type", "summary", "keywords", "line_spans", "search_document",
	}).AddRow(
		int64(4), "42_1", "Startup sequence", "procedure", "System boots and validates.", []byte(`["boot","validate"]`), []byte(`["8:10"]`), "",
	)
	mock.ExpectQuery("FROM kb.scene_objects").
		WithArgs(int64(42)).
		WillReturnRows(rows)

	got, err := buildSceneBlockRegistryRows(context.Background(), db, 42)
	if err != nil {
		t.Fatalf("buildSceneBlockRegistryRows: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows len=%d", len(got))
	}
	if strings.Count(got[0].SearchDocument, "Startup sequence") <= strings.Count(got[0].SearchDocument, "System boots and validates.") {
		t.Fatalf("search_document=%q", got[0].SearchDocument)
	}
	if !strings.Contains(got[0].SearchDocument, "boot validate") {
		t.Fatalf("search_document=%q", got[0].SearchDocument)
	}
}

func TestBuildSummaryRegistryRowsFromDBAppliesConfiguredWeightsToSearchDocument(t *testing.T) {
	oldConfig := appconfig.AppConfig
	t.Cleanup(func() { appconfig.AppConfig = oldConfig })
	appconfig.AppConfig.SummarySearchWeights = appconfig.SummarySearchWeightsConfig{SummaryText: 2.0, Keywords: 1.0, CategoryPaths: 1.0}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "summary_id", "summary_level", "summary_seq_no", "source_line_spans", "keywords", "summary_text", "summary_text_en", "search_document"}).
		AddRow(int64(1), "42_0_1", 0, 1, []byte(`["1:2"]`), []byte(`["energy"]`), "Primary summary", "English summary", "")
	mock.ExpectQuery("FROM kb.summaries").WithArgs(int64(42)).WillReturnRows(rows)

	got, err := buildSummaryRegistryRowsFromDB(context.Background(), db, 42, "doc.pdf")
	if err != nil {
		t.Fatalf("buildSummaryRegistryRowsFromDB: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows len=%d", len(got))
	}
	if strings.Count(got[0].SearchDocument, "Primary summary") <= strings.Count(got[0].SearchDocument, "energy") {
		t.Fatalf("search_document=%q", got[0].SearchDocument)
	}
}

func TestBuildTopicRegistryRowsFromDBAppliesConfiguredWeightsToSearchDocument(t *testing.T) {
	oldConfig := appconfig.AppConfig
	t.Cleanup(func() { appconfig.AppConfig = oldConfig })
	appconfig.AppConfig.TopicSearchWeights = appconfig.TopicSearchWeightsConfig{TopicType: 0.5, TopicDesc: 2.0, Keywords: 1.0, CategoryPaths: 1.0}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "topic_id", "topic_type", "topic_desc", "topic_desc_en", "keywords", "keywords_en", "category_paths", "category_paths_en", "source_line_spans", "search_document"}).
		AddRow(int64(1), "1", "requirement", "Battery safety", "", []byte(`["battery"]`), []byte(`[]`), []byte(`["safety"]`), []byte(`[]`), []byte(`["3:5"]`), "")
	mock.ExpectQuery("FROM kb.topics").WithArgs(int64(42)).WillReturnRows(rows)

	got, err := buildTopicRegistryRowsFromDB(context.Background(), db, 42, "doc.pdf")
	if err != nil {
		t.Fatalf("buildTopicRegistryRowsFromDB: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows len=%d", len(got))
	}
	if strings.Count(got[0].SearchDocument, "Battery safety") <= strings.Count(got[0].SearchDocument, "requirement") {
		t.Fatalf("search_document=%q", got[0].SearchDocument)
	}
}

func TestBuildProvisionRegistryRowsAppliesConfiguredWeightsToSearchDocument(t *testing.T) {
	oldConfig := appconfig.AppConfig
	t.Cleanup(func() { appconfig.AppConfig = oldConfig })
	appconfig.AppConfig.ProvisionSearchWeights = appconfig.ProvisionSearchWeightsConfig{ProvisionName: 0.5, ProvisionType: 0.5, ProvisionDesc: 2.0, Keywords: 1.0, CategoryPaths: 1.0}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "prov_id", "prov_name", "provision_type", "prov_desc", "provision_keywords", "category_paths", "source_line_spans", "input_filename", "search_document"}).
		AddRow(int64(1), "p1", "Voltage limit", "mandatory", "Keep voltage below threshold", []byte(`["voltage"]`), []byte(`["electrical"]`), []byte(`["8:9"]`), "doc.pdf", "")
	mock.ExpectQuery("FROM kb.provisions").WithArgs(int64(42)).WillReturnRows(rows)

	got, err := buildProvisionRegistryRows(context.Background(), db, 42)
	if err != nil {
		t.Fatalf("buildProvisionRegistryRows: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows len=%d", len(got))
	}
	if strings.Count(got[0].SearchDocument, "Keep voltage below threshold") <= strings.Count(got[0].SearchDocument, "Voltage limit") {
		t.Fatalf("search_document=%q", got[0].SearchDocument)
	}
}

func TestBuildProvisionRegistryRowsPreservesStoredSearchDocument(t *testing.T) {
	oldConfig := appconfig.AppConfig
	t.Cleanup(func() { appconfig.AppConfig = oldConfig })
	appconfig.AppConfig.ProvisionSearchWeights = appconfig.ProvisionSearchWeightsConfig{ProvisionName: 1.0, ProvisionType: 1.0, ProvisionDesc: 1.0, Keywords: 1.0, CategoryPaths: 1.0}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "prov_id", "prov_name", "provision_type", "prov_desc", "provision_keywords", "category_paths", "source_line_spans", "input_filename", "search_document"}).
		AddRow(
			int64(1),
			"p1",
			"报告核签要求",
			"mandatory",
			"规定动态血压报告需由执业医师核签",
			[]byte(`["动态血压","核签"]`),
			[]byte(`["检查/报告"]`),
			[]byte(`["8:9"]`),
			"doc.pdf",
			"要求动态血压报告必须由主治医师及以上职称的医师审核签字",
		)
	mock.ExpectQuery("FROM kb.provisions").WithArgs(int64(42)).WillReturnRows(rows)

	got, err := buildProvisionRegistryRows(context.Background(), db, 42)
	if err != nil {
		t.Fatalf("buildProvisionRegistryRows: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows len=%d", len(got))
	}
	if !strings.Contains(got[0].SearchDocument, "审核签字") {
		t.Fatalf("search_document=%q, want stored search text preserved", got[0].SearchDocument)
	}
}

func TestBuildEntityRegistryRowsAppliesConfiguredWeightsToSearchDocument(t *testing.T) {
	oldConfig := appconfig.AppConfig
	t.Cleanup(func() { appconfig.AppConfig = oldConfig })
	appconfig.AppConfig.EntitySearchWeights = appconfig.EntitySearchWeightsConfig{Entity: 2.0, EntityType: 0.5, Aliases: 0.5, DescText: 0.5, Keywords: 1.0}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "entity_id", "language", "entity", "entity_en", "entity_type", "entity_type_en", "aliases", "aliases_en", "desc_text", "desc_text_en", "keywords", "keywords_en", "line_spans", "search_document"}).
		AddRow(int64(1), "e1", "en", "Transformer", "", "equipment", "", []byte(`["converter"]`), []byte(`[]`), "Power device", "", []byte(`["power"]`), []byte(`[]`), []byte(`["10:11"]`), "")
	mock.ExpectQuery("FROM kb.entities").WithArgs(int64(42)).WillReturnRows(rows)

	got, err := buildEntityRegistryRows(context.Background(), db, 42)
	if err != nil {
		t.Fatalf("buildEntityRegistryRows: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows len=%d", len(got))
	}
	if strings.Count(got[0].SearchDocument, "Transformer") <= strings.Count(got[0].SearchDocument, "equipment") {
		t.Fatalf("search_document=%q", got[0].SearchDocument)
	}
}

func TestBuildRelationRegistryRowsAppliesConfiguredWeightsToSearchDocument(t *testing.T) {
	oldConfig := appconfig.AppConfig
	t.Cleanup(func() { appconfig.AppConfig = oldConfig })
	appconfig.AppConfig.RelationSearchWeights = appconfig.RelationSearchWeightsConfig{Subject: 2.0, Predicate: 0.5, Object: 0.5, DescText: 0.5, Keywords: 1.0}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"id", "relation_id", "language", "subject", "subject_en", "predicate", "predicate_en", "object", "object_en", "desc_text", "desc_text_en", "keywords", "keywords_en", "line_spans", "search_document"}).
		AddRow(int64(1), "r1", "en", "Battery", "", "requires", "", "Cooling", "", "Battery requires cooling", "", []byte(`["thermal"]`), []byte(`[]`), []byte(`["12:13"]`), "")
	mock.ExpectQuery("FROM kb.relations").WithArgs(int64(42)).WillReturnRows(rows)

	got, err := buildRelationRegistryRows(context.Background(), db, 42)
	if err != nil {
		t.Fatalf("buildRelationRegistryRows: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows len=%d", len(got))
	}
	if strings.Count(got[0].SearchDocument, "Battery") <= strings.Count(got[0].SearchDocument, "requires") {
		t.Fatalf("search_document=%q", got[0].SearchDocument)
	}
}

func TestReplaceRegistryRowsDeletesThenInserts(t *testing.T) {
	t.Setenv("SEARCH_SEMANTIC_ENABLED", "")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	logger := loggerutil.CreateDefaultLogger("MID-20260708-06")
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
			"{}",
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

	if err := replaceRegistryRows(context.Background(), db, "summary", 42, rows, "test", "MID-20260708-15", logger); err != nil {
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
			"Energy summary",
			"",
			"",
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = ReplaceSummaryArtifactsForRecord(context.Background(), 42, []SummaryItem{{
		SummaryID: "42_1_0001",
		RecordID:  42,
		Level:     1,
		SeqNo:     1,
		Lines:     []string{"2:10"},
		Keywords:  []string{"energy"},
		Summary:   "Energy summary",
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
			"42_tpc_1",
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
