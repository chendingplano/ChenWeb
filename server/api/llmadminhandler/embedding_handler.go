package llmadminhandler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
)

type embeddingRequest struct {
	ModelKey string `json:"model_key"`
	Content  string `json:"content"`
	Save     bool   `json:"save"`
	TopN     int    `json:"top_n"`
}

type embeddingRecord struct {
	ID        int64     `json:"id"`
	ModelKey  string    `json:"model_key"`
	ModelName string    `json:"model_name"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	TimeMS    int64     `json:"time_ms"`
	NumChars  int       `json:"num_chars"`
	NumTokens int       `json:"num_tokens"`
}

type embeddingResult struct {
	vector    []float64
	timeMS    int64
	numChars  int
	numTokens int
}

type embeddingInputError struct{ message string }

func (e embeddingInputError) Error() string { return e.message }

type embeddingModelConfig struct {
	key string
	cfg ApiTypes.LLMModelDef
}

func selectedEmbeddingModel(key string) (embeddingModelConfig, error) {
	models, err := readModelsTOML(modelsTOMLPath())
	if err != nil {
		return embeddingModelConfig{}, err
	}
	cfg, ok := models[strings.TrimSpace(key)]
	if !ok || cfg.ModelType != "embedding" {
		return embeddingModelConfig{}, fmt.Errorf("selected model is not an embedding model")
	}
	if cfg.Dimension != 768 && cfg.Dimension != 1024 && cfg.Dimension != 1536 {
		return embeddingModelConfig{}, fmt.Errorf("embedding model dimension %d is unsupported; supported dimensions are 768, 1024, and 1536", cfg.Dimension)
	}
	if strings.TrimSpace(cfg.ModelName) == "" {
		return embeddingModelConfig{}, fmt.Errorf("embedding model name is empty")
	}
	return embeddingModelConfig{key: strings.TrimSpace(key), cfg: cfg}, nil
}

func embeddingTable(dimension int) (string, error) {
	switch dimension {
	case 768:
		return "testbed.embedding_768", nil
	case 1024:
		return "testbed.embedding_1024", nil
	case 1536:
		return "testbed.embedding_1536", nil
	default:
		return "", fmt.Errorf("unsupported embedding dimension %d", dimension)
	}
}

func embeddingClient(model embeddingModelConfig) *llmclients.OpenAIJSONClient {
	timeout := model.cfg.TimeoutSec
	if timeout <= 0 {
		timeout = 60
	}
	return &llmclients.OpenAIJSONClient{
		ModelName:           strings.TrimSpace(model.cfg.ModelName),
		APIKey:              strings.TrimSpace(model.cfg.APIKey),
		BaseURL:             strings.TrimSpace(model.cfg.BaseURL),
		ProfileName:         model.key,
		EmbeddingDimensions: kbsearch.EmbeddingDimensionsForModel(model.cfg.ModelName, model.cfg.BaseURL),
		HTTPClient:          &http.Client{Timeout: time.Duration(timeout) * time.Second},
	}
}

func embeddingRequestUserID(c echo.Context) string {
	rc := EchoFactory.NewFromEcho(c, "CWB_EMBED_USER_ID")
	defer rc.Close()
	return strings.TrimSpace(rc.GetUserID())
}

func embeddingMaxChars(model embeddingModelConfig) int {
	if model.cfg.MaxChars > 0 {
		return model.cfg.MaxChars
	}
	return 6000
}

func embedText(ctx context.Context, model embeddingModelConfig, content, userID, callReason string) (embeddingResult, error) {
	if strings.TrimSpace(content) == "" {
		return embeddingResult{}, embeddingInputError{message: "content is required"}
	}
	numChars := utf8.RuneCountInString(content)
	if numChars > embeddingMaxChars(model) {
		return embeddingResult{}, embeddingInputError{message: fmt.Sprintf("content has %d characters; selected model accepts at most %d", numChars, embeddingMaxChars(model))}
	}
	client := embeddingClient(model)
	started := time.Now()
	vec, err := client.Embed(ctx, llmclients.EmbedInput{
		UserID:     userID,
		ModelName:  model.cfg.ModelName,
		InputText:  content,
		CallReason: callReason,
		CallLoc:    "CWB_LLM_EMBEDDING",
	})
	if err != nil {
		return embeddingResult{}, err
	}
	if len(vec) != model.cfg.Dimension {
		return embeddingResult{}, fmt.Errorf("embedding model returned dimension %d, configured dimension is %d", len(vec), model.cfg.Dimension)
	}
	numTokens := 0
	if usage := client.LastEmbeddingUsage(); usage != nil {
		numTokens = usage.InputTokens
	}
	if numTokens <= 0 {
		numTokens = llmclients.EstimateEmbeddingTokens(content)
	}
	return embeddingResult{vector: vec, timeMS: time.Since(started).Milliseconds(), numChars: numChars, numTokens: numTokens}, nil
}

func embeddingErrorStatus(err error) int {
	var inputErr embeddingInputError
	if errors.As(err, &inputErr) {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func embeddingDB(c echo.Context) (*sql.DB, error) {
	if ApiTypes.ProjectDBHandle == nil {
		return nil, fmt.Errorf("project database is not initialized")
	}
	return ApiTypes.ProjectDBHandle, nil
}

func CreateEmbedding(c echo.Context) error {
	var req embeddingRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "invalid embedding request"})
	}
	model, err := selectedEmbeddingModel(req.ModelKey)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": err.Error()})
	}
	userID := embeddingRequestUserID(c)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{"message": "authenticated user is required"})
	}
	embedded, err := embedText(c.Request().Context(), model, req.Content, userID, "embedding_test")
	if err != nil {
		return c.JSON(embeddingErrorStatus(err), map[string]any{"message": "embedding request failed", "error": err.Error()})
	}
	result := map[string]any{"model_key": model.key, "model_name": model.cfg.ModelName, "dimension": len(embedded.vector), "embedding": embedded.vector, "time_ms": embedded.timeMS, "num_chars": embedded.numChars, "num_tokens": embedded.numTokens}
	if req.Save {
		db, dbErr := embeddingDB(c)
		if dbErr != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": dbErr.Error()})
		}
		table, _ := embeddingTable(model.cfg.Dimension)
		var id int64
		err = db.QueryRowContext(c.Request().Context(), "INSERT INTO "+table+" (model_key, model_name, content, embedding, time_ms, num_chars, num_tokens) VALUES ($1, $2, $3, $4::vector, $5, $6, $7) RETURNING id", model.key, model.cfg.ModelName, req.Content, kbsearch.FormatVectorLiteral(embedded.vector), embedded.timeMS, embedded.numChars, embedded.numTokens).Scan(&id)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"message": "failed to save embedding", "error": err.Error()})
		}
		result["record_id"] = id
	}
	return c.JSON(http.StatusOK, result)
}

func SearchEmbeddingSimilarity(c echo.Context) error {
	var req embeddingRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "invalid similarity request"})
	}
	model, err := selectedEmbeddingModel(req.ModelKey)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": err.Error()})
	}
	userID := embeddingRequestUserID(c)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{"message": "authenticated user is required"})
	}
	embedded, err := embedText(c.Request().Context(), model, req.Content, userID, "embedding_similarity_search")
	if err != nil {
		return c.JSON(embeddingErrorStatus(err), map[string]any{"message": "embedding request failed", "error": err.Error()})
	}
	db, err := embeddingDB(c)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": err.Error()})
	}
	table, _ := embeddingTable(model.cfg.Dimension)
	topN := req.TopN
	if topN < 1 {
		topN = 10
	}
	rows, err := db.QueryContext(c.Request().Context(), "SELECT id, model_key, model_name, content, created_at, updated_at, time_ms, num_chars, num_tokens, 1 - (embedding <=> $1::vector) AS similarity FROM "+table+" WHERE model_key = $2 ORDER BY embedding <=> $1::vector LIMIT $3", kbsearch.FormatVectorLiteral(embedded.vector), model.key, topN)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": "similarity search failed", "error": err.Error()})
	}
	defer rows.Close()
	type match struct {
		embeddingRecord
		Similarity float64 `json:"similarity"`
	}
	matches := []match{}
	for rows.Next() {
		var item match
		if err := rows.Scan(&item.ID, &item.ModelKey, &item.ModelName, &item.Content, &item.CreatedAt, &item.UpdatedAt, &item.TimeMS, &item.NumChars, &item.NumTokens, &item.Similarity); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"message": err.Error()})
		}
		matches = append(matches, item)
	}
	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"model_key": model.key, "matches": matches})
}

func ListEmbeddingRecords(c echo.Context) error {
	dimension := positiveQueryInt(c, "dimension", 0)
	table, err := embeddingTable(dimension)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": err.Error()})
	}
	db, err := embeddingDB(c)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": err.Error()})
	}
	modelKey := strings.TrimSpace(c.QueryParam("model_key"))
	page, limit := positiveQueryInt(c, "page", 1), positiveQueryInt(c, "limit", 20)
	var total int
	if err := db.QueryRowContext(c.Request().Context(), "SELECT COUNT(*) FROM "+table+" WHERE ($1 = '' OR model_key = $1)", modelKey).Scan(&total); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": err.Error()})
	}
	rows, err := db.QueryContext(c.Request().Context(), "SELECT id, model_key, model_name, content, created_at, updated_at, time_ms, num_chars, num_tokens FROM "+table+" WHERE ($1 = '' OR model_key = $1) ORDER BY id DESC LIMIT $2 OFFSET $3", modelKey, limit, (page-1)*limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": err.Error()})
	}
	defer rows.Close()
	items := []embeddingRecord{}
	for rows.Next() {
		var row embeddingRecord
		if err := rows.Scan(&row.ID, &row.ModelKey, &row.ModelName, &row.Content, &row.CreatedAt, &row.UpdatedAt, &row.TimeMS, &row.NumChars, &row.NumTokens); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"message": err.Error()})
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"records": items, "total": total, "page": page, "limit": limit, "dimension": dimension})
}

func positiveQueryInt(c echo.Context, key string, fallback int) int {
	var n int
	if _, err := fmt.Sscanf(c.QueryParam(key), "%d", &n); err != nil || n < 1 {
		return fallback
	}
	return n
}

func UpdateEmbeddingRecord(c echo.Context) error     { return mutateEmbeddingRecord(c, true) }
func RegenerateEmbeddingRecord(c echo.Context) error { return mutateEmbeddingRecord(c, false) }
func mutateEmbeddingRecord(c echo.Context, updateContent bool) error {
	id := c.Param("id")
	var req embeddingRequest
	if updateContent {
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"message": "invalid record"})
		}
	} else {
		req.ModelKey = c.QueryParam("model_key")
	}
	model, err := selectedEmbeddingModel(req.ModelKey)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": err.Error()})
	}
	userID := embeddingRequestUserID(c)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{"message": "authenticated user is required"})
	}
	db, err := embeddingDB(c)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": err.Error()})
	}
	table, _ := embeddingTable(model.cfg.Dimension)
	content := req.Content
	if !updateContent {
		if err := db.QueryRowContext(c.Request().Context(), "SELECT content FROM "+table+" WHERE id=$1 AND model_key=$2", id, model.key).Scan(&content); err != nil {
			return c.JSON(http.StatusNotFound, map[string]any{"message": "embedding record not found"})
		}
	}
	callReason := "embedding_regenerate"
	if updateContent {
		callReason = "embedding_edit"
	}
	embedded, err := embedText(c.Request().Context(), model, content, userID, callReason)
	if err != nil {
		return c.JSON(embeddingErrorStatus(err), map[string]any{"message": "embedding request failed", "error": err.Error()})
	}
	_, err = db.ExecContext(c.Request().Context(), "UPDATE "+table+" SET content=$1, embedding=$2::vector, time_ms=$3, num_chars=$4, num_tokens=$5, updated_at=NOW() WHERE id=$6 AND model_key=$7", content, kbsearch.FormatVectorLiteral(embedded.vector), embedded.timeMS, embedded.numChars, embedded.numTokens, id, model.key)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": "failed to update embedding", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func DeleteEmbeddingRecord(c echo.Context) error {
	model, err := selectedEmbeddingModel(c.QueryParam("model_key"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": err.Error()})
	}
	db, err := embeddingDB(c)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": err.Error()})
	}
	table, _ := embeddingTable(model.cfg.Dimension)
	_, err = db.ExecContext(c.Request().Context(), "DELETE FROM "+table+" WHERE id=$1 AND model_key=$2", c.Param("id"), model.key)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": "failed to delete embedding", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func ClearEmbeddingRecords(c echo.Context) error {
	table, err := embeddingTable(positiveQueryInt(c, "dimension", 0))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": err.Error()})
	}
	db, err := embeddingDB(c)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": err.Error()})
	}
	_, err = db.ExecContext(c.Request().Context(), "DELETE FROM "+table)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"message": "failed to clear embeddings", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func CompareEmbeddings(c echo.Context) error {
	var req struct {
		ModelKey string  `json:"model_key"`
		IDs      []int64 `json:"ids"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "invalid comparison request"})
	}
	if len(req.IDs) < 2 || len(req.IDs) > 5 {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": "select between 2 and 5 records"})
	}
	model, err := selectedEmbeddingModel(req.ModelKey)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"message": err.Error()})
	}
	db, err := embeddingDB(c)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"message": err.Error()})
	}
	table, _ := embeddingTable(model.cfg.Dimension)
	vectors := make([][]float64, len(req.IDs))
	for i, id := range req.IDs {
		var literal string
		if err := db.QueryRowContext(c.Request().Context(), "SELECT embedding::text FROM "+table+" WHERE id=$1 AND model_key=$2", id, model.key).Scan(&literal); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"message": "all selected records must belong to the selected model"})
		}
		vectors[i], err = parseVectorLiteral(literal)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"message": "invalid stored vector"})
		}
	}
	pairs := []map[string]any{}
	for i := 0; i < len(req.IDs); i++ {
		for j := i + 1; j < len(req.IDs); j++ {
			pairs = append(pairs, map[string]any{"id_a": req.IDs[i], "id_b": req.IDs[j], "similarity": cosineSimilarity(vectors[i], vectors[j])})
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"pairs": pairs})
}

func parseVectorLiteral(s string) ([]float64, error) {
	s = strings.Trim(strings.TrimSpace(s), "[]")
	if s == "" {
		return nil, fmt.Errorf("empty vector")
	}
	parts := strings.Split(s, ",")
	values := make([]float64, len(parts))
	for i, p := range parts {
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%f", &values[i]); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func cosineSimilarity(a, b []float64) float64 {
	var dot, aa, bb float64
	for i := range a {
		dot += a[i] * b[i]
		aa += a[i] * a[i]
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / (math.Sqrt(aa) * math.Sqrt(bb))
}
