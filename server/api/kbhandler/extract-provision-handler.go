package kbhandler

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
	toml "github.com/pelletier/go-toml/v2"
)

type provisionJSONExtractor interface {
	ExtractJSON(context.Context, llmclients.JSONExtractionInput) (map[string]any, error)
}

type structuredProvisionJSONExtractor interface {
	ExtractStructuredJSON(context.Context, llmclients.JSONExtractionInput, llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error)
}

var (
	loadProvisionsPromptForExtractFn   = loadProvisionsPromptForExtract
	loadExtractProvisionsModelConfigFn = loadExtractProvisionsModelConfig
	newExtractProvisionsClientFn       = defaultNewExtractProvisionsClient
)

type extractProvisionRequest struct {
	RecordID        int64                     `json:"record_id"`
	SourceLineSpans []createProvisionLineSpan `json:"source_line_spans"`
}

type extractProvisionResponse struct {
	Status     bool             `json:"status"`
	Provisions []map[string]any `json:"provisions,omitempty"`
	Language   string           `json:"language,omitempty"`
	Error      string           `json:"error,omitempty"`
}

type saveExtractedProvisionsRequest struct {
	RecordID   int64            `json:"record_id"`
	Provisions []map[string]any `json:"provisions"`
}

type saveExtractedProvisionsResponse struct {
	Status   bool   `json:"status"`
	Inserted int64  `json:"inserted,omitempty"`
	Error    string `json:"error,omitempty"`
}

func defaultNewExtractProvisionsClient(cfg ApiTypes.LLMModelDef, logger ApiTypes.JimoLogger) (provisionJSONExtractor, error) {
	return llmclients.NewOpenAIJSONClientFromConfig(llmclients.OpenAIJSONClientConfig{
		ModelName:    cfg.ModelName,
		APIKey:       cfg.APIKey,
		BaseURL:      cfg.BaseURL,
		TimeoutSec:   cfg.TimeoutSec,
		ThinkingType: cfg.ThinkingType,
	}, logger)
}

func extractProvisionHandlerContract() llmclients.StructuredOutputContract {
	return llmclients.StructuredOutputContract{
		Name:        "chenweb_provision_handler_extraction",
		AllowRepair: true,
		MaxRetries:  2,
		Schema: []byte(`{
			"type": "object",
			"properties": {
				"language": { "type": "string" },
				"provisions": { "type": "array" }
			},
			"required": ["provisions"],
			"additionalProperties": true
		}`),
	}
}

