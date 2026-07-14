package sitehandler

import (
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestLoadSiteConfigValid(t *testing.T) {
	cfg, err := LoadSiteConfig("testdata/site-valid.toml")
	if err != nil {
		t.Fatalf("LoadSiteConfig: %v", err)
	}
	if cfg.Branding.SiteName != "SemOS" {
		t.Errorf("SiteName = %q, want SemOS", cfg.Branding.SiteName)
	}
	if len(cfg.Highlights) != 5 {
		t.Errorf("len(Highlights) = %d, want 5", len(cfg.Highlights))
	}
	if len(cfg.Features) != 4 {
		t.Errorf("len(Features) = %d, want 4", len(cfg.Features))
	}
	if len(cfg.Workspace.Apps) != 6 {
		t.Errorf("len(Workspace.Apps) = %d, want 6", len(cfg.Workspace.Apps))
	}
	if cfg.Workspace.Kicker != "Workspace" {
		t.Errorf("Workspace.Kicker = %q, want Workspace", cfg.Workspace.Kicker)
	}
	if len(cfg.Workspace.Announcements) != 1 {
		t.Fatalf("len(Workspace.Announcements) = %d, want 1", len(cfg.Workspace.Announcements))
	}
	if cfg.Workspace.Announcements[0] != "Welcome to your SemOS workspace." {
		t.Errorf("Announcements[0] = %q", cfg.Workspace.Announcements[0])
	}
	if cfg.Hero.CTAPrimaryHref != "/semos/workspace" {
		t.Errorf("CTAPrimaryHref = %q", cfg.Hero.CTAPrimaryHref)
	}
}

func TestLoadSiteConfigMissingFile(t *testing.T) {
	if _, err := LoadSiteConfig("testdata/does-not-exist.toml"); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestGetTenantConfigFilename(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT config_filename FROM site_tenants").
		WithArgs("demo").
		WillReturnRows(sqlmock.NewRows([]string{"config_filename"}).
			AddRow("config/site/tenant-demo.toml"))

	got, err := GetTenantConfigFilename(db, "demo")
	if err != nil {
		t.Fatalf("GetTenantConfigFilename: %v", err)
	}
	if got != "config/site/tenant-demo.toml" {
		t.Errorf("got %q", got)
	}
}

func TestGetTenantConfigFilenameNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT config_filename FROM site_tenants").
		WithArgs("nope").
		WillReturnError(sql.ErrNoRows)

	if _, err := GetTenantConfigFilename(db, "nope"); err == nil {
		t.Fatal("expected error for unknown tenant, got nil")
	}
}
