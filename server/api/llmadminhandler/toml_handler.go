package llmadminhandler

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/labstack/echo/v4"
)

type modelTOMLEntry struct {
	Key                  string `json:"key"`
	Host                 string `json:"host"`
	ModelName            string `json:"model_name"`
	BaseURL              string `json:"base_url"`
	TimeoutSec           int    `json:"timeout_sec"`
	ThinkingType         string `json:"thinking_type"`
	MaxInflight          int    `json:"max_inflight"`
	MaxRequestsPerMinute int    `json:"max_requests_per_minute"`
	MaxTokensPerMinute   int    `json:"max_tokens_per_minute"`
	TokenReservePerCall  int    `json:"token_reserve_per_call"`
}

func modelsTOMLPath() string {
	if p := strings.TrimSpace(os.Getenv("CHENWEB_MODELS_TOML")); p != "" {
		return p
	}
	return filepath.Join(".", ".models.toml")
}

// reservedModelAPIKeysKey is a top-level TOML table used by loadDepositAPIKeyRefs
// (see handler.go) to map deposit-tracking names to account API keys. It does not
// conform to ApiTypes.LLMModelDef, so it must never be decoded/encoded as a model
// entry — doing so silently zeroes it out (go-toml decodes the mismatched shape into
// a zero-value struct instead of erroring). readModelsTOML excludes it from the model
// map and writeModelsTOML re-reads and preserves its raw content untouched.
const reservedModelAPIKeysKey = "model-api-keys"

// reservedDeepSeekBillingKey is the top-level TOML table read by
// applyDeepSeekLocalCNYPricing (llmreporthandler/handler.go) for DeepSeek's CNY
// peak/off-peak rate cards. Same shape mismatch hazard as reservedModelAPIKeysKey
// above, so it gets the same exclude-on-read / preserve-on-write treatment.
const reservedDeepSeekBillingKey = "deepseek-billing"

var reservedTopLevelKeys = []string{reservedModelAPIKeysKey, reservedDeepSeekBillingKey}

func readModelsTOML(path string) (ApiTypes.LLMModelsFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(ApiTypes.LLMModelsFile), nil
		}
		return nil, err
	}
	var models ApiTypes.LLMModelsFile
	if err := toml.Unmarshal(raw, &models); err != nil {
		return nil, err
	}
	if models == nil {
		models = make(ApiTypes.LLMModelsFile)
	}
	for _, key := range reservedTopLevelKeys {
		delete(models, key)
	}
	return models, nil
}

func readReservedTopLevelSections(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var generic map[string]any
	if err := toml.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	reserved := make(map[string]any, len(reservedTopLevelKeys))
	for _, key := range reservedTopLevelKeys {
		if value, ok := generic[key]; ok {
			reserved[key] = value
		}
	}
	return reserved, nil
}

func writeModelsTOML(path string, models ApiTypes.LLMModelsFile) error {
	reserved, err := readReservedTopLevelSections(path)
	if err != nil {
		return err
	}
	out := make(map[string]any, len(models)+len(reserved))
	for k, v := range models {
		out[k] = v
	}
	for k, v := range reserved {
		out[k] = v
	}
	data, err := toml.Marshal(out)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// UpsertModelsTOMLEntry reads, modifies, and writes the TOML file.
// Used internally by AddModel and the UpsertModelTOML handler.
func UpsertModelsTOMLEntry(path, key string, entry ApiTypes.LLMModelDef) error {
	models, err := readModelsTOML(path)
	if err != nil {
		return err
	}
	models[key] = entry
	return writeModelsTOML(path, models)
}

// GetModelsTOML returns all entries from .models.toml as a JSON array.
func GetModelsTOML(c echo.Context) error {
	path := modelsTOMLPath()
	models, err := readModelsTOML(path)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to read .models.toml",
			"path":    path,
			"error":   err.Error(),
		})
	}

	keys := make([]string, 0, len(models))
	for k := range models {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([]modelTOMLEntry, 0, len(keys))
	for _, k := range keys {
		m := models[k]
		entries = append(entries, modelTOMLEntry{
			Key:                  k,
			Host:                 m.Host,
			ModelName:            m.ModelName,
			BaseURL:              m.BaseURL,
			TimeoutSec:           m.TimeoutSec,
			ThinkingType:         m.ThinkingType,
			MaxInflight:          m.MaxInflight,
			MaxRequestsPerMinute: m.MaxRequestsPerMinute,
			MaxTokensPerMinute:   m.MaxTokensPerMinute,
			TokenReservePerCall:  m.TokenReservePerCall,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"path":    path,
		"models":  entries,
	})
}

// UpsertModelTOML adds or updates a single model entry in .models.toml.
func UpsertModelTOML(c echo.Context) error {
	key := c.Param("key")
	if strings.TrimSpace(key) == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "model key is required",
		})
	}

	var req modelTOMLEntry
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "invalid json body",
		})
	}

	path := modelsTOMLPath()
	if err := UpsertModelsTOMLEntry(path, key, ApiTypes.LLMModelDef{
		Host:                 req.Host,
		ModelName:            req.ModelName,
		BaseURL:              req.BaseURL,
		TimeoutSec:           req.TimeoutSec,
		ThinkingType:         req.ThinkingType,
		MaxInflight:          req.MaxInflight,
		MaxRequestsPerMinute: req.MaxRequestsPerMinute,
		MaxTokensPerMinute:   req.MaxTokensPerMinute,
		TokenReservePerCall:  req.TokenReservePerCall,
	}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to write .models.toml",
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":  true,
		"key": key,
	})
}

// DeleteModelTOML removes a model entry from .models.toml.
func DeleteModelTOML(c echo.Context) error {
	key := c.Param("key")
	if strings.TrimSpace(key) == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "model key is required",
		})
	}

	path := modelsTOMLPath()
	models, err := readModelsTOML(path)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to read .models.toml",
			"error":   err.Error(),
		})
	}

	if _, exists := models[key]; !exists {
		return c.JSON(http.StatusNotFound, map[string]any{
			"ok":      false,
			"message": "model key not found in .models.toml",
		})
	}

	delete(models, key)

	if err := writeModelsTOML(path, models); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to write .models.toml",
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":  true,
		"key": key,
	})
}
