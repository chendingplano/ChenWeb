package kbhandler

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/labstack/echo/v4"
)

var extractorDefaults = map[string]map[string]string{
	"metrics-simple": {"EXTRACT_METRICS_SIMPLE_PROMPT": "prompt-extract-metric-simple-v1.md", "EXTRACT_METRICS_SIMPLE_MODEL_NAME": "", "EXTRACT_METRICS_MAX_TASKS": "1"},
	"references":     {"EXTRACT_REFERENCES_PROMPT": "prompt-extract-references-v1.md", "EXTRACT_REFERENCES_MODEL_NAME": ""},
	"provisions":     {"EXTRACT_PROVISIONS_PROMPT": "prompt-extract-provisions-v8.md", "EXTRACT_PROVISIONS_REVIEW_PROMPT": "prompt-audit-provision-extraction-v1.md", "EXTRACT_PROVISIONS_MODEL_NAME": "", "EXTRACT_PROVISIONS_MODEL_FALLBACK": "", "EXTRACT_PROVISIONS_MAX_TASKS": "1", "EXTRACT_PROVISIONS_INPUT": "chunks"},
}
var extractorPromptPrefixes = map[string]string{"EXTRACT_METRICS_SIMPLE_PROMPT": "prompt-extract-metric-simple-", "EXTRACT_REFERENCES_PROMPT": "prompt-extract-references-", "EXTRACT_PROVISIONS_PROMPT": "prompt-extract-provisions-", "EXTRACT_PROVISIONS_REVIEW_PROMPT": "prompt-audit-provision-extraction-"}

func GetExtractorConfig(c echo.Context) error {
	rc, err := requirePendingFilesAdmin(c, "CWB_EXT_CFG_001")
	defer rc.Close()
	if err != nil {
		return err
	}
	name := c.Param("extractor")
	defs, ok := extractorDefaults[name]
	if !ok {
		return c.NoContent(http.StatusNotFound)
	}
	cfg, err := docprocessing.LoadExtractorConfig(name)
	if err != nil {
		rc.GetLogger().Warn("load extractor configuration failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "cannot read configuration"})
	}
	models, err := productConfigModels(resolveKbConfigFilePath())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "cannot read model choices"})
	}
	inherited := map[string]string{}
	prompts := map[string][]string{}
	for key, def := range defs {
		inherited[key] = def
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			inherited[key] = v
		}
		if prefix, ok := extractorPromptPrefixes[key]; ok {
			prompts[key] = []string{}
			seen := map[string]bool{}
			for _, dir := range []string{strings.TrimSpace(os.Getenv("PROMPT_DIR")), filepath.Join(filepath.Dir(resolveKbConfigFilePath()), "prompts")} {
				if dir == "" {
					continue
				}
				entries, _ := os.ReadDir(dir)
				for _, entry := range entries {
					n := entry.Name()
					if !entry.IsDir() && strings.HasPrefix(n, prefix) && strings.HasSuffix(n, ".md") && !seen[n] {
						prompts[key] = append(prompts[key], n)
						seen[n] = true
					}
				}
			}
			sort.Strings(prompts[key])
		}
	}
	if name == "metrics-simple" && inherited["EXTRACT_METRICS_SIMPLE_MODEL_NAME"] == "" {
		for _, key := range []string{"ENRICH_METRICS_MODEL_NAME", "EXTRACT_METRICS_MODEL_NAME"} {
			if v := strings.TrimSpace(os.Getenv(key)); v != "" {
				inherited["EXTRACT_METRICS_SIMPLE_MODEL_NAME"] = v
				break
			}
		}
	}
	if name == "provisions" && strings.TrimSpace(os.Getenv("EXTRACT_PROVISIONS_PROMPT")) == "" && strings.TrimSpace(os.Getenv("PROMPT_FILE_NAME")) != "" {
		inherited["EXTRACT_PROVISIONS_PROMPT"] = strings.TrimSpace(os.Getenv("PROMPT_FILE_NAME"))
	}
	return c.JSON(http.StatusOK, map[string]any{"config": cfg, "models": models, "prompts": prompts, "inherited": inherited})
}
func PutExtractorConfig(c echo.Context) error {
	rc, err := requirePendingFilesAdmin(c, "CWB_EXT_CFG_002")
	defer rc.Close()
	if err != nil {
		return err
	}
	name := c.Param("extractor")
	if _, ok := extractorDefaults[name]; !ok {
		return c.NoContent(http.StatusNotFound)
	}
	var cfg docprocessing.ProductsConfig
	if err := c.Bind(&cfg); err != nil || cfg == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid configuration"})
	}
	if err := docprocessing.SaveExtractorConfig(name, cfg); err != nil {
		rc.GetLogger().Warn("save extractor configuration failed", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