// ExtractProvisions handles POST /api/v1/kb/provisions/extract.
// It reads user-selected lines from the raw line file, composes input with overlap
// context, calls the LLM to extract provisions, and returns them for client-side
// review before persistence.
func ExtractProvisions(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_P_400")
	defer rc.Close()
	logger := rc.GetLogger()

	var req extractProvisionRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, extractProvisionResponse{
			Status: false,
			Error:  fmt.Sprintf("invalid request body (CWB_KB_P_401): %v", err),
		})
	}
	if req.RecordID <= 0 {
		return c.JSON(http.StatusBadRequest, extractProvisionResponse{
			Status: false,
			Error:  "record_id must be a positive integer (CWB_KB_P_402)",
		})
	}
	if len(req.SourceLineSpans) == 0 {
		return c.JSON(http.StatusBadRequest, extractProvisionResponse{
			Status: false,
			Error:  "source_line_spans must not be empty (CWB_KB_P_403)",
		})
	}

	db := ApiTypes.ProjectDBHandle

	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, extractProvisionResponse{
			Status: false,
			Error:  "failed to resolve input table (CWB_KB_P_404)",
		})
	}

	q := fmt.Sprintf(`SELECT i.result_filename FROM %s i WHERE i.id = $1`, inputTable)
	var resultFile sql.NullString
	if err := db.QueryRow(q, req.RecordID).Scan(&resultFile); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, extractProvisionResponse{
				Status: false,
				Error:  fmt.Sprintf("record %d not found (CWB_KB_P_405)", req.RecordID),
			})
		}
		logger.Error("query result_filename failed", "record_id", req.RecordID, "err", err)
		return c.JSON(http.StatusInternalServerError, extractProvisionResponse{
			Status: false,
			Error:  "failed to retrieve record (CWB_KB_P_406)",
		})
	}
	if !resultFile.Valid || strings.TrimSpace(resultFile.String) == "" {
		return c.JSON(http.StatusNotFound, extractProvisionResponse{
			Status: false,
			Error:  "result_filename is empty (CWB_KB_P_407)",
		})
	}

	rawPath := rawLinePathFor(resultFile.String)
	f, err := os.Open(rawPath)
	if err != nil {
		logger.Error("open raw line file failed", "path", rawPath, "err", err)
		return c.JSON(http.StatusNotFound, extractProvisionResponse{
			Status: false,
			Error:  fmt.Sprintf("raw line file not found: %s (CWB_KB_P_408)", filepath.Base(rawPath)),
		})
	}
	defer f.Close()

	// Build set of selected line numbers from the spans.
	selectedSet := make(map[int]bool, len(req.SourceLineSpans))
	lineNums := make([]int, 0, len(req.SourceLineSpans))
	for _, s := range req.SourceLineSpans {
		if s.LineNumber >= 1 && !selectedSet[s.LineNumber] {
			selectedSet[s.LineNumber] = true
			lineNums = append(lineNums, s.LineNumber)
		}
	}
	sort.Ints(lineNums)
	if len(lineNums) == 0 {
		return c.JSON(http.StatusBadRequest, extractProvisionResponse{
			Status: false,
			Error:  "no valid line numbers in source_line_spans (CWB_KB_P_409)",
		})
	}

	// Build overlap set: 5 lines before first and after last selected line.
	minLine, maxLine := lineNums[0], lineNums[len(lineNums)-1]
	overlap := make(map[int]bool)
	for i := minLine - 5; i < minLine; i++ {
		if i >= 1 {
			overlap[i] = true
		}
	}
	for i := maxLine + 1; i <= maxLine+5; i++ {
		overlap[i] = true
	}
	for _, n := range lineNums {
		delete(overlap, n)
	}

	type composedLine struct {
		lineNumber int
		pageNumber int
		lineType   string
		content    string
		isOverlap  bool
	}

	allLines := make([]composedLine, 0, len(lineNums)+len(overlap))
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for scanner.Scan() {
		parsed, ok := parseRawLine(scanner.Text())
		if !ok {
			continue
		}
		n := parsed.LineNumber
		if selectedSet[n] {
			allLines = append(allLines, composedLine{
				lineNumber: n,
				pageNumber: parsed.PageNumber,
				lineType:   parsed.LineType,
				content:    parsed.Content,
			})
		} else if overlap[n] {
			allLines = append(allLines, composedLine{
				lineNumber: n,
				pageNumber: parsed.PageNumber,
				lineType:   parsed.LineType,
				content:    parsed.Content,
				isOverlap:  true,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Error("scan raw line file failed", "err", err)
		return c.JSON(http.StatusInternalServerError, extractProvisionResponse{
			Status: false,
			Error:  "failed to read raw line file (CWB_KB_P_410)",
		})
	}
	if len(allLines) == 0 {
		return c.JSON(http.StatusBadRequest, extractProvisionResponse{
			Status: false,
			Error:  "specified lines not found in file (CWB_KB_P_411)",
		})
	}

	// Compose LLM input: <flag>\t<line_number>\t<page_number>\t<line_type>\t<content>
	var sb strings.Builder
	for _, cl := range allLines {
		flag := "n"
		if cl.isOverlap {
			flag = "o"
		}
		sb.WriteString(fmt.Sprintf("%s\t%d\t%d\t%s\t%s\n",
			flag, cl.lineNumber, cl.pageNumber, cl.lineType, cl.content))
	}
	composedInput := sb.String()

	promptText, promptRef, err := loadProvisionsPromptForExtractFn(logger)
	if err != nil {
		logger.Error("load provisions prompt failed", "err", err, "prompt", promptRef)
		return c.JSON(http.StatusInternalServerError, extractProvisionResponse{
			Status: false,
			Error:  "failed to load provisions prompt (CWB_KB_P_412)",
		})
	}

	modelRef, modelCfgPath, modelCfg, err := loadExtractProvisionsModelConfigFn()
	if err != nil {
		logger.Error("load provisions model config failed", "err", err,
			"model_ref", modelRef, "models_file", modelCfgPath)
		return c.JSON(http.StatusInternalServerError, extractProvisionResponse{
			Status: false,
			Error:  fmt.Sprintf("failed to load provisions model config (CWB_KB_P_413): %v", err),
		})
	}
	if strings.TrimSpace(modelRef) == "" {
		logger.Error("missing env var EXTRACT_PROVISIONS_MODEL_NAME")
		return c.JSON(http.StatusInternalServerError, extractProvisionResponse{
			Status: false,
			Error:  "missing env var EXTRACT_PROVISIONS_MODEL_NAME (CWB_KB_P_414)",
		})
	}

	client, err := newExtractProvisionsClientFn(modelCfg, logger)
	if err != nil {
		logger.Error("create LLM client failed", "err", err, "model_ref", modelRef)
		return c.JSON(http.StatusInternalServerError, extractProvisionResponse{
			Status: false,
			Error:  "failed to create LLM client (CWB_KB_P_415)",
		})
	}

	logger.Info("start extracting provisions",
		"model_ref", modelRef,
		"model_name", modelCfg.ModelName,
		"prompt_name", promptRef,
		"record_id", req.RecordID,
		"span_count", len(req.SourceLineSpans),
	)

	in := llmclients.JSONExtractionInput{
		PromptText: promptText,
		ModelName:  modelCfg.ModelName,
		InputText:  composedInput,
	}
	var payload map[string]any
	if structuredClient, ok := client.(structuredProvisionJSONExtractor); ok {
		var result *llmclients.StructuredOutputResult
		result, err = structuredClient.ExtractStructuredJSON(c.Request().Context(), in, extractProvisionHandlerContract())
		if result != nil {
			payload = result.Parsed
		}
	} else {
		payload, err = client.ExtractJSON(c.Request().Context(), in)
	}
	if err != nil {
		logger.Error("LLM extraction failed", "err", err)
		return c.JSON(http.StatusInternalServerError, extractProvisionResponse{
			Status: false,
			Error:  fmt.Sprintf("LLM extraction failed (CWB_KB_P_416): %v", err),
		})
	}

	language := strings.TrimSpace(anyAsString(payload["language"]))
	provisionsRaw, ok := payload["provisions"].([]any)
	if !ok {
		logger.Error("LLM output missing provisions array")
		return c.JSON(http.StatusInternalServerError, extractProvisionResponse{
			Status: false,
			Error:  "LLM output missing provisions array (CWB_KB_P_417)",
		})
	}

	provisions := normalizeExtractedProvisions(provisionsRaw)
	logger.Info("extracted provisions ready for review",
		"record_id", req.RecordID, "count", len(provisions), "language", language)

	return c.JSON(http.StatusOK, extractProvisionResponse{
		Status:     true,
		Provisions: provisions,
		Language:   language,
	})
}

// SaveExtractedProvisions handles POST /api/v1/kb/provisions/save.
// It persists the reviewed provisions returned from the extract API.
func SaveExtractedProvisions(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_P_430")
	defer rc.Close()
	logger := rc.GetLogger()

	var req saveExtractedProvisionsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, saveExtractedProvisionsResponse{
			Status: false,
			Error:  fmt.Sprintf("invalid request body (CWB_KB_P_431): %v", err),
		})
	}
	if req.RecordID <= 0 {
		return c.JSON(http.StatusBadRequest, saveExtractedProvisionsResponse{
			Status: false,
			Error:  "record_id must be a positive integer (CWB_KB_P_432)",
		})
	}
	if len(req.Provisions) == 0 {
		return c.JSON(http.StatusBadRequest, saveExtractedProvisionsResponse{
			Status: false,
			Error:  "provisions must not be empty (CWB_KB_P_433)",
		})
	}

	inserted, err := saveExtractedProvisions(ApiTypes.ProjectDBHandle, req.RecordID, req.Provisions)
	if err != nil {
		logger.Error("save extracted provisions failed", "record_id", req.RecordID, "err", err)
		return c.JSON(http.StatusInternalServerError, saveExtractedProvisionsResponse{
			Status: false,
			Error:  fmt.Sprintf("failed to save provisions (CWB_KB_P_434): %v", err),
		})
	}

	logger.Info("provisions saved", "record_id", req.RecordID, "inserted", inserted)
	return c.JSON(http.StatusOK, saveExtractedProvisionsResponse{
		Status:   true,
		Inserted: inserted,
	})
}

