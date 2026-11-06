package kbhandler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
)

func TestListGoldProvisionsRequiresRun(t *testing.T) {
	withMockProjectDB(t)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/kb/provisions?input_record_id=416&source=testbed&skill_version=1.0.0", nil), rec)
	if err := ListProvisions(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListGoldProvisionsReadsRun(t *testing.T) {
	mock := withMockProjectDB(t)
	args := []any{int64(416), "1.0.0", "claude-opus-5-5", "20261106_150000"}
	mock.ExpectQuery(regexp.QuoteMeta("FROM testbed.provisions p")).WithArgs(args[0], args[1], args[2], args[3]).WillReturnRows(
		sqlmock.NewRows([]string{"id", "input_record_id", "prov_id", "prov_name", "prov_name_en", "provision_type", "provision_kind",
			"source_line_spans", "source_table_rows", "provision", "provision_en", "provision_subject", "provision_subject_en",
			"location_type", "confidence", "is_explicit", "section", "evidence", "model_name", "created_at"}).
			AddRow(int64(9), int64(416), "416_gprv_1", "容器有盖", "Containers have lids", "mandatory", "requirement",
				`["89"]`, `[]`, "垃圾分类收集容器应有盖", "", "垃圾分类收集容器", "Collection container",
				"sentence", 0.95, true, "7.1 投放要求", "垃圾分类收集容器应有盖", "claude-opus-5-5", "2026-11-06T15:00:00-06"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM testbed.provisions_dropped")).WithArgs(args[0], args[1], args[2], args[3]).WillReturnRows(
		sqlmock.NewRows([]string{"id", "candidate_id", "decision", "drop_reason", "candidate_data"}).
			AddRow(int64(3), "c1", "excluded", "X2", `{"reason":"Scope","text":"本标准适用于农村生活垃圾分类处理。","source_line_spans":["19"],"prov_ids":[]}`))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/kb/provisions?input_record_id=416&source=testbed&skill_version=1.0.0&model_name=claude-opus-5-5&run_id=20261106_150000", nil), rec)
	if err := ListProvisions(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response listProvisionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].ProvisionKind != "requirement" || *response.Results[0].ProvisionType != "mandatory" ||
		response.Results[0].Section != "7.1 投放要求" || string(response.Results[0].SourceLineSpans) != `["89"]` {
		t.Fatalf("results=%+v", response.Results)
	}
	if len(response.Dropped) != 1 || response.Dropped[0].DropReason != "X2" || response.Dropped[0].Text == "" ||
		len(response.Dropped[0].SourceLineSpans) != 1 {
		t.Fatalf("dropped=%+v", response.Dropped)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// The frozen provision scorer runs from the run directory, finds its weights and the gold helper,
// and reports its match validation messages through the runner's allowlist.
func TestProvisionScoreFrozenScorer(t *testing.T) {
	script, err := scoreSkillScriptPath(provisionScoreKind.scriptEnv, provisionScoreKind.scoreSkill)
	if err != nil {
		t.Skip("score-extract-provisions skill not installed")
	}
	root := t.TempDir()
	frozen, hashes, err := freezeScoreTools(root, script, provisionScoreKind.scoreSkill, provisionScoreKind.goldSkill)
	if err != nil || hashes["score-extract-provisions/weights.json"] == "" {
		t.Fatalf("freeze: %v %v", hashes, err)
	}
	input := `{"record_id":416,"gold_run":{"skill_version":"1.0.0","model_name":"gold-model","benchmark_run_id":"20261106_150000"},
"extraction":{"extract_ids":["e"],"model_names":["m"],"prompt_names":["p"],"created_at":["a","b"]},
"source":{"lines_comparable":true},"warnings":[],"gold_ledger":[],
"gold":[{"prov_id":"416_gprv_1","provision":"垃圾分类收集容器应有盖","provision_type":"mandatory","provision_kind":"requirement","provision_subject":"垃圾分类收集容器","location_type":"sentence","is_explicit":true,"source_line_spans":["89"]}],
"predictions":[{"prov_id":"416_prv_1","provision":"7.1.1 垃圾分类收集容器应有盖。","provision_type":"recommended","provision_subject":"垃圾分类收集容器","location_type":"sentence","is_explicit":true,"source_line_spans":["89"]}]}`
	if err = os.WriteFile(filepath.Join(root, "input.json"), []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "matches.json")
	if err = os.WriteFile(path, []byte(`{"pairs":[{"gold":"416_gprv_1","pred":"416_prv_1"}],"missed":[],"false_positives":[],"overrides":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err = runMetricScoreScriptAt(context.Background(), frozen, "score", "--dir", root, "--scorer-model", "model-exact-id"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "score.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Score int `json:"score"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	// Only provision_type (weight 30 of 100) fails.
	if result.Score != 70 {
		t.Fatalf("score.json %s", raw)
	}
	for _, invalid := range []string{
		`{"pairs":[],"missed":[],"false_positives":[{"pred":"416_prv_1","cause":"split","gold":"416_gprv_1"}],"overrides":[]}`,
		`{"pairs":[{"gold":"416_gprv_9","pred":"416_prv_1"}],"missed":[{"gold":"416_gprv_1"}],"false_positives":[],"overrides":[]}`,
	} {
		if err = os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		err = runMetricScoreScriptAt(context.Background(), frozen, "score", "--dir", root, "--scorer-model", "model-exact-id")
		var validation metricScoreValidationError
		if !errors.As(err, &validation) || metricScoreErrorCode(err) != "INVALID_MATCHES" {
			t.Fatalf("invalid matches %s: %v", invalid, err)
		}
	}
}
