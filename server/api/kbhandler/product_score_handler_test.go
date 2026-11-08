package kbhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestProductScoreKindIsCompact(t *testing.T) {
	if !productScoreKind.compact || productScoreKind.matchBasis != "product" || productScoreKind.runsDir != "product-score-runs" {
		t.Fatalf("productScoreKind=%+v", productScoreKind)
	}
	if !provisionScoreKind.compact || metricScoreKind.compact || referenceScoreKind.compact {
		t.Fatal("only the provision and product scorers use the compact contract")
	}
}

// Real projection, decoder and frozen product scorer; only the DB export and the provider are fixtures.
func TestProductCompactRunner(t *testing.T) {
	for _, mode := range []string{"complete", "repair", "invalid_twice"} {
		t.Run(mode, func(t *testing.T) {
			root, err := filepath.Abs("../../..")
			if err != nil {
				t.Fatal(err)
			}
			t.Chdir(root)
			real, err := scoreSkillScriptPath("PRODUCT_BENCHMARK_SCORER_PATH", "score-extract-products")
			if err != nil {
				t.Fatal(err)
			}
			tmp := t.TempDir()
			source := "89\t1\tlist-item\tfont\t10\t[0,0,10,10]\t垃圾分类收集容器应有盖。\n"
			sourcePath := filepath.Join(tmp, "source.txt")
			if err = os.WriteFile(sourcePath, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			gold := map[string]any{"product_rel_id": "gold-nonordinal-99", "product_key": "416_gp_1", "product_name": "垃圾分类收集容器",
				"canonical_name": "垃圾分类收集容器", "product_type": "equipment", "relation_type": "design_requirement",
				"evidence_lines": []string{"89"}, "obligation_level": "mandatory", "requirement_text": "垃圾分类收集容器应有盖",
				"product_name_en": "MUST_NOT_SEND_TRANSLATION", "conditions": []any{}, "exceptions": []any{}, "parameters": []any{},
				"related_products": []any{}, "responsible_actor": ""}
			pred := map[string]any{}
			for k, v := range gold {
				pred[k] = v
			}
			delete(pred, "product_key")
			pred["product_rel_id"] = "prediction-nonordinal-77"
			input := map[string]any{
				"record_id": 416, "title": "fixture", "gold": []any{gold}, "predictions": []any{pred},
				"gold_run":    map[string]any{"skill_version": "1.0.0", "model_name": "gold", "benchmark_run_id": "fixture"},
				"extraction":  map[string]any{"model_names": []string{"pred"}, "prompt_names": []string{"extract"}, "created_at": []string{"a", "b"}},
				"source":      map[string]any{"path": sourcePath, "current_sha256": metricScoreDigest([]byte(source)), "lines_comparable": true},
				"gold_ledger": []any{}, "warnings": []string{}, "suggestions": "MUST_NOT_SEND_SUGGESTIONS",
			}
			data, _ := json.Marshal(input)
			t.Setenv("PRODUCT_TEST_INPUT", string(data))
			t.Setenv("PRODUCT_TEST_SCORER", real)
			script := filepath.Join(tmp, "skills", "score-extract-products", "scripts", "score_io.py")
			helperRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(real))), "extract-products-benchmark")
			for _, name := range []string{"scripts/benchmark_io.py", "rules/fixture.md"} {
				target := filepath.Join(tmp, "skills", "extract-products-benchmark", name)
				if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
					t.Fatal(err)
				}
				content := []byte("fixture rules")
				if strings.HasSuffix(name, ".py") {
					if content, err = os.ReadFile(filepath.Join(helperRoot, name)); err != nil {
						t.Fatal(err)
					}
				}
				if err = os.WriteFile(target, content, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err = os.MkdirAll(filepath.Dir(script), 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(filepath.Dir(filepath.Dir(script)), "weights.json"), []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
			stub := `import os,sys,pathlib
if sys.argv[1] == 'export':
 out = pathlib.Path(sys.argv[sys.argv.index('--out-dir')+1])
 out.mkdir(parents=True,exist_ok=False)
 (out/'input.json').write_text(os.environ['PRODUCT_TEST_INPUT'])
else:
 exec(compile(pathlib.Path(os.environ['PRODUCT_TEST_SCORER']).read_text(), __file__, 'exec'))
`
			if err = os.WriteFile(script, []byte(stub), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PRODUCT_BENCHMARK_SCORER_PATH", script)
			t.Setenv("DATA_HOME_DIR", tmp)
			t.Setenv("MODEL_DEFAULT_REASONING_POLICY", "no-reasoning")
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []struct{ Content string } `json:"messages"`
					Thinking any                        `json:"thinking"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body.Thinking != nil {
					t.Errorf("product matching must keep the profile's reasoning, got thinking=%v", body.Thinking)
				}
				serialized, _ := json.Marshal(body)
				request := string(serialized)
				if strings.Contains(request, "MUST_NOT_SEND") || strings.Contains(request, "gold-nonordinal-99") ||
					!strings.Contains(request, `\"g\":[[[\"89\"],\"垃圾分类收集容器\",\"垃圾分类收集容器\",\"design_requirement\"`) {
					t.Errorf("expected only compact matching rows: %s", request)
				}
				calls++
				if calls > 1 && !strings.Contains(request, "MATCH_REPAIR") {
					t.Error("missing compact repair evidence")
				}
				content := `{"v":2,"p":[[1,1]],"m":[],"f":[],"o":[]}`
				if mode == "invalid_twice" || (mode == "repair" && calls == 1) {
					content = `{"v":2,"p":[[1,1]],"m":[],"f":[],"o":[[1,"relation_type",true,"close"]]}`
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
			}))
			defer server.Close()
			modelFile := filepath.Join(tmp, "models.toml")
			if err = os.WriteFile(modelFile, []byte(fmt.Sprintf("[fixture]\nmodel_name='compact-test'\nmodel_type='llm'\napi_key='test-only'\nbase_url='%s'\ntimeout_sec=20\n", server.URL)), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("MODEL_DEF_FILE", modelFile)
			previousConfig := ApiTypes.CommonConfig.PGConf
			t.Cleanup(func() { ApiTypes.CommonConfig.PGConf = previousConfig })
			ApiTypes.CommonConfig.PGConf = ApiTypes.DatabaseConfig{ProjectDBName: "fixture-db", UserName: "fixture-user", Host: "localhost", Port: 5432}
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			expectTaskLLMExecution(mock, "testbed.product_score_runs", 93)
			mock.ExpectQuery(`SELECT current_database\(\),current_user`).WillReturnRows(sqlmock.NewRows([]string{"db", "user"}).AddRow("fixture-db", "fixture-user"))
			logger := loggerutil.CreateDefaultLogger("20261108-731")
			defer logger.Close()
			evidence, err := executeScore(context.Background(), db, logger, productScoreKind, 93, metricScoreRequest{RecordID: 416, Model: "fixture", Lang: "en"}, "fixture-user")
			if mode == "invalid_twice" {
				if metricScoreErrorCode(err) != "INVALID_MATCHES" || calls != 2 || len(evidence.Score) != 0 || len(evidence.Matches) != 0 {
					t.Fatalf("invalid response not bounded/rejected: calls=%d err=%v", calls, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(evidence.Report, "Score: 100 / 100") || !strings.Contains(string(evidence.Matches), "gold-nonordinal-99") {
				t.Fatalf("missing expanded score evidence: %s %s", evidence.Matches, evidence.Report)
			}
			var score map[string]any
			if err = json.Unmarshal(evidence.Score, &score); err != nil {
				t.Fatal(err)
			}
			if detection, _ := score["product_detection"].(map[string]any); detection["f1"] != 1.0 {
				t.Fatalf("product detection missing: %v", score["product_detection"])
			}
			artifactRoot := filepath.Join(tmp, "product-score-runs", "93", "evidence")
			for _, name := range []string{"matching-input.json", "matching-request.txt", "matches.json", "matches-expanded.json"} {
				if _, err = os.Stat(filepath.Join(artifactRoot, name)); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "repair" && calls != 2 {
				t.Fatalf("repair calls=%d", calls)
			}
		})
	}
}
