package kbhandler

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

type kbFrontendConfig struct {
	TopicTypes             []string            `json:"topic_types"`
	SupportedLanguages     []string            `json:"supported_languages"`
	DefaultLanguage        []string            `json:"default_language"`
	MandatoryProcessors    []string            `json:"mandatory_processors"`
	RequiredProcessors     []string            `json:"required_processors"`
	DefaultProcessors      []string            `json:"default_processors"`
	ProcessorPackages      map[string][]string `json:"processor_packages"`
	MaxDocProcessPipelines int                 `json:"max_doc_process_pipelines"`
	ImageGenerationModels  []string            `json:"image_generation_models"`
}

type kbFrontendConfigResponse struct {
	Status bool             `json:"status"`
	Config kbFrontendConfig `json:"config"`
}

// mandatoryProcessorIDs are always executed regardless of config or event operation list.
// "blocking" is excluded here because it is rendered as a separate always-on row in the UI.
var mandatoryProcessorIDs = []string{"static_analyzer", "chunking", "extract_doc_metadata"}

// GetKbFrontendConfig reads the [frontend] and [doc-processing] sections from config.toml
// and returns frontend configuration. The config file path is resolved via the
// KB_CONFIG_FILE env var, falling back to ./config.toml.
func GetKbFrontendConfig(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_CFG_001")
	defer rc.Close()

	cfg, err := LoadKbFrontendConfig()
	if err != nil {
		rc.GetLogger().Warn("load kb frontend config failed", "err", err)
		return c.JSON(http.StatusOK, kbFrontendConfigResponse{
			Status: true,
			Config: kbFrontendConfig{
				TopicTypes:             []string{},
				SupportedLanguages:     defaultSupportedLanguages(),
				DefaultLanguage:        defaultLanguageList(),
				MandatoryProcessors:    mandatoryProcessorIDs,
				RequiredProcessors:     []string{},
				DefaultProcessors:      []string{},
				ProcessorPackages:      map[string][]string{},
				MaxDocProcessPipelines: maxDocProcessPipelinesFromEnv(),
				ImageGenerationModels:  appconfig.GetImageGenerationModels(),
			},
		})
	}

	return c.JSON(http.StatusOK, kbFrontendConfigResponse{Status: true, Config: cfg})
}

type rawKbFrontendSection struct {
	Frontend struct {
		TopicTypes         []string `toml:"topic_types"`
		SupportedLanguages []string `toml:"supported_languages"`
		DefaultLanguage    []string `toml:"default_language"`
	} `toml:"frontend"`
	DocProcessing struct {
		RequiredProcessors []string `toml:"required_processors"`
		DefaultProcessors  []string `toml:"default_processors"`
	} `toml:"doc-processing"`
	ProcessorPackages map[string][]string `toml:"doc-processing-packages"`
}

// LoadKbFrontendConfig reads the [frontend] and [doc-processing] sections from
// config.toml. Exported so other packages (e.g. doc-reviews, for on-demand
// finding translation) can reuse the same config source.
func LoadKbFrontendConfig() (kbFrontendConfig, error) {
	path := resolveKbConfigFilePath()
	body, err := os.ReadFile(path)
	if err != nil {
		return kbFrontendConfig{}, err
	}
	var raw rawKbFrontendSection
	if err := toml.Unmarshal(body, &raw); err != nil {
		return kbFrontendConfig{}, err
	}
	types := raw.Frontend.TopicTypes
	if types == nil {
		types = []string{}
	}
	supportedLanguages := raw.Frontend.SupportedLanguages
	if len(supportedLanguages) == 0 {
		supportedLanguages = defaultSupportedLanguages()
	}
	defaultLanguage := raw.Frontend.DefaultLanguage
	if len(defaultLanguage) == 0 {
		defaultLanguage = defaultLanguageList()
	}
	reqProcs := raw.DocProcessing.RequiredProcessors
	if reqProcs == nil {
		reqProcs = []string{}
	}
	defaultProcs := raw.DocProcessing.DefaultProcessors
	if defaultProcs == nil {
		defaultProcs = []string{}
	}
	packages := raw.ProcessorPackages
	if packages == nil {
		packages = map[string][]string{}
	}
	return kbFrontendConfig{
		TopicTypes:             types,
		SupportedLanguages:     supportedLanguages,
		DefaultLanguage:        defaultLanguage,
		MandatoryProcessors:    mandatoryProcessorIDs,
		RequiredProcessors:     reqProcs,
		DefaultProcessors:      defaultProcs,
		ProcessorPackages:      packages,
		MaxDocProcessPipelines: maxDocProcessPipelinesFromEnv(),
		ImageGenerationModels:  appconfig.GetImageGenerationModels(),
	}, nil
}

func defaultSupportedLanguages() []string {
	return []string{"en", "zh-cn"}
}

func defaultLanguageList() []string {
	return []string{"en"}
}

func maxDocProcessPipelinesFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("MAX_DOC_PROCESS_PIPELINES"))
	if raw == "" {
		return 10
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 10
	}
	return n
}

func resolveKbConfigFilePath() string {
	if v := strings.TrimSpace(os.Getenv("KB_CONFIG_FILE")); v != "" {
		return v
	}
	// Walk up from the current working directory looking for config.toml
	cur, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(cur, "config.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "./config.toml"
}
