package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

func TestMetricScoreRequestValidation(t *testing.T) {
	for _, req := range []metricScoreRequest{
		{RecordID: 0, Model: "model", Lang: "en"},
		{RecordID: 1, Lang: "en"},
		{RecordID: 1, Model: "model", Lang: "fr"},
		{RecordID: 1, Model: "model", Lang: "en", GoldRunID: "run-only"},
	} {
		if err := req.validate(); err == nil {
			t.Fatalf("accepted invalid request: %+v", req)
		}
	}
	req := metricScoreRequest{RecordID: 416, Model: "model", Lang: "zh-cn", GoldVersion: "3.0.0", GoldModel: "gold-model", GoldRunID: "20261007_120000"}
	if err := req.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMetricScoreAdminRoles(t *testing.T) {
	for _, user := range []*ApiTypes.UserInfo{nil, {UserId: "guest", Roles: []string{"guest"}}, {UserId: "-", Admin: true}} {
		if metricScoreIsAdmin(user) {
			t.Fatal("non-admin accepted")
		}
	}
	for _, user := range []*ApiTypes.UserInfo{{UserId: "owner", IsOwner: true}, {UserId: "admin", Admin: true}, {UserId: "root", Roles: []string{" ROOT "}}} {
		if !metricScoreIsAdmin(user) {
			t.Fatal("admin rejected")
		}
	}
}

func TestMetricScoreArtifactWhitelist(t *testing.T) {
	run := metricScoreRun{Input: json.RawMessage(`{"gold":[]}`), Matches: json.RawMessage(`{"pairs":[]}`), Score: json.RawMessage(`{"score":100}`), Report: "# Score"}
	for _, kind := range []string{"input", "matches", "score", "report"} {
		body, _, _, ok := metricScoreArtifact(run, kind)
		if !ok || len(body) == 0 {
			t.Fatalf("missing %s artifact", kind)
		}
	}
	for _, kind := range []string{"../../.env", "source", "report.md"} {
		if _, _, _, ok := metricScoreArtifact(run, kind); ok {
			t.Fatalf("accepted %s", kind)
		}
	}
}

func TestMetricScoreInputIncludesSourceAndLocale(t *testing.T) {
	input, err := buildMetricScoreInput(json.RawMessage(`{"gold":[{"metric_id":"g1"}],"predictions":[]}`), []rawLine{{LineNumber: 12, LineType: "paragraph", Content: "每日检查一次"}}, "zh-cn")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"zh-cn", `"metric_id":"g1"`, "L12\tparagraph\t每日检查一次"} {
		if !strings.Contains(input, want) {
			t.Fatalf("input missing %s", want)
		}
	}
	if _, err = buildMetricScoreInput(json.RawMessage(`{}`), []rawLine{{Content: strings.Repeat("x", metricReviewMaxInputChars)}}, "en"); err == nil {
		t.Fatal("oversized source accepted")
	}
}

