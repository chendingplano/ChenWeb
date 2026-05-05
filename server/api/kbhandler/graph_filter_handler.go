package kbhandler

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
	toml "github.com/pelletier/go-toml/v2"
)

type graphFilterEmbedder interface {
	Embed(ctx context.Context, in llmclients.EmbedInput) ([]float64, error)
}

type filterGraphNodesRequest struct {
	Mode           string   `json:"mode"`
	CandidatePaths []string `json:"candidatePaths"`
	SemanticText   string   `json:"semanticText"`
	Threshold      float64  `json:"threshold"`
}

type graphFilterSemanticMatch struct {
	CategoryPath string  `json:"categoryPath"`
	Score        float64 `json:"score"`
}

type filterGraphNodesResponse struct {
	Status  bool                       `json:"status"`
	Matches []graphFilterSemanticMatch `json:"matches"`
}

type graphFilterSemanticParams struct {
	RootDir          string
	CandidatePaths   []string
	QueryText        string
	Threshold        float64
	EmbeddingModel   string
	EmbedderOverride graphFilterEmbedder
}

type graphFilterEmbedderConfig struct {
	Embedder  graphFilterEmbedder
	ModelName string
	Err       error
}

func FilterGraphNodes(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_GF_001")
	defer rc.Close()
	logger := rc.GetLogger()

	var req filterGraphNodesRequest
	if err := c.Bind(&req); err != nil {
		logger.Warn("invalid graph filter request", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid graph filter request (CWB_KB_GF_010)",
		})
	}
	req.Mode = strings.TrimSpace(req.Mode)
	req.SemanticText = strings.TrimSpace(req.SemanticText)
	if req.Mode != "summary" && req.Mode != "topic" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "mode must be summary or topic (CWB_KB_GF_011)",
		})
	}
	if req.SemanticText == "" {
		return c.JSON(http.StatusOK, filterGraphNodesResponse{Status: true, Matches: []graphFilterSemanticMatch{}})
	}
	if req.Threshold < 0 || req.Threshold > 1 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "semantic threshold must be between 0.0 and 1.0 (CWB_KB_GF_012)",
		})
	}

	rootDir := graphFilterRootDir(req.Mode)
	if rootDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "graph filter root directory is not configured (CWB_KB_GF_013)",
		})
	}
	embedderCfg := newGraphFilterEmbedder(req.Mode)
	if embedderCfg.Err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("%v (CWB_KB_GF_014)", embedderCfg.Err),
		})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 45*time.Second)
	defer cancel()
	matches, err := filterGraphSemanticMatches(ctx, graphFilterSemanticParams{
		RootDir:          rootDir,
		CandidatePaths:   req.CandidatePaths,
		QueryText:        req.SemanticText,
		Threshold:        req.Threshold,
		EmbeddingModel:   embedderCfg.ModelName,
		EmbedderOverride: embedderCfg.Embedder,
	})
	if err != nil {
		logger.Error("semantic graph filter failed", "mode", req.Mode, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("semantic graph filter failed: %v (CWB_KB_GF_015)", err),
		})
	}
	return c.JSON(http.StatusOK, filterGraphNodesResponse{Status: true, Matches: matches})
}

func graphFilterRootDir(mode string) string {
	if mode == "summary" {
		return strings.TrimSpace(os.Getenv("SUMMARY_TREE_DIR"))
	}
	return strings.TrimSpace(os.Getenv("TOPIC_TREE_ROOT_DIR"))
}

func newGraphFilterEmbedder(mode string) graphFilterEmbedderConfig {
	modelEnv := graphFilterEmbeddingModelEnv(mode)
	modelRef := strings.TrimSpace(os.Getenv(modelEnv))
	if modelRef == "" {
		return graphFilterEmbedderConfig{
			Err: fmt.Errorf("missing %s for %s semantic graph filtering", modelEnv, mode),
		}
	}
	cfg, ok := resolveGraphFilterEmbeddingConfig(modelRef)
	if ok {
		timeoutSec := cfg.TimeoutSec
		if timeoutSec <= 0 {
			timeoutSec = 45
		}
		client := &llmclients.OpenAIJSONClient{
			ModelName:  cfg.ModelName,
			APIKey:     cfg.APIKey,
			BaseURL:    cfg.BaseURL,
			HTTPClient: &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		}
		return graphFilterEmbedderConfig{Embedder: client, ModelName: cfg.ModelName}
	}

	modelPath, pathErr := resolveGraphFilterModelsFilePath()
	if pathErr != nil {
		return graphFilterEmbedderConfig{
			Err: fmt.Errorf("%s=%q but .models.toml could not be found; set MODELS_FILE or place .models.toml in the working tree", modelEnv, modelRef),
		}
	}
	return graphFilterEmbedderConfig{
		Err: fmt.Errorf("%s=%q not found in %s or missing model_name", modelEnv, modelRef, modelPath),
	}
}

