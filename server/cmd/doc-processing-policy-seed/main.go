// Command doc-processing-policy-seed authors Doc Processing Policies from a
// TOML config file's [doc-processing-policy-*] sections into
// kb.pipelines/kb.pipeline_bindings, then compiles and activates them as a
// new kb.pipeline_policies version -- archiving whatever was active before.
// Modeled on server/cmd/ontology-seed. See
// docs/superpowers/specs/2026-08-08-doc-processing-policy-design.md.
//
// Usage:
//
//	doc-processing-policy-seed [--config path/to/config.local.toml]
//
// Re-running after editing the config file is safe: kb.pipelines rows are
// upserted by name, and each run creates and activates a new policy
// version rather than mutating a previous one in place.
//
// The seeded policy only takes effect in an already-running doc-processor
// process after that process restarts (loadProductionPipelinePolicyState
// loads the active policy once, at process startup).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

func main() {
	log.SetFlags(0)
	configPath := flag.String("config", "config.local.toml", "path to the TOML file holding [doc-processing-policy-*] sections")
	flag.Parse()

	cfg, err := docprocessing.LoadDocProcessingPolicySeedConfig(*configPath)
	if err != nil {
		log.Fatalf("load config %s: %v", *configPath, err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config %s: %v", *configPath, err)
	}

	db := connect()
	defer db.Close()

	result, err := docprocessing.SeedDocProcessingPolicies(context.Background(), db, cfg)
	if err != nil {
		log.Fatalf("seed: %v", err)
	}

	fmt.Printf("pipelines created: %v\n", result.PipelinesCreated)
	fmt.Printf("pipelines updated: %v\n", result.PipelinesUpdated)
	fmt.Printf("bindings written: %d\n", result.BindingsWritten)
	fmt.Printf("activated policy id=%d version=%d\n", result.PolicyID, result.PolicyVersion)
	fmt.Println("NOTE: restart the doc-processor service for this policy to take effect.")
}

func connect() *sql.DB {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), envOr("PG_USER", "cding"),
		envOr("PG_DB_NAME", "chenweb_test"))
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	return db
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
