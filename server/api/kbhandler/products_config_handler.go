package kbhandler

import (
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/labstack/echo/v4"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

func GetProductsConfig(c echo.Context) error {
	rc, err := requirePendingFilesAdmin(c, "CWB_PRD_CFG_001")
	defer rc.Close()
	if err != nil {
		return err
	}
	cfg, err := docprocessing.LoadProductsConfig(docprocessing.ProductsConfigPath())
	if err != nil {
		rc.GetLogger().Warn("load products config failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "cannot read configuration"})
	}
	models, err := productConfigModels(resolveKbConfigFilePath())
	if err != nil {
		rc.GetLogger().Warn("load product model choices failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "cannot read model choices"})
	}
	defaults := map[string]string{
		"EXTRACT_PRODUCT_MENTIONS_PROMPT": "prompt-extract-product-mentions-v2.md", "ENRICH_PRODUCT_MENTION_PROMPT": "prompt-enrich-product-mention-v4.md", "EXTRACT_PRODUCT_MERGED_PROMPT": "prompt-extract-products-merged-v3.md", "TRANSLATE_PRODUCTS_PROMPT": "prompt-translate-products-v2.md",
		"EXTRACT_PRODUCTS_MERGED_PASS": "false", "EXTRACT_PRODUCT_PASS_1_ONLY": "false", "EXTRACT_PRODUCTS_REASONING": "true", "EXTRACT_PRODUCT_TRANSLATION": "on",
		"EXTRACT_PRODUCTS_MAX_TASKS": "1", "EXTRACT_PRODUCT_MENTIONS_MODEL_NAME": "", "ENRICH_PRODUCT_RELATIONS_MODEL_NAME": "", "EXTRACT_PRODUCT_MODEL_NAME": "", "EXTRACT_PRODUCT_MODEL_FALLBACK": "", "TRANSLATION_MODEL_NAME": "",
		"TRANSLATE_PRODUCTS_BATCH_SIZE": "10", "TRANSLATE_PRODUCTS_THINKING": "disabled",
	}
	for key := range defaults {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			defaults[key] = value
		}
	}
	if n, err := strconv.Atoi(defaults["EXTRACT_PRODUCTS_MAX_TASKS"]); err != nil || n < 1 {
		defaults["EXTRACT_PRODUCTS_MAX_TASKS"] = "1"
	}
	if n, err := strconv.Atoi(defaults["TRANSLATE_PRODUCTS_BATCH_SIZE"]); err != nil || n < 1 {
		defaults["TRANSLATE_PRODUCTS_BATCH_SIZE"] = "10"
	}
	defaults["TRANSLATE_PRODUCTS_THINKING"] = strings.ToLower(defaults["TRANSLATE_PRODUCTS_THINKING"])
	for _, key := range []string{"EXTRACT_PRODUCTS_MERGED_PASS", "EXTRACT_PRODUCT_PASS_1_ONLY", "EXTRACT_PRODUCTS_REASONING"} {
		v, err := strconv.ParseBool(defaults[key])
		if err != nil {
			v = key == "EXTRACT_PRODUCTS_REASONING"
		}
		defaults[key] = strconv.FormatBool(v)
	}
	if strings.EqualFold(defaults["EXTRACT_PRODUCT_TRANSLATION"], "off") {
		defaults["EXTRACT_PRODUCT_TRANSLATION"] = "off"
	} else {
		defaults["EXTRACT_PRODUCT_TRANSLATION"] = "on"
	}
	prompts := productPromptChoices(filepath.Dir(resolveKbConfigFilePath()))
	return c.JSON(http.StatusOK, map[string]any{"config": cfg, "models": models, "prompts": prompts, "inherited": defaults})
}
func PutProductsConfig(c echo.Context) error {
	rc, err := requirePendingFilesAdmin(c, "CWB_PRD_CFG_002")
	defer rc.Close()
	if err != nil {
		return err
	}
	var cfg docprocessing.ProductsConfig
	if err := c.Bind(&cfg); err != nil || cfg == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid configuration"})
	}
	if err := docprocessing.SaveProductsConfig(docprocessing.ProductsConfigPath(), cfg); err != nil {
		rc.GetLogger().Warn("save products config failed", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// productConfigModels applies the same base/local TOML overlay as the KB settings.
func productConfigModels(path string) ([]string, error) {
	var raw struct {
		DocProcessing struct {
			Models []string `toml:"doc_processing_models"`
		} `toml:"doc-processing"`
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err = toml.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	local := filepath.Join(filepath.Dir(path), "config.local.toml")
	if filepath.Clean(local) != filepath.Clean(path) {
		body, err = os.ReadFile(local)
		if err == nil {
			if err = toml.Unmarshal(body, &raw); err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	if raw.DocProcessing.Models == nil {
		return []string{}, nil
	}
	return raw.DocProcessing.Models, nil
}
func productPromptChoices(configDir string) map[string][]string {
	prefixes := map[string]string{"EXTRACT_PRODUCT_MENTIONS_PROMPT": "prompt-extract-product-mentions-", "ENRICH_PRODUCT_MENTION_PROMPT": "prompt-enrich-product-mention-", "EXTRACT_PRODUCT_MERGED_PROMPT": "prompt-extract-products-merged-", "TRANSLATE_PRODUCTS_PROMPT": "prompt-translate-products-"}
	result := make(map[string][]string, len(prefixes))
	dirs := []string{strings.TrimSpace(os.Getenv("PROMPT_DIR")), filepath.Join(configDir, "prompts")}
	for key, prefix := range prefixes {
		seen := map[string]bool{}
		result[key] = []string{}
		for _, dir := range dirs {
			if dir == "" {
				continue
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				name := entry.Name()
				if !entry.IsDir() && strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".md") && !seen[name] {
					result[key] = append(result[key], name)
					seen[name] = true
				}
			}
		}
		sort.Strings(result[key])
	}
	return result
}
