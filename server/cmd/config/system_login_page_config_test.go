package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// loadSystemLoginPageConfig unmarshals the given TOML into AppConfig and
// restores the previous global state afterwards.
func loadSystemLoginPageConfig(t *testing.T, sample string) {
	t.Helper()
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

	if err := viper.ReadConfig(strings.NewReader(sample)); err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := viper.Unmarshal(&AppConfig); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	appConfigViper = viper.GetViper()
}

func TestGetLoginPageDefaultsWhenUnset(t *testing.T) {
	loadSystemLoginPageConfig(t, `
[system]
access_roles = ["admin"]
`)
	if got := GetLoginPage(); got != DefaultLoginPage {
		t.Fatalf("expected %q when login_page unset, got %q", DefaultLoginPage, got)
	}
}

func TestGetLoginPageReturnsConfiguredValidValue(t *testing.T) {
	loadSystemLoginPageConfig(t, `
[system]
login_page = "login-cell-phone-only"
`)
	if got := GetLoginPage(); got != "login-cell-phone-only" {
		t.Fatalf("expected %q, got %q", "login-cell-phone-only", got)
	}
}

func TestGetLoginPageNormalizesWhitespaceAndCase(t *testing.T) {
	loadSystemLoginPageConfig(t, `
[system]
login_page = "  LOGIN-Cell-Phone-Only  "
`)
	if got := GetLoginPage(); got != "login-cell-phone-only" {
		t.Fatalf("expected normalized %q, got %q", "login-cell-phone-only", got)
	}
}

func TestGetLoginPageFallsBackOnUnknownValue(t *testing.T) {
	loadSystemLoginPageConfig(t, `
[system]
login_page = "login-magic-link"
`)
	if got := GetLoginPage(); got != DefaultLoginPage {
		t.Fatalf("expected %q for unrecognized value, got %q", DefaultLoginPage, got)
	}
}

func TestGetLoginPageAcceptsExplicitDefault(t *testing.T) {
	loadSystemLoginPageConfig(t, `
[system]
login_page = "login-email-google"
`)
	if got := GetLoginPage(); got != "login-email-google" {
		t.Fatalf("expected %q, got %q", "login-email-google", got)
	}
}