func saveExtractedProvisions(db *sql.DB, inputRecordID int64, provisions []map[string]any) (int64, error) {
	var maxProvID int
	if err := db.QueryRow(
		`SELECT COALESCE(MAX(prov_id), 0) FROM kb.provisions WHERE input_record_id = $1`,
		inputRecordID,
	).Scan(&maxProvID); err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("query max prov_id: %w", err)
	}

	const stmt = `
INSERT INTO kb.provisions (
	input_record_id, extract_id, input_filename,
	prov_id, prov_name, prov_name_en,
	provision_type, source_text, source_line_spans,
	provision, provision_en,
	provision_subject, provision_subject_en,
	prov_desc, prov_desc_en,
	prov_context, prov_context_en,
	provision_keywords, provision_keywords_en,
	category_paths, category_paths_en,
	location_type, confidence, is_explicit, need_verify,
	num_blocks, num_provisions, time_per_provision,
	model_name, prompt_name, status,
	public_info, private_info, notes, error_msg
) VALUES (
	$1,$2,$3,
	$4,$5,$6,
	$7,$8,$9::jsonb,
	$10,$11,
	$12,$13,
	$14,$15,
	$16,$17,
	$18::jsonb,$19::jsonb,
	$20::jsonb,$21::jsonb,
	$22,$23,$24,$25,
	$26,$27,$28,
	$29,$30,$31,
	$32::jsonb,$33::jsonb,$34,$35
)
ON CONFLICT (input_record_id, prov_id) DO UPDATE SET
	prov_name             = EXCLUDED.prov_name,
	prov_name_en          = EXCLUDED.prov_name_en,
	provision_type        = EXCLUDED.provision_type,
	source_text           = EXCLUDED.source_text,
	source_line_spans     = EXCLUDED.source_line_spans,
	provision             = EXCLUDED.provision,
	provision_en          = EXCLUDED.provision_en,
	provision_subject     = EXCLUDED.provision_subject,
	provision_subject_en  = EXCLUDED.provision_subject_en,
	prov_desc             = EXCLUDED.prov_desc,
	prov_desc_en          = EXCLUDED.prov_desc_en,
	prov_context          = EXCLUDED.prov_context,
	prov_context_en       = EXCLUDED.prov_context_en,
	provision_keywords    = EXCLUDED.provision_keywords,
	provision_keywords_en = EXCLUDED.provision_keywords_en,
	category_paths        = EXCLUDED.category_paths,
	category_paths_en     = EXCLUDED.category_paths_en,
	location_type         = EXCLUDED.location_type,
	confidence            = EXCLUDED.confidence,
	is_explicit           = EXCLUDED.is_explicit,
	need_verify           = EXCLUDED.need_verify,
	model_name            = EXCLUDED.model_name,
	prompt_name           = EXCLUDED.prompt_name,
	public_info           = EXCLUDED.public_info,
	modify_time           = NOW()`

	extractID := time.Now().Format("20060102-150405") + "-rest-api"
	var inserted int64
	for i, p := range provisions {
		provID := maxProvID + i + 1
		spansJSON, _ := json.Marshal(p["source_line_spans"])
		keywordsJSON, _ := json.Marshal(p["provision_keywords"])
		keywordsEnJSON, _ := json.Marshal(p["provision_keywords_en"])
		categoryPathsJSON, _ := json.Marshal(p["category_paths"])
		categoryPathsEnJSON, _ := json.Marshal(p["category_paths_en"])
		publicInfoJSON, _ := json.Marshal(map[string]any{
			"provision_type":    anyAsString(p["provision_type"]),
			"source_line_spans": p["source_line_spans"],
		})
		privateInfoJSON, _ := json.Marshal(map[string]any{})

		provisionText := strings.TrimSpace(anyAsString(p["provision"]))
		var provisionValue any
		if provisionText != "" {
			provisionValue = provisionText
		}

		_, err := db.Exec(stmt,
			inputRecordID,                                                           // $1
			extractID,                                                               // $2
			"",                                                                      // $3 input_filename
			provID,                                                                  // $4
			strings.TrimSpace(anyAsString(p["prov_name"])),                          // $5
			strings.TrimSpace(anyAsString(p["prov_name_en"])),                       // $6
			strings.TrimSpace(anyAsString(p["provision_type"])),                     // $7
			strings.TrimSpace(anyAsString(p["source_text"])),                        // $8
			string(spansJSON),                                                       // $9
			provisionValue,                                                          // $10
			strings.TrimSpace(anyAsString(p["provision_en"])),                       // $11
			strings.TrimSpace(anyAsString(p["provision_subject"])),                  // $12
			strings.TrimSpace(anyAsString(p["provision_subject_en"])),               // $13
			strings.TrimSpace(anyAsString(p["prov_desc"])),                          // $14
			strings.TrimSpace(anyAsString(p["prov_desc_en"])),                       // $15
			strings.TrimSpace(anyAsString(p["prov_context"])),                       // $16
			strings.TrimSpace(anyAsString(p["prov_context_en"])),                    // $17
			string(keywordsJSON),                                                    // $18
			string(keywordsEnJSON),                                                  // $19
			string(categoryPathsJSON),                                               // $20
			string(categoryPathsEnJSON),                                             // $21
			strings.TrimSpace(anyAsString(p["location_type"])),                      // $22
			confidenceVal(p, "confidence"),                                          // $23
			boolVal(p, "is_explicit"),                                               // $24
			boolVal(p, "need_verify"),                                               // $25
			0,                                                                       // $26 num_blocks
			len(provisions),                                                         // $27 num_provisions
			float64(0),                                                              // $28 time_per_provision
			"",                                                                      // $29 model_name
			"",                                                                      // $30 prompt_name
			"active",                                                                // $31 status
			string(publicInfoJSON),                                                  // $32
			string(privateInfoJSON),                                                 // $33
			"",                                                                      // $34 notes
			"",                                                                      // $35 error_msg
		)
		if err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}

