package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestGetImageGenerationModelsReadsConfiguredModels(t *testing.T) {
	oldConfig := AppConfig
	oldViper := appConfigViper
	t.Cleanup(func() {
		AppConfig = oldConfig
		appConfigViper = oldViper
		viper.Reset()
	})

	viper.Reset()
	appConfigViper = viper.New()
	appConfigViper.SetConfigType("toml")
	if err := appConfigViper.ReadConfig(strings.NewReader(`
[frontend]
image_generation_models = ["Flux", "OpenAI"]
`)); err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := appConfigViper.Unmarshal(&AppConfig); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got := GetImageGenerationModels()
	if len(got) != 2 || got[0] != "Flux" || got[1] != "OpenAI" {
		t.Fatalf("GetImageGenerationModels() = %#v, want [Flux OpenAI]", got)
	}
}

func TestGetImageGenerationModelsDefaultsWhenAbsent(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() { AppConfig = oldConfig })
	AppConfig = AppConfigDef{}

	got := GetImageGenerationModels()
	if len(got) != 2 || got[0] != "Qwen" || got[1] != "OpenAI" {
		t.Fatalf("GetImageGenerationModels() = %#v, want [Qwen OpenAI]", got)
	}
}
