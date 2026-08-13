package main

import (
	"os"
	"strings"
	"testing"
)

func TestCLIUsesTheServicePostgresEnvironmentContract(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	text := string(source)
	if !strings.Contains(text, `PG_USER_NAME`) {
		t.Error("ontology seed must accept PG_USER_NAME, the service database-user setting")
	}
	if strings.Contains(text, `"chenweb_test"`) {
		t.Error("ontology seed must not default to the chenweb_test database")
	}
}

func TestCLIUserResolutionUsesLiteralFallback(t *testing.T) {
	t.Setenv("PG_USER_NAME", "")
	t.Setenv("PG_USER", "")
	if got := postgresUserName(); got != "cding" {
		t.Fatalf("postgres user fallback = %q, want cding", got)
	}

	t.Setenv("PG_USER", "legacy-user")
	if got := postgresUserName(); got != "legacy-user" {
		t.Fatalf("PG_USER fallback = %q, want legacy-user", got)
	}

	t.Setenv("PG_USER_NAME", "   ")
	if got := postgresUserName(); got != "legacy-user" {
		t.Fatalf("whitespace-only PG_USER_NAME fallback = %q, want legacy-user", got)
	}

	t.Setenv("PG_USER_NAME", "service-user")
	if got := postgresUserName(); got != "service-user" {
		t.Fatalf("PG_USER_NAME preference = %q, want service-user", got)
	}
}

func TestMiseOntologySeedUsesCLICompatiblePostgresUserFallback(t *testing.T) {
	source, err := os.ReadFile("../../../mise.toml")
	if err != nil {
		t.Fatalf("read mise.toml: %v", err)
	}

	text := string(source)
	if strings.Contains(text, `: "${PG_USER_NAME:?`) {
		t.Fatal("ontology-seed mise task must not require PG_USER_NAME")
	}
	if strings.Contains(text, `POSTGRES_USER="${PG_USER_NAME:-${PG_USER:-cding}}"`) {
		t.Fatal("ontology-seed mise task must not use the untrimmed shell expansion")
	}
	if !strings.Contains(text, "trim_whitespace()") {
		t.Fatal("ontology-seed mise task must trim PostgreSQL user environment values")
	}
	if !strings.Contains(text, `POSTGRES_USER="$(trim_whitespace "${PG_USER_NAME:-}")"`) {
		t.Fatal("ontology-seed mise task must trim PG_USER_NAME before resolving the fallback")
	}
	if !strings.Contains(text, `POSTGRES_USER="$(trim_whitespace "${PG_USER:-}")"`) {
		t.Fatal("ontology-seed mise task must trim PG_USER before resolving the literal fallback")
	}
	if !strings.Contains(text, "POSTGRES_USER=cding") {
		t.Fatal("ontology-seed mise task must retain cding as the literal fallback")
	}
	if !strings.Contains(text, `-U "$POSTGRES_USER"`) {
		t.Fatal("ontology-seed mise task must use the resolved PostgreSQL user for psql")
	}
	if !strings.Contains(text, `: "${PG_PASSWORD:?set PG_PASSWORD}"`) {
		t.Fatal("ontology-seed mise task must continue to require PG_PASSWORD")
	}
}

func TestServiceStartupsEnsureCuratedOntologyModulesAfterMigrations(t *testing.T) {
	for _, filename := range []string{"../deepdoc/main.go", "../doc-processor/main.go"} {
		t.Run(filename, func(t *testing.T) {
			source, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("read %s: %v", filename, err)
			}
			text := string(source)
			migrations := strings.Index(text, "config.RunMigrations")
			bootstrap := strings.Index(text, "seed.EnsureCuratedModules")
			if migrations < 0 {
				t.Fatalf("%s startup must run database migrations before bootstrap", filename)
			}
			if bootstrap < 0 {
				t.Fatalf("%s startup must ensure curated ontology modules", filename)
			}
			if bootstrap < migrations {
				t.Fatalf("%s must bootstrap curated ontology modules after migrations", filename)
			}
		})
	}
}

func TestServiceStartupsLogDeferredCuratedModuleWarnings(t *testing.T) {
	for _, filename := range []string{"../deepdoc/main.go", "../doc-processor/main.go"} {
		t.Run(filename, func(t *testing.T) {
			source, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("read %s: %v", filename, err)
			}
			text := string(source)
			if !strings.Contains(text, "warnings, err := seed.EnsureCuratedModules") {
				t.Fatalf("%s must receive nonfatal curated module warnings", filename)
			}
			if !strings.Contains(text, `logger.Warn("curated ontology bootstrap warning", args...)`) {
				t.Fatalf("%s must log curated module warnings with a static message key", filename)
			}
			if !strings.Contains(text, `"warning_kind", warning.Kind`) {
				t.Fatalf("%s must log the structured warning kind", filename)
			}
			if !strings.Contains(text, `if warning.DependencyModuleID != ""`) {
				t.Fatalf("%s must emit dependency_module_id only when set", filename)
			}
		})
	}
}

func TestDeepdocUsesDedicatedBootstrapTimeout(t *testing.T) {
	source, err := os.ReadFile("../deepdoc/main.go")
	if err != nil {
		t.Fatalf("read deepdoc main.go: %v", err)
	}
	text := string(source)
	if !strings.Contains(text, "bootstrapCtx, bootstrapCancel := context.WithTimeout(context.Background(), 3*time.Minute)") {
		t.Fatal("Deepdoc bootstrap must use a dedicated three-minute timeout")
	}
	if !strings.Contains(text, "seed.EnsureCuratedModules(bootstrapCtx, project_db)") {
		t.Fatal("Deepdoc bootstrap must not use main's startup context")
	}
}
