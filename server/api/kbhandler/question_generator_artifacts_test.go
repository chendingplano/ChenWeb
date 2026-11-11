package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DATA-DOG/go-sqlmock"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

func TestQuestionPurposesCoverSearchArtifactTypes(t *testing.T) {
	bound := []string{}
	for _, p := range questionPurposes {
		if p.Purpose == questionPurposeMixed {
			continue
		}
		bound = append(bound, p.ArtifactType)
	}
	slices.Sort(bound)
	types := slices.Clone(searchArtifactPartitionTypes)
	slices.Sort(types)
	if !slices.Equal(bound, types) {
		t.Fatalf("purpose types %v differ from search registry types %v", bound, types)
	}
	if got, ok := questionPurposeArtifactTypes(questionPurposeMixed); !ok || !slices.Equal(got, searchArtifactPartitionTypes) {
		t.Fatalf("mixed types=%v ok=%v", got, ok)
	}
	if got, ok := questionPurposeArtifactTypes("search_provisions"); !ok || !slices.Equal(got, []string{"provision"}) {
		t.Fatalf("provision types=%v ok=%v", got, ok)
	}
	t.Setenv("PROMPT_GENERATE_ARTIFACT_QUESTIONS", "")
	for purpose, want := range map[string]string{"search_summaries": questionArtifactPrompt, questionPurposeMixed: questionArtifactPrompt, questionPurposeMetrics: questionGenerationPrompt} {
		if got, err := questionPurposePrompt(purpose); err != nil || got != want {
			t.Errorf("%s prompt=%q error=%v", purpose, got, err)
		}
	}
	t.Setenv("PROMPT_GENERATE_ARTIFACT_QUESTIONS", "prompt-custom-artifact-questions-v2.md")
	if got, _ := questionPurposePrompt("search_entities"); got != "prompt-custom-artifact-questions-v2.md" {
		t.Errorf("artifact prompt override ignored: %q", got)
	}
	for _, purpose := range []string{"search_tables", "", "Search Metrics"} {
		req := questionGenerationRequest{Purpose: purpose}
		if err := req.normalize(); err == nil {
			t.Errorf("purpose %q accepted", purpose)
		}
	}
}

func artifactFixtureLines(n int) []docprocessing.Line {
	lines := make([]docprocessing.Line, 0, n)
	for i := 1; i <= n; i++ {
		lines = append(lines, docprocessing.Line{LineNo: i, Content: fmt.Sprintf("Source line %d.", i)})
	}
	return lines
}

func artifactRow(artifactType, id, spans string) questionArtifactRow {
	return questionArtifactRow{ArtifactType: artifactType, ArtifactID: id, PrimaryLabel: "label " + id, SourceLineSpans: json.RawMessage(spans), SemanticPayload: json.RawMessage(`{}`)}
}

func TestQuestionArtifactEligibility(t *testing.T) {
	lines := artifactFixtureLines(800)
	huge := strings.Repeat("长", 25000)
	lines[499].Content = huge // line 500
	index, err := newQuestionLineIndex(lines)
	if err != nil {
		t.Fatal(err)
	}
	rows := []questionArtifactRow{
		artifactRow("provision", "416_prv_40", `["116"]`),
		artifactRow("provision", "416_prv_40", `["117"]`),  // duplicate ID: first row wins
		artifactRow("summary", "416_sum_1", `["900-950"]`), // stale: lines end at 800
		artifactRow("summary", "416_sum_2", `["498-502"]`), // context over the size limit
		artifactRow("summary", "416_sum_3", `[]`),
		artifactRow("topic", "416_tpc_1", `["abc"]`),
		artifactRow("topic", "", `["5"]`),
		artifactRow("semantic_projection", "416_smp_1", `["1-4","22-35"]`),
	}
	got := eligibleQuestionArtifacts(rows, index)
	ids := []string{}
	for _, row := range got {
		ids = append(ids, row.label())
	}
	if want := []string{"provision 416_prv_40", "semantic_projection 416_smp_1"}; !slices.Equal(ids, want) {
		t.Fatalf("eligible=%v want %v", ids, want)
	}
	if string(got[0].SourceLineSpans) != `["116"]` {
		t.Fatalf("duplicate replaced the first row: %s", got[0].SourceLineSpans)
	}
	contextIDs, err := index.contextIDs("semantic_projection 416_smp_1", json.RawMessage(`["1-4","22-35"]`))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range contextIDs {
		if id >= 5 && id <= 21 {
			t.Fatalf("context includes gap line %d: %v", id, contextIDs)
		}
	}
	if contextIDs[len(contextIDs)-1] != 45 || len(contextIDs) != 4+14+10 {
		t.Fatalf("context=%v", contextIDs)
	}
}

