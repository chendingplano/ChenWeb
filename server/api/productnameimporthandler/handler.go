package productnameimporthandler

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
	toml "github.com/pelletier/go-toml/v2"
)

const source = "china-mechanical"
const maxUploadBytes = 20 << 20
const translationBatchSize = 50

var logger = loggerutil.CreateDefaultLogger("20260925-407")

type csvRecord struct {
	seqNo          int
	subCatalog     string
	categoryL1     string
	categoryL2     string
	productName    string
	notes          string
	codeGroup      string
	childCodeGroup string
	industryCode   string
	cpc            string
	entryNo        string
	entryNoNew     string
	productNameEn  string
}

type response struct {
	Status   bool   `json:"status"`
	Error    string `json:"error,omitempty"`
	RowCount int    `json:"row_count,omitempty"`
	Inserted int64  `json:"inserted,omitempty"`
	Skipped  int64  `json:"skipped,omitempty"`
}

func Preview(c echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	rows, err := readUpload(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, response{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, response{Status: true, RowCount: len(rows)})
}

func Import(c echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	logger.Info("mechanical product-name import started")
	rows, err := readUpload(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, response{Error: err.Error()})
	}
	if len(rows) == 0 {
		return c.JSON(http.StatusBadRequest, response{Error: "CSV contains no data rows"})
	}
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, response{Error: "project database is unavailable"})
	}
	existingRows, err := db.QueryContext(c.Request().Context(), `SELECT seq_no, product_name FROM kb.product_names WHERE source = $1`, source)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, response{Error: "could not inspect existing product names"})
	}
	existing := make(map[string]struct{})
	for existingRows.Next() {
		var seqNo int
		var name string
		if err := existingRows.Scan(&seqNo, &name); err != nil {
			existingRows.Close()
			return c.JSON(http.StatusInternalServerError, response{Error: "could not read existing product names"})
		}
		existing[fmt.Sprintf("%d\x00%s", seqNo, name)] = struct{}{}
	}
	if err := existingRows.Err(); err != nil {
		existingRows.Close()
		return c.JSON(http.StatusInternalServerError, response{Error: "could not read existing product names"})
	}
	if err := existingRows.Close(); err != nil {
		return c.JSON(http.StatusInternalServerError, response{Error: "could not close existing product-name query"})
	}
	rowsToInsert := make([]csvRecord, 0, len(rows))
	for _, row := range rows {
		if _, ok := existing[fmt.Sprintf("%d\x00%s", row.seqNo, row.productName)]; !ok {
			rowsToInsert = append(rowsToInsert, row)
		}
	}
	if len(rowsToInsert) == 0 {
		return c.JSON(http.StatusOK, response{Status: true, RowCount: len(rows), Skipped: int64(len(rows))})
	}

	translations, err := translateNames(c.Request().Context(), rowsToInsert, c)
	if err != nil {
		return c.JSON(http.StatusBadGateway, response{Error: "product-name translation failed: " + err.Error()})
	}
	for i := range rowsToInsert {
		rowsToInsert[i].productNameEn = translations[i]
	}

	tx, err := db.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, response{Error: "could not begin product-name import"})
	}
	defer tx.Rollback()
	var inserted int64
	for _, row := range rowsToInsert {
		result, execErr := tx.ExecContext(c.Request().Context(), `
			INSERT INTO kb.product_names (
				seq_no, sub_catalog, category_l1, category_l2, product_name,
				product_name_en, source, notes, code_group, child_code_group,
				industry_code, cpc, entry_no, entry_no_new, status
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'approved')
			ON CONFLICT (source, seq_no, product_name) DO NOTHING`,
			row.seqNo, row.subCatalog, row.categoryL1, row.categoryL2, row.productName,
			row.productNameEn, source, row.notes, row.codeGroup, row.childCodeGroup,
			row.industryCode, row.cpc, row.entryNo, row.entryNoNew)
		if execErr != nil {
			return c.JSON(http.StatusInternalServerError, response{Error: fmt.Sprintf("could not insert source row %d", row.seqNo)})
		}
		n, _ := result.RowsAffected()
		inserted += n
	}
	if err := tx.Commit(); err != nil {
		return c.JSON(http.StatusInternalServerError, response{Error: "could not commit product-name import"})
	}
	logger.Info("mechanical product-name import completed", "rows", len(rows), "inserted", inserted, "skipped", len(rowsToInsert)-int(inserted))
	return c.JSON(http.StatusOK, response{Status: true, RowCount: len(rows), Inserted: inserted, Skipped: int64(len(rows)) - inserted})
}

