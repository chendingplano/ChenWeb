package docprocessing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PipelineDefaults struct {
	Processors []string `json:"processors"`
	Mode       string   `json:"mode"`
}

// PipelineDefaultsPath locates the operator defaults alongside the application config.
func PipelineDefaultsPath() string {
	path := strings.TrimSpace(os.Getenv("KB_CONFIG_FILE"))
	if path == "" {
		for _, key := range []string{"DOC_PROCESSOR_CONFIG", "EXTRACT_DOCMETA_CONFIG", "FILE_CONVERTER_CONFIG"} {
			if path = strings.TrimSpace(os.Getenv(key)); path != "" {
				break
			}
		}
	}
	if path == "" {
		dir, _ := os.Getwd()
		for i := 0; i < 6; i++ {
			candidate := filepath.Join(dir, "config.toml")
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	if path == "" {
		path = "config.toml"
	}
	return filepath.Join(filepath.Dir(path), "doc-pipeline-defaults.json")
}

func LoadPipelineDefaults(path string) (PipelineDefaults, error) {
	var cfg PipelineDefaults
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

func SavePipelineDefaults(path string, cfg PipelineDefaults, available []string) error {
	switch cfg.Mode {
	case "auto_offpeak", "auto", "upload_only", "pdf_parsing":
	default:
		return fmt.Errorf("invalid processing mode")
	}
	if len(cfg.Processors) == 0 {
		return fmt.Errorf("select at least one processor")
	}
	valid := make(map[string]bool, len(available))
	for _, name := range available {
		valid[name] = true
	}
	seen := map[string]bool{}
	for _, name := range cfg.Processors {
		if !valid[name] || seen[name] {
			return fmt.Errorf("invalid processor %q", name)
		}
		seen[name] = true
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".pipeline-defaults-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0644); err == nil {
		_, err = f.Write(append(data, '\n'))
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
