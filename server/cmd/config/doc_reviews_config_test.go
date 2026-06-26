package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// Verifies that a [doc-reviews] TOML section with hyphenated keys unmarshals
// into the DocReviews map and is returned by GetDocReviewsConfig.
func TestGetDocReviewsConfigUnmarshalsHyphenatedKeys(t *testing.T) {
	oldConfig := AppConfig
	oldViper := appConfigViper
	t.Cleanup(func() {
		AppConfig = oldConfig
		appConfigViper = oldViper
		viper.Reset()
	})
	viper.Reset()
	appConfigViper = viper.New()
	viper.SetConfigType("toml")

	const sample = `
[doc-reviews]
must-review = ["logical_flow", "completeness"]
should-review = ["readability"]
quick-pass = ["clarity", "security"]
`
	if err := viper.ReadConfig(strings.NewReader(sample)); err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := viper.Unmarshal(&AppConfig); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	appConfigViper = viper.GetViper()

	cfg := GetDocReviewsConfig()
	if len(cfg) != 3 {
		t.Fatalf("expected 3 tiers, got %d (%v)", len(cfg), cfg)
	}
	if got := cfg["must-review"]; len(got) != 2 || got[0] != "logical_flow" || got[1] != "completeness" {
		t.Fatalf("must-review=%v", got)
	}
	if got := cfg["should-review"]; len(got) != 1 || got[0] != "readability" {
		t.Fatalf("should-review=%v", got)
	}
	if got := cfg["quick-pass"]; len(got) != 2 {
		t.Fatalf("quick-pass=%v", got)
	}
}

func TestGetLanguagesReadsNestedLanguagesSection(t *testing.T) {
	oldConfig := AppConfig
	oldViper := appConfigViper
	t.Cleanup(func() {
		AppConfig = oldConfig
		appConfigViper = oldViper
		viper.Reset()
	})
	viper.Reset()
	appConfigViper = viper.New()
	viper.SetConfigType("toml")

	const sample = `
[languages]
languages = ["en", "zh"]
`
	if err := viper.ReadConfig(strings.NewReader(sample)); err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := viper.Unmarshal(&AppConfig); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	appConfigViper = viper.GetViper()

	got := GetLanguages()
	if len(got) != 2 {
		t.Fatalf("expected 2 languages, got %d (%v)", len(got), got)
	}
	if got[0] != "en" || got[1] != "zh" {
		t.Fatalf("languages=%v", got)
	}
}