func normalizeExtractedProvisions(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		provisionText := firstNonEmptyVal(raw, "provision", "provision_original")
		out = append(out, map[string]any{
			"prov_name":            firstNonEmptyVal(raw, "provision_name", "name"),
			"prov_name_en":         stringVal(raw, "name_en"),
			"provision_type":       firstNonEmptyVal(raw, "provision_type", "type"),
			"source_text":          firstNonEmptyVal(raw, "source_text", "provision", "provision_original"),
			"source_line_spans":    raw["source_line_spans"],
			"provision":            provisionText,
			"provision_en":         stringVal(raw, "provision_en"),
			"provision_subject":    firstNonEmptyVal(raw, "provision_subject", "subject"),
			"provision_subject_en": firstNonEmptyVal(raw, "provision_subject_en", "subject_en"),
			"prov_desc":            firstNonEmptyVal(raw, "provision_desc", "prov_desc"),
			"prov_desc_en":         firstNonEmptyVal(raw, "provision_desc_en", "prov_desc_en"),
			"prov_context":         firstNonEmptyVal(raw, "context", "prov_context"),
			"prov_context_en":      firstNonEmptyVal(raw, "context_en", "prov_context_en"),
			"provision_keywords":   anySlice(raw, "keywords", "provision_keywords"),
			"provision_keywords_en": anySlice(raw, "keywords_en", "provision_keywords_en"),
			"category_paths":       raw["category_paths"],
			"category_paths_en":    provisionFirstAny(raw, "category_path_en", "category_paths_en"),
			"location_type":        stringVal(raw, "location_type"),
			"confidence":           confidenceVal(raw, "confidence"),
			"is_explicit":          boolVal(raw, "is_explicit"),
			"need_verify":          boolVal(raw, "need_verify"),
		})
	}
	return out
}