func resolveGraphFilterEmbeddingConfig(modelRef string) (ApiTypes.LLMModelDef, bool) {
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return ApiTypes.LLMModelDef{}, false
	}
	modelPath, err := resolveGraphFilterModelsFilePath()
	if err != nil {
		return ApiTypes.LLMModelDef{}, false
	}
	raw, err := os.ReadFile(modelPath)
	if err != nil {
		return ApiTypes.LLMModelDef{}, false
	}
	parsed := ApiTypes.LLMModelsFile{}
	if err := toml.Unmarshal(raw, &parsed); err != nil {
		return ApiTypes.LLMModelDef{}, false
	}
	cfg, ok := parsed[modelRef]
	if !ok || strings.TrimSpace(cfg.ModelName) == "" {
		return ApiTypes.LLMModelDef{}, false
	}
	cfg.ModelName = strings.TrimSpace(cfg.ModelName)
	cfg.APIKey = strings.TrimSpace(firstNonEmpty(cfg.APIKey, os.Getenv("KB_GRAPH_FILTER_EMBEDDING_API_KEY"), os.Getenv("SHARED_LLM_API_KEY")))
	cfg.BaseURL = strings.TrimSpace(firstNonEmpty(cfg.BaseURL, os.Getenv("KB_GRAPH_FILTER_EMBEDDING_BASE_URL"), os.Getenv("SHARED_LLM_BASE_URL")))
	return cfg, true
}

func resolveGraphFilterModelsFilePath() (string, error) {
	for _, envName := range []string{"KB_GRAPH_FILTER_EMBEDDING_MODELS_FILE", "MODELS_FILE"} {
		if override := strings.TrimSpace(os.Getenv(envName)); override != "" {
			if _, err := os.Stat(override); err != nil {
				return "", err
			}
			return override, nil
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cur := wd
	for {
		candidate := filepath.Join(cur, ".models.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", os.ErrNotExist
}

func graphFilterEmbeddingModelEnv(mode string) string {
	if mode == "summary" {
		return "SUMMARY_EMBEDDING_MODEL_NAME"
	}
	return "TOPIC_EMBEDDING_MODEL_NAME"
}

func filterGraphSemanticMatches(ctx context.Context, params graphFilterSemanticParams) ([]graphFilterSemanticMatch, error) {
	rootDir := strings.TrimSpace(params.RootDir)
	if rootDir == "" {
		return nil, errors.New("root dir is empty")
	}
	queryText := strings.TrimSpace(params.QueryText)
	if queryText == "" {
		return []graphFilterSemanticMatch{}, nil
	}
	if params.Threshold < 0 || params.Threshold > 1 {
		return nil, fmt.Errorf("threshold %.4f is outside [0,1]", params.Threshold)
	}
	embedder := params.EmbedderOverride
	if embedder == nil {
		return nil, errors.New("embedder is nil")
	}
	queryVec, err := embedder.Embed(ctx, llmclients.EmbedInput{
		ModelName: params.EmbeddingModel,
		InputText: queryText,
	})
	if err != nil {
		return nil, err
	}

	matches := make([]graphFilterSemanticMatch, 0)
	for _, categoryPath := range params.CandidatePaths {
		categoryPath = strings.TrimSpace(filepath.ToSlash(categoryPath))
		if categoryPath == "" || strings.Contains(categoryPath, "..") {
			continue
		}
		vec, err := loadGraphFilterEmbedding(filepath.Join(rootDir, filepath.FromSlash(categoryPath), "category.embed"))
		if err != nil || len(vec) == 0 {
			continue
		}
		score := cosineSimilarity(queryVec, vec)
		if score >= params.Threshold {
			matches = append(matches, graphFilterSemanticMatch{CategoryPath: categoryPath, Score: score})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].CategoryPath < matches[j].CategoryPath
		}
		return matches[i].Score > matches[j].Score
	})
	return matches, nil
}

func loadGraphFilterEmbedding(path string) ([]float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(string(data))
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []float64{}, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]float64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		f, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, fmt.Errorf("parse embedding float %q: %w", part, err)
		}
		out = append(out, f)
	}
	return out, nil
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}
