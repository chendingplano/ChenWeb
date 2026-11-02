package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// DocProcessorTaskEngine describes the engine-level scheduling policy.
type DocProcessorTaskEngine struct {
	DefaultMode   string `mapstructure:"default_mode"`
	CrossPeakMode string `mapstructure:"cross_peak_mode"`
}

func docProcessorTaskEngineConfig(v *viper.Viper) (DocProcessorTaskEngine, error) {
	cfg := DocProcessorTaskEngine{DefaultMode: "none", CrossPeakMode: "ignore"}
	var entries []DocProcessorTaskEngine
	if err := v.UnmarshalKey("task-engines.doc-processor", &entries); err != nil {
		return cfg, fmt.Errorf("invalid doc-processor task engine: %w", err)
	}
	if len(entries) > 1 {
		return cfg, fmt.Errorf("task-engines.doc-processor must contain at most one entry")
	}
	if len(entries) == 1 {
		if entries[0].DefaultMode != "" {
			cfg.DefaultMode = strings.TrimSpace(entries[0].DefaultMode)
		}
		if entries[0].CrossPeakMode != "" {
			cfg.CrossPeakMode = strings.TrimSpace(entries[0].CrossPeakMode)
		}
	}
	switch cfg.DefaultMode {
	case "none", "auto_peak", "auto_offpeak":
	default:
		return cfg, fmt.Errorf("invalid doc-processor default_mode %q", cfg.DefaultMode)
	}
	switch cfg.CrossPeakMode {
	case "ignore", "pause-llm", "abort":
	default:
		return cfg, fmt.Errorf("invalid or unsupported doc-processor cross_peak_mode %q", cfg.CrossPeakMode)
	}
	return cfg, nil
}

func GetDocProcessorTaskEngine() (DocProcessorTaskEngine, error) {
	return docProcessorTaskEngineConfig(appConfigViper)
}