func provisionFirstAny(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func loadProvisionsPromptForExtract(logger ApiTypes.JimoLogger) (string, string, error) {
	ref := strings.TrimSpace(os.Getenv("EXTRACT_PROVISIONS_PROMPT"))
	if ref == "" {
		ref = strings.TrimSpace(os.Getenv("PROMPT_FILE_NAME"))
	}
	if ref == "" {
		ref = "prompt-extract-provisions.md"
	}

	var candidates []string
	if filepath.IsAbs(ref) {
		candidates = []string{ref}
	} else {
		candidates = []string{ref}
		if promptDir := strings.TrimSpace(os.Getenv("PROMPT_DIR")); promptDir != "" {
			candidates = append(candidates, filepath.Join(promptDir, ref))
		}
		candidates = append(candidates,
			"prompts/"+ref,
			filepath.Join("server", "cmd", "doc-processor", ref),
			filepath.Join("server", "cmd", "doc-processor", "prompts", ref),
		)
	}

	for _, candidate := range candidates {
		bs, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(bs))
		if text != "" {
			logger.Info("loaded provisions prompt", "path", candidate, "ref", ref)
			return text, ref, nil
		}
	}
	return "", ref, fmt.Errorf("(CWB_KB_P_418) provisions prompt not found: %q", ref)
}