func TestMetricScoreSubprocessNoGold(t *testing.T) {
	script := filepath.Join(t.TempDir(), "score_io.py")
	if err := os.WriteFile(script, []byte("import sys\nprint('SECRET',file=sys.stderr)\nsys.exit(3)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("METRIC_BENCHMARK_SCORER_PATH", script)
	err := runMetricScoreScript(context.Background(), "export", "--record-id", "416")
	if metricScoreErrorCode(err) != "NO_GOLD" || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestMetricScoreScorerParityAndInvalidMatches(t *testing.T) {
	script, err := metricScoreScriptPath()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	frozen, hashes, err := freezeMetricScoreTools(root, script)
	if err != nil || len(hashes) != 2 {
		t.Fatalf("freeze: %v %v", hashes, err)
	}
	input := `{"record_id":416,"gold_run":{"skill_version":"3.0.0","model_name":"gold-model","benchmark_run_id":"20261007_120000","stand_in":false},"extraction":{"prompts_models_events":[],"created_at":["2026-10-07","2026-10-07"],"log_run_id":null,"exclusion_log":[]},"source":{"lines_comparable":true},"warnings":[],"gold_ledger":[],"gold":[{"metric_id":"g1","metric_name":"Temperature","metric_value":"20","value_range_type":"exact","kind":"requirement_with_criterion","metric_unit":"C","source_line_spans":["12"]}],"predictions":[{"metric_id":"p1","metric_name":"Temperature","metric_value":"30","value_range_type":"exact","kind":"requirement_with_criterion","metric_unit":"C","source_line_spans":["12"]}]}`
	if err = os.WriteFile(filepath.Join(root, "input.json"), []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	matches := `{"pairs":[{"gold":"g1","pred":"p1"}],"missed":[],"false_positives":[],"overrides":[]}`
	path := filepath.Join(root, "matches.json")
	if err = os.WriteFile(path, []byte(matches), 0600); err != nil {
		t.Fatal(err)
	}
	if err = runMetricScoreScriptAt(context.Background(), frozen, "score", "--dir", root, "--scorer-model", "model-exact-id"); err != nil {
		t.Fatal(err)
	}
	score, err := os.ReadFile(filepath.Join(root, "score.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Score  int    `json:"score"`
		Scorer string `json:"scorer_model"`
	}
	if err = json.Unmarshal(score, &result); err != nil {
		t.Fatal(err)
	}
	if result.Score != 70 || result.Scorer != "model-exact-id" {
		t.Fatalf("scorer result %s", score)
	}
	for _, invalid := range []string{
		`{"pairs":[],"missed":[],"false_positives":[],"overrides":[]}`,
		`{"pairs":[{"gold":"g1","pred":"p1"}],"overrides":[{"gold":"g1","field":"kind","correct":true,"reason":"minor mistake"}]}`,
		`{"pairs":[{"gold":"g1","pred":"p1"},{"gold":"g1","pred":"p1"}]}`,
	} {
		if err = os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if code := metricScoreErrorCode(runMetricScoreScriptAt(context.Background(), frozen, "score", "--dir", root, "--scorer-model", "model-exact-id")); code != "INVALID_MATCHES" {
			t.Fatalf("invalid matches code: %s", code)
		}
	}
}

func TestMetricScoreSubprocessNoPredictionsAndTimeout(t *testing.T) {
	script := filepath.Join(t.TempDir(), "score_io.py")
	if err := os.WriteFile(script, []byte("import sys\nsys.exit(4)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := metricScoreErrorCode(runMetricScoreScriptAt(context.Background(), script, "export")); code != "NO_PREDICTIONS" {
		t.Fatalf("code: %s", code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := metricScoreErrorCode(runMetricScoreScriptAt(ctx, script, "export")); code != "TIMEOUT" {
		t.Fatalf("code: %s", code)
	}
}

func TestMetricScoreUsesPoolConfigRatherThanLibpqDefaults(t *testing.T) {
	original := ApiTypes.CommonConfig.PGConf
	t.Cleanup(func() { ApiTypes.CommonConfig.PGConf = original })
	ApiTypes.CommonConfig.PGConf = ApiTypes.DatabaseConfig{ProjectDBName: "pool-db", UserName: "pool-user", Password: "pool-password", Host: "pool-host", Port: 5432}
	t.Setenv("PGDATABASE", "unrelated-db")
	t.Setenv("PGUSER", "unrelated-user")
	t.Setenv("PGHOSTADDR", "203.0.113.1")
	t.Setenv("PGSERVICE", "unrelated-service")
	values := map[string]string{}
	for _, entry := range metricScoreCommandEnv() {
		parts := strings.SplitN(entry, "=", 2)
		values[parts[0]] = parts[1]
	}
	if values["PGDATABASE"] != "pool-db" || values["PGUSER"] != "pool-user" || values["PGHOST"] != "pool-host" || values["PGHOSTADDR"] != "" || values["PGSERVICE"] != "" {
		t.Fatal("subprocess did not use pool configuration")
	}
}

func TestMetricScoreMiddlewareStopsUnauthorizedRequests(t *testing.T) {
	originalAuth, originalDB := EchoFactory.DefaultAuthenticator, ApiTypes.ProjectDBHandle
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = originalAuth; ApiTypes.ProjectDBHandle = originalDB })
	ApiTypes.ProjectDBHandle = &sql.DB{}
	for _, tc := range []struct {
		user   *ApiTypes.UserInfo
		status int
		called bool
	}{
		{nil, http.StatusUnauthorized, false},
		{&ApiTypes.UserInfo{UserId: "guest", Roles: []string{"guest"}}, http.StatusForbidden, false},
		{&ApiTypes.UserInfo{UserId: "admin", Roles: []string{"admin"}}, http.StatusNoContent, true},
	} {
		EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) { return tc.user, nil }
		e := echo.New()
		rec := httptest.NewRecorder()
		ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/kb/metric-scores", nil), rec)
		called := false
		err := MetricScoreAdmin(func(c echo.Context) error { called = true; return c.NoContent(http.StatusNoContent) })(ctx)
		if err != nil || rec.Code != tc.status || called != tc.called {
			t.Fatalf("status=%d called=%v error=%v", rec.Code, called, err)
		}
	}
}

// Exercise the Go export→LLM→authoritative Python scorer bridge without paid model calls
// or changing production metrics. The fixture exporter replaces only the database reads.
func TestMetricScoreRunnerEndToEnd(t *testing.T) {
	projectRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(projectRoot)
	realScript, err := metricScoreScriptPath()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	rawSource := "12\t1\tparagraph\tfont\t10\t[0,0,10,10]\tTemperature must be 20 C\n"
	sourcePath := filepath.Join(tmp, "source.txt")
	if err = os.WriteFile(sourcePath, []byte(rawSource), 0600); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{
		"record_id": 416, "title": "Temperature document",
		"gold_run":   map[string]any{"skill_version": "3.0.0", "model_name": "gold-model", "benchmark_run_id": "20261007_120000", "stand_in": false},
		"extraction": map[string]any{"prompts_models_events": [][]string{}, "created_at": []string{"2026-10-07", "2026-10-07"}, "log_run_id": nil, "exclusion_log": []any{}},
		"source":     map[string]any{"path": sourcePath, "current_sha256": metricScoreDigest([]byte(rawSource)), "lines_comparable": true},
		"warnings":   []string{}, "gold_ledger": []any{},
		"gold":        []any{map[string]any{"metric_id": "g1", "metric_name": "Temperature", "metric_value": "20", "value_range_type": "exact", "kind": "requirement_with_criterion", "metric_unit": "C", "source_line_spans": []string{"12"}}},
		"predictions": []any{map[string]any{"metric_id": "p1", "metric_name": "Temperature", "metric_value": "30", "value_range_type": "exact", "kind": "requirement_with_criterion", "metric_unit": "C", "source_line_spans": []string{"12"}}},
	}
	inputBytes, _ := json.Marshal(input)
	t.Setenv("METRIC_TEST_INPUT", string(inputBytes))
	t.Setenv("METRIC_TEST_REAL_SCORER", realScript)
	scriptPath := filepath.Join(tmp, "skills", "score-extract-metrics", "scripts", "score_io.py")
	helperPath := filepath.Join(tmp, "skills", "extract-metrics-benchmark", "scripts", "benchmark_io.py")
	for _, path := range []string{scriptPath, helperPath} {
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
	}
	helper, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(realScript))), "extract-metrics-benchmark", "scripts", "benchmark_io.py"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(helperPath, helper, 0600); err != nil {
		t.Fatal(err)
	}
	stub := `import os,sys,pathlib
if sys.argv[1] == 'export':
 out = pathlib.Path(sys.argv[sys.argv.index('--out-dir')+1])
 out.mkdir(parents=True,exist_ok=False)
 (out/'input.json').write_text(os.environ['METRIC_TEST_INPUT'])
else:
 exec(compile(pathlib.Path(os.environ['METRIC_TEST_REAL_SCORER']).read_text(), __file__, 'exec'))
`
	if err = os.WriteFile(scriptPath, []byte(stub), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("METRIC_BENCHMARK_SCORER_PATH", scriptPath)
	t.Setenv("DATA_HOME_DIR", tmp)
	called := false
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if body.Model != "scorer-exact-model" {
			t.Errorf("model=%s", body.Model)
		}
		serialized, _ := json.Marshal(body.Messages)
		if !strings.Contains(string(serialized), "Temperature must be 20 C") || !strings.Contains(string(serialized), "zh-cn") {
			t.Error("source/locale missing from LLM request")
		}
		called = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"pairs":[{"gold":"g1","pred":"p1","note":"同一断言"}],"missed":[],"false_positives":[],"overrides":[]}`}, "finish_reason": "stop"}}})
	}))
	defer llmServer.Close()
	modelFile := filepath.Join(tmp, "models.toml")
	modelText := fmt.Sprintf("[fixture]\nmodel_name='scorer-exact-model'\nmodel_type='llm'\napi_key='test-only'\nbase_url='%s'\ntimeout_sec=20\n", llmServer.URL)
	if err = os.WriteFile(modelFile, []byte(modelText), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MODEL_DEF_FILE", modelFile)
	originalConfig := ApiTypes.CommonConfig.PGConf
	t.Cleanup(func() { ApiTypes.CommonConfig.PGConf = originalConfig })
	ApiTypes.CommonConfig.PGConf = ApiTypes.DatabaseConfig{ProjectDBName: "fixture-db", UserName: "fixture-user", Host: "localhost", Port: 5432}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT current_database\(\),current_user`).WillReturnRows(sqlmock.NewRows([]string{"db", "user"}).AddRow("fixture-db", "fixture-user"))
	logger := loggerutil.CreateDefaultLogger("20261007-672")
	defer logger.Close()
	evidence, err := executeMetricScore(context.Background(), db, logger, 93, metricScoreRequest{RecordID: 416, Model: "fixture", Lang: "zh-cn"}, "fixture-user")
	if err != nil {
		t.Fatal(err)
	}
	if !called || evidence.Model != "scorer-exact-model" || !strings.Contains(evidence.Report, "Score: 70 / 100") {
		t.Fatalf("unexpected result model=%s report=%s", evidence.Model, evidence.Report)
	}
	var saved map[string]any
	if err = json.Unmarshal(evidence.Input, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["source_text"] != rawSource || saved["provenance"] == nil {
		t.Fatal("missing immutable source/provenance")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMetricScoreArtifactDownloadUsesSavedSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	originalDB := ApiTypes.ProjectDBHandle
	t.Cleanup(func() { ApiTypes.ProjectDBHandle = originalDB })
	ApiTypes.ProjectDBHandle = db
	saved := `{"record_id":416,"gold":[{"metric_id":"g1"}]}`
	mock.ExpectQuery("SELECT .* FROM kb.metric_score_runs WHERE id=\\$1").WithArgs(int64(91)).WillReturnRows(sqlmock.NewRows(strings.Split(metricScoreColumns, ",")).AddRow(int64(91), int64(416), "Title", "en", "done", "model", metricScorePrompt, "admin", time.Now(), time.Now(), "", "", []byte(saved), []byte(`{}`), []byte(`{"score":70}`), "# Score"))
	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/kb/metric-scores/91/artifacts/input", nil), rec)
	ctx.SetParamNames("id", "kind")
	ctx.SetParamValues("91", "input")
	if err := DownloadMetricScoreArtifact(ctx); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || rec.Body.String() != saved || rec.Header().Get("Content-Type") != "application/json" || !strings.Contains(rec.Header().Get("Content-Disposition"), "benchmark-91-input.json") {
		t.Fatalf("artifact response: code=%d headers=%v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
