package config

import "testing"

func TestBrandingReadFromBrandingSection(t *testing.T) {
	loadSystemLoginPageConfig(t, `
[config]
config_filename = "config/site/site-default.toml"

[branding]
logo_file = " /data/branding/logo.png "
logo_white_file = "branding/logo-white.png"
footer_email = "contact@tenant.example"
footer_address = """
1 Example Road
Example City
"""
`)
	want := BrandingSection{
		LogoFile:      "/data/branding/logo.png",
		LogoWhiteFile: "branding/logo-white.png",
		FooterEmail:   "contact@tenant.example",
		FooterAddress: "1 Example Road\nExample City",
	}
	if got := GetBranding(); got != want {
		t.Fatalf("GetBranding() = %+v, want %+v", got, want)
	}
	if GetLogoFile() != want.LogoFile || GetLogoWhiteFile() != want.LogoWhiteFile {
		t.Fatalf("logo getters = %q, %q", GetLogoFile(), GetLogoWhiteFile())
	}
}

func TestBrandingEmptyWhenUnset(t *testing.T) {
	loadSystemLoginPageConfig(t, `
[config]
config_filename = "config/site/site-default.toml"
`)
	if got := GetBranding(); got != (BrandingSection{}) {
		t.Fatalf("GetBranding() = %+v; want empty", got)
	}
}
