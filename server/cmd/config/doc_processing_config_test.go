package config

import (
	"reflect"
	"testing"

	"github.com/spf13/viper"
)

func TestGetDocProcessingProcessorsReadsMergedViperConfig(t *testing.T) {
	old := appConfigViper
	t.Cleanup(func() { appConfigViper = old })
	appConfigViper = viper.New()
	appConfigViper.Set("doc-processing.required_processors", []string{"extract_metrics", "extract_products"})
	appConfigViper.Set("doc-processing.default_processors", []string{"extract_metrics"})

	required, defaults := GetDocProcessingProcessors()
	if !reflect.DeepEqual(required, []string{"extract_metrics", "extract_products"}) {
		t.Fatalf("required=%v", required)
	}
	if !reflect.DeepEqual(defaults, []string{"extract_metrics"}) {
		t.Fatalf("defaults=%v", defaults)
	}
}
