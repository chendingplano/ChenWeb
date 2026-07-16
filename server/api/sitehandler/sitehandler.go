// Package sitehandler serves site-config for the SemOS customer-facing
// frontend (ADR 2026071102). Two sources: the tenant-independent file named
// by [config].config_filename in config.local.toml, and per-tenant files
// whose names are looked up from the site_tenants table.
package sitehandler

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	toml "github.com/pelletier/go-toml/v2"

	"github.com/chendingplano/deepdoc/server/cmd/config"
	ApiTypes "github.com/chendingplano/shared/go/api/ApiTypes"
)

type Branding struct {
	SiteName  string `toml:"site_name" json:"site_name"`
	LogoText  string `toml:"logo_text" json:"logo_text"`
	LogoImage string `toml:"logo_image" json:"logo_image"`
	PoweredBy string `toml:"powered_by" json:"powered_by"`
}

type Hero struct {
	Kicker            string `toml:"kicker" json:"kicker"`
	Slogan            string `toml:"slogan" json:"slogan"`
	Subtitle          string `toml:"subtitle" json:"subtitle"`
	Image             string `toml:"image" json:"image"`
	CTAPrimaryLabel   string `toml:"cta_primary_label" json:"cta_primary_label"`
	CTAPrimaryHref    string `toml:"cta_primary_href" json:"cta_primary_href"`
	CTASecondaryLabel string `toml:"cta_secondary_label" json:"cta_secondary_label"`
	CTASecondaryHref  string `toml:"cta_secondary_href" json:"cta_secondary_href"`
}

// SectionCopy is the heading block above a page section.
type SectionCopy struct {
	Kicker   string `toml:"kicker" json:"kicker"`
	Title    string `toml:"title" json:"title"`
	Subtitle string `toml:"subtitle" json:"subtitle"`
}

// Stat is one figure in the stats band.
type Stat struct {
	Label string `toml:"label" json:"label"`
	Value string `toml:"value" json:"value"`
}

// FooterLink is one entry in a footer link column.
type FooterLink struct {
	Label string `toml:"label" json:"label"`
	Href  string `toml:"href" json:"href"`
}

type Highlight struct {
	Title       string `toml:"title" json:"title"`
	Description string `toml:"description" json:"description"`
	Image       string `toml:"image" json:"image"`
}

type Feature struct {
	Key         string `toml:"key" json:"key"`
	Title       string `toml:"title" json:"title"`
	Description string `toml:"description" json:"description"`
	Href        string `toml:"href" json:"href"`
}

type Footer struct {
	Text       string       `toml:"text" json:"text"`
	Address    string       `toml:"address" json:"address"`
	Newsletter string       `toml:"newsletter" json:"newsletter"`
	Email      string       `toml:"email" json:"email"`
	QuickLinks []FooterLink `toml:"quick_links" json:"quick_links"`
	Resources  []FooterLink `toml:"resources" json:"resources"`
}

// AboutValue is one numbered conviction on the About page.
type AboutValue struct {
	Title       string `toml:"title" json:"title"`
	Description string `toml:"description" json:"description"`
}

// About holds all copy for the 关于我们 (About Us) page. Contact details
// (email, address) are not duplicated here — the page reads them from Footer.
type About struct {
	Kicker        string       `toml:"kicker" json:"kicker"`
	Title         string       `toml:"title" json:"title"`
	Lede          string       `toml:"lede" json:"lede"`
	Image         string       `toml:"image" json:"image"`
	StoryKicker   string       `toml:"story_kicker" json:"story_kicker"`
	Story         []string     `toml:"story" json:"story"`
	ValuesKicker  string       `toml:"values_kicker" json:"values_kicker"`
	ValuesTitle   string       `toml:"values_title" json:"values_title"`
	ContactKicker string       `toml:"contact_kicker" json:"contact_kicker"`
	ContactTitle  string       `toml:"contact_title" json:"contact_title"`
	ContactText   string       `toml:"contact_text" json:"contact_text"`
	Values        []AboutValue `toml:"values" json:"values"`
}

type WorkspaceApp struct {
	Name        string `toml:"name" json:"name"`
	Description string `toml:"description" json:"description"`
	Href        string `toml:"href" json:"href"`
	Icon        string `toml:"icon" json:"icon"`
}