func requireAdmin(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_PNI_010")
	defer rc.Close()
	user := rc.IsAuthenticated()
	if user == nil {
		return c.JSON(http.StatusUnauthorized, response{Error: "authentication required"})
	}
	admin := user.IsOwner || user.Admin
	for _, role := range user.Roles {
		role = strings.ToLower(strings.TrimSpace(role))
		admin = admin || role == "admin" || role == "root"
	}
	if !admin {
		return c.JSON(http.StatusForbidden, response{Error: "admin access required"})
	}
	return nil
}

func readUpload(c echo.Context) ([]csvRecord, error) {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxUploadBytes+(1<<20))
	file, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, errors.New("CSV file exceeds 20 MB")
		}
		return nil, errors.New("select a CSV file first")
	}
	if file.Size > maxUploadBytes {
		return nil, errors.New("CSV file exceeds 20 MB")
	}
	in, err := file.Open()
	if err != nil {
		return nil, errors.New("could not open uploaded CSV")
	}
	defer in.Close()
	data, err := io.ReadAll(io.LimitReader(in, maxUploadBytes+1))
	if err != nil || len(data) > maxUploadBytes {
		return nil, errors.New("could not read uploaded CSV or file exceeds 20 MB")
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff")))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, errors.New("CSV header is missing")
	}
	indexes := make(map[string]int, len(header))
	for i, name := range header {
		indexes[strings.TrimSpace(name)] = i
	}
	for _, required := range []string{"code_class_large", "code_class_medium", "code_class_small", "product_name", "note", "code_group", "industry_code", "cpc", "entry_no", "entry_no_new"} {
		if _, ok := indexes[required]; !ok {
			return nil, fmt.Errorf("required CSV column %q is missing", required)
		}
	}
	childColumn := "child_code_group"
	if _, ok := indexes[childColumn]; !ok {
		return nil, errors.New("required CSV column child_code_group is missing")
	}
	get := func(record []string, key string) string {
		idx, ok := indexes[key]
		if !ok || idx >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[idx])
	}
	rows := make([]csvRecord, 0)
	for sourceRow := 2; ; sourceRow++ {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("invalid CSV row %d: %w", sourceRow, readErr)
		}
		name := get(record, "product_name")
		if name == "" {
			return nil, fmt.Errorf("product_name is empty at CSV row %d", sourceRow)
		}
		rows = append(rows, csvRecord{
			seqNo: len(rows) + 1, subCatalog: get(record, "code_class_large"),
			categoryL1: get(record, "code_class_medium"), categoryL2: get(record, "code_class_small"),
			productName: name, notes: get(record, "note"), codeGroup: get(record, "code_group"),
			childCodeGroup: get(record, childColumn), industryCode: get(record, "industry_code"),
			cpc: get(record, "cpc"), entryNo: get(record, "entry_no"), entryNoNew: get(record, "entry_no_new"),
		})
	}
	return rows, nil
}

