package config

import "testing"

func TestBrandingLogoFilesReadFromBrandingSection(t *testing.T) {
	loadSystemLoginPageConfig(t, `
[config]
config_filename = "config/site/site-default.toml"

[branding]
logo_file = " /data/branding/logo.png "
logo_white_file = "branding/logo-white.png"
`)
	if got := GetLogoFile(); got != "/data/branding/logo.png" {
		t.Fatalf("GetLogoFile() = %q", got)
	}
	if got := GetLogoWhiteFile(); got != "branding/logo-white.png" {
		t.Fatalf("GetLogoWhiteFile() = %q", got)
	}
}

func TestBrandingLogoFilesEmptyWhenUnset(t *testing.T) {
	loadSystemLoginPageConfig(t, `
[config]
config_filename = "config/site/site-default.toml"
`)
	if GetLogoFile() != "" || GetLogoWhiteFile() != "" {
		t.Fatalf("logo files = %q, %q; want empty", GetLogoFile(), GetLogoWhiteFile())
	}
}
