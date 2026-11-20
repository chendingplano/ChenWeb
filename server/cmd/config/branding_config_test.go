package config

import "testing"

func TestBrandingLogoFilesReadFromBrandingSection(t *testing.T) {
	loadSystemLoginPageConfig(t, `
[config]
config_filename = "config/site/site-default.toml"

[branding]
logo_file = " /data/branding/logo.png "
logo_white_file = "branding/logo-white.png"
footer_email = "contact@tenant.example"
`)
	if got := GetLogoFile(); got != "/data/branding/logo.png" {
		t.Fatalf("GetLogoFile() = %q", got)
	}
	if got := GetLogoWhiteFile(); got != "branding/logo-white.png" {
		t.Fatalf("GetLogoWhiteFile() = %q", got)
	}
	if got := GetFooterEmail(); got != "contact@tenant.example" {
		t.Fatalf("GetFooterEmail() = %q", got)
	}
}

func TestBrandingLogoFilesEmptyWhenUnset(t *testing.T) {
	loadSystemLoginPageConfig(t, `
[config]
config_filename = "config/site/site-default.toml"
`)
	if GetLogoFile() != "" || GetLogoWhiteFile() != "" || GetFooterEmail() != "" {
		t.Fatalf("branding = %q, %q, %q; want empty", GetLogoFile(), GetLogoWhiteFile(), GetFooterEmail())
	}
}