func translateNames(ctx context.Context, rows []csvRecord, c echo.Context) ([]string, error) {
	modelRef := strings.TrimSpace(os.Getenv("TRANSLATION_MODEL_NAME"))
	if modelRef == "" {
		return nil, errors.New("TRANSLATION_MODEL_NAME is not configured")
	}
	modelsPath := strings.TrimSpace(os.Getenv("MODEL_DEF_FILE"))
	if modelsPath == "" {
		return nil, errors.New("MODEL_DEF_FILE is not configured")
	}
	if !filepath.IsAbs(modelsPath) {
		modelsPath = filepath.Clean(modelsPath)
	}
	raw, err := os.ReadFile(modelsPath)
	if err != nil {
		return nil, fmt.Errorf("read model definitions: %w", err)
	}
	models := ApiTypes.LLMModelsFile{}
	if err := toml.Unmarshal(raw, &models); err != nil {
		return nil, fmt.Errorf("parse model definitions: %w", err)
	}
	model, ok := models[modelRef]
	if !ok || strings.TrimSpace(model.ModelName) == "" || strings.TrimSpace(model.APIKey) == "" || strings.TrimSpace(model.BaseURL) == "" || model.TimeoutSec <= 0 {
		return nil, fmt.Errorf("translation model %q is missing or incomplete", modelRef)
	}
	llmclients.RegisterModelBudget(model)
	rc := EchoFactory.NewFromEcho(c, "CWB_PNI_020")
	defer rc.Close()
	userID := strings.TrimSpace(rc.GetUserID())
	if userID == "" {
		return nil, errors.New("authenticated user ID is unavailable for translation usage attribution")
	}
	client, err := llmclients.NewOpenAIJSONClientFromConfig(llmclients.OpenAIJSONClientConfig{
		ModelName: model.ModelName, APIKey: model.APIKey, BaseURL: model.BaseURL,
		ProfileName: modelRef, TimeoutSec: model.TimeoutSec, ThinkingType: model.ThinkingType,
		MaxInflight: model.MaxInflight, MaxRequestsPerMinute: model.MaxRequestsPerMinute,
		MaxTokensPerMinute: model.MaxTokensPerMinute, TokenReservePerCall: model.TokenReservePerCall,
	}, rc.GetLogger())
	if err != nil {
		return nil, fmt.Errorf("create translation client: %w", err)
	}
	prompt, err := readTranslationPrompt()
	if err != nil {
		return nil, err
	}
	translated := make([]string, len(rows))
	for start := 0; start < len(rows); start += translationBatchSize {
		end := min(start+translationBatchSize, len(rows))
		products := make([]map[string]any, 0, end-start)
		for idx := start; idx < end; idx++ {
			products = append(products, map[string]any{"idx": idx - start, "product_name": rows[idx].productName})
		}
		input, _ := json.Marshal(map[string]any{"products": products})
		out, callErr := client.ExtractJSON(ctx, llmclients.JSONExtractionInput{
			UserID: userID, PromptName: "translate_china_mechanical_product_names_v1", PromptText: string(prompt), ModelName: model.ModelName,
			InputText: string(input), CallReason: "translate_china_mechanical_product_names", CallLoc: "CWB_PNI_020",
		})
		if callErr != nil {
			return nil, fmt.Errorf("batch %d: %w", start/translationBatchSize+1, callErr)
		}
		items, _ := out["products"].([]any)
		if len(items) != end-start {
			return nil, fmt.Errorf("batch %d returned %d translations for %d names", start/translationBatchSize+1, len(items), end-start)
		}
		seen := make([]bool, end-start)
		for _, item := range items {
			m, _ := item.(map[string]any)
			idxFloat, _ := m["idx"].(float64)
			idx := int(idxFloat)
			name, _ := m["product_name_en"].(string)
			if idxFloat != float64(idx) || idx < 0 || idx >= end-start || strings.TrimSpace(name) == "" || seen[idx] {
				return nil, fmt.Errorf("batch %d returned an invalid translation row", start/translationBatchSize+1)
			}
			translated[start+idx] = strings.TrimSpace(name)
			seen[idx] = true
		}
		for _, ok := range seen {
			if !ok {
				return nil, fmt.Errorf("batch %d omitted a translation", start/translationBatchSize+1)
			}
		}
	}
	return translated, nil
}

func readTranslationPrompt() ([]byte, error) {
	const name = "prompt-translate-china-mechanical-product-names-v1.md"
	if promptDir := strings.TrimSpace(os.Getenv("PROMPT_DIR")); promptDir != "" {
		if data, err := os.ReadFile(filepath.Join(promptDir, name)); err == nil {
			return data, nil
		}
	}
	data, err := os.ReadFile(filepath.Join("prompts", name))
	if err != nil {
		return nil, fmt.Errorf("read translation prompt %q: %w", name, err)
	}
	return data, nil
}