func TestArtifactQuestionSetsDrawTypeFirst(t *testing.T) {
	old := questionGenerationRandomInt
	t.Cleanup(func() { questionGenerationRandomInt = old })
	targets := []questionArtifactRow{artifactRow("entity", "e0", `["1"]`)}
	for i := 1; i < 300; i++ {
		targets = append(targets, artifactRow("entity", fmt.Sprintf("e%d", i), `["1"]`))
	}
	targets = append(targets, artifactRow("provision", "p0", `["1"]`), artifactRow("provision", "p1", `["1"]`))
	// Always take the last option: the type draw sees two types, not 302 targets.
	var draws []int
	questionGenerationRandomInt = func(n int) int { draws = append(draws, n); return n - 1 }
	sets, err := makeArtifactQuestionSets(targets, 4)
	if err != nil {
		t.Fatal(err)
	}
	types := []string{}
	for _, set := range sets {
		types = append(types, set.row.ArtifactType)
	}
	if want := []string{"provision", "provision", "entity", "entity"}; !slices.Equal(types, want) {
		t.Fatalf("types=%v want %v", types, want)
	}
	if draws[0] != 2 || draws[1] != 2 {
		t.Fatalf("first draws=%v: want a type draw over 2 types, then a draw over 2 provisions", draws[:2])
	}
	// Refill only after every pool is empty.
	questionGenerationRandomInt = func(int) int { return 0 }
	sets, err = makeArtifactQuestionSets([]questionArtifactRow{artifactRow("provision", "p0", `["1"]`), artifactRow("relation", "r0", `["1"]`)}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{sets[0].row.ArtifactID, sets[1].row.ArtifactID, sets[2].row.ArtifactID}; !slices.Equal(got, []string{"p0", "r0", "p0"}) {
		t.Fatalf("refill order=%v", got)
	}
	for i, set := range sets {
		if set.SetNo != i+1 || len(set.Questions) != 3 {
			t.Fatalf("set %d=%+v", i, set)
		}
	}
}

func TestArtifactQuestionInputs(t *testing.T) {
	row := artifactRow("inventory_item", "371_inv_11", `["47"]`)
	row.PrimaryLabel, row.SecondaryLabel, row.SnippetBasis = "秒表", "stopwatch", strings.Repeat("s", 1500)
	row.SemanticPayload = json.RawMessage(`{"brand":"","aliases":["timer"],"normalized_specs":{"range":"","accuracy":"0.1 s"}}`)
	sets, source, hash, err := buildArtifactQuestionInputs(artifactFixtureLines(60), []artifactQuestionSet{{SetNo: 1, row: row, Questions: newQuestionSlots()}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(sets[0])
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	artifact := decoded["artifact"].(map[string]any)
	if artifact["artifact_type"] != "inventory_item" || artifact["artifact_id"] != "371_inv_11" || artifact["secondary_label"] != "stopwatch" {
		t.Fatalf("artifact=%v", artifact)
	}
	if snippet := artifact["snippet"].(string); utf8.RuneCountInString(snippet) != questionArtifactSnippetMaxChars+1 || !strings.HasSuffix(snippet, "…") {
		t.Fatalf("snippet not cut: %d characters", utf8.RuneCountInString(snippet))
	}
	if payload, _ := json.Marshal(artifact["semantic_payload"]); string(payload) != `{"aliases":["timer"],"normalized_specs":{"accuracy":"0.1 s"}}` {
		t.Fatalf("payload not pruned: %s", payload)
	}
	if ranges := artifact["context_line_ranges"].([]any); len(ranges) != 1 || ranges[0] != "37:57" {
		t.Fatalf("ranges=%v", ranges)
	}
	if _, leaked := decoded["row"]; leaked || hash == "" || len(source) != 21 || !strings.Contains(artifact["context"].(string), "Source line 47.") {
		t.Fatalf("set JSON=%s source=%d hash=%q", raw, len(source), hash)
	}
}

func artifactQuestion(setNo int, hop, lang, question string, lines ...int) map[string]any {
	reasoning := []any{}
	for i := 0; i <= map[string]int{"one_hop": 0, "two_hop": 1, "multi_hop": 2}[hop]; i++ {
		reasoning = append(reasoning, map[string]any{"text": fmt.Sprintf("Step %d.", i+1), "line_ids": lines})
	}
	return map[string]any{"set_no": setNo, "hop_type": hop, "lang": lang, "question": question, "answer": "Use a stopwatch.", "reasoning": reasoning}
}

func TestArtifactQuestionValidation(t *testing.T) {
	row := artifactRow("inventory_item", "371_inv_11", `["47"]`)
	row.PrimaryLabel, row.SecondaryLabel = "秒表", "stopwatch"
	slots := []questionSlot{{HopType: "one_hop", Lang: "en"}, {HopType: "two_hop", Lang: "en"}, {HopType: "multi_hop", Lang: "zh-cn"}}
	sets, source, hash, err := buildArtifactQuestionInputs(artifactFixtureLines(100), []artifactQuestionSet{{SetNo: 1, row: row, Questions: slots}})
	if err != nil {
		t.Fatal(err)
	}
	req := questionRequest(t, `{"input_record_id":371,"purpose":"search_inventory_items","metrics_per_document":1}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	payload := func(items ...map[string]any) map[string]any {
		raw, _ := json.Marshal(map[string]any{"questions": items})
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return out
	}
	valid := payload(
		artifactQuestion(1, "one_hop", "en", "Which instrument times the verification step?", 47),
		artifactQuestion(1, "two_hop", "en", "What do I need to measure how long the verification run takes?", 46, 47),
		artifactQuestion(1, "multi_hop", "zh-cn", "按检定规程计时需要准备什么器具？", 45, 47),
	)
	questions, err := collectArtifactQuestions(valid, req, source, sets, hash, nil)
	if err != nil || len(questions) != 3 {
		t.Fatalf("valid questions rejected: %d error=%v", len(questions), err)
	}
	q := questions[0]
	if q.ArtifactType != "inventory_item" || q.ArtifactID != "371_inv_11" || q.MetricID != "" || q.SourceSHA256 != hash || !strings.Contains(string(q.ArtifactSnapshot), `"secondary_label":"stopwatch"`) {
		t.Fatalf("question not stamped: %+v", q)
	}
	for name, bad := range map[string]map[string]any{
		"outside context": artifactQuestion(1, "one_hop", "en", "Which instrument is listed far away?", 90),
		"not anchored":    artifactQuestion(1, "one_hop", "en", "Which instrument is listed nearby?", 46),
		"leaks label":     artifactQuestion(1, "two_hop", "en", "Which stopwatch accuracy is needed for timing?", 47, 47),
		"wrong language":  artifactQuestion(1, "two_hop", "zh-cn", "计时需要什么？", 47, 47),
	} {
		if _, err := collectArtifactQuestions(payload(bad), req, source, sets, hash, nil); err == nil || !strings.Contains(err.Error(), "requested inventory_item questions are missing or rejected") {
			t.Errorf("%s accepted or misreported: %v", name, err)
		} else if name == "leaks label" && !strings.Contains(err.Error(), `contains its target inventory_item name "stopwatch"`) {
			t.Errorf("leak not reported: %v", err)
		} else if name == "not anchored" && !strings.Contains(err.Error(), "does not cite its selected inventory_item's source lines (source_line_spans 47)") {
			t.Errorf("anchor hint missing: %v", err)
		}
	}
}

func installArtifactQuestionFixture(t *testing.T, rows []questionArtifactRow) *[]string {
	t.Helper()
	oldRows, oldLines, oldRandom := loadQuestionArtifactRows, loadQuestionGenerationLines, questionGenerationRandomInt
	t.Cleanup(func() {
		loadQuestionArtifactRows, loadQuestionGenerationLines, questionGenerationRandomInt = oldRows, oldLines, oldRandom
	})
	installQuestionMetadataFixture(t)
	questionGenerationRandomInt = func(int) int { return 0 }
	requested := []string{}
	loadQuestionArtifactRows = func(_ context.Context, _ *sql.DB, _ int64, types []string) ([]questionArtifactRow, error) {
		requested = append(requested, strings.Join(types, ","))
		out := []questionArtifactRow{}
		for _, row := range rows {
			if slices.Contains(types, row.ArtifactType) {
				out = append(out, row)
			}
		}
		return out, nil
	}
	loadQuestionGenerationLines = func(context.Context, *sql.DB, int64) ([]docprocessing.Line, error) {
		return artifactFixtureLines(200), nil
	}
	return &requested
}

func TestArtifactQuestionDocumentWithCorrection(t *testing.T) {
	db, _ := installPolicyDB(t)
	provision := artifactRow("provision", "416_prv_40", `["116"]`)
	provision.PrimaryLabel = "其他垃圾无害化处理"
	installArtifactQuestionFixture(t, []questionArtifactRow{provision, artifactRow("entity", "416_ent_1", `["5"]`)})
	calls := 0
	client := questionExtractorFunc(func(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
		calls++
		if in.CallReason != "question_generation_search_provisions" || in.CallLoc != "20261111-742" || in.PromptName != questionArtifactPrompt {
			t.Errorf("call provenance=%+v", in)
		}
		var input struct {
			ArtifactType *string `json:"artifact_type"`
			Purpose      string  `json:"purpose"`
			Sets         []struct {
				SetNo     int                   `json:"set_no"`
				Artifact  questionArtifactInput `json:"artifact"`
				Questions []questionSlot        `json:"questions"`
			} `json:"sets"`
			ValidationError string `json:"validation_error"`
		}
		if err := json.Unmarshal([]byte(in.InputText), &input); err != nil {
			t.Fatal(err)
		}
		if input.ArtifactType != nil || input.Purpose != "search_provisions" || len(input.Sets) != 1 || input.Sets[0].Artifact.ArtifactType != "provision" || input.Sets[0].Artifact.ArtifactID != "416_prv_40" {
			t.Fatalf("model input=%s", in.InputText)
		}
		slots := input.Sets[0].Questions
		items := []map[string]any{
			artifactQuestion(1, "one_hop", slots[0].Lang, "What does the standard require for residual waste?", 116),
			artifactQuestion(1, "two_hop", slots[1].Lang, "Where must leftover household waste that is neither recyclable nor hazardous go?", 115, 116),
			artifactQuestion(1, "multi_hop", slots[2].Lang, "After sorting at a district station, how is the remaining non-recyclable fraction finally treated?", 114, 116),
		}
		if calls == 1 {
			items[2] = artifactQuestion(1, "multi_hop", slots[2].Lang, "How is the remainder handled?", 150) // outside the context
		} else if !strings.Contains(input.ValidationError, "cites an unknown or repeated line 150") {
			t.Errorf("correction lacks the citation error: %q", input.ValidationError)
		}
		raw, _ := json.Marshal(map[string]any{"questions": items})
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		return payload, nil
	})
	req := questionRequest(t, `{"input_record_id":416,"purpose":"search_provisions","metrics_per_document":1}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	logger := loggerutil.CreateDefaultLogger("20261111-914")
	defer logger.Close()
	saved := 0
	questions, err := generateArtifactQuestionDocument(context.Background(), db, 12, req, ApiTypes.LLMModelDef{ModelName: "fixture"}, client, questionArtifactPrompt, "fixture prompt", "admin", nil, logger,
		func(accepted []generatedQuestion) error { saved = len(accepted); return nil })
	if err != nil || len(questions) != 3 || calls != 2 || saved != 2 {
		t.Fatalf("questions=%d calls=%d saved before correction=%d error=%v", len(questions), calls, saved, err)
	}
	for _, q := range questions {
		if q.ArtifactType != "provision" || q.ArtifactID != "416_prv_40" || q.MetricID != "" {
			t.Fatalf("question=%+v", q)
		}
	}
}

func TestArtifactQuestionDocumentWithoutTargets(t *testing.T) {
	db, _ := installPolicyDB(t)
	requested := installArtifactQuestionFixture(t, []questionArtifactRow{artifactRow("summary", "416_sum_1", `["900"]`)})
	logger := loggerutil.CreateDefaultLogger("20261111-915")
	defer logger.Close()
	for purpose, want := range map[string]string{
		"search_knowledge": "document has no knowledge artifacts",
		"search_summaries": "document has no summary artifacts linked to usable source lines",
	} {
		req := questionRequest(t, fmt.Sprintf(`{"input_record_id":416,"purpose":%q,"metrics_per_document":1}`, purpose))
		if err := req.normalize(); err != nil {
			t.Fatal(err)
		}
		_, err := generateArtifactQuestionDocument(context.Background(), db, 12, req, ApiTypes.LLMModelDef{}, nil, "", "", "", nil, logger)
		if err == nil || err.Error() != want {
			t.Errorf("%s error=%v want %q", purpose, err, want)
		}
	}
	if !slices.Equal(*requested, []string{"knowledge", "summary"}) {
		t.Fatalf("requested types=%v", *requested)
	}
}

func TestQuestionDocumentSamplingQueries(t *testing.T) {
	for purpose, query := range map[string]string{
		questionPurposeMetrics: `FROM kb.metrics m`,
		"search_entities":      `FROM kb.search_artifacts a .*artifact_type = ANY\(\$1\)`,
		questionPurposeMixed:   `FROM kb.search_artifacts a .*artifact_type = ANY\(\$1\)`,
	} {
		db, mock := installPolicyDB(t)
		expect := mock.ExpectQuery(query)
		if purpose == "search_entities" {
			expect = expect.WithArgs(pq.Array([]string{"entity"}))
		} else if purpose == questionPurposeMixed {
			expect = expect.WithArgs(pq.Array(searchArtifactPartitionTypes))
		}
		expect.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(416).AddRow(371))
		ids, err := randomQuestionDocumentIDs(context.Background(), db, purpose)
		if err != nil || !slices.Equal(ids, []int64{416, 371}) {
			t.Errorf("%s ids=%v error=%v", purpose, ids, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("%s: %v", purpose, err)
		}
	}
}

func TestQuestionGeneratorBatchFailsWithoutTypeRows(t *testing.T) {
	db, mock := installPolicyDB(t)
	oldCandidates, oldClient := questionGenerationCandidates, newQuestionGenerationClient
	t.Cleanup(func() { questionGenerationCandidates, newQuestionGenerationClient = oldCandidates, oldClient })
	promptDir, err := filepath.Abs("../../../prompts")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROMPT_DIR", promptDir)
	questionGenerationCandidates = func(_ context.Context, _ *sql.DB, purpose string) ([]int64, error) {
		if purpose != "search_knowledge" {
			t.Errorf("sampled documents for %q", purpose)
		}
		return nil, nil
	}
	newQuestionGenerationClient = func(string, ApiTypes.JimoLogger) (ApiTypes.LLMModelDef, metricJSONExtractor, error) {
		return ApiTypes.LLMModelDef{ModelName: "fixture"}, questionExtractorFunc(func(context.Context, llmclients.JSONExtractionInput) (map[string]any, error) {
			t.Fatal("model called without documents")
			return nil, nil
		}), nil
	}
	expectTaskLLMExecution(mock, "testbed.generated_question_runs", 12)
	mock.ExpectExec("UPDATE testbed.generated_question_runs SET model_name").WithArgs(int64(12), "fixture").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), "no eligible document could generate the remaining questions: no documents with knowledge artifacts are available").WillReturnResult(sqlmock.NewResult(0, 1))
	req := questionRequest(t, `{"purpose":"search_knowledge"}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	runQuestionGeneration(db, 12, req, "fixture", "admin")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionGenerationArtifactShortageDoesNotStartRun(t *testing.T) {
	_, mock := installPolicyDB(t)
	rows := []questionArtifactRow{artifactRow("provision", "p-stale", `["999"]`)}
	for i := range 4 {
		rows = append(rows, artifactRow("provision", fmt.Sprintf("p%d", i), fmt.Sprintf(`["%d"]`, 10+i)))
	}
	installArtifactQuestionFixture(t, rows)
	oldLaunch := launchQuestionGeneration
	t.Cleanup(func() { launchQuestionGeneration = oldLaunch })
	launchQuestionGeneration = func(*sql.DB, int64, questionGenerationRequest, string, string) { t.Fatal("unconfirmed run launched") }
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(416)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"input_record_id":416,"purpose":"search_provisions","num_questions_to_generate":30}`)), rec)
	if err := StartQuestionGeneration(c); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusConflict || result["available_targets"] != float64(4) || result["available_questions"] != float64(12) || result["confirmation_required"] != true {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, legacy := result["available_metrics"]; legacy {
		t.Fatalf("legacy field returned: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMixedQuestionRunStoresEachTargetType(t *testing.T) {
	db, mock := installPolicyDB(t)
	requested := installArtifactQuestionFixture(t, []questionArtifactRow{
		artifactRow("provision", "416_prv_40", `["116"]`), artifactRow("relation", "365_rel_705", `["20"]`), // registry order: by type
	})
	// First type draw: last open type (relation), then the only one left (provision).
	draw := 0
	questionGenerationRandomInt = func(n int) int { draw++; return map[bool]int{true: n - 1, false: 0}[draw == 1] }
	oldClient := newQuestionGenerationClient
	t.Cleanup(func() { newQuestionGenerationClient = oldClient })
	promptDir, err := filepath.Abs("../../../prompts")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROMPT_DIR", promptDir)
	newQuestionGenerationClient = func(string, ApiTypes.JimoLogger) (ApiTypes.LLMModelDef, metricJSONExtractor, error) {
		return ApiTypes.LLMModelDef{ModelName: "fixture"}, questionExtractorFunc(func(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
			if in.CallReason != "question_generation_search_mixed" {
				t.Errorf("call reason=%q", in.CallReason)
			}
			var input struct {
				Sets []struct {
					SetNo     int                   `json:"set_no"`
					Artifact  questionArtifactInput `json:"artifact"`
					Questions []questionSlot        `json:"questions"`
				} `json:"sets"`
			}
			if err := json.Unmarshal([]byte(in.InputText), &input); err != nil {
				t.Fatal(err)
			}
			items := []map[string]any{}
			for _, set := range input.Sets {
				line := map[string]int{"relation": 20, "provision": 116}[set.Artifact.ArtifactType]
				for i, slot := range set.Questions {
					items = append(items, artifactQuestion(set.SetNo, slot.HopType, slot.Lang, fmt.Sprintf("Question %d about line %d?", i, line), line))
				}
			}
			raw, _ := json.Marshal(map[string]any{"questions": items})
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			return payload, nil
		}), nil
	}
	expectTaskLLMExecution(mock, "testbed.generated_question_runs", 12)
	mock.ExpectExec("UPDATE testbed.generated_question_runs SET model_name").WithArgs(int64(12), "fixture").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	for _, target := range []string{"365_rel_705", "365_rel_705", "365_rel_705", "416_prv_40", "416_prv_40", "416_prv_40"} {
		artifactType := map[bool]string{true: "relation", false: "provision"}[strings.Contains(target, "rel")]
		mock.ExpectExec("INSERT INTO testbed.generated_questions").WithArgs(int64(12), int64(416), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			"", "{}", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), artifactType, target, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectExec("UPDATE testbed.generated_question_runs").WithArgs(int64(12), true, 2, 2, 2).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	req := questionRequest(t, `{"input_record_id":416,"purpose":"search_mixed","metrics_per_document":2}`)
	if err := req.normalize(); err != nil {
		t.Fatal(err)
	}
	runQuestionGeneration(db, 12, req, "fixture", "admin")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if len(*requested) != 1 || (*requested)[0] != strings.Join(searchArtifactPartitionTypes, ",") {
		t.Fatalf("mixed run loaded types %v", *requested)
	}
}
