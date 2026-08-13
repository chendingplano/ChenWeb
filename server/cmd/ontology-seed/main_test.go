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
			if bootstrap < 0 {
				t.Fatalf("%s startup must ensure curated ontology modules", filename)
			}
			if bootstrap < migrations {
				t.Fatalf("%s must bootstrap curated ontology modules after migrations", filename)
			}
		})
	}
}