type Workspace struct {
	Kicker         string         `toml:"kicker" json:"kicker"`
	BannerTitle    string         `toml:"banner_title" json:"banner_title"`
	BannerSubtitle string         `toml:"banner_subtitle" json:"banner_subtitle"`
	BannerImage    string         `toml:"banner_image" json:"banner_image"`
	Announcements  []string       `toml:"announcements" json:"announcements"`
	Apps           []WorkspaceApp `toml:"apps" json:"apps"`
}

type SiteConfig struct {
	Branding        Branding    `toml:"branding" json:"branding"`
	Hero            Hero        `toml:"hero" json:"hero"`
	Highlights      []Highlight `toml:"highlights" json:"highlights"`
	FeaturesSection SectionCopy `toml:"features_section" json:"features_section"`
	Features        []Feature   `toml:"features" json:"features"`
	Stats           []Stat      `toml:"stats" json:"stats"`
	CTA             SectionCopy `toml:"cta" json:"cta"`
	About           About       `toml:"about" json:"about"`
	Footer          Footer      `toml:"footer" json:"footer"`
	Workspace       Workspace   `toml:"workspace" json:"workspace"`
}

// LoadSiteConfig parses one complete site-config TOML file.
func LoadSiteConfig(path string) (*SiteConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("(CWB_SITE_001) read site config %s: %w", path, err)
	}
	var cfg SiteConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("(CWB_SITE_002) parse site config %s: %w", path, err)
	}
	return &cfg, nil
}

// GetTenantConfigFilename looks up a tenant's site-config filename from the
// site_tenants table. Tenant-dependent filenames come only from this table,
// never from config.local.toml (ADR 2026071102).
func GetTenantConfigFilename(db *sql.DB, tenantID string) (string, error) {
	var filename string
	err := db.QueryRow(
		"SELECT config_filename FROM site_tenants WHERE tenant_id = $1",
		tenantID,
	).Scan(&filename)
	if err != nil {
		return "", fmt.Errorf("(CWB_SITE_003) tenant %q: %w", tenantID, err)
	}
	return filename, nil
}

// GetSiteConfig serves the tenant-independent site config.
// Endpoint: GET /api/site-config (public — all pre-login pages use this).
func GetSiteConfig(c echo.Context) error {
	path := config.GetSiteConfigFilename()
	if path == "" {
		log.Printf("***** Alarm: [config].config_filename not set in config.local.toml (CWB_SITE_004)")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "(CWB_SITE_004) [config].config_filename not set in config.local.toml",
		})
	}
	cfg, err := LoadSiteConfig(path)
	if err != nil {
		log.Printf("***** Alarm: LoadSiteConfig failed: %v (CWB_SITE_006)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	log.Printf("Served site config from %s (CWB_SITE_201)", path)
	return c.JSON(http.StatusOK, cfg)
}

// GetTenantSiteConfig serves a tenant's site config.
// Endpoint: GET /api/v1/site-config/tenant/:tenant_id (authenticated).
// Tenants are placeholders this pass (no user<->tenant binding yet); any
// authenticated session may fetch any tenant's config until the follow-up
// auth ADR lands.
func GetTenantSiteConfig(c echo.Context) error {
	tenantID := c.Param("tenant_id")
	if tenantID == "" {
		log.Printf("***** Alarm: tenant_id is required (CWB_SITE_005)")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "(CWB_SITE_005) tenant_id is required",
		})
	}
	filename, err := GetTenantConfigFilename(ApiTypes.ProjectDBHandle, tenantID)
	if err != nil {
		log.Printf("***** Alarm: GetTenantConfigFilename failed for tenant %q: %v (CWB_SITE_003)", tenantID, err)
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	}
	cfg, err := LoadSiteConfig(filename)
	if err != nil {
		log.Printf("***** Alarm: LoadSiteConfig failed for tenant %q: %v (CWB_SITE_007)", tenantID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	log.Printf("Served site config for tenant %q from %s (CWB_SITE_202)", tenantID, filename)
	return c.JSON(http.StatusOK, cfg)
}
