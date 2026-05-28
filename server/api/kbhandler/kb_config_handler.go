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
)

type kbFrontendConfig struct {
	TopicTypes             []string `json:"topic_types"`
	MandatoryProcessors    []string `json:"mandatory_processors"`
	RequiredProcessors     []string `json:"required_processors"`
	MaxDocProcessPipelines int      `json:"max_doc_process_pipelines"`
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

	cfg, err := loadKbFrontendConfig()
	if err != nil {
		rc.GetLogger().Warn("load kb frontend config failed", "err", err)
		return c.JSON(http.StatusOK, kbFrontendConfigResponse{
			Status: true,
			Config: kbFrontendConfig{
				TopicTypes:             []string{},
				MandatoryProcessors:    mandatoryProcessorIDs,
				RequiredProcessors:     []string{},
				MaxDocProcessPipelines: maxDocProcessPipelinesFromEnv(),
			},
		})
	}

	return c.JSON(http.StatusOK, kbFrontendConfigResponse{Status: true, Config: cfg})
}

type rawKbFrontendSection struct {
	Frontend struct {
		TopicTypes []string `toml:"topic_types"`
	} `toml:"frontend"`
	DocProcessing struct {
		RequiredProcessors []string `toml:"required_processors"`
	} `toml:"doc-processing"`
}

func loadKbFrontendConfig() (kbFrontendConfig, error) {
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
	reqProcs := raw.DocProcessing.RequiredProcessors
	if reqProcs == nil {
		reqProcs = []string{}
	}
	return kbFrontendConfig{
		TopicTypes:             types,
		MandatoryProcessors:    mandatoryProcessorIDs,
		RequiredProcessors:     reqProcs,
		MaxDocProcessPipelines: maxDocProcessPipelinesFromEnv(),
	}, nil
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