func loadExtractProvisionsModelConfig() (modelRef string, modelPath string, cfg ApiTypes.LLMModelDef, err error) {
	modelRef = strings.TrimSpace(os.Getenv("EXTRACT_PROVISIONS_MODEL_NAME"))
	if modelRef == "" {
		return "", "", ApiTypes.LLMModelDef{}, nil
	}

	modelPath, err = resolveModelsFilePathForKBHandler("EXTRACT_PROVISIONS_MODELS_FILE")
	if err != nil {
		return modelRef, "", ApiTypes.LLMModelDef{}, err
	}

	raw, err := os.ReadFile(modelPath)
	if err != nil {
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(CWB_KB_P_419) read %s: %w", modelPath, err)
	}

	parsed := ApiTypes.LLMModelsFile{}
	if err := toml.Unmarshal(raw, &parsed); err != nil {
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(CWB_KB_P_420) parse %s: %w", modelPath, err)
	}

	modelDef, ok := parsed[modelRef]
	if !ok {
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(CWB_KB_P_421) model %q not found in %s", modelRef, modelPath)
	}

	modelDef.ModelName = strings.TrimSpace(modelDef.ModelName)
	modelDef.APIKey = strings.TrimSpace(modelDef.APIKey)
	modelDef.BaseURL = strings.TrimSpace(modelDef.BaseURL)
	modelDef.ThinkingType = strings.TrimSpace(modelDef.ThinkingType)

	switch {
	case modelDef.ModelName == "":
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(CWB_KB_P_422) model %q missing model_name", modelRef)
	case modelDef.APIKey == "":
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(CWB_KB_P_423) model %q missing api_key", modelRef)
	case modelDef.BaseURL == "":
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(CWB_KB_P_424) model %q missing base_url", modelRef)
	case modelDef.TimeoutSec <= 0:
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(CWB_KB_P_425) model %q invalid timeout_sec", modelRef)
	}

	return modelRef, modelPath, modelDef, nil
}
