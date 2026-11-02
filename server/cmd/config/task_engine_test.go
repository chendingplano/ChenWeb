package config

import (
	"github.com/spf13/viper"
	"strings"
	"testing"
)

func TestDocProcessorTaskEngineConfig(t *testing.T) {
	tests := []struct {
		name, toml, mode, boundary string
		bad                        bool
	}{
		{"defaults", "", "none", "ignore", false},
		{"configured", "[[task-engines.doc-processor]]\ndefault_mode = 'auto_offpeak'\ncross_peak_mode = 'pause-llm'\n", "auto_offpeak", "pause-llm", false},
		{"unsupported pause", "[[task-engines.doc-processor]]\ncross_peak_mode = 'pause-exec'\n", "", "", true},
		{"bad mode", "[[task-engines.doc-processor]]\ndefault_mode = 'wrong'\n", "", "", true},
		{"multiple", "[[task-engines.doc-processor]]\n[[task-engines.doc-processor]]\n", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			v.SetConfigType("toml")
			if err := v.ReadConfig(strings.NewReader(tt.toml)); err != nil {
				t.Fatal(err)
			}
			got, err := docProcessorTaskEngineConfig(v)
			if (err != nil) != tt.bad {
				t.Fatalf("error = %v, want error %v", err, tt.bad)
			}
			if err == nil && (got.DefaultMode != tt.mode || got.CrossPeakMode != tt.boundary) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
