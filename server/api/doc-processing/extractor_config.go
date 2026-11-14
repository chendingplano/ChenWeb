package docprocessing

import (
	"fmt"
	"path/filepath"
)

var extractorConfigKeys = map[string]map[string]string{
	"metrics-simple": {"EXTRACT_METRICS_SIMPLE_MODEL_NAME": "text", "EXTRACT_METRICS_SIMPLE_PROMPT": "text", "EXTRACT_METRICS_MAX_TASKS": "int"},
	"references":     {"EXTRACT_REFERENCES_MODEL_NAME": "text", "EXTRACT_REFERENCES_PROMPT": "text"},
	"provisions":     {"EXTRACT_PROVISIONS_MODEL_NAME": "text", "EXTRACT_PROVISIONS_MODEL_FALLBACK": "text", "EXTRACT_PROVISIONS_PROMPT": "text", "EXTRACT_PROVISIONS_REVIEW_PROMPT": "text", "EXTRACT_PROVISIONS_MAX_TASKS": "int", "EXTRACT_PROVISIONS_INPUT": "input"},
}

func ExtractorConfigPath(name string) (string, error) {
	if _, ok := extractorConfigKeys[name]; !ok {
		return "", fmt.Errorf("unknown extractor")
	}
	return filepath.Join(filepath.Dir(PipelineDefaultsPath()), "extract-"+name+"-config.json"), nil
}
func LoadExtractorConfig(name string) (ProductsConfig, error) {
	path, err := ExtractorConfigPath(name)
	if err != nil {
		return nil, err
	}
	return LoadProductsConfig(path)
}
func SaveExtractorConfig(name string, config ProductsConfig) error {
	path, err := ExtractorConfigPath(name)
	if err != nil {
		return err
	}
	return saveProcessorConfig(path, config, extractorConfigKeys[name])
}
